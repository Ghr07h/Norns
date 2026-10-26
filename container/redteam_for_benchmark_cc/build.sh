#!/usr/bin/env bash
# 构建红队普通 Claude Code Benchmark 对照组自包含镜像。
#
# 用法：
#   container/redteam_for_benchmark_cc/build.sh --backend docker
#   container/redteam_for_benchmark_cc/build.sh --backend apple --use-mirror
#   container/redteam_for_benchmark_cc/build.sh --backend docker --proxy 127.0.0.1:1080
#
# 参数：
#   --backend apple|docker   构建后端，默认 docker。
#   --image IMAGE            Python 基础镜像，默认 python:3.13.15-slim。
#   --tag IMAGE               输出镜像 tag，默认 nornsd-redteam-benchmark-cc:latest。
#   --proxy host:port        国外资源下载使用的构建期代理。
#   --use-mirror             使用阿里云 APT/PyPI 和 npmmirror，默认关闭。
#   --use-ecs-mirror         使用阿里云 ECS 内网 APT/PyPI，默认关闭，并启用 --use-mirror。
#
# 前置：项目根目录已执行 ./release.sh 生成 release/linux-<arch> 二进制(镜像构建需要)。

set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
project_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)

backend="docker"
python_image="python:3.13.15-slim"
node_version="24.22.1"
image_tag="nornsd-redteam-benchmark-cc:latest"
proxy=""
use_mirror=0
use_ecs_mirror=0

usage() {
  sed -n '2,17p' "$0"
  exit "${1:-0}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --backend) backend="${2:-}"; shift 2 ;;
    --image) python_image="${2:-}"; shift 2 ;;
    --tag) image_tag="${2:-}"; shift 2 ;;
    --proxy) proxy="${2:-}"; shift 2 ;;
    --use-mirror) use_mirror=1; shift ;;
    --use-ecs-mirror) use_mirror=1; use_ecs_mirror=1; shift ;;
    -h|--help) usage 0 ;;
    *) echo "未知参数: $1" >&2; usage 2 ;;
  esac
done

case "$backend" in
  apple|docker) ;;
  *) echo "错误: --backend 仅支持 apple|docker，收到 '$backend'" >&2; exit 2; ;;
esac

if [ -n "$proxy" ]; then
  case "$proxy" in
    *:*) ;;
    *) echo "错误: --proxy 格式应为 host:port，收到 '$proxy'" >&2; exit 2; ;;
  esac
fi

case "$(uname -m)" in
  arm64|aarch64) release_arch="arm64"; expected_arch="ARM aarch64" ;;
  x86_64|amd64) release_arch="x64"; expected_arch="x86-64" ;;
  *) echo "错误: 不支持的主机架构 $(uname -m)" >&2; exit 1; ;;
esac
release_dir="$project_root/release/linux-$release_arch"

for binary in norns nornsd; do
  path="$release_dir/$binary"
  [ -f "$path" ] || {
    echo "错误: 缺少 $path，请先在项目根目录执行 ./release.sh" >&2
    exit 1
  }
  if command -v file >/dev/null 2>&1 && ! file "$path" | grep -q "ELF 64-bit.*$expected_arch"; then
    echo "错误: $path 不是预期的 Linux $release_arch ELF 文件" >&2
    file "$path" >&2
    exit 1
  fi
done

case "$backend" in
  docker) command -v docker >/dev/null 2>&1 || { echo "错误: 未找到 docker" >&2; exit 1; } ;;
  apple) command -v container >/dev/null 2>&1 || { echo "错误: 未找到 Apple container CLI" >&2; exit 1; } ;;
esac

http_proxy_url=""
all_proxy_url=""
if [ -n "$proxy" ]; then
  http_proxy_url="http://$proxy"
  all_proxy_url="socks5://$proxy"
fi
no_proxy_list="localhost,127.0.0.1,::1,mirrors.aliyun.com,mirrors.cloud.aliyuncs.com,registry.npmmirror.com,npmmirror.com,cdn.npmmirror.com"

npm_registry=""
node_dist_url="https://nodejs.org/dist"
pip_index_url=""
pip_trusted_host=""
apt_source_block=""
if [ "$use_mirror" = 1 ]; then
  npm_registry="https://registry.npmmirror.com"
  node_dist_url="https://npmmirror.com/mirrors/node"
  pip_index_url="https://mirrors.aliyun.com/pypi/simple/"
  pip_trusted_host="mirrors.aliyun.com"
  apt_mirror_base="https://mirrors.aliyun.com"
  if [ "$use_ecs_mirror" = 1 ]; then
    apt_mirror_base="http://mirrors.cloud.aliyuncs.com"
    pip_index_url="http://mirrors.cloud.aliyuncs.com/pypi/simple/"
    pip_trusted_host="mirrors.cloud.aliyuncs.com"
  fi
  apt_source_block=$(printf 'Types: deb\nURIs: %s/debian\nSuites: trixie trixie-updates\nComponents: main contrib\nSigned-By: /usr/share/keyrings/debian-archive-keyring.gpg\n\nTypes: deb\nURIs: %s/debian-security\nSuites: trixie-security\nComponents: main contrib\nSigned-By: /usr/share/keyrings/debian-archive-keyring.gpg' "$apt_mirror_base" "$apt_mirror_base")
fi

build_args=(
  --build-arg "PYTHON_IMAGE=$python_image"
  --build-arg "NODE_VERSION=$node_version"
  --build-arg "NODE_DIST_URL=$node_dist_url"
  --build-arg "NORNS_RELEASE_DIR=release/linux-$release_arch"
  --build-arg "NPM_REGISTRY=$npm_registry"
  --build-arg "PIP_INDEX_URL=$pip_index_url"
  --build-arg "PIP_TRUSTED_HOST=$pip_trusted_host"
  --build-arg "APT_SOURCE_BLOCK=$apt_source_block"
)
if [ -n "$proxy" ]; then
  build_args+=(
    --build-arg "HTTP_PROXY=$http_proxy_url" --build-arg "http_proxy=$http_proxy_url"
    --build-arg "HTTPS_PROXY=$http_proxy_url" --build-arg "https_proxy=$http_proxy_url"
    --build-arg "ALL_PROXY=$all_proxy_url" --build-arg "all_proxy=$all_proxy_url"
    --build-arg "NO_PROXY=$no_proxy_list" --build-arg "no_proxy=$no_proxy_list"
  )
fi

echo "==== norns redteam benchmark CC 对照组镜像构建(自包含) ===="
echo "  backend : $backend"
echo "  base    : $python_image"
echo "  node    : $node_version ($node_dist_url)"
echo "  tag     : $image_tag"
echo "  release : release/linux-$release_arch"
echo "  proxy   : ${proxy:-<none>}"
echo "  mirror  : $([ "$use_mirror" = 1 ] && echo yes || echo no)"
echo "  apt     : ${apt_mirror_base:-image default}"
echo "  pypi    : ${pip_index_url:-pip default}"

case "$backend" in
  apple)
    container build "${build_args[@]}" \
      --file "$script_dir/Dockerfile" \
      --tag "$image_tag" \
      "$project_root"
    ;;
  docker)
    (
      cd "$project_root"
      docker build "${build_args[@]}" \
        --file "$script_dir/Dockerfile" \
        --tag "$image_tag" \
        .
    )
    ;;
esac

echo
echo "构建完成。镜像内验证："
if [ "$backend" = apple ]; then
  echo "  container run --rm --entrypoint python3 $image_tag /usr/local/bin/nornsd-verify"
else
  echo "  docker run --rm --entrypoint python3 $image_tag /usr/local/bin/nornsd-verify"
fi
