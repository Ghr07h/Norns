#!/usr/bin/env bash
# 将构建普通 Claude Code benchmark 对照组所需的最小项目树打包。

set -euo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
project_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
archive="$script_dir/$(basename "$script_dir").zip"

command -v zip >/dev/null 2>&1 || {
  echo "错误: 未找到 zip 命令" >&2
  exit 1
}

required_files=(
  "release/linux-arm64/norns"
  "release/linux-arm64/nornsd"
  "release/linux-x64/norns"
  "release/linux-x64/nornsd"
  "nornsd/internal/config/agent.example.yaml"
  "domain/redteam/norns_domain_agents/meta.json"
  "domain/redteam/skills"
  "container/redteam_for_benchmark_cc/Dockerfile"
  "container/redteam_for_benchmark_cc/Dockerfile.dockerignore"
  "container/redteam_for_benchmark_cc/build.sh"
  "container/redteam_for_benchmark_cc/benchmark.py"
  "container/redteam_for_benchmark_cc/entrypoint.py"
  "container/redteam_for_benchmark_cc/verify.py"
)

for relative_path in "${required_files[@]}"; do
  [ -e "$project_root/$relative_path" ] || {
    echo "错误: 缺少 $project_root/$relative_path" >&2
    echo "请先在项目根目录执行 ./release.sh 生成 Linux release。" >&2
    exit 1
  }
done

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/norns-benchmark-cc-package.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT
tmp_archive="$tmp_dir/$(basename "$archive")"

cd "$project_root"
zip -X -q -r "$tmp_archive" \
  release/linux-arm64 \
  release/linux-x64 \
  nornsd/internal/config/agent.example.yaml \
  domain/redteam \
  container/redteam_for_benchmark_cc/Dockerfile \
  container/redteam_for_benchmark_cc/Dockerfile.dockerignore \
  container/redteam_for_benchmark_cc/build.sh \
  container/redteam_for_benchmark_cc/benchmark.py \
  container/redteam_for_benchmark_cc/entrypoint.py \
  container/redteam_for_benchmark_cc/verify.py \
  -x '*/.DS_Store' '*/__pycache__/*' '*.pyc' '*.zip'

mv -f "$tmp_archive" "$archive"

echo "打包完成: $archive"
echo "归档大小: $(du -h "$archive" | awk '{print $1}')"
echo "解压后从项目树根目录执行:"
echo "  container/redteam_for_benchmark_cc/build.sh --backend docker"
