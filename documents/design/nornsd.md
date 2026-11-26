# nornsd 设计

本文记录 `nornsd` 的长期产品设计、HTTP 契约与当前实现设计。`nornsd` 可以独立管理通用 Agent Runtime，也可以选择启用 Norns 模式；Norns 协议语义以 [`../protocol/protocol.md`](../protocol/protocol.md) 为准，`nornsd` 只通过公开的 `norns` CLI 查询或修改协议状态，不直接解释或修改其数据库；目录下载时允许对 SQLite 做不解释实体语义的一致性快照。

# 第一部分：原始设计

## 1. 定位与边界

`nornsd` 是面向批量任务的 Agent Runtime 管理服务。它将业务对象或问题映射为相互隔离的 Subject，为其创建 Runtime、排队并发执行、持续投递 instruction，并保存 Event、运行历史和 artifacts。调用方可以按相同业务模式批量创建 Subject，由 nornsd 使用每个 Runtime 的独立配置与 Profile 驱动智能体完成处理。

典型场景包括企业内部批量代码审查、资料分析、工单处理、合规检查、迁移评估，以及其他输入对象不同但处理模式相似的智能体任务。nornsd 负责执行控制与可观察性，不定义上层业务批次、任务模板或结果汇总规则；这些由调用方或 Well 等客户端组织。

nornsd 包含两个公开运行方式：

- `nornsd server`：主要的批量管理方式，持久化管理多个 Subject、Runtime、队列和并发槽，并提供 HTTP API；
- `nornsd agent`：面向调试和快速启动，在终端中直接创建或恢复一个 Subject，并交互式运行 Agent。

Subject 是 nornsd 的长期管理单位，通常对应一个需要持续处理的业务对象或问题。一个 Subject 可以先后拥有多个 Runtime，但同一时间最多存在一个 current 或 queued Runtime。业务 Task 属于 Norns 或其他 Agent workflow，不是 nornsd 资源；nornsd 的批量能力来自对多个 Subject 与 Runtime 的统一排队、并发控制和观察，而不是把业务 Task 固化为 nornsd 实体。

nornsd 的通用 Runtime 管理不依赖 Norns。普通模式直接批量驱动 Agent 处理 instruction；可选的 Norns 模式在相同 Runtime 管理能力上增加 Norns Plugin、开放任务求解过程和 Norns View。启用 Norns 时，独立 adapter 负责 CLI 预检、Plugin 生成、Task close 和 View 读取；Norns 不参与 nornsd 的 Subject、队列或并发模型。

当前只实现 Claude Code adapter，并固定兼容 Claude Code `2.1.199`。实现为单一 Go 可执行文件，不依赖 Python 或 Claude Agent SDK。

## 2. 核心模型

### 2.1 Root

`nornsd_root` 是一组 Subject、数据库与运行记录的控制域。每个 root 具有稳定 UUID。同一个 root 同时最多由一个 `nornsd server` 或 direct agent 持有；不同 root 相互独立。

### 2.2 Runtime Profile

Profile 属于 Runtime，由领取槽位时的模板与请求覆盖共同确定；Subject 和 Service 不固定绑定 Profile。当前只实现 `cc`，版本预检保留既有规则。普通模式示例：

```json
{"agent":"cc","agent_version":"2.1.199","capabilities":[]}
```

启用 `AGENT_NORNS_ENABLE` 后 capabilities 包含 `norns`。同一 Subject 的不同 Runtime 可以切换普通/Norns 模式。排队期间尚未分配模板，Profile 可为空；Runtime 详情以 `runtime_profile: null` 表达未确定状态。停止、异常清理和内存恢复提示均依据对应 Runtime 的 Profile。

Subject 存在有效 `.norns/` 时，Status/Trace/Visual 与 Cog/Exp 查询不受最新 Runtime 模式影响；仍需可用的 Norns CLI。

### 2.3 Subject

Subject 包含稳定 UUID、可选名称、工作目录身份与 Runtime 历史。状态：

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
- `running`：存在尚未结算的用户输入，或根 Agent 正在处理输入/后台通知；单纯子 Agent 进度不改变根状态；
- `waiting`：根 Agent 已返回当前处理的 result，且没有待发送或未结算输入；仍占用槽位；
- `ended`：Norns Runtime 通过正常 Task close 结束；
- `error`：启动、协议、资源或 Agent 错误；
- `cancelled`：外部优雅或强制停止。

普通 Agent 的成功或可继续的错误 result 都不自动结束 Runtime；没有剩余输入时进入 waiting。`is_error`、`subtype`、`terminal_reason` 与进程是否退出分别记录。Runtime 终态不等于业务目标完成。

Runtime ID 同时是该次 Claude Session ID。每个 Runtime 都创建新 Claude 会话，同一个 Runtime 内的 initial instruction 和后续 instruction 共享该会话。默认情况下，不同 Runtime 即使属于同一 Subject，也不继承 Claude 对话；显式 `resume-session` 时通过 `--resume` 和 `--fork-session` 读取最新 Runtime 的上下文，并派生到以新 Runtime ID 命名的新 Session。

- 普通模式跨 Runtime 只复用 Subject 工作目录中的文件；
- Norns 模式跨 Runtime 复用 `.norns/` Workspace 和 Graph，由新 Coordinator 会话读取持久状态并重新评估；
- Norns View 是持久状态的只读投影，不承担状态存储或会话恢复。

### 2.5 Instruction 与 Event

instruction 是 Runtime 内待写入 Agent 的 FIFO。每个 Runtime 最多保留 16 条 pending instruction；已经确认写入 Agent 的记录不再出现在待发送数组中，但持久记录不删除。结果不确定的 instruction 不得自动回到 pending。首批 pending 可合为一个 dispatch group；后续依 FIFO 逐项投递，每组使用稳定 UUID。管道写入成功、CC 回显确认与 result 结算是三个独立阶段，多组输入可被同一个 result 吸收。

Event 是 Subject 内单调递增、永不复用序号的人类可读投影。原始 Agent 帧和 readable Event 分开保存；公共 API 只暴露 readable Event，不把 Claude stream-json 变成公共协议。

## 3. 调度与生命周期

批量调用方可以连续创建多个带首条 instruction 的 Subject，或向既有 Subject 提交 instruction。每条空闲 Subject 的新执行请求创建 queued Runtime；配置的 `SERVICE_CONCURRENCY` 限制同时取得槽位的 Runtime 数，超出部分进入稳定 FIFO。direct agent 不参与 server 队列。

队列遵循稳定 FIFO。取得槽位后才物化尚未创建的 Subject 目录、发布配置与 Plugin、启动 Agent。waiting Runtime 持续占用槽位，不因队列压力或等待时间自动回收；只有明确停止、正常结束或异常退出才释放。

向 idle Subject 提交 instruction 会创建新 queued Runtime；向 queued/running/waiting Runtime 提交 instruction 会追加 FIFO。提交给 waiting Runtime 后唤醒为 running。进入 stopping、closing 或终态后拒绝新 instruction。

停止模式：

- `graceful`：请求 Agent 收尾，超时后升级为强制终止；
- `force`：立即终止托管进程树。

Norns Runtime 外部停止时，nornsd 先停止 Agent/Explorer 写入，再在宽限期内执行符合协议的 failed/cancelled close。Agent 主动完成 Task 时进入 closing，等待正常进程退出；业务 Task outcome 与 Runtime 控制状态保持分离。

外部停止或 Runtime 错误导致的 Norns close 是复合语义：当前 Runtime 进入 `cancelled` 或 `error`，未完成 Exploration 以 `cancelled` 结束，但 Exploration 元数据、已经持久化的 Cognition、Event 和该 Runtime 的 transcript 保留。后续 Runtime 使用新的 Claude Session 读取既有 Graph，可以选择对历史 Exploration 执行 `retry`；retry 创建新 Exploration，不覆盖原中断记录。

恢复时用户全局断言和历史 Exploration 的 hints 同样保留为协议事实。retry 只复制探索定义和 sources，新 Exp 不复制 hints 或其 Cog；Explorer 从 Trace 的完整目标 retry 链读取历史提示。nornsd 不从 transcript、instruction FIFO 或 Event 重建这些认知关联。

## 4. 配置模型

外部配置统一为 `nornsd.yaml`，不内嵌、不含 `VERSION`。规范示例为 `nornsd/configs/nornsd/nornsd.example.yaml`。server 必须显式使用可重复的 `--configs FILE`；direct agent 必须使用 `--config FILE`。删除旧配置 CLI 参数和配置自动发现；保留 `--resume-subject`、`--resume-session` 与 `--` 后的 Agent 参数。

| 字段 | 含义及默认值 |
| --- | --- |
| `SERVICE_NORNSD_ROOT_DIR` | 必填绝对路径，指定 Root。 |
| `SERVICE_HOST` | 默认 `127.0.0.1`。 |
| `SERVICE_PORT` | 默认 `28081`，范围 1–65535。 |
| `SERVICE_TOKEN` | server 必填、非空且不含空白。 |
| `SERVICE_CONCURRENCY` | 正整数，默认 `1`。 |
| `AGENT_TYPE` | 默认 `cc`，当前仅支持该类型。 |
| `AGENT_EXEC` | 可执行文件绝对路径及固定 argv；省略或设为 `[]` 时从 PATH 查找 `claude` 并转为绝对路径。Runtime 覆盖传入 `[]` 恢复此默认命令，省略则继承模板。 |
| `AGENT_LLM_URL` / `AGENT_LLM_KEY` / `AGENT_LLM_MODEL` | 必填、非空。 |
| `AGENT_LLM_FAST_MODEL` | 为空时复用主模型。 |
| `AGENT_NORNS_ENABLE` | 是否启用 Norns，默认 false。 |
| `AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS` | 正整数，默认 `3`。 |
| `AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS` | 正整数，默认 `1200` 秒。 |
| `AGENT_NORNS_ALLOW_HUMAN_INTERVENTION` | 默认 false。 |
| `AGENT_MAINAGENT_FILE` | 两种模式通用的 Main Agent Markdown 文件绝对路径；默认空。除非了解影响，否则不要改动。 |
| `AGENT_NORNS_EXTENDS_AGENTS_DIR` | Norns 模式：沿用原领域元数据与知识拼接逻辑，基于通用角色生成领域角色。 |
| `AGENT_NORNS_CUSTOM_AGENTS_DIR` | Norns 模式：完整自定义的 Norns 角色 Agent Markdown，原样复制并按名称加入 Coordinator 白名单。 |
| `AGENT_NORNS_CUSTOM_SKILLS_DIR` | Norns 模式：复制到 Norns Plugin 的自定义 Skill。 |
| `AGENT_CUSTOM_AGENTS_DIR` / `AGENT_CUSTOM_SKILLS_DIR` | 普通模式：生成独立 custom Plugin 的资源。 |
| `AGENT_CUSTOM_PLUGIN_DIRS` | 仅普通模式：按声明顺序复制到 Runtime 后加载的外部 Plugin 目录列表。 |
| `AGENT_REUSE_RUNTIME_DIR` | 旧 Runtime 目录绝对路径；复制其中的 plugins/，从 launch.json 恢复加载顺序和 Main Agent。放在配置文件末尾。 |

非空配置路径必须为绝对路径，可选资源目录允许空字符串。`AGENT_EXEC` 的其余项是原样 argv，不作为路径解析。配置文件自身的 CLI 路径允许相对路径。严格拒绝未知字段、重复 YAML key、null 字段、错误类型与多文档。

资源按有效 `AGENT_NORNS_ENABLE` 互斥：开启时允许非空 `AGENT_NORNS_EXTENDS_AGENTS_DIR` 与 `AGENT_NORNS_CUSTOM_*`，拒绝非空 `AGENT_CUSTOM_*`；关闭时反之。空字符串与空列表表示未配置或清空，允许保留在全量配置中。没有 Norns 额外 Plugin 入口，`AGENT_NORNS_CUSTOM_PLUGIN_DIR` 不属于合法字段。旧 `AGENT_NORNS_CUSTOM_DOMAIN_DIR` 不保留兼容别名。

创建 Runtime 的覆盖若明确指定模式，则提交阶段即可拒绝与该模式冲突的资源；未指定模式时，基于领取槽位后合并的有效配置检查。切换模式不会自动忽略继承资源，须在同一覆盖对象中显式用空字符串/空列表清空另一模式的资源。

一份模板可供全部槽位复用；多份模板的数量必须等于第一份的 `SERVICE_CONCURRENCY`。所有 `SERVICE_*` 以第一份为准，后续模板只提供各自 Agent 配置。启动时读取并冻结模板，并按顺序保存到 `.nornsd/configs/nornsd.yaml`、`nornsd2.yaml`、`nornsd3.yaml` 等；修改源文件需重启。

创建 Runtime 时可传 `config` JSON 对象，仅允许 `AGENT_*`：创建 Subject 的字段位于 `runtime.config`；已有 Subject 的提交接口位于请求顶级。省略、整体 null 或 `{}` 均无覆盖。按字段出现与否合并，显式 false、0、空字符串和空数组不视为遗漏，随后执行有效性校验；数组整体替换，逐字段 null、未知键及错误类型拒绝。

提交阶段校验覆盖的字段、类型、路径及可独立判定的值；领取槽位后使用该槽模板合并并再次校验完整配置和资源、执行预检，失败进入可诊断终态。排队期间只保存覆盖，不绑定最终模板或 Profile。向已有 current Runtime（包括 queued）追加 instruction 时，非空 config 返回 `409 RUNTIME_CONFIG_CONFLICT`，不得热更新。

Runtime 的 `configs/nornsd.yaml` 保存包含 SERVICE 与 AGENT 字段的全量有效配置及原始凭据；每次执行独立保存，不修改模板或其他 Runtime。

### 4.1 基于通用角色扩展领域知识

`AGENT_NORNS_EXTENDS_AGENTS_DIR` 仅是原 `AGENT_NORNS_CUSTOM_DOMAIN_DIR` 的改名，保留原结构与拼接逻辑；不引入新的 Agent Markdown 继承语法。目录下的 `meta.json` 使用严格 JSON：

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

### 4.2 完整自定义 Norns 角色

`AGENT_NORNS_CUSTOM_AGENTS_DIR` 下的 `.md` 为完整 Claude Code Plugin Agent 定义，使用 YAML frontmatter 记录 name、description、tools 及其他 Agent 配置。角色内容由提供方保证符合 Norns 协议；nornsd 不拼接通用角色正文、不改写其工具或其他 frontmatter。

发布时复制到 norns-plugin/agents/，解析 frontmatter.name 并以 `norns:<name>` 加入 Coordinator 的 `Agent(...)` 白名单。文件名或 Agent 名不得覆盖/重复内置角色及自动生成的领域角色，保留现有名称校验与冲突检查。

### 4.3 Main Agent 与插件复用

`AGENT_MAINAGENT_FILE` 在普通与 Norns 模式下均生效，须指向普通 `.md` 文件。使用 frontmatter.name 选择 Agent，文件名不决定启动 Agent 名称；示例配置明确提示：除非了解修改影响，否则不要改动。

Norns 模式复制到 norns-plugin/agents/，作为新的 Coordinator，启动参数为 `--agent norns:<name>`。所选 Main Agent 必须声明 Agent 工具；生成器替换其 `Agent(...)` 白名单与 `{{NORNS_RUNTIME_CONFIGURATION}}`，白名单排除通用 Coordinator 和自身。普通模式原样复制到 custom-plugin/agents/，启动 `--agent nornsd-custom:<name>`；仅指定 Main 文件也发布 custom-plugin。未指定时保持各模式的默认 Main Agent。

Coordinator 与派生领域角色使用完整 YAML Node 树解析和输出，只更新工具白名单、name、description 等对应字段；保留未知字段、嵌套配置、注释和 YAML 引用，不通过固定结构重建。tools 支持字符串和字符串列表。完整自定义子角色保持原文；选择新的 Main 后，未使用的通用 Coordinator 也保持原文。

`AGENT_REUSE_RUNTIME_DIR` 指向旧 `.nornsd/subjects/<subject_id>/runtimes/<runtime_id>/`，两种模式都支持。读取旧 launch.json 的 agent.type、agent.argv 中的 `--agent` 和有序 plugins 列表，将整个 plugins/ 复制到新 Runtime 的 plugins/，启动参数只加载新副本。旧普通 Runtime 未指定 `--agent` 时保留 CC 默认 Main Agent；允许无插件的普通 Runtime。插件内容、Hooks 可执行权限和 manifest 原样保留，不重新注入、生成或修改旧 Runtime ID。新 launch.json 记录新 Runtime 和有序副本路径与 hash。

复用与非空 AGENT_MAINAGENT_FILE、AGENT_NORNS_EXTENDS_AGENTS_DIR、全部 AGENT_NORNS_CUSTOM_*、AGENT_CUSTOM_* 互斥，合并后的有效配置也须满足此限制。可用空字符串/空数组清空继承资源。复用时忽略探索并发、超时问询周期和人类干预等插件注入设置；LLM 等非插件配置仍按新的有效配置生成。缺失/无效 launch、不兼容 Agent 类型、不完整或多次声明 Main Agent、插件目录与加载列表不匹配、符号链接及源目标重叠都必须失败。

## 5. HTTP API

### 5.1 通用约定

- 基础路径：`/api/`；
- 请求和普通响应：UTF-8 JSON；Visual View 成功时为 HTML；
- 所有接口，包括 service info 与 health，都要求 `Authorization: Bearer <token>`；
- 时间：UTC RFC 3339，至少毫秒精度；
- 字段和枚举：英文 `snake_case`；
- 请求体上限 2 MiB，单条 instruction 上限 1 MiB；
- 普通 handler deadline 为 5 秒；
- 未知请求字段返回 `400 INVALID_REQUEST`；
- 当前使用普通请求/轮询，不提供 SSE 或 WebSocket。

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

稳定错误至少包括 `INVALID_REQUEST`、`INVALID_CURSOR`、`UNAUTHORIZED`、`SUBJECT_NOT_FOUND`、`RUNTIME_NOT_FOUND`、`RUNTIME_SESSION_NOT_FOUND`、`SUBJECT_NOT_IDLE`、`SUBJECTS_NOT_IDLE`、`ARCHIVE_BUSY`、`SUBJECT_RUNTIME_ACTIVE`、`RUNTIME_PROFILE_MISMATCH`、`RUNTIME_STOPPING`、`RUNTIME_CLOSING`、`CAPABILITY_NOT_ENABLED`、`SUBJECT_NOT_MATERIALIZED`、`REQUEST_TOO_LARGE`、`RUNTIME_INSTRUCTION_REJECTED`、`AGENT_UNAVAILABLE`、`SERVICE_UNAVAILABLE`、`DEADLINE_EXCEEDED` 和 `INTERNAL_ERROR`。

### 5.2 路由

| 方法 | 路径 | 成功 | 用途 |
| --- | --- | --- | --- |
| GET | `/api/` | 200 | 服务身份、版本、容量与能力。 |
| GET | `/api/health` | 200 | 数据库和调度器健康。 |
| POST | `/api/subjects` | 201 | 创建 Subject，可原子创建首个 queued Runtime。 |
| GET | `/api/subjects` | 200 | 按状态分页列出 Subject。 |
| GET | `/api/subjects/{subject_id}` | 200 | Subject 详情。 |
| POST | `/api/subjects/{subject_id}/runtime` | 202 | 创建 Runtime、追加 instruction，或 fork 最新 Session。 |
| GET | `/api/subjects/{subject_id}/runtime` | 200 | 当前/排队 Runtime 快捷查询。 |
| POST | `/api/subjects/{subject_id}/runtime/stop` | 202 | `graceful` 或 `force` 停止。 |
| GET | `/api/queue` | 200 | 全局 Runtime Queue。 |
| GET | `/api/subjects/{subject_id}/events` | 200 | 人类可读 Event 最新窗口、增量与历史读取。 |
| GET | `/api/subjects/{subject_id}/events/updates` | 200 | 检查指定 Event 游标后是否有新记录。 |
| GET | `/api/subjects/{subject_id}/runtimes` | 200 | Runtime 历史。 |
| GET | `/api/subjects/{subject_id}/runtimes/{runtime_id}` | 200 | 稳定 ID Runtime 详情。 |
| GET | `/api/subjects/{subject_id}/norns/views/status` | 200 | Norns Status View，默认 YAML，可选 `format=yaml\|json\|md`。 |
| GET | `/api/subjects/{subject_id}/norns/views/trace` | 200 | Norns Trace View，默认 YAML，可选 `format=yaml\|json\|md`。 |
| GET | `/api/subjects/{subject_id}/norns/views/visual` | 200 HTML | Norns Visual View。 |
| GET | `/api/subjects/{subject_id}/norns/cognitions/{cognition_id}` | 200 JSON | 完整 Cognition，包括非空的 meta。 |
| GET | `/api/subjects/{subject_id}/norns/explorations/{exploration_id}` | 200 JSON | 完整 Exploration。 |
| GET | `/api/archive` | 200 gzip | 通过 scope 与可选 Subject/Runtime ID 生成并下载目录包。 |

### 5.3 服务身份与能力

`GET /api/` 返回 `service=nornsd`、稳定 `server_id`、产品版本 `version`、整数接口格式版本 `api_schema`、协议版本、容量和 capability 列表。Well 使用 `server_id` 检测同一 URL 后的数据域变化。

API schema 由根目录 `versions.json` 的 `nornsd.api_schema` 声明，不编码在请求路径中。客户端应先读取服务信息并检查是否支持返回的 schema，再发送业务请求；`nornsd version --json` 同样返回 `api_schema`。

health 只说明 HTTP、SQLite 和调度器可以继续接受请求，不保证外部 LLM 可用。

### 5.4 Subject 与 Runtime 表示

Subject 详情包含：

- `id`、`name`、`state`；
- `workspace.materialized`、目录名和服务端绝对路径；
- current/queued Runtime 或 null；
- `last_event_seq`、创建和更新时间。`last_event_seq` 是实体统计字段，不是 HTTP Event
  续读游标，客户端不得将它传给 `after` 或 `cursor`。

Runtime 详情包含稳定 ID、Subject ID、状态、runtime_profile、Agent/session、排队/开始/结束时间、end reason、settings 槽与配置 hash、进程结果、usage、资源摘要、错误和 artifact 定位路径。新 Runtime 的 `session_id` 与 Runtime `id` 相同；恢复模式在 `process.resume_session_id` 记录其来源 Session。API 不返回 LLM Key、完整 settings、环境变量、raw log 或 transcript 内容。

上述常规 JSON 接口不直接提供配置和日志正文；目录打包下载会包含这些文件。Runtime 历史分页响应包含摘要数组、`next_cursor` 和当前 Subject 的稳定 `total_count`；每条摘要包含 `created_at`、`started_at` 和 `ended_at`，客户端无需逐条读取详情来显示历史列表。

创建 Subject 可选携带首条 instruction。携带时，Subject、Runtime、instruction 与 queue 记录在同一事务中创建；不携带时，Subject 在目录和本地身份成功物化后以 idle 返回。

提交 instruction 对 idle Subject 创建 Runtime，对已有可接收 Runtime 追加 FIFO。请求体可以增加 `"resume_session": true`；此时 Subject 必须为 idle 且至少存在一个历史 Runtime，服务读取最新 Runtime 的 Session ID，创建新的 queued Runtime，并通过 `--resume`、`--fork-session` 和新 Runtime ID 派生会话。如果 Subject 已有 current/queued Runtime，则返回 `409 SUBJECT_RUNTIME_ACTIVE`，不会追加 instruction；没有历史会话则返回 `409 RUNTIME_SESSION_NOT_FOUND`。`202 Accepted` 只表示持久化接受，不表示已经执行。

### 5.5 分页与 Event

Subject、Queue 和 Runtime 历史使用不透明 keyset cursor：`limit` 默认 50，范围 1..200。cursor 绑定路由、稳定排序和筛选条件，不绑定 limit；跨路由或修改筛选条件复用返回 `INVALID_CURSOR`。

Subject、Queue 和 Runtime 历史的分页 cursor 与 Event cursor 互不通用。Event 直接读取
`.nornsd/subjects/<subject_id>/logs/readable.jsonl`，每次最多返回 5000 条，并使用绑定
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

Event API 只返回 readable Event，不提供 raw 类型切换、任意文件读取或诊断包接口。目录打包下载独立提供整个目录快照，其中包含已保存的日志文件。

### 5.6 Norns View

View API 根据 Subject 的有效 `.norns/` 状态提供，不要求最新 Runtime 的 Profile 启用 `norns`。status/trace 支持 `format=yaml|json|md`，省略或为空时使用 YAML；非法或重复参数返回 `400 INVALID_REQUEST`。保留 status 的 `task_id` 和 trace 必填的 `exploration_id`。nornsd 调用 CLI 生成所选格式，原样返回完整正文，不添加 envelope，也不自行重排 Markdown。Content-Type 分别为 `application/yaml`、`application/json` 和 `text/markdown`，均使用 UTF-8；错误始终使用 JSON。visual 必须验证 CLI 返回文件位于当前 Subject `.norns/view/` 内，再以 `text/html` 返回。

所有 Norns View 和 Cog/Exp 详情接口按顺序校验：Subject 下必须同时存在 `.norns/` 目录和 `.norns/norns.sqlite` 常规文件，否则返回 `409 WORKSPACE_NOT_INITIALIZED`（非 Norns 模式的工作区）；工作区存在后，再检查当前服务是否具备可用、兼容的 Norns CLI，否则返回 `503 NORNS_UNAVAILABLE`。不以最新 Runtime 或某个配置模板的 Norns 开关替代这两步检查。

三个 View 接口都在 `X-Norns-View-Filename` 响应头返回本次 Norns 已发布 View 的文件名。nornsd 读取 Status/Trace 时要求 CLI 返回已发布文件引用，即使正文未超过 CLI 的内联输出上限也不通过目录扫描猜测文件；HTTP 正文格式不受此元数据影响。

Subject idle 时仍可读取 View；未物化返回 `SUBJECT_NOT_MATERIALIZED`。生成 View 会写入文件，因此在对应 Subject 或 Root 打包期间返回 `409 ARCHIVE_BUSY`。

Cog/Exp 详情接口同样执行上述工作区与当前服务可用性校验，通过 CLI get 查询，直接以 JSON 返回成功信封中的完整实体，不返回 CLI envelope。可读取历史 Task 下任意状态的实体，不要求 Open Task 或 idle Subject；详情读取不会生成 View 文件，不受 6 KiB 内联阈值限制。非法 ID 返回 `400 INVALID_ARGUMENT`，不存在返回 `404 COGNITION_NOT_FOUND` 或 `404 EXPLORATION_NOT_FOUND`。

### 5.7 目录打包下载

统一为 `GET /api/archive`，沿用 Bearer 认证。scope 必填，旧 Subject archive 路由不保留。

| scope | subject_id | runtime_id | 范围 |
| --- | --- | --- | --- |
| subject | 无 | 无 | 整个 Root，含 `.nornsd/` 和 Root 中其他文件。 |
| subject | 有 | 无 | Subject 工作目录，含 `.norns/`，不含运行配置、日志和 transcript。 |
| rundata | 无 | 无 | 整个 `.nornsd/`。 |
| rundata | 有 | 无 | `.nornsd/subjects/<subject_id>/`。 |
| rundata | 有 | 有 | 该 Subject 下 `runtimes/<runtime_id>/`。 |

未知 scope、非法 ID、runtime_id 单独传入及 subject scope 携带 runtime_id 返回 400。Runtime 必须属于所选 Subject，否则返回 404。

全量范围要求所有 Subject idle；指定 Subject（含单 Runtime 范围）要求该 Subject idle 且无 current/queued Runtime。全量归档生成期间阻止新 Subject、指令提交和 View 写入；指定范围阻止对应 Subject 的冲突操作。全部范围共用一个生成额度，冲突返回 `409 ARCHIVE_BUSY`；生成完成释放额度，下载不占用它。

请求内生成临时 `.tar.gz` 后直接下载，顶层保留源目录名。临时包在 Root 外创建，下载结束、失败或取消时清理。普通请求仍为 5 秒，归档独立使用 10 分钟 deadline，覆盖生成和下载，并响应客户端取消。

nornsd/Norns SQLite 使用 `VACUUM INTO` 一致性快照，排除对应 WAL/SHM/journal；排除服务锁、owner、内部 staging、Norns 活锁及 socket/FIFO/device。保留普通文件和符号链接，后者不跟随读取。响应使用 `application/gzip`、attachment 文件名、准确 Content-Length 和 `Cache-Control: no-store`。

### 5.8 当前明确不提供

当前不提供 Subject 修改/删除/迁移、归档生命周期与导入恢复、配置 API、独立 queue cancel、独立原始日志下载、diagnostics、幂等键、用户/角色或 Token 轮换。客户端不得自动重试创建和 instruction 提交；结果不确定时先查询 Subject、Runtime 与 Event。

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
| `config` | nornsd.yaml、settings、custom 资源与领域目录。 |
| `plugin/cc` | 临时 Norns Plugin 发布。 |
| `runtimeartifact` | Runtime 分组启动快照 launch.json。 |
| `eventlog` | raw/readable 日志和 Event 投影。 |
| `norns` | Norns CLI lifecycle 与 View adapter。 |
| `process` | 跨平台进程树和资源控制。 |

domain 不依赖 HTTP、Cobra、SQLite、Agent 或操作系统进程 API。

## 7. 文件布局

```text
<nornsd_root>/
  .nornsd/
    service/
      root.json
      owner.json
      nornsd_lock
      nornsd.sqlite
      nornsd.sqlite-wal
      nornsd.sqlite-shm
    configs/
      nornsd.yaml                    # 首份模板；单模板时供全部槽位复用
      nornsd2.yaml                   # 第二份模板，存在时保存
      nornsd3.yaml                   # 第三份模板，后续依次编号
    staging/
    queue_record/
      <UTC>.json
    subjects/
      <subject_id>/
        subject.json
        staging/
        logs/
          raw.jsonl
          readable.jsonl
        runtimes/
          <runtime_id>/
            launch.json
            configs/
              nornsd.yaml
              agent/
                settings.json
            plugins/
              norns-plugin/
                plugin.manifest.json
              custom-plugin/
                plugin.manifest.json
              external-plugin-1/     # 外部 Plugin 按声明顺序编号，各自含 manifest
            memory/
              memory-samples.jsonl
            transcripts/
              transcript.manifest.json
              <runtime_id>.jsonl
              <runtime_id>/
                xxxx.jsonl
  <subject_uuid>_<UTC时间>/
    # Subject 工作文件；不创建 .nornsd/
    .norns/                          # Norns 模式的任务状态仍保留在工作目录
```

Subject 目录以 UUID 与纳秒精度 UTC 时间命名，Agent cwd 只存工作文件与可选 `.norns/`。身份与日志按稳定 Subject ID 存在控制目录；Runtime 配置、Plugin、内存采样和 transcript 按 Runtime ID 隔离。Root 锁为 `.nornsd/service/nornsd_lock`，是否被进程持有决定是否占用；owner 文件存在本身不代表进程存活。

布局按需生成：空闲 Subject 创建身份和日志，Runtime 启动准备发布配置和 Plugin，进程结束或恢复收尾归档 transcript。主 transcript 使用 Runtime ID 命名，manifest 在 transcripts/，各 Plugin manifest 在 Plugin 自身目录。不支持旧目录布局或旧数据库的自动迁移。

## 8. SQLite schema

root 数据库 schema 由 `versions.json` 的 `nornsd.database_schema` 声明，实际二进制通过 `nornsd version --json` 的 `database_schema` 报告；不自动迁移不匹配的数据库。数据库表：

| 表 | 职责 |
| --- | --- |
| `metadata` | schema、root ID、创建时间。 |
| `subjects` | Subject 身份、目录和状态。 |
| `runtimes` | Runtime 永久记录、Profile、请求覆盖、有效配置、状态、进程、usage 和资源。 |
| `runtime_queue` | 稳定 FIFO 调度顺序。 |
| `runtime_instructions` | 有序输入、dispatch_group/input_uuid、dispatch_committed_at、write_confirmed_at、accepted_at、settled_by 及投递诊断。 |
| `runtime_results` | 按 Runtime/result UUID 去重的根处理结果、raw_seq、原始结果 payload 和时间；无 UUID 的帧以 raw_seq 定位。 |
| `runtime_config_snapshots` | 与 launch.json 相同的启动快照。 |
| `events` | Subject 内单调序号的 readable Event。 |

`runtimes.inflight_inputs` 记录已经写入但未被结果结算的 dispatch group 数量，不代表 Claude 的 num_turns 或期望 result 数量。结果去重、结算所有已回显但未结算的输入、更新剩余输入计数与 usage 在同一事务完成。未知或重复回显不会结算其他输入，重复 result 不累计用量、不吸收后续输入。

关键事务：创建 Subject+Runtime+instruction+queue、领取 queue 槽、追加 instruction、确认投递、确认回显、结算结果、状态转换、stop 接受和 Runtime 收尾。队列领取在数据库事务中完成，避免多个 scheduler 重复启动。`runtimes.profile_json` 与 `effective_config_json` 在领取后冻结；`config_override_json` 为创建时的 JSON 对象。队列导出先发布文件、再在同一写事务中取消并清队列；失败回滚。

## 9. 配置与 artifacts

Root configs 保存启动时冻结的模板；每个 Runtime 保存全量有效 `configs/nornsd.yaml`、生成的 `configs/agent/settings.json` 和独立 Plugin 副本。配置 hash 为实际落盘 YAML 字节的 SHA-256。

`launch.json` 是唯一文件形式的启动快照，不再生成 snapshot.json 或 config.snapshot.json。分组：

| 分组 | 字段 |
| --- | --- |
| runtime | id、subject_id、created_at、managed。 |
| session | id、mode、resume_session_id。 |
| agent | type、version、capabilities、完整实际 argv（含可执行文件与 wrapper 参数，不脱敏）。 |
| config | slot、source、hash，不重复配置摘要。 |
| plugins | 按实际加载顺序记录 Runtime 副本 path 与 manifest hash。 |

完整配置含凭据；launch 保留完整 argv。终态结果、usage、resource 仍在数据库；Plugin/transcript 文件清单各自保存在 manifest，不在 launch 重复。API 普通详情不返回完整配置和 argv。

Claude settings 基础模板 `configs/cc/settings.json` 仍内嵌；结构化解析并注入 LLM 参数。Runtime 发布路径有独立权限和 staging；不再向 Subject 工作目录发布“当前配置/Plugin”。

## 10. Plugin 生成

构建时从正式协议和整个 `plugin/` 目录生成按 Agent 类型分层的确定性资源并 `go:embed`。运行时根据 Runtime Profile 的 Agent 类型选择对应 Plugin；当前 `cc` 使用 `plugin/cc/`。运行时只依赖内嵌资源，不依赖源码相对路径，未知 Agent 类型必须明确失败。

运行时生成器：

- 注入 Task 并发与人类干预策略；
- 如果存在领域配置，从标准 Decider/Explorer 派生 `decider-for-<domain>.md` 与 `explorer-for-<domain>.md`；domain 内部命名保留下划线，例如 `decider-for-code_review`；
- 保留完整 YAML frontmatter，仅更新派生角色的 name/description；
- 把 extra-info 原文附加到相应 Markdown 末尾；
- 把领域 Agent 加入 Coordinator allowlist 与运行提示；
- 合并 `AGENT_NORNS_CUSTOM_AGENTS_DIR` 和 `AGENT_NORNS_CUSTOM_SKILLS_DIR`；
- 复制可选 AGENT_MAINAGENT_FILE 并选择实际 Main Agent，从 norns-plugin/agents/ 中解析 Agent 名称，生成排除通用 Coordinator 与自身的 `Agent(...)` 白名单；仅改写所选 Main，自定义子角色保持原文；
- 拒绝覆盖标准 Agent、动态领域 Agent 或 Norns Skill；
- 对完整发布树生成 manifest/hash。

仅普通模式允许 `AGENT_CUSTOM_AGENTS_DIR` 和 `AGENT_CUSTOM_SKILLS_DIR`，生成命名为 `nornsd-custom` 的独立 Runtime Plugin；`AGENT_CUSTOM_PLUGIN_DIRS` 则复制到 Runtime plugins/external-plugin-N/，生成各自 manifest 后加载。普通模式通过重复的 `--plugin-dir` 按“外部副本、custom-plugin”顺序加载。Norns 模式只加载生成的 norns-plugin，其全部领域扩展、自定义角色和 Skill 都位于这个 Plugin 内。

托管与 direct Runtime 共用插件准备入口；AGENT_REUSE_RUNTIME_DIR 非空时跳过上述生成流程，按 4.3 节复制旧插件并恢复加载顺序与 Main Agent。

## 11. Claude Code adapter

省略 `AGENT_EXEC` 或设为 `[]` 时从 PATH 解析 claude 的绝对路径，预检精确版本 `2.1.199`。显式非空 argv 覆盖表示用户承担包装器版本兼容性，但仍需可执行和 stream-json 兼容。

托管模式使用 Claude Code stream-json：

- 所有 Runtime 固定传入 `--dangerously-skip-permissions`，避免批量任务等待无法响应的权限确认；
- 默认命令以及显式直接指向 `claude`、`claude.exe` 或 `claude.cmd` 的 `AGENT_EXEC` 必须通过固定 Claude Code 版本检查；`cfuse` 等包装器无法可靠探测内部版本，保留兼容模式；
- 所有 Claude Code 子进程固定设置 `IS_SANDBOX=1`，使 root/sudo 环境可以使用 `--dangerously-skip-permissions`；
- 所有托管 Runtime 固定传入 `--setting-sources=`，仅加载显式 `--settings`、显式 Plugin 和不可关闭的 managed settings；direct agent 同样固定启用该隔离；
- 未指定 Main Agent 时普通 Profile 沿用 CC 默认主角色，Norns Profile 使用 `--agent norns:coordinator`；AGENT_MAINAGENT_FILE 选择对应 Plugin 中的自定义主角色，插件复用则从旧 launch.json 恢复主角色；
- 固定 Runtime ID 作为新 session ID，每个 Runtime 使用独立 Claude 会话；显式 `resume-session` 时传入 `--resume=<latest_session_id> --fork-session --session-id=<runtime_id>`；
- initialize/interrupt 使用带 request_id 的 control_request/control_response；initial input 和后续 instruction 通过普通 user 帧写入 stdin，不存在每条 user 对应的控制确认；
- 固定启用 `--replay-user-messages`，每个 dispatch group 的 user 帧携带独立稳定 UUID；OS 写入成功标记 write_confirmed_at，只有根 user/isReplay 回显的 UUID 匹配本 Runtime 的已投递组才记录 accepted_at；字符串和内容块数组两种 message.content 均可解析；
- 每个根 result 结算此前已回显且未结算的全部输入，未回显的输入继续在途。不能按“每个写入 +1、每个 result -1”推导状态；
- `origin.kind=task-notification` 保留为结果来源：纯通知不消耗用户输入；通知处理吸收了新用户输入时，同样结算这些输入。零/非零 num_turns、token 均不是过滤依据；
- 显式 resume 在 initialize 控制请求成功后记录一次 `runtime.session_resumed`，confirmation=initialize，不等待或吞掉所谓首次启动 result。无效 Session 等初始化前的普通错误 result 保留为 agent.result，并以 AGENT_STARTUP_FAILED 记录结构化启动失败；旧任务 failed 通知不表示新 Runtime 失败；
- API 400/401/429/500、interrupt、预算/轮数限制等错误 result 如实保留 is_error、subtype、terminal_reason、api_error_status 和 errors。它们结束当前处理，不无条件终止仍存活的进程；剩余输入为零时等待新 instruction，不由 nornsd 自动重投或重试。进程实际退出、协议/启动/资源错误与外部 stop 分别驱动终态；
- system/api_retry 仅记录重试进度，不结算输入；退避、重试次数和是否重试由 CC 决定，nornsd 不再叠加模型请求重试；
- 根 assistant/stream_event、system/status=requesting 或 api_retry 标记正在工作，后续根 result 重新计算 running/waiting；仅后台任务进度不强制根保持 running。已取消/结束/失败的终态不被尾部消息重新唤醒；
- assistant 文本、工具调用/返回、结果、回显、协议中断标记及部分 system 生命周期消息映射为不同 readable Event，保留工具错误与整次处理错误的区别；
- 不启用 permission-prompt-tool=stdio。托管模式不支持反向人类问答/权限控制；意外收到反向 control_request 时明确返回 error，避免永久等待；
- EOF 仅关闭输入，不等于 interrupt；优雅 stop 保留 interrupt 后关闭 stdin 的流程。进程退出后完整消费 stdout/stderr，才归档 transcript、执行 Norns 清理、释放 Subject 和槽位；没有结果也可能正常退出，退出码本身不是业务成功依据；
- Norns 主动 task close 后，等待所有已投递输入按结果结算，再关闭 stdin 并进入 ended；通知来源的 result 也可完成此收尾，原 close 宽限期保留。

direct agent 保留终端交互，但仍使用同一 Subject、配置、Plugin、identity 和 Runtime artifact 逻辑。

### 11.1 Runtime 用量

Runtime 详情的 `usage` 为累计摘要，未收到结果前为 null：

| 字段 | 口径 |
| --- | --- |
| result_count / error_result_count | 本 Runtime 已去重的根 result 数量及其中 is_error=true 的数量，含通知结果。 |
| totals | 对每个 result.usage 的数值计数递归求和；不累加字符串/数组元数据。只涵盖这些结果实际报告的用量。 |
| latest | 最近一个根 result 的原始 usage。 |
| latest_result | 最近结果 id、subtype、is_error，以及存在时的 terminal_reason、api_error_status、errors、origin。 |
| model_usage | 最近 result.modelUsage 的进程累计快照，直接替换，不逐轮相加；可能包含未单列到根 result.usage 的内部/子 Agent 用量。 |
| total_cost_usd | 最近 result.total_cost_usd 的进程累计金额快照，直接替换，不相加。 |

通知结果参与统计；system/task_progress 等中间用量不另外累加，避免重复计费。totals 与 model_usage 的覆盖范围不同，不能相加当作总用量。错误历史完整保存在 agent.result 与 runtime_results，不用 Runtime.error 表示单次可恢复 API 错误。

## 12. Scheduler、停止与资源

Scheduler 按固定槽位领取 queue。Manager 持有每个 active Runtime 的 cancel、stdin、进程树、状态和 Norns close 协调。一个 Runtime 的状态变化不能阻塞其他槽位。

stop 请求先原子持久化接受，再异步执行。优雅停止设置 deadline；超时后终止进程树。Norns close 使用独立宽限期，避免把 HTTP handler deadline 当作智能体收尾期限。

资源监控在所有平台统一使用进程树 RSS，不使用 Windows Job Object 或 Linux cgroup 执行硬限制。Manager 在 Runtime 启动后立即采样，此后每 500ms 采样一次；每次结果追加到 `.nornsd/subjects/<subject_id>/runtimes/<runtime_id>/memory/memory-samples.jsonl`，并更新 Runtime 的 current、peak、limit、accounting 和采样间隔摘要。

每个 Runtime 的终止额度为当前系统可用内存基准的 50%。Windows 与 macOS 使用系统物理内存；Linux 默认使用 `/proc/meminfo` 的 `MemTotal`，同时读取当前 cgroup v2 `memory.max` 或 cgroup v1 `memory.limit_in_bytes`，存在更小的有限额度时将其视为容器内存基准，`max` 或大于宿主机物理内存的值视为未限制。RSS 超过额度时，nornsd 将 Runtime 的 `end_reason` 和 `error_code` 统一写为 `RUNTIME_MEMORY_LIMIT_EXCEEDED`，记录 diagnostic，并终止对应 Runtime 进程树。

如果紧邻的上一 Runtime 因该错误结束，Norns Subject 的下一 Runtime 会在首条 instruction 前收到内存恢复提示；普通 Subject 仅在显式 `resume_session=true` 时收到相同提示。普通 Subject 创建全新 Session 时不注入该提示，避免把上一会话的异常背景带入无关会话。

## 13. 日志与 Event

每个 Agent 输入输出先写带时间和 Runtime ID 的 raw envelope，再投影为 readable JSONL 与数据库 Event。文件写入失败和数据库 Event 失败都按可诊断错误处理，不能静默丢失协议控制状态。

Event 类型保持稳定，payload 可以向后增加字段。Server Token、LLM Key、完整环境和生成 settings 永不进入普通日志、Event 或 API。

新增诊断投影包括 `agent.api_retry`（attempt/max_retries/retry_delay_ms/error_status/error）、`agent.status`、`agent.task_started`、`agent.task_progress`、`agent.task_updated` 和 `agent.task_notification`。合成 assistant 错误在文本事件中保留 error/model；`agent.result` 保留 errors、origin_kind、model_usage 和 total_cost_usd。用户回显为 `instruction.replayed`，匹配本次投递后另记 `instruction.accepted`；完成处理记录 `instruction.settled` 和对应 instruction_sequences/result_id。协议生成的用户中断文本为 `agent.interrupted`，其他 user 文本为 `agent.user_message`，不冒充 assistant 文本。部分帧仅保留 raw，不直接暴露完整 stream-json。

事件中的 `agent` 除角色名外还保留实例关联：`instance_id` 是 Runtime 内稳定的关联键（例如 `tool:<Agent 调用 ID>`），可选 `tool_use_id`、`task_id`、`agent_id` 保留 CC 原始标识。调用同名角色不会合并实例；仅依据明确 ID 连接别名。Agent/Task 工具调用的 `spawned_agent` 描述被启动的子实例，调用事件本身仍归调用者。

关联子实例的 `agent.tool_result` 可额外保留 `subagent_status`：结构化同步结果为 `completed/failed/stopped/cancelled`；`isAsync=true` 或 `status=async_launched` 的后台启动结果映射为 `running`；SendMessage 明确返回 `resumedAgentId` 时也为 `running`。缺少可靠结构化依据时省略，SendMessage 发送失败不视为目标子实例结束。后台执行终态仍由关联的 Agent task 通知确认，Bash task 完成不能代替子实例完成。此字段随 Event JSON payload 保存，无需新增数据库列或接口。

优先从 Agent 工具结果的 `tool_use_result.agentId`（也接受 `agent_id`）读取实际 Subagent ID；缺少结构化 ID 时，仅对关联到 Agent 调用的工具结果解析独立 `agentId: xxx` 行。用该结果的 `tool_use_id` 将实际 ID 与调用关联；单帧有多个工具结果时不使用帧级 agentId，避免串号。`input.resume` 可连接同一 Runtime 内已知实例；2.1.199 实测还通过 `SendMessage(to=<agentId>)` 恢复，返回 `tool_use_result.resumedAgentId`。SendMessage 使用 `target_agent` 表示明确的关联目标，任务与输出归入原实例，即使任务通知采用新的 SendMessage 调用 ID、转发输出仍采用原 Agent 调用 ID。实际 ID 尚未返回时使用调用 ID，绝不按角色名或自由文本猜测。

转发输出通过 `parent_tool_use_id` 找实例，工具结果通过工具调用记录恢复调用者；后台 Agent task 通过自身 `tool_use_id`、`task_id`、`agent_id` 关联，不将 task 事件中的父调用误认成子实例；`local_bash` task 通过 Bash 的工具调用者及 task ID 恢复归属，不创建额外的子实例。缺少明确实例的角色输出保留角色并标识实例未知；未知背景任务不标为 Main Agent。关联状态在 Runtime 内隔离，并从 raw 按顺序重建，保持重放确定性、稳定事件序号及原始归属依据。实例元数据在现有 JSON payload 中存储，无需新增 SQLite 列。


`runtime.finished` 的公共 payload 包含 `state`、`exit_code` 和持久化的 `end_reason`；当
`state=error` 时额外包含 `error_code`。进程等待错误只写入 raw lifecycle 记录，不作为
面向用户的 Event 字段。

## 14. 启动恢复与遗留队列

Root 持有排他锁后，在启动调度器和覆盖模板副本之前，先将遗留 queued Runtime 导出到 `.nornsd/queue_record/<UTC纳秒>.json`，没有遗留队列不生成文件。正常停服停止新提交与领取，等待在途领取和 active 收尾，再导出剩余队列；异常退出由下次启动补做。服务运行期间 FIFO 仍保留，取消的只是跨 Service 生命周期续跑旧队列。

导出包含 export_id、Root ID、时间、原因、原模板，以及按 FIFO 排列的 Subject 身份/目录、Runtime ID、queue sequence/time、原 Profile（未确定时为 null）、请求 config、session mode/process 元数据与全部待执行 instructions。配置不脱敏。先同步并原子发布文件，再取消 Runtime、记录原因/Event、清理 queue 和释放 Subject，保留历史与 instructions。导出失败回滚队列并阻止启动；文件发布后事务提交前崩溃，使用内容摘要验证并复用已有导出。记录导出路径和条数。

随后以数据库和控制目录 subject.json 交叉验证，恢复缺失的 idle 身份注册，完成 materializing/planned Subject 的物化；已物化目录缺失、ID/目录冲突或符号链接替换标记 unavailable。数据库整体丢失而已有身份时拒绝猜测重建。Agent 工作目录没有 identity 的普通同名目录不作为已注册 Subject 接管。

已经启动的 Runtime 保留原有进程身份确认、进程树清理、投递不确定标记、终态与 transcript 收尾。不能确认进程身份时保留 Subject busy 并记录 recovery blocked；不能把未知外部进程当作可接管进程。Norns 清理依据原 Runtime Profile；所需 CLI 不可用或清理失败则阻止启动，避免提前释放 Subject。

Session 始终使用 Runtime 自身 ID或持久化实际 Session ID；不再提供旧 Subject Session 回退。日志回放、readable 修复、staging 清理继续保留。

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
- Scheduler 并发、队列公平性、遗留队列导出/失败回滚/幂等、重启清理和资源限制；
- HTTP 认证、限制、cursor、状态码和跨 Subject 隔离；
- Norns close、View adapter 和异常 cleanup；
- Status/Trace 三种格式与完整文件透传、Cog/Exp 详情及真实 CLI 集成；
- 五种归档范围的空闲检查和打包互斥、WAL 快照、符号链接、超时/取消清理与慢速下载；
- macOS、Linux 与 Windows 的进程树行为。
