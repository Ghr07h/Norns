# Norns 快速开始

本文介绍如何安装并启动 Norns。项目定位和模块结构见 [README.md](README.md)，
各模块的完整参考资料见本文的[文档](#文档)章节。

## 安装和使用

### 1. 获取 Norns 二进制

`norns`、`nornsd` 和同目录的 `norns-*` 插件必须来自同一次构建或同一个 Release。
可以选择从源码构建，也可以直接下载预编译产物。

使用 Go 1.25 或更高版本从源码构建：

```sh
./release.sh
```

脚本会交叉编译 `darwin/linux/windows × x64/arm64` 六个平台，并将结果写入
`release/<os>-<arch>/`。选择当前系统对应的目录，把其中全部文件复制到 `bin/`：

```sh
# 按当前系统修改为 darwin-arm64、darwin-x64、linux-arm64 或 linux-x64。
platform=darwin-arm64

mkdir -p ./bin
cp "./release/$platform/"* ./bin/
chmod +x ./bin/*
```

不安装 Go 时，可以从 [GitHub Releases](https://github.com/Ghr07h/norns/releases)
下载当前系统和架构对应的预编译产物，解压后同样将该平台目录中的全部文件放入
`./bin/`。不要混用不同版本或不同构建中的 `norns`、`nornsd` 与插件。

项目不会自动修改用户环境。启动前需要手动将 `bin/` 加入 PATH；以下设置只对当前
shell 生效，需要长期使用时请自行写入对应的 shell 配置：

```sh
export PATH="$(pwd)/bin:$PATH"

norns version --json
nornsd version --json
```

Windows 用户将 `release/windows-x64/` 或 `release/windows-arm64/` 中的 `.exe`
文件复制到 `bin\`，再手动将该目录加入用户 PATH。

### 2. 安装 Claude Code

Norns 当前要求 Claude Code 2.1.199，并需要可用的模型 API 地址、密钥和模型名称。

建议在当前用户的 `~/cc` 目录本地安装要求的 Claude Code 版本：

```sh
mkdir -p ~/cc
cd ~/cc
npm install @anthropic-ai/claude-code@2.1.199 --registry=https://registry.npmmirror.com
```

全新安装且从未运行过的 Claude Code 初次启动时会尝试联网进入 onboarding，可以
写入完成标记跳过该步骤。如果此前已有 Claude Code 运行记录，则不需要也不应执行
以下命令，因为它会覆盖已有的 `~/.claude.json`：

```sh
cat > ~/.claude.json <<'EOF'
{
  "hasCompletedOnboarding": true
}
EOF
```

本地安装后，Claude Code 可执行文件位于 `~/cc/node_modules/.bin/claude`。由于
`AGENT_EXEC` 不经过 shell，配置时需要将 `~` 展开为当前用户主目录的绝对路径：

```yaml
AGENT_EXEC:
  - "/absolute/path/to/home/cc/node_modules/.bin/claude"
```

### 3. 配置 Agent

回到 Norns 项目根目录。首次使用时复制配置模板；如果 `bin/agent.yaml` 已存在，不要
覆盖其中的访问密钥和自定义配置：

```sh
test -e ./bin/agent.yaml || cp ./nornsd/internal/config/agent.example.yaml ./bin/agent.yaml
```

编辑 `bin/agent.yaml`，至少设置模型连接信息，并确认 `AGENT_EXEC` 指向上一步安装的
Claude Code：

```yaml
VERSION: 1
LLM_URL: "https://your-api.example.com"
LLM_KEY: "replace-with-secret"
LLM_MODEL: "model-name"
LLM_FAST_MODEL: ""

NORNS_ENABLE: true
NORNS_SERVER_HOST: "0.0.0.0"
NORNS_SERVER_PORT: 28081
NORNS_SERVER_TOKEN: "replace-with-a-long-random-token"

NORNS_MAX_CONCURRENT_EXPLORATIONS: 3
NORNS_EXPLORATION_TIMEOUT: 1200
NORNS_ALLOW_HUMAN_INTERVENTION: false

NORNS_CUSTOM_DOMAIN_DIR: ""
NORNS_CUSTOM_SKILLS_DIR: ""
NORNS_CUSTOM_AGENTS_DIR: ""

CUSTOM_AGENTS_DIR: ""
CUSTOM_SKILLS_DIR: ""
CUSTOM_PLUGIN_DIRS: []

AGENT_EXEC:
  - "/absolute/path/to/home/cc/node_modules/.bin/claude"
```

路径字段以声明它的 `agent.yaml` 所在目录为基准解析。`agent.yaml` 包含密钥，
不要提交到版本控制，也不要随 Runtime 产物公开分发。

### 4. 启动一个 Norns Agent

在希望保存 Subject 数据的目录中启动 Direct Agent：

```sh
mkdir -p ./runtime-root
cd ./runtime-root

nornsd agent \
  --setting=../bin/agent.yaml
```

nornsd 会为该 Subject 生成 Norns Plugin 和 Workspace，然后启动 Claude Code。
它会固定传入 `--dangerously-skip-permissions`，并为 Claude Code 子进程设置
`IS_SANDBOX=1` 以兼容 root/sudo 环境，因此只应在可信且权限受限的工作目录中运行。

### 批量服务与 Well

在项目根目录启动两个并发槽的 nornsd Server：

```sh
nornsd server \
  --workspace-root ./runtime-root \
  --settings ./bin/agent.yaml \
  --concurrency 2
```

`NORNS_ENABLE`、`NORNS_SERVER_HOST`、`NORNS_SERVER_PORT` 和
`NORNS_SERVER_TOKEN` 已由 `agent.yaml` 提供；显式配置时，它们优先于同用途的命令行参数。
Server 的 API 基础路径为 `/api/v1`，所有请求都需要 Bearer Token。生产环境中
不要直接将未加密的 nornsd HTTP 端口暴露到不可信网络，应使用 TLS 反向代理并限制访问来源。

Well 是该 API 的桌面客户端。打开 Well 后添加 Server：

- URL：`http://127.0.0.1:28081`，不要包含 `/api/v1`
- Token：与 `NORNS_SERVER_TOKEN` 相同

Well 可以创建 Subject、提交 Runtime、查看队列和事件，以及显示 Norns Status、
Trace 与 Visual View。构建和本地开发方式见 [well/README.md](well/README.md)。

### 领域知识

`agent.yaml` 可以通过 `NORNS_CUSTOM_DOMAIN_DIR` 插入领域知识。项目提供数学示例：

```yaml
NORNS_CUSTOM_DOMAIN_DIR: "../domain/math"
```

nornsd 会以 `bin/agent.yaml` 所在目录为基准解析该相对路径。

nornsd 会根据 `domain/math/meta.json` 动态生成数学领域的 Decider 和 Explorer，
而不会改变协议状态机或通用角色边界。格式说明见 [domain/README.md](domain/README.md)。

### 单独使用 norns

`norns` 也可以作为纯本地协议 CLI 使用，不需要启动 nornsd：

```sh
mkdir -p ./example-workspace
cd ./example-workspace

norns workspace init
norns task open \
  --purpose "确认一个开放任务是否可完成" \
  --concurrency 3 \
  --info "任务背景与当前已知信息"

norns view status
norns view visual --lang zh --open
```

完整命令、状态机和输出格式见 [norns/README.md](norns/README.md)。

### Docker

当前镜像只封装预编译文件，不在 Docker 内编译 Go 源码。先生成六个平台目标：

```sh
./release.sh
```

`release/` 将包含 `darwin/linux/windows × x64/arm64` 的 `norns`、`nornsd` 和
自动发现的 `norns-*` 插件。Docker 构建直接读取与当前主机架构一致的 Linux Release
目录，不读取本地 `bin/`。通用镜像默认使用 `python:3.13.15-slim`，固定安装 Node.js
24.22.1，并命名为 `nornsd-server:latest`：

```sh
container/nornsd/build.sh
```

构建脚本也支持 Apple Container，以及 `--image`、`--tag`、`--proxy`、`--use-mirror`
和 `--use-ecs-mirror`。Claude Code 2.1.199 通过 npm 非全局安装到
`/norns/engine/cc/`，项目 `domain/` 会复制到镜像的 `/norns/domain/`。

启动镜像时，入口读取 `/norns/config/agent.example.yaml`，通过白名单环境变量生成
`/norns/config/agent.yaml`：

```sh
docker run --detach --rm \
  --name nornsd \
  --publish 28081:28081 \
  --volume nornsd-data:/workspace \
  --env LLM_URL='https://example.test/anthropic' \
  --env LLM_KEY='replace-me' \
  --env LLM_MODEL='model-name' \
  --env NORNS_SERVER_CONCURRENCY=1 \
  --env NORNS_MAX_CONCURRENT_EXPLORATIONS=3 \
  --env NORNS_EXPLORATION_TIMEOUT=1200 \
  --env NORNS_ALLOW_HUMAN_INTERVENTION=false \
  nornsd-server:latest
```

镜像以 root 运行，数据目录为 `/workspace`。`LLM_URL`、`LLM_KEY` 和 `LLM_MODEL` 必须
显式传入且非空。`NORNS_ENABLE` 默认是 `true`，可通过同名环境变量关闭；
`NORNS_SERVER_CONCURRENCY` 默认是 `1`，可设置为其他正整数；
`NORNS_MAX_CONCURRENT_EXPLORATIONS` 默认是 `3`。

入口脚本会将最终使用的 Server Token 持久化到数据卷；若配置缺失或仍为模板占位值，
首次启动会自动生成高强度 Token：

```sh
docker exec nornsd cat /workspace/.nornsd-token
```

随后可在 Well 中使用 `http://127.0.0.1:28081` 和该 Token 添加 Server。查看日志
或停止服务：

```sh
docker logs --follow nornsd
docker stop nornsd
```

镜像从指定的 `release/linux-<arch>/` 复制可执行文件到 `/norns/bin/`，并将公开的
`nornsd/internal/config/agent.example.yaml` 安装为
`/norns/config/agent.example.yaml`。本地 `bin/agent.yaml` 不会进入 Docker 构建上下文。
完整构建参数、配置白名单和领域目录说明见 [container/README.md](container/README.md)。

同一目录还提供 Norns benchmark 镜像和强制关闭 Norns 的普通 Claude Code 对照组镜像；
两者使用相同红队工具链、模型配置和三槽 FIFO 调度，具体构建与运行方式见上述 Container
文档。

## 当前版本边界

| 项目 | 当前版本 |
| --- | --- |
| Norns protocol | `2`（stable） |
| norns database schema | `9` |
| nornsd HTTP API | `v1` |
| nornsd database schema | `1` |
| agent.yaml schema | `1` |
| Claude Code | `2.1.199` |
| Go | `1.25` |

`norns` 和 `nornsd` 必须来自同一次构建或同一个 Release。nornsd 启用 Norns
模式时会检查 `norns` 支持的协议版本，但不负责迁移不兼容的 Workspace。

## 文档列表

- [项目说明](README.md)
- [文档索引](documents/README.md)
- [Norns Protocol](documents/protocol/protocol.md)
- [norns CLI](norns/README.md)
- [nornsd](nornsd/README.md)
- [Well](well/README.md)
- [领域知识包](domain/README.md)

## 数据与安全

- 不要直接编辑 `.norns/norns.sqlite` 或 nornsd 的内部数据库; nornsd 负责 Runtime 生命周期；关闭 Norns Task 前应确保相关 Explorer 已停止写入。
- Subject 的 `.nornsd/` 可能包含生成配置、Agent transcript、日志和访问凭据。
- `agent.yaml` 中的 `LLM_KEY`、Server Token 和 Well 保存的凭据都应按密钥处理。
