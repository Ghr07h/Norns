# Container Images

本目录维护四个基于 Python slim 的独立镜像。所有构建都以项目根目录为 Context，直接
读取 `release/linux-<arch>/` 中的预编译二进制，并使用公开的
`nornsd/internal/config/agent.example.yaml`。构建过程不会读取本地 `bin/`。

| 目录 | 默认镜像 | 用途 |
| --- | --- | --- |
| `nornsd/` | `nornsd-server:latest` | 通用 nornsd Server，仅包含基础运行环境 |
| `redteam/` | `nornsd-redteam-server:latest` | 内置 redteam Domain、Skill 和安全工具的 nornsd Server |
| `redteam_for_benchmark/` | `nornsd-redteam-benchmark:latest` | 内置 redteam nornsd 与 benchmark FIFO 调度器 |
| `redteam_for_benchmark_cc/` | `nornsd-redteam-benchmark-cc:latest` | 使用普通 Claude Code Runtime 的 benchmark 对照组 |

每个镜像目录都提供专用 `Dockerfile.dockerignore` 限制构建 Context。真实
`agent.yaml`、Runtime 数据、日志和本地密钥不会作为镜像输入。

## 共同前置条件

先在项目根目录生成六个平台的 Release：

```sh
./release.sh
```

镜像只使用 `release/linux-x64` 或 `release/linux-arm64`。`norns`、`nornsd` 和
Release 中已有的 `norns-*` 插件必须来自同一次构建。

四个镜像使用相同的容器目录约定：

```text
/norns/bin/                         norns、nornsd、插件和 benchmark 脚本
/norns/domain/                      Domain 内容
/norns/config/agent.example.yaml    镜像内配置模板
/norns/config/agent.yaml            容器启动时生成的最终配置
/norns/engine/cc/                   Claude Code npm 安装目录
/workspace/                         SQLite、Subject、Runtime、日志和 Token
```

通用镜像复制整个项目 `domain/`；redteam 和两个 benchmark 镜像仅复制
`domain/redteam/`。四个目录各自保留完整 Dockerfile、入口和验证代码，不依赖
`container/` 下的公共运行文件。

四个 `build.sh` 支持相同参数，`--backend` 默认为 `docker`，`--use-mirror` 和
`--use-ecs-mirror` 默认关闭：

- `--backend apple|docker`
- `--image IMAGE`
- `--tag IMAGE`
- `--proxy host:port`
- `--use-mirror`
- `--use-ecs-mirror`

四个 Dockerfile 公开相同的 Build ARG：

```text
PYTHON_IMAGE
CLAUDE_CODE_VERSION
NODE_VERSION
NODE_DIST_URL
NORNS_RELEASE_DIR
HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY
http_proxy / https_proxy / all_proxy / no_proxy
PIP_INDEX_URL
PIP_TRUSTED_HOST
NPM_REGISTRY
APT_SOURCE_BLOCK
```

直接执行 `docker build` 时可用 `--build-arg` 覆盖；构建脚本根据镜像源和代理选项生成
这些参数。默认版本为 Python 3.13.15 slim、Node.js 24.22.1 和 Claude Code 2.1.199。

四个镜像目录都提供独立的 `package.sh`，用于生成可上传到远端构建机的最小项目树：

```sh
container/nornsd/package.sh
container/redteam/package.sh
container/redteam_for_benchmark/package.sh
container/redteam_for_benchmark_cc/package.sh
```

归档分别写入对应目录下的 `nornsd.zip`、`redteam.zip`、`redteam_for_benchmark.zip` 和
`redteam_for_benchmark_cc.zip`。每个压缩包包含两套 Linux Release、该镜像需要的
Domain、配置模板及自身构建文件；解压后从项目树根目录运行压缩包内的 `build.sh`。

## 通用 nornsd

`nornsd/` 默认使用 `python:3.13.15-slim`，固定安装 Node.js 24.22.1 和 Claude Code，
并以 root 启动 nornsd Server。镜像只内置公开配置模板，运行时通过白名单环境变量生成
最终配置；默认启用 Norns 模式，Server Runtime 槽位数为 1，Norns 探索并发数为 3。

构建脚本会按当前主机架构选择 `release/linux-x64` 或 `release/linux-arm64`：

```sh
container/nornsd/build.sh
```

Apple Container 使用 `--backend apple`。可用 `--image` 或 `--tag` 覆盖基础镜像和输出
镜像，并可使用 `--proxy`、`--use-mirror` 或 `--use-ecs-mirror` 控制构建网络。

启动示例：

```sh
docker run --detach --rm \
  --name nornsd \
  --publish 28081:28081 \
  --volume nornsd-data:/workspace \
  --env LLM_URL='https://example.test/anthropic' \
  --env LLM_KEY='replace-me' \
  --env LLM_MODEL='model-name' \
  nornsd-server:latest
```

`LLM_URL`、`LLM_KEY` 和 `LLM_MODEL` 必须显式传入且非空，否则入口终止。运行配置默认值：

| 变量 | 通用/Redteam Server | Norns Benchmark | CC Benchmark |
| --- | --- | --- | --- |
| `LLM_FAST_MODEL` | 跟随 `LLM_MODEL` | 跟随 `LLM_MODEL` | 跟随 `LLM_MODEL` |
| `NORNS_ENABLE` | `true` | `true` | 强制 `false` |
| `NORNS_SERVER_HOST` / `NORNS_SERVER_PORT` | `0.0.0.0` / `28081` | `0.0.0.0` / `28081` | `0.0.0.0` / `28081` |
| `NORNS_MAX_CONCURRENT_EXPLORATIONS` | `3` | `3` | `3`，非 Norns 模式不使用 |
| `NORNS_EXPLORATION_TIMEOUT` | `1200` | `1200` | `1200`，非 Norns 模式不使用 |
| `NORNS_CUSTOM_DOMAIN_DIR` | 通用为空；Redteam 为内置目录 | 内置 Redteam 目录 | 强制为空 |
| `NORNS_CUSTOM_SKILLS_DIR` | 通用为空；Redteam 为内置目录 | 内置 Redteam 目录 | 强制为空 |
| `CUSTOM_SKILLS_DIR` | 空 | 空 | 内置 Redteam Skill |

`NORNS_CUSTOM_AGENTS_DIR`、`CUSTOM_AGENTS_DIR`、`CUSTOM_SKILLS_DIR` 和
`CUSTOM_PLUGIN_DIRS` 默认均为空。`CUSTOM_PLUGIN_DIRS` 接受单个路径字符串或 YAML/JSON
字符串列表。`NORNS_SERVER_CONCURRENCY` 控制 Server Runtime 槽位，通用与 redteam Server
默认均为 1，两个 benchmark 镜像固定为 3。

前三个入口共同识别以下 nornsd 配置变量：

```text
LLM_URL
LLM_KEY
LLM_MODEL
LLM_FAST_MODEL
NORNS_ENABLE
NORNS_SERVER_HOST
NORNS_SERVER_PORT
NORNS_SERVER_TOKEN
NORNS_MAX_CONCURRENT_EXPLORATIONS
NORNS_EXPLORATION_TIMEOUT
NORNS_ALLOW_HUMAN_INTERVENTION
NORNS_CUSTOM_DOMAIN_DIR
NORNS_CUSTOM_SKILLS_DIR
NORNS_CUSTOM_AGENTS_DIR
CUSTOM_AGENTS_DIR
CUSTOM_SKILLS_DIR
CUSTOM_PLUGIN_DIRS
```

CC 对照组只允许覆盖 LLM、Server 监听配置和普通 Claude Code 的
`CUSTOM_AGENTS_DIR`、`CUSTOM_SKILLS_DIR`、`CUSTOM_PLUGIN_DIRS`。它会忽略外部传入的
`NORNS_ENABLE` 和 `NORNS_CUSTOM_*`，确保 Runtime 始终使用非 Norns Profile。

如果未提供 `NORNS_SERVER_TOKEN`，入口会复用 `/workspace/.nornsd-token`，文件不存在时
生成新 Token；显式传入时会覆盖并持久化到该文件。启动阶段会检查三个必填 LLM 变量、
Claude Code、`norns`、`nornsd` 和所有非空自定义目录，失败时不会启动 Server。

查看当前持久化的 Server Token：

```sh
docker exec nornsd cat /workspace/.nornsd-token
```

自定义 Domain、Skill、Agent 或 Plugin 应通过只读 Volume 挂载，并通过对应配置环境变量
指向容器内路径。

## Redteam nornsd

`redteam/` 在 Python slim 上安装 Node.js 24、Claude Code、渗透测试工具链，以及
`domain/redteam` 的 Agent 扩展和 Skill。默认启用 Norns 模式并监听 `0.0.0.0:28081`。

构建脚本会按当前主机架构选择 Linux Release：

```sh
container/redteam/build.sh
```

Apple Container 使用 `--backend apple`。可选构建参数：

- `--image IMAGE`：覆盖 Python 基础镜像；
- `--tag IMAGE`：覆盖输出镜像；
- `--proxy host:port`：为国外资源配置构建期代理；
- `--use-mirror`：使用阿里云公网 APT/PyPI 和 npmmirror；
- `--use-ecs-mirror`：使用阿里云 ECS 内网 APT/PyPI，并自动启用其他国内镜像。

启动示例：

```sh
docker run --detach \
  --name norns-redteam \
  --publish 28081:28081 \
  --volume norns-redteam-workspace:/workspace \
  --env LLM_URL='https://example.test/anthropic' \
  --env LLM_KEY='replace-me' \
  --env LLM_MODEL='model-name' \
  nornsd-redteam-server:latest
```

默认工作区是 `/workspace`，Server Token 持久化到 `/workspace/.nornsd-token`。镜像以
root 运行以保留部分安全工具能力，但默认不增加额外 capability；仅在任务确有需要时
增加 `NET_RAW` 或 `NET_ADMIN`，不要默认使用 `--privileged`。

镜像验证：

```sh
docker run --rm --entrypoint python3 \
  nornsd-redteam-server:latest \
  /usr/local/bin/nornsd-verify
```

## Redteam benchmark

`redteam_for_benchmark/` 是自包含评测镜像：镜像内启动一个并发为 3 的 nornsd，三个
worker 持续消费同一个 FIFO 尝试队列。未完成题目会增加单题限时后回到队尾，不等待
其他题目的首轮结束；第二轮及后续轮次使用 `resume_session` 延续最新 Runtime Session。

构建：

```sh
container/redteam_for_benchmark/build.sh
```

构建脚本支持与 redteam Server 相同的 `--backend`、`--image`、`--tag`、`--proxy`、
`--use-mirror` 和 `--use-ecs-mirror` 参数。

需要上传到远端构建时，运行该目录的统一打包脚本：

```sh
container/redteam_for_benchmark/package.sh
```

压缩包写入 `container/redteam_for_benchmark/redteam_for_benchmark.zip`。解压后应从项目树
根目录执行同一构建命令。

运行时必须提供 benchmark 地址、Token 和单一模型配置：

```sh
docker run --rm \
  --name norns-redteam-benchmark \
  --volume norns-redteam-benchmark-workspace:/workspace \
  --env BENCHMARK_BASE_URL='https://benchmark.example.test' \
  --env BENCHMARK_TOKEN='replace-me' \
  --env LLM_URL='https://llm.example.test/anthropic' \
  --env LLM_KEY='replace-me' \
  --env LLM_MODEL='model-name' \
  nornsd-redteam-benchmark:latest
```

主要调度参数：

| 变量 | 默认值 | 用途 |
| --- | ---: | --- |
| `BENCHMARK_INITIAL_LIMIT_MINUTES` | 40 | 首轮单题限时 |
| `BENCHMARK_LIMIT_INCREMENT_MINUTES` | 30 | 每轮限时增量 |
| `BENCHMARK_MAX_CHALLENGE_TIMEOUT_MINUTES` | 240 | 单题限时上限 |
| `BENCHMARK_EVENT_LIMIT` | 5000 | 单次 Event 读取上限 |
| `BENCHMARK_POLL_INTERVAL` | 4 | Runtime 和 Event 轮询间隔，单位秒 |
| `BENCHMARK_STOP_GRACE` | 90 | Runtime 优雅停止等待时间，单位秒 |
| `BENCHMARK_RESULT_FILE` | `/workspace/benchmark-result.json` | 增量结果文件 |
| `BENCHMARK_ONLY_CODES` | 空 | 仅运行指定 unique_code，逗号分隔 |
| `BENCHMARK_OVERALL_TIMEOUT` | 0 | 全局秒级限时，0 表示不限 |

过程日志写入 `/workspace/benchmark-process.log`，nornsd Token 持久化到
`/workspace/.nornsd-token`。镜像验证：

```sh
docker run --rm --entrypoint python3 \
  nornsd-redteam-benchmark:latest \
  /usr/local/bin/nornsd-verify
```

## Redteam benchmark CC 对照组

`redteam_for_benchmark_cc/` 与 Norns benchmark 使用相同的基础镜像、红队工具链、
Claude Code 版本、三槽 nornsd 和 FIFO 调度器。差异仅在 Runtime Profile：该镜像强制
`NORNS_ENABLE=false`，不加载 Norns Domain、Norns Skill 或 Norns Agent，只通过
`CUSTOM_SKILLS_DIR=/norns/domain/redteam/skills` 为普通 Claude Code 加载相同领域 Skill。

构建和打包：

```sh
container/redteam_for_benchmark_cc/build.sh
container/redteam_for_benchmark_cc/package.sh
```

运行参数与 Norns benchmark 相同，默认镜像为
`nornsd-redteam-benchmark-cc:latest`：

```sh
docker run --rm \
  --name norns-redteam-benchmark-cc \
  --volume norns-redteam-benchmark-cc-workspace:/workspace \
  --env BENCHMARK_BASE_URL='https://benchmark.example.test' \
  --env BENCHMARK_TOKEN='replace-me' \
  --env LLM_URL='https://llm.example.test/anthropic' \
  --env LLM_KEY='replace-me' \
  --env LLM_MODEL='model-name' \
  nornsd-redteam-benchmark-cc:latest
```

对照组的结果、过程日志、nornsd 日志和 Subject 映射分别使用
`benchmark-cc-result.json`、`benchmark-cc-process.log`、`nornsd-cc.log` 和
`.benchmark-cc-subjects-<port>.json`。实验组和对照组仍应使用独立 Volume，避免共享
SQLite、Subject 工作区和 Runtime 产物。普通 Claude Code 没有 status view，因此 Runtime
进入 `waiting` 且 flag 未找齐时，调度器直接发送继续指令；第二轮及后续轮次仍使用
`resume_session` 延续会话。

## 安全边界

- 四个镜像都只复制公开的配置模板，不复制 `bin/agent.yaml`；
- `LLM_KEY`、benchmark Token 和 Server Token 只在运行时注入；
- 不要把包含真实凭据的 `.env`、Runtime 工作区或构建缓存发布为镜像层；
- nornsd HTTP API 使用明文 Bearer Token，跨主机部署时应位于可信网络或 TLS 反向代理后。
