# nornsd 设计

状态：v1 当前设计  
服务 API：v1  
数据库 schema：v1  
Agent 配置 schema：v1

本文记录 `nornsd` 的长期产品设计、HTTP 契约与当前实现设计。`nornsd` 可以独立管理通用 Agent Runtime，也可以选择启用 Norns 模式；Norns 协议语义以 [`../protocol/protocol.md`](../protocol/protocol.md) 为准，`nornsd` 只通过公开的 `norns` CLI 与 Norns Workspace 交互，不直接读写其数据库。

# 第一部分：原始设计

## 1. 定位与边界

`nornsd` 是面向批量任务的 Agent Runtime 管理服务。它将业务对象或问题映射为相互隔离的 Subject，为其创建 Runtime、排队并发执行、持续投递 instruction，并保存 Event、运行历史和 artifacts。调用方可以按相同业务模式批量创建 Subject，由 nornsd 在固定 Runtime Profile 下驱动智能体完成处理。

典型场景包括企业内部批量代码审查、资料分析、工单处理、合规检查、迁移评估，以及其他输入对象不同但处理模式相似的智能体任务。nornsd 负责执行控制与可观察性，不定义上层业务批次、任务模板或结果汇总规则；这些由调用方或 Well 等客户端组织。

nornsd 包含两个公开运行方式：

- `nornsd server`：主要的批量管理方式，持久化管理多个 Subject、Runtime、队列和并发槽，并提供 HTTP API；
- `nornsd agent`：面向调试和快速启动，在终端中直接创建或恢复一个 Subject，并交互式运行 Agent。

Subject 是 nornsd 的长期管理单位，通常对应一个需要持续处理的业务对象或问题。一个 Subject 可以先后拥有多个 Runtime，但同一时间最多存在一个 current 或 queued Runtime。业务 Task 属于 Norns 或其他 Agent workflow，不是 nornsd 资源；nornsd 的批量能力来自对多个 Subject 与 Runtime 的统一排队、并发控制和观察，而不是把业务 Task 固化为 nornsd 实体。

nornsd 的通用 Runtime 管理不依赖 Norns。普通模式直接批量驱动 Agent 处理 instruction；可选的 Norns 模式在相同 Runtime 管理能力上增加 Norns Plugin、开放任务求解过程和 Norns View。启用 Norns 时，独立 adapter 负责 CLI 预检、Plugin 生成、Task close 和 View 读取；Norns 不参与 nornsd 的 Subject、队列或并发模型。

v1 只实现 Claude Code adapter，并固定兼容 Claude Code `2.1.199`。实现为单一 Go 可执行文件，不依赖 Python 或 Claude Agent SDK。

## 2. 核心模型

### 2.1 Root

`nornsd_root` 是一组 Subject、数据库与运行记录的控制域。每个 root 具有稳定 UUID。同一个 root 同时最多由一个 `nornsd server` 或 direct agent 持有；不同 root 相互独立。

### 2.2 Runtime Profile

Profile 在 server 启动时固定并由 Subject 永久继承。普通模式不声明额外 capability：

```json
{
  "agent": "cc",
  "agent_version": "2.1.199",
  "capabilities": []
}
```

Norns 模式在同一 Profile 上启用 `norns` capability：

```json
{
  "agent": "cc",
  "agent_version": "2.1.199",
  "capabilities": ["norns"]
}
```

HTTP 请求不能覆盖 Agent 类型、Norns 模式或 settings。Profile 不兼容的 Subject 不得在当前 server 中启动，因此同一个 server 管理的批量任务具有一致的 Agent 类型与 capability 基线。

### 2.3 Subject

Subject 包含稳定 UUID、可选名称、永久 Profile、工作目录身份与 Runtime 历史。状态：

| 状态 | 含义 |
| --- | --- |
| `idle` | 没有 current 或 queued Runtime。 |
| `queuing` | 存在 queued Runtime，尚未取得并发槽。 |
| `using` | Runtime 已取得槽位，可能处于 running、waiting 或收尾状态。 |

Subject 不因 Runtime 正常、异常或取消结束而删除。目录和历史默认永久保留。

Subject ID 只标识长期业务实体和工作目录，不作为新 Runtime 的 Agent 会话 ID。数据库和 `subject.json` 中的 Subject 级 session 字段仅为 v1 历史数据兼容保留。新 Runtime 的 Claude Session ID 始终等于 Runtime ID；显式恢复上下文时，从最新 Runtime Session fork 到这个新 Session，而不是复用旧 Session ID。

### 2.4 Runtime

Runtime 在进入队列时获得独立 UUID，并永久归属于创建它的 Subject：

```text
queued -> running <-> waiting -> ended
                    \--------> error
                    \--------> cancelled
```

- `queued`：尚未取得槽位；
- `running`：Agent 正在处理输入；
- `waiting`：Agent 已完成当前 turn，等待 instruction；仍占用槽位；
- `ended`：Norns Runtime 通过正常 Task close 结束；
- `error`：启动、协议、资源或 Agent 错误；
- `cancelled`：外部优雅或强制停止。

普通 Agent 的成功 turn 不会自动结束 Runtime，而是进入 waiting。Runtime 终态不等于业务目标完成。

Runtime ID 同时是该次 Claude Session ID。每个 Runtime 都创建新 Claude 会话，同一个 Runtime 内的 initial instruction 和后续 instruction 共享该会话。默认情况下，不同 Runtime 即使属于同一 Subject，也不继承 Claude 对话；显式 `resume-session` 时通过 `--resume` 和 `--fork-session` 读取最新 Runtime 的上下文，并派生到以新 Runtime ID 命名的新 Session。

- 普通模式跨 Runtime 只复用 Subject 工作目录中的文件；
- Norns 模式跨 Runtime 复用 `.norns/` Workspace 和 Graph，由新 Coordinator 会话读取持久状态并重新评估；
- Norns View 是持久状态的只读投影，不承担状态存储或会话恢复。

### 2.5 Instruction 与 Event

instruction 是 Runtime 内待写入 Agent 的 FIFO。每个 Runtime 最多保留 16 条 pending instruction；已经确认写入 Agent 的记录不再出现在待发送数组中，但持久记录不删除。结果不确定的 instruction 不得自动回到 pending。

Event 是 Subject 内单调递增、永不复用序号的人类可读投影。原始 Agent 帧和 readable Event 分开保存；公共 API 只暴露 readable Event，不把 Claude stream-json 变成公共协议。

## 3. 调度与生命周期

批量调用方可以连续创建多个带首条 instruction 的 Subject，或向既有 Subject 提交 instruction。每条空闲 Subject 的新执行请求创建 queued Runtime；server 的 `--concurrency` 限制同时取得槽位的 Runtime 数，超出部分进入稳定 FIFO。direct agent 不参与 server 队列。

队列遵循稳定 FIFO。取得槽位后才物化尚未创建的 Subject 目录、发布配置与 Plugin、启动 Agent。waiting Runtime 持续占用槽位，不因队列压力或等待时间自动回收；只有明确停止、正常结束或异常退出才释放。

向 idle Subject 提交 instruction 会创建新 queued Runtime；向 queued/running/waiting Runtime 提交 instruction 会追加 FIFO。提交给 waiting Runtime 后唤醒为 running。进入 stopping、closing 或终态后拒绝新 instruction。

停止模式：

- `graceful`：请求 Agent 收尾，超时后升级为强制终止；
- `force`：立即终止托管进程树。

Norns Runtime 外部停止时，nornsd 先停止 Agent/Explorer 写入，再在宽限期内执行符合协议的 failed/cancelled close。Agent 主动完成 Task 时进入 closing，等待正常进程退出；业务 Task outcome 与 Runtime 控制状态保持分离。

外部停止或 Runtime 错误导致的 Norns close 是复合语义：当前 Runtime 进入 `cancelled` 或 `error`，未完成 Exploration 以 `cancelled` 结束，但 Exploration 元数据、已经持久化的 Cognition、Event 和该 Runtime 的 transcript 保留。后续 Runtime 使用新的 Claude Session 读取既有 Graph，可以选择对历史 Exploration 执行 `retry`；retry 创建新 Exploration，不覆盖原中断记录。

## 4. 配置模型

`agent.yaml` 是外部配置，不编译进 nornsd。配置版本键为大写 `VERSION`。规范示例位于 `nornsd/internal/config/agent.example.yaml`。

| 字段 | 含义 |
| --- | --- |
| `VERSION` | Agent 配置 schema，当前为 `1`。 |
| `LLM_URL` | LLM API 基础地址。 |
| `LLM_KEY` | LLM 凭据。 |
| `LLM_MODEL` | 主模型。 |
| `LLM_FAST_MODEL` | 快速模型；空值回退主模型。 |
| `NORNS_ENABLE` | 是否启用 Norns Runtime Profile；显式配置时优先于 `--norns`。 |
| `NORNS_SERVER_HOST` | Server 监听地址；显式配置时优先于 `--host`。 |
| `NORNS_SERVER_PORT` | Server 监听端口；显式配置时优先于 `--port`。 |
| `NORNS_SERVER_TOKEN` | Server Bearer Token；显式配置时优先于 `--token`。 |
| `NORNS_MAX_CONCURRENT_EXPLORATIONS` | Coordinator 使用的额定并发，默认 `3`。 |
| `NORNS_EXPLORATION_TIMEOUT` | running Exploration 的超时问询周期，单位为秒，默认 `1200`。 |
| `NORNS_ALLOW_HUMAN_INTERVENTION` | 是否允许 Coordinator 等待人类实时回答。 |
| `NORNS_CUSTOM_DOMAIN_DIR` | 领域元数据与知识目录。 |
| `NORNS_CUSTOM_SKILLS_DIR` | 合并进临时 Norns Plugin 的 Skill。 |
| `NORNS_CUSTOM_AGENTS_DIR` | 合并进临时 Norns Plugin 的 Agent。 |
| `CUSTOM_AGENTS_DIR` | 复制到 Runtime 临时 `nornsd-custom` Plugin 的普通 Agent。 |
| `CUSTOM_SKILLS_DIR` | 复制到 Runtime 临时 `nornsd-custom` Plugin 的普通 Skill。 |
| `CUSTOM_PLUGIN_DIRS` | 按声明顺序直接传给 Agent CLI 的已有 Plugin 目录列表，可为空。 |
| `AGENT_EXEC` | Agent 可执行文件和固定 argv，默认 `claude`。 |

相对目录以声明值的 YAML 文件为基准。解析严格拒绝未知字段、重复 key、错误类型和多文档输入。

配置来源：

| 调用 | 行为 |
| --- | --- |
| agent 不提供 `--setting` | 不生成显式 settings，也不传 `--setting-sources=`，沿用 Claude Code 的默认配置来源。 |
| agent 裸 `--setting` | 合并可执行文件同目录 `agent.yaml` 与 `--resume-subject` 选中 Subject 的配置。 |
| agent `--setting=FILE` | 只读取指定文件。 |
| server `--settings FILE` | 启动时加载一个冻结 settings 槽，可重复；第一份配置提供全局 `NORNS_ENABLE` 与 `NORNS_SERVER_*`。 |

裸 `--setting` 完成解析和预检后打印实际来源，等待 3 秒再启动，允许 `Ctrl+C` 取消。显式文件和 server 不等待。

### 4.1 领域目录

`NORNS_CUSTOM_DOMAIN_DIR/meta.json` 使用严格 JSON：

```json
{
  "domain_name": "code_review",
  "domain_decider_description": "选择代码审查方向。",
  "domain_decider_extra_info_file": "decider.md",
  "domain_explorer_description": "执行代码审查并收集证据。",
  "domain_explorer_extra_info_file": "explorer.md"
}
```

`domain_name` 匹配 `^[a-z0-9][a-z0-9_-]{0,63}$`。description 非空；extra-info 是领域目录内的相对普通文件，符号链接解析后也不能逃逸。配置加载时即读取并冻结内容。

## 5. HTTP API v1

### 5.1 通用约定

- 基础路径：`/api/v1`；
- 请求和普通响应：UTF-8 JSON；Visual View 成功时为 HTML；
- 所有接口，包括 service info 与 health，都要求 `Authorization: Bearer <token>`；
- 时间：UTC RFC 3339，至少毫秒精度；
- 字段和枚举：英文 `snake_case`；
- 请求体上限 2 MiB，单条 instruction 上限 1 MiB；
- 普通 handler deadline 为 5 秒；
- 未知请求字段返回 `400 INVALID_REQUEST`；
- v1 使用普通请求/轮询，不提供 SSE 或 WebSocket。

错误 envelope：

```json
{
  "error": {
    "code": "SUBJECT_NOT_FOUND",
    "message": "subject was not found",
    "request_id": "req_...",
    "details": {}
  }
}
```

稳定错误至少包括 `INVALID_REQUEST`、`INVALID_CURSOR`、`UNAUTHORIZED`、`SUBJECT_NOT_FOUND`、`RUNTIME_NOT_FOUND`、`RUNTIME_SESSION_NOT_FOUND`、`SUBJECT_NOT_IDLE`、`SUBJECT_RUNTIME_ACTIVE`、`RUNTIME_PROFILE_MISMATCH`、`RUNTIME_STOPPING`、`RUNTIME_CLOSING`、`CAPABILITY_NOT_ENABLED`、`SUBJECT_NOT_MATERIALIZED`、`REQUEST_TOO_LARGE`、`RUNTIME_INSTRUCTION_REJECTED`、`AGENT_UNAVAILABLE`、`SERVICE_UNAVAILABLE`、`DEADLINE_EXCEEDED` 和 `INTERNAL_ERROR`。

### 5.2 路由

| 方法 | 路径 | 成功 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/v1` | 200 | 服务身份、版本、Profile、容量与能力。 |
| GET | `/api/v1/health` | 200 | 数据库和调度器健康。 |
| POST | `/api/v1/subjects` | 201 | 创建 Subject，可原子创建首个 queued Runtime。 |
| GET | `/api/v1/subjects` | 200 | 按状态分页列出 Subject。 |
| GET | `/api/v1/subjects/{subject_id}` | 200 | Subject 详情。 |
| POST | `/api/v1/subjects/{subject_id}/runtime` | 202 | 创建 Runtime、追加 instruction，或 fork 最新 Session。 |
| GET | `/api/v1/subjects/{subject_id}/runtime` | 200 | 当前/排队 Runtime 快捷查询。 |
| POST | `/api/v1/subjects/{subject_id}/runtime/stop` | 202 | `graceful` 或 `force` 停止。 |
| GET | `/api/v1/queue` | 200 | 全局 Runtime Queue。 |
| GET | `/api/v1/subjects/{subject_id}/events` | 200 | 人类可读 Event 最新窗口、增量与历史读取。 |
| GET | `/api/v1/subjects/{subject_id}/events/updates` | 200 | 检查指定 Event 游标后是否有新记录。 |
| GET | `/api/v1/subjects/{subject_id}/runtimes` | 200 | Runtime 历史。 |
| GET | `/api/v1/subjects/{subject_id}/runtimes/{runtime_id}` | 200 | 稳定 ID Runtime 详情。 |
| GET | `/api/v1/subjects/{subject_id}/norns/views/status` | 200 | Norns JSON Status View。 |
| GET | `/api/v1/subjects/{subject_id}/norns/views/trace` | 200 | Norns JSON Trace View。 |
| GET | `/api/v1/subjects/{subject_id}/norns/views/visual` | 200 HTML | Norns Visual View。 |

### 5.3 服务身份与能力

`GET /api/v1` 返回 `service=nornsd`、稳定 `server_id`、产品/API 版本、Runtime Profile、协议版本、容量和 capability 列表。Well 使用 `server_id` 检测同一 URL 后的数据域变化。

health 只说明 HTTP、SQLite 和调度器可以继续接受请求，不保证外部 LLM 可用。

### 5.4 Subject 与 Runtime 表示

Subject 详情包含：

- `id`、`name`、`state` 与固定 `runtime_profile`；
- `workspace.materialized`、目录名和服务端绝对路径；
- current/queued Runtime 或 null；
- `last_event_seq`、创建和更新时间。`last_event_seq` 是实体统计字段，不是 HTTP Event
  续读游标，客户端不得将它传给 `after` 或 `cursor`。

Runtime 详情包含稳定 ID、Subject ID、状态、Agent/session、排队/开始/结束时间、end reason、settings 槽与配置 hash、进程结果、usage、资源摘要、错误和 artifact 定位路径。新 Runtime 的 `session_id` 与 Runtime `id` 相同；恢复模式在 `process.resume_session_id` 记录其来源 Session。API 不返回 LLM Key、完整 settings、环境变量、raw log 或 transcript 内容。

Runtime 历史分页响应包含摘要数组、`next_cursor` 和当前 Subject 的稳定 `total_count`；每条摘要包含 `created_at`、`started_at` 和 `ended_at`，客户端无需逐条读取详情来显示历史列表。

创建 Subject 可选携带首条 instruction。携带时，Subject、Runtime、instruction 与 queue 记录在同一事务中创建；不携带时，Subject 在目录和本地身份成功物化后以 idle 返回。

提交 instruction 对 idle Subject 创建 Runtime，对已有可接收 Runtime 追加 FIFO。请求体可以增加 `"resume_session": true`；此时 Subject 必须为 idle 且至少存在一个历史 Runtime，服务读取最新 Runtime 的 Session ID，创建新的 queued Runtime，并通过 `--resume`、`--fork-session` 和新 Runtime ID 派生会话。如果 Subject 已有 current/queued Runtime，则返回 `409 SUBJECT_RUNTIME_ACTIVE`，不会追加 instruction；没有历史会话则返回 `409 RUNTIME_SESSION_NOT_FOUND`。`202 Accepted` 只表示持久化接受，不表示已经执行。

### 5.5 分页与 Event

Subject、Queue 和 Runtime 历史使用不透明 keyset cursor：`limit` 默认 50，范围 1..200。cursor 绑定路由、稳定排序和筛选条件，不绑定 limit；跨路由或修改筛选条件复用返回 `INVALID_CURSOR`。

Subject、Queue 和 Runtime 历史的分页 cursor 与 Event cursor 互不通用。Event 直接读取
Subject workspace 中的 `.nornsd/log/readable.jsonl`，每次最多返回 5000 条，并使用绑定
Subject、读取方向、文件偏移、Event 序号和记录摘要的不透明 cursor：

- 不传 cursor：返回当前最新窗口，同时返回用于后续增量同步的 `cursor`；存在更早记录时
  通过 `next_cursor` 提供向前翻页位置；
- `after=<cursor>`：从上次同步位置正向读取新 Event，响应中的 `cursor` 是下一次续读位置；
- `cursor=<next_cursor>`：读取更早的历史窗口；
- `GET .../events/updates?cursor=<cursor>`：仅判断该同步位置之后是否有新 Event。

Event cursor 是服务端签名的不透明值。整数 `seq`、`last_event_seq`、其他 Subject 的 cursor，
以及把历史 cursor 用作 `after`，均返回 `INVALID_CURSOR`。客户端必须持久保存响应中的
`cursor`，增量响应存在 `has_more=true` 时应立即继续读取，直至追到最新位置。

当前 Runtime 接口只表示 Subject 此刻正在排队或执行的 Runtime；Runtime 收尾后会返回
`runtime: null`。需要可靠观察某次执行终态的调用方必须保存提交响应中的 `runtime.id`，并
使用 `/subjects/{subject_id}/runtimes/{runtime_id}` 读取稳定详情。

API 只返回 readable Event，不提供 raw 类型切换、任意文件读取或诊断包接口。

### 5.6 Norns View

View API 仅在 Profile 启用 `norns` 时可用。status/trace 原样返回 `norns ... --json` 数据，不添加 nornsd envelope。visual 必须验证 CLI 返回文件位于当前 Subject `.norns/view/` 内，再以 `text/html` 返回。

三个 View 接口都在 `X-Norns-View-Filename` 响应头返回本次 Norns 已发布 View 的文件名。nornsd 读取 Status/Trace 时要求 CLI 返回已发布文件引用，即使正文未超过 CLI 的内联输出上限也不通过目录扫描猜测文件；HTTP 正文格式不受此元数据影响。

Subject idle 时仍可读取 View；未启用能力返回 `CAPABILITY_NOT_ENABLED`，未物化返回 `SUBJECT_NOT_MATERIALIZED`。nornsd 不直接读取 Norns SQLite。

### 5.7 v1 明确不提供

v1 不提供 Subject 修改/删除/归档/迁移、配置 API、独立 queue cancel、原始日志下载、diagnostics、幂等键、用户/角色或 Token 轮换。客户端不得自动重试创建和 instruction 提交；结果不确定时先查询 Subject、Runtime 与 Event。

# 第二部分：实现设计

## 6. 技术和模块

nornsd 使用 Go 实现，核心依赖方向：

```text
cli/httpapi -> app -> domain
runtime -> agent/cc + scheduler + store + artifacts
config/plugin/root -> domain + filesystem
```

主要包：

| 包 | 职责 |
| --- | --- |
| `cli` | Cobra 命令、direct agent 与 server 启动。 |
| `httpapi` | 路由、认证、限制、cursor 和 DTO。 |
| `app` | Subject/Runtime 用例。 |
| `domain` | 状态、实体和错误。 |
| `store/sqlite` | root 数据库和原子事务。 |
| `root` | root lock、Subject identity 和目录。 |
| `runtime` | Scheduler、Runtime manager、恢复和停止。 |
| `agent/cc` | Claude Code 预检、argv、stream-json 和 transcript。 |
| `config` | agent.yaml、settings、custom 资源与领域目录。 |
| `plugin/cc` | 临时 Norns Plugin 发布。 |
| `runtimeartifact` | Runtime 配置、launch 和脱敏快照。 |
| `eventlog` | raw/readable 日志和 Event 投影。 |
| `norns` | Norns CLI lifecycle 与 View adapter。 |
| `process` | 跨平台进程树和资源控制。 |

domain 不依赖 HTTP、Cobra、SQLite、Agent 或操作系统进程 API。

## 7. 文件布局

```text
<nornsd_root>/
  .nornsd_root/
    root.json
    nornsd.sqlite
    owner.lock
  <timestamp>_<subject_uuid>/
    .nornsd/
      subject.json
      config/
      plugin/
      staging/
      log/raw.jsonl
      log/readable.jsonl
      runtimes/<runtime_id>/
        config.snapshot.json
        launch.json
        plugin.manifest.json
        custom-plugin.manifest.json
        artifacts/config/
        artifacts/plugin/             # Norns Profile
        artifacts/custom-plugin/      # 配置普通 CUSTOM_* 时
        <subject_id>.jsonl
    .norns/                 # Norns Profile 时由 norns 管理
```

`root.json` 与 `subject.json` 是本地身份，不是数据库替代品。Subject 目录名创建后不变。用户数据、配置、Plugin、日志、transcript 和 artifacts 默认永久保留，权限按包含秘密的数据处理。

## 8. SQLite schema v1

root 数据库表：

| 表 | 职责 |
| --- | --- |
| `metadata` | schema、root ID、创建时间。 |
| `subjects` | Subject 身份、Profile、目录和状态。 |
| `runtimes` | Runtime 永久记录、状态、配置、进程、usage 和资源。 |
| `runtime_queue` | 稳定 FIFO 调度顺序。 |
| `runtime_instructions` | 有序输入及投递状态。 |
| `runtime_config_snapshots` | 脱敏配置解释信息。 |
| `events` | Subject 内单调序号的 readable Event。 |

关键事务：创建 Subject+Runtime+instruction+queue、领取 queue 槽、追加 instruction、确认投递、状态转换、stop 接受和 Runtime 收尾。队列领取在数据库事务中完成，避免多个 scheduler 重复启动。

## 9. 配置与 artifacts

server 启动时加载全部 `--settings` 并冻结到内存槽。文件随后变化不影响已启动 server。每个 Runtime 保存实际槽位、生成配置 hash 和脱敏快照。

Agent 基础配置模板按类型保存在 `internal/config/templates/<agent>/` 并编译进 nornsd。运行时根据 Runtime Profile 的 Agent 类型选择所需模板；当前 `cc` 模板经结构化 JSON 解析后注入 URL、token、主模型、fast model 和默认模型字段，禁止文本替换或 shell 展开。未知 Agent 类型必须明确失败，不能回退到其他 Agent 的模板。

发布流程使用同父目录 staging、受限权限与原子 rename：

1. 生成不可变 Runtime artifact；
2. 校验和计算 hash；
3. 从 artifact 发布当前 `.nornsd/config/` 与 Norns `.nornsd/plugin/`；
4. 普通自定义 Plugin 直接使用当前 Runtime 的不可变 artifact，不写入 Subject `.claude/`；
5. 保存各 Plugin 的 manifest/hash。

## 10. Plugin 生成

构建时从正式协议和整个 `plugin/` 目录生成按 Agent 类型分层的确定性资源并 `go:embed`。运行时根据 Runtime Profile 的 Agent 类型选择对应 Plugin；当前 `cc` 使用 `plugin/cc/`。运行时只依赖内嵌资源，不依赖源码相对路径，未知 Agent 类型必须明确失败。

运行时生成器：

- 注入 Task 并发与人类干预策略；
- 如果存在领域配置，从标准 Decider/Explorer 派生 `decider-for-<domain>.md` 与 `explorer-for-<domain>.md`；domain 内部命名保留下划线，例如 `decider-for-code_review`；
- 结构化改写 Agent frontmatter 的 name/description；
- 把 extra-info 原文附加到相应 Markdown 末尾；
- 把领域 Agent 加入 Coordinator allowlist 与运行提示；
- 合并 `NORNS_CUSTOM_AGENTS_DIR` 和 `NORNS_CUSTOM_SKILLS_DIR`；
- 解析 Agent frontmatter 和外部 Plugin metadata，为 Coordinator 结构化生成覆盖全部已加载 Agent 的 `Agent(...)` 工具白名单；
- 拒绝覆盖标准 Agent、动态领域 Agent 或 Norns Skill；
- 对完整发布树生成 manifest/hash。

普通 `CUSTOM_AGENTS_DIR` 和 `CUSTOM_SKILLS_DIR` 生成命名为 `nornsd-custom` 的独立 Runtime Plugin，不依赖 Norns Profile，资源不并入 `norns` 命名空间。`CUSTOM_PLUGIN_DIRS` 则直接引用已有 Plugin 目录。最终通过重复的 `--plugin-dir` 加载，顺序固定为 `CUSTOM_PLUGIN_DIRS` 中的目录、动态生成的 `nornsd-custom`、Norns Profile 的 `norns` Plugin。

## 11. Claude Code adapter

默认 `AGENT_EXEC=claude` 时，预检 PATH 和精确版本 `2.1.199`。显式 argv 覆盖表示用户承担包装器版本兼容性，但仍需可执行和 stream-json 兼容。

托管模式使用 Claude Code stream-json：

- 所有 Runtime 固定传入 `--dangerously-skip-permissions`，避免批量任务等待无法响应的权限确认；
- 默认命令以及显式直接指向 `claude`、`claude.exe` 或 `claude.cmd` 的 `AGENT_EXEC` 必须通过固定 Claude Code 版本检查；`cfuse` 等包装器无法可靠探测内部版本，保留兼容模式；
- 所有 Claude Code 子进程固定设置 `IS_SANDBOX=1`，使 root/sudo 环境可以使用 `--dangerously-skip-permissions`；
- 所有托管 Runtime 固定传入 `--setting-sources=`，仅加载显式 `--settings`、显式 Plugin 和不可关闭的 managed settings；direct agent 仅在提供 `--setting` 时启用该隔离；
- 普通 Profile 不覆盖系统提示词；Norns Profile 传入 `--agent norns:coordinator`，以 Norns Coordinator 作为根 Agent；
- 固定 Runtime ID 作为新 session ID，每个 Runtime 使用独立 Claude 会话；显式 `resume-session` 时传入 `--resume=<latest_session_id> --fork-session --session-id=<runtime_id>`；
- initial input 和后续 instruction 使用控制协议写入 stdin；
- 只有收到成功 control response 后才确认 instruction 已投递；
- assistant、tool、result、system 和 diagnostic 映射为 readable Event；
- 恢复 Session 启动时由 Claude 重放的旧 `result` 保留在 raw log，但投影为 `runtime.session_resumed`，不生成 `agent.result`，也不推进新 Runtime 的 turn。Claude 可能在首条 instruction 发出后才返回多个 `origin.kind=task-notification` 的结果；恢复遗留任务时这些结果常见零 turn、零 token，当前进程创建的后台任务则可能携带非零 `num_turns`、用量和结果文本。nornsd 只按 `origin.kind` 识别通知，不依赖计数或到达时序，运行期间的同类结果也不推进 instruction turn；
- Runtime waiting/running 由实际 turn 边界驱动，不从普通文本猜测；
- 进程结束后把 Claude 原生 transcript 归档到对应 Runtime 目录，并记录实际 Session ID、缺失或复制失败诊断；

direct agent 保留终端交互，但仍使用同一 Subject、配置、Plugin、identity 和 Runtime artifact 逻辑。

## 12. Scheduler、停止与资源

Scheduler 按固定槽位领取 queue。Manager 持有每个 active Runtime 的 cancel、stdin、进程树、状态和 Norns close 协调。一个 Runtime 的状态变化不能阻塞其他槽位。

stop 请求先原子持久化接受，再异步执行。优雅停止设置 deadline；超时后终止进程树。Norns close 使用独立宽限期，避免把 HTTP handler deadline 当作智能体收尾期限。

资源监控在所有平台统一使用进程树 RSS，不使用 Windows Job Object 或 Linux cgroup 执行硬限制。Manager 在 Runtime 启动后立即采样，此后每 500ms 采样一次；每次结果追加到 `.nornsd/runtimes/<runtime_id>/memory-samples.jsonl`，并更新 Runtime 的 current、peak、limit、accounting 和采样间隔摘要。

每个 Runtime 的终止额度为当前系统可用内存基准的 50%。Windows 与 macOS 使用系统物理内存；Linux 默认使用 `/proc/meminfo` 的 `MemTotal`，同时读取当前 cgroup v2 `memory.max` 或 cgroup v1 `memory.limit_in_bytes`，存在更小的有限额度时将其视为容器内存基准，`max` 或大于宿主机物理内存的值视为未限制。RSS 超过额度时，nornsd 将 Runtime 的 `end_reason` 和 `error_code` 统一写为 `RUNTIME_MEMORY_LIMIT_EXCEEDED`，记录 diagnostic，并终止对应 Runtime 进程树。

如果紧邻的上一 Runtime 因该错误结束，Norns Subject 的下一 Runtime 会在首条 instruction 前收到内存恢复提示；普通 Subject 仅在显式 `resume_session=true` 时收到相同提示。普通 Subject 创建全新 Session 时不注入该提示，避免把上一会话的异常背景带入无关会话。

## 13. 日志与 Event

每个 Agent 输入输出先写带时间和 Runtime ID 的 raw envelope，再投影为 readable JSONL 与数据库 Event。文件写入失败和数据库 Event 失败都按可诊断错误处理，不能静默丢失协议控制状态。

Event 类型保持稳定，payload 可以向后增加字段。Server Token、LLM Key、完整环境和生成 settings 永不进入普通日志、Event 或 API。

## 14. 启动恢复

启动时以数据库和本地 identity 交叉验证：

- 两者一致：正常加载；
- identity 合法但数据库缺失 Subject：只恢复可证明的身份/Profile/idle 注册，并产生 diagnostic；
- 数据库记录存在但目录缺失：标记 unavailable；
- 普通同名目录没有 identity：忽略；
- ID、目录、root 或 Profile 冲突：标记 unavailable；
- 数据库整体缺失但 root 已有 Subject：视为控制数据损坏，不批量猜测重建。

服务重启时，queued Runtime 可继续调度；此前 active Runtime 已失去托管进程所有权，必须进入可解释终态并执行必要 Norns cleanup，禁止假设外部进程仍可安全接管。

恢复归档优先使用 Runtime 进程元数据中持久化的 Session ID。旧记录没有该字段时回退到 Subject 级 Session ID，以继续读取升级前累计的 transcript；新 Runtime 不再写入该累计会话。

## 15. HTTP 实现边界

middleware 顺序为 request ID、恢复、认证、body 限制、deadline、handler 与统一错误编码。路由只做解析和 DTO 转换，业务状态由 app/domain 判断。

cursor 是带完整性校验的不透明 payload，绑定路由、筛选和最后稳定排序键。嵌套 Runtime 路由同时校验 Runtime 的 Subject 归属，跨 Subject 查询统一返回 not found，避免资源存在性泄露。

server 使用预绑定 listener，完成所有配置和 Agent/Norns 预检后才开始服务。关闭时停止接受新请求、持久化停止意图、等待短期 handler，再处理 active Runtime 和数据库释放。

## 16. 安全边界

- 非 loopback 明文 HTTP 启动时必须警告；TLS 由可信网络层或反向代理提供；
- Bearer Token 不写日志，API details 不含秘密；
- 路径由服务端构造并校验位于 root/Subject 内；
- custom 和领域目录拒绝符号链接逃逸；
- Visual 文件读取必须限制在 `.norns/view/`；
- Agent argv 直接执行，不经过 shell；
- Runtime artifacts 与 transcript 视为敏感本地数据；
- API 仅暴露明确资源，不提供任意路径文件读取。

## 17. 验证基线

验证覆盖：

- domain 状态机与 store 事务；
- root/Subject identity、排他锁、权限和恢复；
- 配置合并、settings 注入、领域目录与 custom 冲突；
- Plugin 确定性生成、协议版本和 manifest；
- fake Claude CLI 下的 instruction、Event、waiting、stop 与 transcript；
- Scheduler 并发、队列公平性、重启恢复和资源限制；
- HTTP 认证、限制、cursor、状态码和跨 Subject 隔离；
- Norns close、View adapter 和异常 cleanup；
- macOS、Linux 与 Windows 的进程树行为。
