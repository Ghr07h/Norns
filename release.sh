#!/usr/bin/env bash

set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
release_dir="$script_dir/release"
staging_dir=$(mktemp -d "${TMPDIR:-/tmp}/norns-release.XXXXXX")

cleanup() {
  rm -rf "$staging_dir"
}
trap cleanup EXIT

if ! command -v go >/dev/null 2>&1; then
  echo "release: Go 1.25 or newer is required" >&2
  exit 1
fi

go_version=$(go env GOVERSION)
go_major_minor=${go_version#go}
go_major=${go_major_minor%%.*}
go_minor=${go_major_minor#*.}
go_minor=${go_minor%%.*}
if (( go_major < 1 || (go_major == 1 && go_minor < 25) )); then
  echo "release: Go 1.25 or newer is required, found ${go_version}" >&2
  exit 1
fi

product_version=${NORNS_VERSION:-dev}
vcs_ref=${VCS_REF:-}
if [[ -z "$vcs_ref" ]] && command -v git >/dev/null 2>&1; then
  vcs_ref=$(git -C "$script_dir" rev-parse --short=12 HEAD 2>/dev/null || true)
fi
vcs_ref=${vcs_ref:-unknown}
build_time=${BUILD_TIME:-$(date -u '+%Y-%m-%dT%H:%M:%SZ')}
go_cache=${GOCACHE:-${TMPDIR:-/tmp}/norns-release-go-cache}

export CGO_ENABLED=0
export GOCACHE="$go_cache"

echo "release: generating nornsd embedded assets"
(cd "$script_dir/nornsd" && go generate ./internal/assets)

plugin_dirs=()
while IFS= read -r plugin_dir; do
  plugin_dirs+=("$plugin_dir")
done < <(find "$script_dir/norns/cmd" -mindepth 1 -maxdepth 1 -type d -name 'norns-*' | LC_ALL=C sort)

norns_ldflags="-s -w \
-X github.com/Ghr07h/norns/norns/internal/version.ProductVersion=$product_version \
-X github.com/Ghr07h/norns/norns/internal/version.Commit=$vcs_ref \
-X github.com/Ghr07h/norns/norns/internal/version.BuildTime=$build_time"

nornsd_ldflags="-s -w \
-X github.com/Ghr07h/norns/nornsd/internal/version.ProductVersion=$product_version \
-X github.com/Ghr07h/norns/nornsd/internal/version.Commit=$vcs_ref \
-X github.com/Ghr07h/norns/nornsd/internal/version.BuildTime=$build_time"

targets=(
  "darwin:x64:amd64"
  "darwin:arm64:arm64"
  "linux:x64:amd64"
  "linux:arm64:arm64"
  "windows:x64:amd64"
  "windows:arm64:arm64"
)

for target in "${targets[@]}"; do
  IFS=: read -r target_os label_arch target_arch <<<"$target"
  target_dir="$staging_dir/${target_os}-${label_arch}"
  extension=""
  if [[ "$target_os" == "windows" ]]; then
    extension=".exe"
  fi
  mkdir -p "$target_dir"

  echo "release: building ${target_os}/${target_arch}"
  (
    cd "$script_dir/norns"
    GOOS="$target_os" GOARCH="$target_arch" go build \
      -trimpath -ldflags "$norns_ldflags" \
      -o "$target_dir/norns${extension}" ./cmd/norns
    for plugin_dir in "${plugin_dirs[@]}"; do
      plugin_name=$(basename "$plugin_dir")
      GOOS="$target_os" GOARCH="$target_arch" go build \
        -trimpath -ldflags "$norns_ldflags" \
        -o "$target_dir/${plugin_name}${extension}" "./cmd/${plugin_name}"
    done
  )
  (
    cd "$script_dir/nornsd"
    GOOS="$target_os" GOARCH="$target_arch" go build \
      -trimpath -ldflags "$nornsd_ldflags" \
      -o "$target_dir/nornsd${extension}" ./cmd/nornsd
  )
done

if [[ -L "$release_dir" ]]; then
  echo "release: refusing to replace symbolic link $release_dir" >&2
  exit 1
fi
rm -rf "$release_dir"
mkdir -p "$release_dir"
cp -R "$staging_dir/." "$release_dir/"

(
  cd "$release_dir"
  find . -type f ! -name SHA256SUMS | LC_ALL=C sort | while IFS= read -r artifact; do
    shasum -a 256 "$artifact"
  done > SHA256SUMS
)

touch "$release_dir/.gitkeep"

echo "release: wrote six platform directories and SHA256SUMS to $release_dir"
