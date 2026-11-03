#!/usr/bin/env bash

set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

usage() {
  echo "Usage: ./release.sh [norns|nornsd|well|all] [--all-platforms]"
  echo "Without a product, builds norns (including plugins) and nornsd."
  echo "Well builds only for the current platform; run npm ci in well first."
}

fail() {
  echo "release: $*" >&2
  exit 1
}

ensure_directory() {
  if [[ -L "$1" || ( -e "$1" && ! -d "$1" ) ]]; then
    fail "refusing to replace non-directory or symbolic link $1"
  fi
  mkdir -p "$1"
}

selection=""
all_platforms=()
help=false
for argument in "$@"; do
  case "$argument" in
    norns|nornsd|well|all)
      [[ -z "$selection" ]] || fail "select only one product or all"
      selection=$argument
      ;;
    --all-platforms) all_platforms=(--all-platforms) ;;
    --help|-h) help=true ;;
    *) fail "unknown argument: $argument" ;;
  esac
done

if [[ ${#all_platforms[@]} -gt 0 && ( "$selection" == well || "$selection" == all ) ]]; then
  fail "--all-platforms supports only norns and nornsd; build Well on each target platform"
fi
if [[ "$help" == true ]]; then
  usage
  exit 0
fi

case "$selection" in
  "") products=(norns nornsd) ;;
  all) products=(norns nornsd well) ;;
  *) products=("$selection") ;;
esac

cd "$script_dir"
if [[ "$selection" != well ]]; then
  command -v go >/dev/null 2>&1 || fail "Go 1.25 or newer is required"
  go_version=$(go env GOVERSION)
  go_major_minor=${go_version#go}
  go_major=${go_major_minor%%.*}
  go_minor=${go_major_minor#*.}
  go_minor=${go_minor%%.*}
  if (( go_major < 1 || (go_major == 1 && go_minor < 25) )); then
    fail "Go 1.25 or newer is required, found ${go_version}"
  fi
  host_os=$(go env GOHOSTOS)
  host_arch=$(go env GOHOSTARCH)
else
  command -v node >/dev/null 2>&1 || fail "Well requires Node.js"
  host_os=$(node -p 'process.platform === "win32" ? "windows" : process.platform')
  host_arch=$(node -p 'process.arch')
fi
if [[ "$host_arch" == amd64 ]]; then host_arch=x64; fi

if command -v sha256sum >/dev/null 2>&1; then
  checksum=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  checksum=(shasum -a 256)
else
  fail "sha256sum or shasum is required"
fi

platforms=("$host_os-$host_arch")
if [[ ${#all_platforms[@]} -gt 0 ]]; then
  platforms=(darwin-x64 darwin-arm64 linux-x64 linux-arm64 windows-x64 windows-arm64)
fi
ensure_directory "$script_dir/release"
for platform in "${platforms[@]}"; do
  ensure_directory "$script_dir/release/$platform"
done

staging=$(mktemp -d "$script_dir/.release.tmp.XXXXXX")
trap 'rm -rf -- "$staging"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export VCS_REF="${VCS_REF:-$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)}"
export BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

for product in "${products[@]}"; do
  if [[ "$product" == well ]]; then
    (cd well && npm run release -- --output "$staging/artifacts")
  else
    (cd "$product" && go run ./scripts/release.go ${all_platforms[@]+"${all_platforms[@]}"} --output "$staging/artifacts")
  fi
done

for platform in "${platforms[@]}"; do
  ensure_directory "$script_dir/release/$platform"
  [[ -d "$staging/artifacts/$platform" ]] || fail "missing staged artifacts for $platform"
done
for platform in "${platforms[@]}"; do
  destination="$script_dir/release/$platform"
  for product in "${products[@]}"; do
    case "$product" in
      norns) rm -rf -- "$destination/norns" "$destination/norns.exe" "$destination"/norns-* ;;
      nornsd) rm -rf -- "$destination/nornsd" "$destination/nornsd.exe" ;;
      well) rm -rf -- "$destination/well" ;;
    esac
  done
  for artifact in "$staging/artifacts/$platform"/*; do
    mv -- "$artifact" "$destination/"
  done
done

(
  cd release
  find . -type f ! -name SHA256SUMS ! -name .gitkeep -print | LC_ALL=C sort |
    while IFS= read -r artifact; do "${checksum[@]}" "$artifact"; done
) > "$staging/SHA256SUMS"
mv -- "$staging/SHA256SUMS" "$script_dir/release/SHA256SUMS"
echo "release: wrote ${products[*]} to $script_dir/release and updated SHA256SUMS"
