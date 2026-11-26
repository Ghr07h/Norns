# nornsd

`nornsd` 将业务对象隔离为 Subject，为其排队和执行 Runtime、持续投递 instruction，并保存日志、Event、会话与运行历史。正式执行使用 Go 单一可执行文件和 Claude Code CLI 2.1.199，不依赖 Python 或 Claude Agent SDK。

普通 Agent 与 Norns 模式由每个 Runtime 的配置决定；同一 Subject 可以在不同 Runtime 使用不同模式。Norns 模式额外需要支持 `nornsd.support_protocol` 的 Norns CLI 和内嵌 Plugin。

## 构建

在仓库根目录执行 `./release.sh nornsd`；六平台使用 `--all-platforms`。也可在本模块执行 `go run ./scripts/release.go`，支持 `--output` 和可重复的 `--target <os>/<arch>`（不能与 `--all-platforms` 合用）。发布入口读取根 `versions.json`、生成版本代码和内嵌资源、编译并输出校验和。

模块开发检查：

```sh
go build -o ./bin/nornsd ./cmd/nornsd
go test -race ./...
go vet ./...
```

`go build` 不刷新版本或内嵌资源。需要更新协议/Plugin 资源时单独执行 `go generate ./internal/assets`。配置示例是外部文件，不内嵌，也不含 VERSION。旧数据库和目录布局不自动迁移。

## 启动与配置

```text
nornsd server --configs FILE [--configs FILE ...]
nornsd agent --config FILE [--resume-subject SUBJECT_UUID] [--resume-session] -- [AGENT_ARGS...]
nornsd version [--json]
```

必须显式提供配置，不再自动发现，也不提供旧 host/port/token/workspace-root/norns/settings 等配置参数。

```sh
cp configs/nornsd/nornsd.example.yaml ./nornsd.yaml
# 修改 Root、凭据、Agent 路径等字段后启动
nornsd server --configs ./nornsd.yaml
```

配置分为 `SERVICE_*` 与 `AGENT_*`：

- `SERVICE_NORNSD_ROOT_DIR`：必填绝对路径，工作根目录。
- `SERVICE_HOST` / `SERVICE_PORT`：默认 `127.0.0.1:28081`。
- `SERVICE_TOKEN`：server 必填、非空且不含空白；所有 API 包括 health 均要求 Bearer 认证。
- `SERVICE_CONCURRENCY`：正整数，默认 1。
- `AGENT_TYPE`：默认 cc，当前只实现 cc。
- `AGENT_EXEC`：可执行文件绝对路径与固定 argv 数组；省略或设为 `[]` 时从 PATH 查找 claude。Runtime 覆盖传入 `[]` 可恢复此默认命令，省略则继承模板。
- `AGENT_LLM_URL` / `AGENT_LLM_KEY` / `AGENT_LLM_MODEL`：必填；`AGENT_LLM_FAST_MODEL` 留空使用主模型。
- `AGENT_NORNS_ENABLE`：默认 false；探索并发 `AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS` 默认 3，问询周期 `AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS` 默认 1200 秒，`AGENT_NORNS_ALLOW_HUMAN_INTERVENTION` 默认 false。

配置中的非空路径必须绝对，可选资源目录可以留空。YAML 拒绝未知字段、重复键、null 字段、错误类型和多文档。

一份模板供全部槽位复用；多份模板数量必须等于第一份的 SERVICE_CONCURRENCY：

```sh
nornsd server --configs ./slot1.yaml --configs ./slot2.yaml
```

多份配置以第一份的 SERVICE_* 为准。启动时冻结并按顺序保存到 `.nornsd/configs/nornsd.yaml`、`nornsd2.yaml` 等；修改源文件需重启。Runtime 领取哪个槽位，就使用该槽模板与创建时的 config 覆盖形成最终配置。

服务启动会报告目录/数据库检查和恢复阶段，长阶段每 5 秒报告进度；显示 listening 后 API 就绪。非 loopback 明文监听保留风险提示。health 检查数据库写事务与调度器；事务错误保留请求 ID 和可用 SQLite 诊断。

## HTTP 执行

```sh
curl -X POST http://127.0.0.1:28081/api/subjects \
  -H 'Authorization: Bearer replace-with-a-long-random-token' \
  -H 'Content-Type: application/json' \
  --data '{"name":"review-a","runtime":{"instruction":"审查 repository A","config":{"AGENT_LLM_MODEL":"review-model"}}}'
```

创建 Subject 可不传 runtime，此时立即物化为空闲目录；传入时 Subject、Runtime、instruction、queue 在同一事务创建。空闲 Subject 提交创建新 Runtime，已有 queued/running/waiting Runtime 提交追加 instruction FIFO：

```sh
curl -X POST http://127.0.0.1:28081/api/subjects/SUBJECT_UUID/runtime \
  -H 'Authorization: Bearer replace-with-a-long-random-token' \
  -H 'Content-Type: application/json' \
  --data '{"instruction":"继续工作","config":{"AGENT_NORNS_ENABLE":false}}'
```

config 仅允许 AGENT_*，省略、null、{} 均无覆盖。显式 false/0/空字符串/空数组会参与覆盖，再校验有效值；数组整体替换。逐字段 null、错误类型、未知字段和 SERVICE_* 拒绝。已有 Runtime 追加 instruction 时非空 config 返回 `409 RUNTIME_CONFIG_CONFLICT`，包括尚未领取槽位的 queued Runtime。

Runtime 详情中的 runtime_profile 在领取模板前为 null，领取后冻结。配置、完整启动 argv 和凭据不通过普通详情响应返回。

常用查询：

```text
GET /api/
GET /api/health
GET /api/subjects
GET /api/subjects/{subject_id}
GET /api/subjects/{subject_id}/runtime
GET /api/subjects/{subject_id}/runtimes
GET /api/subjects/{subject_id}/runtimes/{runtime_id}
GET /api/subjects/{subject_id}/events
GET /api/queue
```

API 路径固定 `/api/`，整数 api_schema 报告格式版本。Event 分页使用接口返回的不透明 cursor，不把 Subject.last_event_seq 当作续读游标。调用方不要自动重试创建/提交，结果不确定时先查询。

## 会话与 Direct agent

每个 Runtime 的 ID 同时是新 Claude Session ID。默认后续 Runtime 只复用 Subject 文件，不继承对话；Norns 的跨 Runtime 状态保留在 `.norns/`。同一 Runtime 的追加 instruction 使用相同 Session。

HTTP 提交 `resume_session: true` 或 direct 使用 `--resume-session` 可从最新 Runtime 会话派生新 Session，实际传入 `--resume=<previous_session_id> --fork-session --session-id=<new_runtime_id>`。Subject 必须 idle 且存在历史会话；使用中的 Subject 返回 SUBJECT_RUNTIME_ACTIVE，无历史返回 RUNTIME_SESSION_NOT_FOUND。

```sh
nornsd agent --config ./nornsd.yaml
nornsd agent --config ./nornsd.yaml --resume-subject SUBJECT_UUID
nornsd agent --config ./nornsd.yaml --resume-subject SUBJECT_UUID --resume-session
```

direct 不进入服务队列，但与 server 共用 Root 锁、身份、配置发布和 Runtime 记录；同一个 Root 不能同时运行两个进程。Root 由配置决定，不由当前目录决定。恢复时允许更换 Runtime 配置和模式；仍须使用原 Root、空闲 Subject 和兼容的 cc 会话。

托管 Agent 使用 stream-json，固定开启 `--replay-user-messages`。每组输入带 UUID，分别记录管道写入、CC 回显接受和结果结算；多条输入可由一个 result 完成。后台通知如吸收了新输入也会结算对应指令，纯通知不会消耗其他输入。未确认输入不会自动重投。

恢复会话在 initialize 成功后记录一次 `runtime.session_resumed`，无需首次启动 result；不存在的会话保留真实错误并记为 `AGENT_STARTUP_FAILED`。API 错误、429 重试耗尽和中断只表示本次处理失败；CC 仍存活且无剩余输入时回到 waiting，允许在同一 Runtime 继续提交。nornsd 不叠加模型调用重试。进程退出时先读完两路输出再归档和释放槽位。Direct 保留终端交互。

Runtime 详情的 `usage` 包含 result_count、error_result_count、totals（累计已报告 result.usage 数值）、latest、latest_result，以及 model_usage/total_cost_usd 进程累计快照。快照只替换、不累加；后台通知也计入结果统计。`totals` 与 model_usage 覆盖范围不同，不应相加。事件增加 API 重试、任务进度、输入确认/结算与中断标记，错误详情保留 errors、api_error_status 和合成消息 error。

## 资源与 Plugin

资源配置按 `AGENT_NORNS_ENABLE` 区分，两类非空资源不得混用；空字符串/空列表可用于清空继承值。

Norns 模式（true）：

- `AGENT_NORNS_EXTENDS_AGENTS_DIR`：原 `AGENT_NORNS_CUSTOM_DOMAIN_DIR` 改名，继续读取 meta.json 与指定知识文件，拼接到内置 Decider/Explorer 正文，生成领域角色；文件结构不变。
- `AGENT_NORNS_CUSTOM_AGENTS_DIR`：完整的自定义 Norns 角色 Markdown，原样复制到 norns-plugin/agents/，解析 YAML frontmatter.name 并加入 Coordinator 白名单；tools 和其他 Agent 配置保留原文，角色需由提供方遵循 Norns 协议。
- `AGENT_NORNS_CUSTOM_SKILLS_DIR`：复制到 Norns Plugin 的 Skill。

Norns 模式只加载生成的 norns-plugin，不允许额外 Plugin；`AGENT_NORNS_CUSTOM_PLUGIN_DIR` 不是合法配置字段。扩展角色、完整自定义角色都加入白名单，名称不得与内置或其他角色冲突。

普通模式（false）：

- `AGENT_CUSTOM_AGENTS_DIR` / `AGENT_CUSTOM_SKILLS_DIR`：复制为独立 custom-plugin，不写 Subject .claude/。
- `AGENT_CUSTOM_PLUGIN_DIRS`：按顺序复制为 Runtime plugins/external-plugin-N/，每个副本生成 manifest；不直接加载外部源目录。

普通模式依次加载外部 Plugin 副本与 custom-plugin。未指定 Main Agent 时，Norns 使用 `--agent norns:coordinator`，普通模式沿用 Agent 默认系统提示词。目录结构与 meta.json 示例见[设计文档](../documents/design/nornsd.md#41-基于通用角色扩展领域知识)。资源继续拒绝符号链接逃逸。

`AGENT_MAINAGENT_FILE` 可在两种模式下指定 Agent Markdown 文件的绝对路径，以 YAML frontmatter.name 选择 Main Agent。**除非了解修改的影响，否则不要改动此配置。** Norns 模式将文件复制到 norns-plugin/agents/，启动 `--agent norns:<name>`，替换 `Agent(...)` 白名单和正文中的 `{{NORNS_RUNTIME_CONFIGURATION}}`；白名单排除通用 Coordinator 和自身。普通模式原样复制到 custom-plugin/agents/，启动 `--agent nornsd-custom:<name>`。仅指定 Main 文件也会生成 custom-plugin。

改写 Coordinator、所选 Main Agent 和派生领域角色时保留完整 YAML frontmatter，只更新指定字段；未知字段、嵌套配置和注释保留。未选中的通用 Coordinator、完整自定义子角色不进行改写。

配置末尾的 `AGENT_REUSE_RUNTIME_DIR` 指向旧 Runtime 目录，例如 `.nornsd/subjects/<subject_id>/runtimes/<runtime_id>/` 的绝对路径。读取其中的 `launch.json` 恢复插件加载顺序和 Main Agent，将整个 `plugins/` 复制到新 Runtime 的 `plugins/`；启动只引用新副本，源目录保持独立。两种模式均支持复用，不重新生成插件、改写 Agent 或注入配置，旧 manifest 的 Runtime ID 也原样保留；新 `launch.json` 记录本次 Runtime 与新副本路径。

复用与非空 `AGENT_MAINAGENT_FILE`、`AGENT_NORNS_EXTENDS_AGENTS_DIR`、全部 `AGENT_NORNS_CUSTOM_*`、`AGENT_CUSTOM_*` 互斥；须显式清空模板中继承的冲突字段。复用时忽略探索并发、超时问询周期和人类干预设置，LLM 等非插件配置仍使用新配置。缺失或无效的 launch、插件加载记录与文件不匹配、符号链接或源目标 Runtime 重叠均拒绝复用。

Runtime 覆盖切换模式时，须同时清空模板中另一模式的资源字段；不静默忽略。路径仍须为绝对路径，旧 `AGENT_NORNS_CUSTOM_DOMAIN_DIR` 不保留别名。

所有 Runtime 固定 `--dangerously-skip-permissions`、`IS_SANDBOX=1` 和 `--setting-sources=`；仅加载生成 settings、显式 Plugin 和不可关闭的 managed settings。默认或显式直接指向 claude 的命令预检精确版本 2.1.199；wrapper 保留兼容模式，仍要求可执行且兼容控制协议。

Linux/macOS 的内嵌 Norns Plugin 保留 POSIX sh/awk PreToolUse hook，防止 Claude Code 2.1.199 内嵌 ugrep 7.5.0 因多个 bounded repeat 正则编译耗尽内存；显式 /usr/bin/grep、内置 Grep 或 awk 不受该拦截影响。

托管 Runtime 启动即采样进程树 RSS，此后每 500ms 采样。单 Runtime 额度为系统内存基准 50%，Linux 容器有更小有限 cgroup 额度时采用该额度。超限终止对应进程树，记录 RUNTIME_MEMORY_LIMIT_EXCEEDED。下一 Norns Runtime 或显式恢复会话的普通 Runtime 保留原有内存恢复提示。

## View 与详情

Subject 存在有效 `.norns/` 时，查询不受最新 Runtime 是否启用 Norns 影响；需要可用 Norns CLI。

```text
GET /api/subjects/{subject_id}/norns/views/status?format=yaml
GET /api/subjects/{subject_id}/norns/views/trace?exploration_id=exp_1&format=md
GET /api/subjects/{subject_id}/norns/views/visual
GET /api/subjects/{subject_id}/norns/cognitions/cog_1
GET /api/subjects/{subject_id}/norns/explorations/exp_1
```

所有 Norns View 和 Cog/Exp 详情接口按顺序校验：Subject 下必须同时存在 `.norns/` 目录和 `.norns/norns.sqlite` 常规文件，否则返回 `409 WORKSPACE_NOT_INITIALIZED`（非 Norns 模式的工作区）；工作区存在后，再检查当前服务是否具备可用、兼容的 Norns CLI，否则返回 `503 NORNS_UNAVAILABLE`。不以最新 Runtime 或某个配置模板的 Norns 开关替代这两步检查。

Status/Trace 默认 YAML，可选 json/md，原样返回完整 UTF-8 正文。Norns CLI 返回文件引用时读取完整文件，不受 CLI 6 KiB 内联阈值影响。错误统一为 JSON。Visual 文件限制在 Subject .norns/view/ 内。

## 打包下载

统一使用 `GET /api/archive`，scope 必填：

| 参数 | 目录 |
| --- | --- |
| scope=subject | 整个 Root，含 .nornsd/ 和其他文件。 |
| scope=subject&subject_id=UUID | 对应 Subject 工作目录，含 .norns/，不含移出的配置、日志和 transcript。 |
| scope=rundata | 整个 .nornsd/。 |
| scope=rundata&subject_id=UUID | 对应 Subject 的运行数据目录。 |
| scope=rundata&subject_id=UUID&runtime_id=UUID | 对应 Subject 下的 Runtime 运行数据目录。 |

runtime_id 必须同时提供 subject_id 且只能用于 rundata，归属不匹配返回 404。旧 Subject archive 路由不保留。

全量要求全部 Subject idle，指定范围要求该 Subject idle（包括单 Runtime）。五种范围共享一个生成额度；冲突返回 ARCHIVE_BUSY。请求内生成临时 tar.gz 后下载，独立 10 分钟超时覆盖生成和下载，客户端断开即取消。生成完成释放额度，下载结束/失败清理临时文件。

保留目录名、隐藏文件和不跟随的符号链接；SQLite 使用一致性快照，排除 WAL/SHM、活锁、owner、内部 staging 和 socket 等临时项。

## 目录与重启

```text
<nornsd_root>/
  .nornsd/
    service/{root.json,owner.json,nornsd_lock,nornsd.sqlite[-wal|-shm]}
    configs/{nornsd.yaml,nornsd2.yaml,...}
    staging/
    queue_record/<UTC>.json
    subjects/<subject_id>/
      subject.json
      staging/
      logs/{raw.jsonl,readable.jsonl}
      runtimes/<runtime_id>/
        launch.json
        configs/{nornsd.yaml,agent/settings.json}
        plugins/{external-plugin-N,custom-plugin,norns-plugin}/
        memory/memory-samples.jsonl
        transcripts/{transcript.manifest.json,<runtime_id>.jsonl,<runtime_id>/...}
  <subject_uuid>_<UTC纳秒>/
    # Agent 工作文件与可选 .norns/；不创建 .nornsd/
```

launch.json 合并启动快照，包含 runtime、session、agent、config、plugins 分组，记录完整 argv 和有序副本 hash，不生成旧 snapshot.json/config.snapshot.json。全量有效配置（含 SERVICE_* 和凭据）单独保存在 Runtime configs/nornsd.yaml。终态、usage 和资源结果留在数据库。

服务运行期间保留 FIFO；跨服务生命周期不再恢复旧排队 Runtime。正常停服停止新提交与领取并导出未启动队列，异常退出在下次启动调度器前补导出。文件位于 queue_record，包含原模板、请求覆盖、身份、顺序、instructions 和恢复会话元数据。

导出成功后原 Runtime cancelled、释放 Subject，历史与 instructions 保留；失败回滚并阻止启动。文件发布与事务提交之间崩溃可验证并复用已有导出。无遗留队列不生成文件。已经启动的 Runtime 继续进行进程身份确认、进程树清理、transcript 和 Norns 收尾；无法确认进程时保留 busy，不假定安全接管。

Root 锁是否被持有决定占用，owner 文件存在不能判断进程存活。配置、launch、queue export 和 transcript 可能含敏感信息；不要提交到版本库。非 loopback 明文 HTTP 应由可信网络或 TLS 反向代理保护。

完整说明见 [设计与 HTTP API](../documents/design/nornsd.md) 和 [Norns 协议](../documents/protocol/protocol.md)。

事件的 agent 元数据记录 Runtime 内的 Subagent 实例关联；Agent 工具返回的实际 agentId 与 parent_tool_use_id、后台 task ID 通过明确标识关联，同名角色不合并。调用/结果事件的 spawned_agent 描述子实例，agent 仍表示调用者。关联依据随现有 JSON payload 保存，raw 重放可重建。

关联子实例的工具结果还保留可选 `subagent_status`：同步完成/失败与后台启动、恢复的进行中状态分别记录；后台启动结果不会被当作执行结束。客户端可结合 Agent task 的完成/失败/停止通知显示实例状态，不能用内部 Bash task 或 Main 的一轮 result 推断整个 Agent/Runtime 结束。
