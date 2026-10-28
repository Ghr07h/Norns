# nornsd

`nornsd` 是面向批量任务的 Agent Runtime 管理服务。它将不同业务对象或问题隔离为 Subject，统一创建和排队 Runtime、限制并发、投递 instruction，并保存 Event、运行历史和 artifacts，适合企业内部大量相似模式任务的持续处理。

nornsd 可以独立批量驱动普通 Agent，也可以通过 `--norns` 启用可选的 Norns 模式。Norns 模式在相同的 Subject、Runtime 和队列能力上增加 Norns Plugin、开放任务求解流程与 View；它不是运行 nornsd server 的前置条件。

正式运行路径为 Go 单一可执行文件与 Claude Code CLI 2.1.199，不依赖 Python 或 Claude Agent SDK。运行前需要确保 `claude` 可执行文件版本为 `2.1.199`；启用 Norns 模式时，还需要在 PATH 或 nornsd 同目录提供支持 protocol v2 的 `norns`。

Norns Plugin 在 Linux 和 macOS 上使用 POSIX `sh`/`awk` PreToolUse hook，拦截 Bash 工具中由 shell snapshot shadow 到 Claude Code 内嵌 ugrep 7.5.0 的危险 `grep` 正则；其他系统不会发布该 hook。两个 bounded repeat（例如同一模式内出现两次 `{0,40}`）会触发旧版 ugrep 的正则编译内存爆炸；显式使用 `/usr/bin/grep`、Claude Code 内置 Grep 工具，或者改用 `awk` 不会命中此拦截。Claude Code 2.1.235 已通过升级内嵌 ugrep 修复该问题，但固定使用 2.1.199 的 Runtime 必须保留此 hook。

## 工作模式

- `nornsd server` 是主要的批量管理方式。调用方通过 HTTP API 创建多个 Subject 和 Runtime，nornsd 使用稳定 FIFO 和全局并发槽驱动执行。
- `nornsd agent` 用于本地调试与快速启动，直接在终端中创建或恢复一个 Subject，不进入 server 队列。
- 普通模式只管理 Agent Runtime；两个入口都可以通过 `--norns` 选择启用 Norns 能力。

Subject 通常对应一个需要持续处理的业务对象或问题，可以先后运行多个 Runtime。业务 Task 仍由具体 Agent workflow 或 Norns 定义，不是 nornsd 的资源。

Subject、Runtime 与 Claude Session 使用彼此清晰的身份边界：Subject ID 标识长期实体和工作目录；Runtime ID 标识单次执行，并同时作为该次新 Claude Session ID。默认情况下，每个 Runtime 都通过 `--session-id=<runtime_id>` 启动空白会话。调用方也可以显式恢复最新 Runtime 的会话上下文；nornsd 使用 `--resume=<previous_session_id> --fork-session --session-id=<runtime_id>` 派生新会话，仍保持 Runtime、Session 和 transcript 一一对应。

- 普通模式下，后续 Runtime 复用 Subject 工作目录中的文件，但不继承上一 Runtime 的 Claude 对话上下文。
- Norns 模式下，后续 Runtime 同样使用新 Claude Session；跨 Runtime 的持续状态来自 Subject 工作目录中的 `.norns/` Workspace、Task 和 Graph。Status、Trace 与 Visual View 是该状态的只读投影，不是会话存储。
- 同一个 Runtime 内追加的多条 instruction 继续使用该 Runtime 的同一个 Claude Session。
- `resume-session` 只继承最新 Runtime 的 Claude 对话上下文，不复用其 Session ID；派生会话的 Session ID 仍等于新 Runtime ID。

## 构建

从 `nornsd/` 目录执行：

```sh
go generate ./internal/assets
go build -o ./bin/nornsd ./cmd/nornsd
go test ./...
```

生成步骤将 `documents/protocol/protocol.md` 与整个 `plugin/` 目录编译为按 Agent 类型分层的内嵌 Plugin 资源。当前运行时按 `cc` 选择 `plugin/cc/`；协议版本必须与 `internal/version` 中的常量一致。

## 命令

```text
nornsd agent [--cc] [--resume-subject SUBJECT_UUID] [--resume-session]
              [--norns[=true|false]] [--setting[=FILE]] -- [AGENT_ARGS...]

nornsd server [--agent cc] [--host HOST] [--port PORT] [--token TOKEN]
              [--workspace-root PATH] [--norns[=true|false]]
              [--settings FILE]... [--concurrency N]

nornsd version [--json]
```

server 默认监听 `127.0.0.1:28081`，API 基础路径为 `/api/v1`。所有接口（包括健康检查）都要求 `Authorization: Bearer <token>`。Token 必须通过 `--token` 或 `agent.yaml` 的 `NORNS_SERVER_TOKEN` 提供。

## 快速开始

### 批量 Runtime 服务

启动一个普通 Agent 模式、具有两个并发槽的 server：

先将 `agent.yaml` 中的 `NORNS_ENABLE` 设置为 `false`，再启动：

```sh
nornsd server \
  --workspace-root ./runtime-root \
  --settings ./agent.yaml \
  --concurrency 2
```

通过同一个处理模式提交多个相互隔离的任务：

```sh
curl -X POST \
  -H 'Authorization: Bearer replace-with-a-long-random-token' \
  -H 'Content-Type: application/json' \
  --data '{"name":"review-repository-a","runtime":{"instruction":"审查 repository A 并输出风险清单"}}' \
  http://127.0.0.1:28081/api/v1/subjects

curl -X POST \
  -H 'Authorization: Bearer replace-with-a-long-random-token' \
  -H 'Content-Type: application/json' \
  --data '{"name":"review-repository-b","runtime":{"instruction":"审查 repository B 并输出风险清单"}}' \
  http://127.0.0.1:28081/api/v1/subjects
```

创建 Subject 与首个 Runtime 是一次原子操作。两个 Runtime 会根据 `--concurrency` 立即运行或进入 FIFO；调用方可以通过 `/api/v1/subjects`、`/api/v1/queue`、Runtime 和 Event 接口持续观察进展。

检查服务与队列：

```sh
curl -H 'Authorization: Bearer replace-with-a-long-random-token' \
  http://127.0.0.1:28081/api/v1

curl -H 'Authorization: Bearer replace-with-a-long-random-token' \
  http://127.0.0.1:28081/api/v1/queue
```

在空闲的已有 Subject 下创建 Runtime，并从其最新 Claude Session 派生对话上下文：

```sh
curl -X POST \
  -H 'Authorization: Bearer replace-with-a-long-random-token' \
  -H 'Content-Type: application/json' \
  --data '{"instruction":"继续上一轮尚未完成的工作","resume_session":true}' \
  http://127.0.0.1:28081/api/v1/subjects/SUBJECT_UUID/runtime
```

该请求要求 Subject 当前没有 Runtime 且至少有一个历史 Runtime；否则分别返回 `SUBJECT_RUNTIME_ACTIVE` 或 `RUNTIME_SESSION_NOT_FOUND`。不传或传入 `false` 时保持原行为：空闲时创建全新 Session，使用中时向当前 Runtime 追加 instruction。

### Norns 模式

需要用 Norns 协议处理开放任务时，将 `agent.yaml` 中的 `NORNS_ENABLE` 设置为 `true`：

```sh
nornsd server \
  --workspace-root ./runtime-root \
  --settings ./agent.yaml \
  --concurrency 2
```

HTTP 批量提交方式保持不变。每个 Subject 会获得 Norns Plugin 和 Workspace；Runtime 由 Norns Coordinator 驱动，服务额外提供 Status、Trace 与 Visual View API。`NORNS_ENABLE` 为 `false` 时不会预检 `norns` CLI，也不会生成 Norns Plugin。

### Direct agent

在终端中直接创建一个 Subject：

```sh
mkdir -p ./runtime-root
cd ./runtime-root
../nornsd/bin/nornsd agent --setting=../agent.yaml
```

`NORNS_ENABLE` 可以启用 Norns 模式；恢复 Subject 时必须保持原 Subject 的 Runtime Profile：

```sh
nornsd agent --resume-subject SUBJECT_UUID --setting=./agent.yaml
```

`--resume-subject` 选择已有 Subject，复用其工作目录并创建新的 Runtime。它不恢复上一 Runtime 的 Claude Session：新 Runtime ID 同时作为新的 Claude Session ID，Claude Code 启动参数中不会包含 `--resume`。普通模式只能从工作目录中的文件延续工作；Norns 模式则由新的 Coordinator 读取 `.norns/` 持久状态，重新评估并继续探索。

需要同时恢复最新 Runtime 的 Claude 对话上下文时，显式增加 `--resume-session`：

```sh
nornsd agent --resume-subject SUBJECT_UUID --resume-session --setting=./agent.yaml
```

`--resume-session` 必须与 `--resume-subject` 一起使用，Subject 必须空闲且至少存在一个历史 Runtime。nornsd 读取最新 Runtime 实际使用的 Session ID，并通过 Claude Code 的 `--resume` 与 `--fork-session` 将上下文派生到新 Runtime ID 对应的新 Session；上一 Session 和 transcript 不会被继续写入。

Claude Code 会为后台任务通知额外输出 `origin.kind=task-notification` 的 `result`。当前进程创建的后台任务可能产生非零 `num_turns` 和 token 用量；恢复 Session 时，上一进程遗留任务的通知则常见零 turn、零 token，而且可能晚于新 Runtime 的首条 instruction 到达。nornsd 只按 `origin.kind` 识别这类通知，保留其结果和诊断字段，但不会将其视为 instruction 完成，也不会减少 `inflight_turns`。

恢复 Subject 时有以下边界：

- 必须在原 `<nornsd_root>` 中执行，而不是进入 Subject 目录执行；当前目录决定要打开的 `.nornsd_root/nornsd.sqlite`。
- server 必须已经完全停止。direct agent 与 server 使用同一 root lock，不能同时操作同一个根目录。
- Subject 必须处于 `idle`，并保持原 Runtime Profile（Agent 类型和是否启用 Norns）不变。
- `--resume-session` 还要求存在历史 Runtime；连续使用时始终从最新 Runtime 的派生 Session 继续。
- direct agent 会在原数据库中写入新的 Runtime、Event、日志和 transcript；退出后重启 server 可以继续读取这些记录。
- 不提供 `--resume-subject` 会创建新 Subject。仅在 Subject 目录执行裸 `nornsd agent` 也不会恢复该 Subject，而会把该目录当作新的 nornsd root。

`--settings` 可以不提供、提供一份供全部槽复用，或提供与 `--concurrency` 数量相同的多份配置。server 启动时读取并冻结配置；修改源文件需要重启 server 才会生效。多份配置中的第一份负责 `NORNS_ENABLE` 和 `NORNS_SERVER_*` 全局字段。YAML 中显式声明的值优先于 `--norns`、`--host`、`--port` 和 `--token`。

## Runtime 内存保护

server 托管的 Runtime 在所有平台统一按完整进程树 RSS 监控，不依赖 Windows Job Object 或 Linux cgroup 硬限制。Runtime 启动后立即采样，随后每 500ms 采样一次，记录追加到 `.nornsd/runtimes/<runtime_id>/memory-samples.jsonl`；同一采样同时用于 Runtime 资源摘要和超限判断。

单 Runtime 终止额度是系统内存基准的 50%。Windows 与 macOS 使用物理内存；Linux 使用宿主机 `MemTotal`，但在 Docker、Podman、Kubernetes 等 cgroup 环境中发现更小的有限 `memory.max` 或 `memory.limit_in_bytes` 时，改用该限额作为内存基准。容器未设置有限内存额度时仍使用宿主机内存。

RSS 超限后仅终止对应 Runtime 进程树，并将 `end_reason` 和 `error_code` 统一记录为 `RUNTIME_MEMORY_LIMIT_EXCEEDED`。若紧邻的上一 Runtime 因此结束，Norns Subject 创建下一 Runtime 时会在首条 instruction 前注入恢复提示；普通 Subject 只有在显式使用 `resume_session` 时才注入，创建全新 Session 时不注入。

## agent.yaml

示例配置位于 `internal/config/agent.example.yaml`，它是启动 nornsd 时提供的外部输入，不会编译进二进制：

```sh
cp internal/config/agent.example.yaml ./agent.yaml
```

`nornsd agent --setting=./agent.yaml` 显式使用该文件。裸 `nornsd agent --setting` 自动读取可执行文件同目录的 `agent.yaml`；使用 `--resume-subject` 时再合并 Subject 当前配置。自动发现成功并完成预检后，nornsd 会在 stderr 列出实际配置来源，等待 3 秒再启动 Claude Code，期间可以按 `Ctrl+C` 取消。

`NORNS_ENABLE` 同样适用于 direct agent，并优先于 `--norns`。`NORNS_SERVER_HOST`、`NORNS_SERVER_PORT` 和 `NORNS_SERVER_TOKEN` 仅供 server 使用。

各 Agent 的基础配置模板按类型位于 `internal/config/templates/<agent>/`，并编译进 nornsd；Claude Code 使用 `internal/config/templates/cc/settings.json`。运行时根据 Runtime Profile 的 Agent 类型选择模板，解析 JSON，并以 `agent.yaml` 中的 URL、密钥和模型字段生成本次 Runtime settings。server 托管 Runtime 和提供了 `--setting` 的 direct agent 会传入 `--setting-sources=`，不额外读取用户、Subject project 或 local settings；裸 `nornsd agent` 不传该参数，沿用 Claude Code 的默认配置来源。显式生成的临时 settings 和 `--plugin-dir` 仍会加载。

`NORNS_EXPLORATION_TIMEOUT` 设置 running Exploration 的超时问询周期，单位为秒，默认 `1200`。该值会注入 Coordinator 的 Runtime 配置；Coordinator 在运行时长达到该周期及其后续整数倍时执行问询或终止规则。

nornsd 启动的所有 Claude Code Runtime 都固定传入 `--dangerously-skip-permissions`，并在子进程环境中强制设置 `IS_SANDBOX=1`，避免 Claude Code 在 root/sudo 环境下拒绝该参数。Runtime 不会停下来请求工具权限确认，因此只应在可信且权限受限的 Subject 工作目录中运行 nornsd。

`CUSTOM_PLUGIN_DIRS` 是现有 Plugin 目录列表，可以为空或包含一个或多个目录；相对路径以声明它的 `agent.yaml` 为基准解析。每个目录都会按列表顺序通过独立的 `--plugin-dir` 参数传给 Agent CLI。

`CUSTOM_AGENTS_DIR` 和 `CUSTOM_SKILLS_DIR` 会复制到当前 Runtime 独立生成的 `nornsd-custom` Plugin，不会写入 Subject 的 `.claude/`。Plugin 加载顺序依次为 `CUSTOM_PLUGIN_DIRS`、动态生成的 `nornsd-custom`、Norns Profile 的 `norns` Plugin。Norns Profile 通过 `--agent norns:coordinator` 让 Coordinator 作为根 Agent；普通 Profile 不传 `--agent` 或 `--system-prompt`，沿用 Agent 默认系统提示词。

生成 Norns Plugin 时，Coordinator 的 `Agent(...)` 工具白名单会从最终 Plugin 集合动态生成：包括 Norns 内置 Agent、领域 Agent、`NORNS_CUSTOM_AGENTS_DIR`、`CUSTOM_AGENTS_DIR`，以及各 `CUSTOM_PLUGIN_DIRS/agents/` 下声明的 Agent。外部 Plugin 的命名空间读取自其 `.claude-plugin/plugin.json`。

`NORNS_CUSTOM_DOMAIN_DIR` 指向一个包含 `meta.json` 和领域知识文件的目录：

```json
{
  "domain_name": "code_review",
  "domain_decider_description": "选择代码审查方向。",
  "domain_decider_extra_info_file": "decider.md",
  "domain_explorer_description": "执行代码审查并收集证据。",
  "domain_explorer_extra_info_file": "explorer.md"
}
```

启用 Norns Profile 时，它会生成 `decider-for-<domain_name>.md` 和 `explorer-for-<domain_name>.md`。例如 `domain_name` 为 `code_review` 时，Agent 名称分别是 `decider-for-code_review` 和 `explorer-for-code_review`。生成器会将 description 写入 frontmatter，把相应 extra-info 文件追加到正文末尾，并把二者加入 Coordinator 的可调用 Agent 列表。文件路径相对领域目录解析且不能逃逸目录。

三个 `NORNS_CUSTOM_*` 字段只在 Norns Profile 下参与 Norns Plugin 生成；`CUSTOM_AGENTS_DIR` 和 `CUSTOM_SKILLS_DIR` 生成独立的 Runtime Plugin，`CUSTOM_PLUGIN_DIRS` 直接加载已有 Plugin。`LLM_KEY`、生成 settings、Runtime artifacts 和 transcript 可能包含敏感信息。

## 数据与安全

`--workspace-root` 中的 `.nornsd_root/` 保存服务身份和 SQLite，Subject 子目录保存配置、Plugin、日志、Runtime artifacts、Claude transcript 和可选 Norns Workspace。每个 transcript 归档到 `.nornsd/runtimes/<runtime_id>/<runtime_id>.jsonl`，历史版本创建的 Subject 级 transcript 仍可读取。nornsd 不自动删除这些内容，备份时应将整个 root 作为一致的数据域。

不要把 `agent.yaml`、Subject `.nornsd/` 或服务 Token 提交到版本库。绑定非 loopback 地址仍使用明文 HTTP Bearer Token；应放在可信网络或 TLS 反向代理后。由于 Agent 固定跳过权限确认，Subject 工作目录及 Agent 进程权限本身也是安全边界。

## 文档

- [nornsd 设计与 HTTP API v1](../documents/design/nornsd.md)
- [Norns 协议](../documents/protocol/protocol.md)
