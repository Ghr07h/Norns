# Norns CLI 设计

本文记录 `norns` CLI 的长期产品设计与当前实现设计。协议实体、状态机、Graph 和角色语义以 [`../protocol/protocol.md`](../protocol/protocol.md) 为准；本文只定义该协议在本地 Workspace CLI 中的映射。

# 第一部分：原始设计

## 1. 定位与边界

`norns` 是单 Workspace 的协议状态工具。它负责：

- 初始化和重置 Workspace；
- 原子维护 Task、Cognition、Exploration 和派生 Graph；
- 强制协议版本、状态转换和引用不变量；
- 生成 Status、Trace 和 Visual View；
- 提供稳定的结构化输出、错误码与外部扩展入口。

`norns` 不负责：

- 启动、观察或停止智能体；
- 管理远程 Subject、Runtime 或任务队列；
- 解释具体智能体平台的会话协议；
- 自动判断业务 Task 是否完成；
- 提供数据库迁移或跨协议版本兼容。

Runtime 必须在关闭 Task 前停止并确认相关 Explorer 不再写入。CLI 只原子记录协议状态，不探测外部执行单元。

## 2. 版本边界

以下版本独立演进：

| 版本 | `versions.json` 字段 | 含义 |
| --- | --- | --- |
| 协议规范版本 | `protocol` | 对外可观察的 Norns 语义。 |
| 支持的协议版本 | `norns.support_protocol` | CLI 可接受的 Workspace 协议版本。 |
| SQLite schema | `norns.database_schema` | 本实现的物理存储格式。 |
| 产品版本 | `norns.version` | `norns` 可执行文件版本。 |

以上版本由构建入口写入二进制，实际值可通过 `norns version --json` 查看。CLI 只接受与自身支持的协议和 schema 相同的 Workspace。版本检查必须发生在任何持久化修改之前；`workspace reset` 也不能绕过版本检查。

## 3. Workspace 行为

只有 `<workspace>/.norns/norns.sqlite` 存在时才视为已初始化。仅存在 `.norns/`、`history/` 或锁文件不代表初始化完成。

`workspace init` 创建空 Workspace。重复执行时必须先校验 application ID、协议版本、schema 版本和关键状态，只有合法 Workspace 才幂等成功。

`workspace check` 使用共享生命周期锁和只读事务，复用 application ID、协议版本、schema 版本及协议不变量校验。成功返回 JSON 信封，data 包含 `valid: true`、`state`、`runtime_occupied` 和 `summary`；state 为 `open_task` 时附带当前 `task`，为 `no_open_task` 时附带按创建时间升序的历史 `tasks`，为 `empty` 时返回 `tasks: []` 并明确提示空 Workspace。`runtime_occupied` 从同一读取事务中的 `workspace_meta.runtime_occupied` 读取并转换为布尔值，false 时也输出，表示协议占用而非进程存活。summary 的 `task_count`、`cognition_count`、`exploration_count` 为整个 Workspace 的记录总数。检查不生成 View、不自动初始化工作区，非法工作区沿用既有结构化错误。

未初始化时，只有 init 和 `task open` 可以执行。首个 `task open` 可以自动初始化，但参数校验、数据库创建、首个 Task、首个 Task-init Cognition 与 Runtime 占用必须共同成功或共同回滚。

`workspace free` 使用独占生命周期锁及单次 SQLite 写事务，将当前 Open Task 的 Todo/Running Exploration 取消、将 Task 以 cancelled 关闭并清除 Runtime 占用；理由由命令自动记录。保持既有完成结果、Cognition 和编号计数，没有 Open Task 时幂等成功。仍执行既有版本和完整性校验，不修复非法工作区。成功 data 返回 `changed`、`runtime_occupied: false`、非空的 `closed_task_id`，以及 `cancelled_todo`、`cancelled_running` 数量。命令不停止实际 Agent，调用方须事先停止或隔离相关执行单元。

`workspace reset` 用于结束当前 Subject：

- 只允许在没有 Open Task 且状态可信时执行；
- 将 `.norns/` 下除 `history/` 外的内容归档到唯一的 `.norns/history/<timestamp>/`；
- 创建全新的空 Workspace，不继承旧 Task、Graph 或 Cog / Exp 计数，编号重新从 1 开始；
- 使用恢复日志保证目录移动中断后可以确定性回滚或前滚；
- 不兼容版本或无法确认 Task 状态时拒绝执行且不移动内容。

## 4. CLI 操作面

公开命令分为：

```text
norns version [--json]
norns workspace init|check|free|reset
norns task list|open|close|reopen
norns cognition add|get
norns exploration add|get|retry|assign|finish|todo|running
norns view status|trace|visual
norns <external-extension> ...
```

命令参数和示例见 [`../../norns/README.md`](../../norns/README.md)。核心行为遵循以下约束：

- 首个 Task 必须提供 Subject 信息；后续 Task 未提供新信息时，按 `closed_at DESC, id ASC` 选择最近关闭 Task 并复用其 `init_cognition_ref`，提供新信息时创建独立的 Task-init Cognition；新建 Task-init Cognition 的 `origin_ref` 和 `task_id` 均为新 Task ID；复用旧 Cog 不改写其内容或来源，不建立 Cog 之间的来源链；
- `task close` 原子取消剩余 Todo/Running Exploration，并清除 Runtime 占用；
- `task reopen` 保留已有 Graph，不自动重试历史 Exploration；
- `task list [--status open|closed]` 只读列出 Task，默认包含全部历史状态，返回 JSON 信封的 `data.tasks` 数组；按创建时间升序、同一时间按 ID 升序，无匹配项时返回 `[]`。结果包含 Task 定义、状态和已有的关闭字段。使用共享生命周期锁和同一次只读事务，保留版本及协议完整性校验，但只为结果读取 Task 行，不加载 Cog/Exp 实体、不构造 Graph 或发布 View；不适用 View 的 6 KiB 文件引用阈值；
- Exploration 默认 `sources` 是当前 Task 的 Task-init Cognition；显式来源非空、有序且不得重复，可以引用任何来源的 Cog，包括用户全局认知及其他 Exploration 的用户提示；
- `cognition add --origin user` 不指定 Exploration 时添加全局断言，`origin_ref` 为当前 Open Task ID、`task_id` 为空；指定 Exploration 时原子保存用户断言并追加目标的 `hints` ID，目标可以是任意状态或历史 Task 的 Exploration；
- assign 必须同时提供非空 executor 与 output reference；
- Todo 只能直接 finish 为 cancelled；
- retry 从任意历史 Task 的 Finished Exploration 创建当前 Task 的新 Todo，复制定义和有序 `sources`，新 `hints` 为空，不复制旧 Cog 或 hints 成员；
- Todo/Running 查询只面向当前 Open Task，没有 Open Task 时返回 `NO_OPEN_TASK`；Running 返回 `explorer_id`、`assigned_at` 和 `duration_seconds`，不返回 `output_ref`，需要输出引用时通过 `exploration get` 获取。

来源输入保留可重复的 CLI 参数 `--source-cognitions COG_ID`，映射到协议字段 `sources`；CLI 参数名与序列化字段名分别定义。实体结果、调度查询及结构化 View 均使用 `sources`，不继续输出 `source_cognitions`。本轮不增加创建 hints 的独立命令，也不允许调用方直接编辑 `hints` ID 列表。

`cognition add` 提供可选的 `--meta JSON`，只接受键和值均为字符串的 JSON 对象，省略时保存 `{}`。显式空字符串、JSON null、数组及任何非字符串值均作为 `INVALID_ARGUMENT` 在写入前拒绝。领域键不在 CLI 中预定义或解释；元信息与认知共同创建且不可修改，不改变来源类别或图关系。非空时成功结果附带解码后的 `meta` 对象。


`cognition get --cognition COG_ID` 与 `exploration get --exploration EXP_ID` 按 ID 返回单个完整实体，覆盖所有来源、状态和历史 Task，不要求 Open Task。ID 在读取前验证，不存在时分别返回 `COGNITION_NOT_FOUND` / `EXPLORATION_NOT_FOUND`。查询使用既有共享生命周期锁、版本预检和完整性校验，在只读事务中按主键读取；实体解码与 View 全量加载共享扫描逻辑，不为单实体查询构造全图或发布 View 文件。

成功信封的 data 直接为实体：Cognition 包含 id、title、content、origin、origin_ref、task_purpose_relevant、created_at，以及非空 meta 和非空 task_id；内部 origin_task_id / origin_exploration_id 不输出。Exploration 包含 id、title、content、task_id、sources、hints、status、created_at，以及非空 retry_of、explorer_id、output_ref、assigned_at、outcome、outcome_reason、finished_at。sources / hints 为有序 ID 数组，sources 保持非空，空 hints 为 []，不展开正文；空可选字段省略。get 不适用 View 的 6 KiB 文件引用阈值。

## 5. 输出契约

除结构化 View 和人类模式的 version 外，成功结果使用单行 JSON：

```json
{"ok":true,"data":{},"warnings":[],"message":"Workspace initialized"}
```

失败结果写入 stderr，stdout 保持为空：

```json
{"ok":false,"error":{"code":"NO_OPEN_TASK","message":"Workspace has no open Task","details":{}}}
```

调用方只依赖稳定 code、字段和枚举，不解析 message。退出码分类：

| 退出码 | 类别 |
| --- | --- |
| `0` | 成功，包括带 warning 的成功。 |
| `2` | 参数或输入错误。 |
| `3` | 协议或业务状态冲突。 |
| `4` | Workspace、存储或内部错误。 |

`max_concurrency` 是软限制。合法修改提交后若仍超额，返回 `CONCURRENCY_LIMIT_EXCEEDED` warning，并包含 `operation_applied=true`、实际 Running 数和额度。

人类提示支持中文和英文，按主机 locale 选择；机器字段、枚举、ID、时间、warning code 和 error code 不本地化。

## 6. View 绑定

Status 与 Trace 默认输出 UTF-8 YAML，`--json` 输出等价 JSON，`--md` 输出适合阅读的 Markdown；两个格式参数互斥，冲突返回 INVALID_ARGUMENT。输出同时原子发布到 `.norns/view/`。字段、投影、顺序和最小子图规则以正式协议为准。

YAML 中 Exploration 的 `sources` 使用单行 flow sequence，例如 `sources: [cog_1, cog_2]`；JSON 仍为数组，来源顺序不变。

- Task-init 断言和 Explorer 派生认知留在图的 `cognitions` 中；全局用户断言只放顶层 `global_hints`，用户探索断言只放所属 Exploration 的 `hints`。
- 两种用户认知列表的对象均只有 `id`、`content`、`created_at`；空 `global_hints` 保留为 `[]`，空 Exploration `hints` 省略。
- `sources` 始终保留原 Cog ID；引用图外或嵌入认知时不复制节点，不替换成所属 Exp ID。
- 全部认知仍是共享协议事实；特殊投影位置不代表内容已被系统验证。
- Status / Trace 的图内认知、global_hints 和 Exp.hints 均不输出 `meta`，保持既有精简投影。

Visual View 是自包含 HTML：

- 展示全部 Task、完整 Graph、全部来源的认知、Exploration 状态、Runtime 占用和所有 Task 的 Purpose 相关认知；retry 连线单独表达执行沿袭重试；
- 不依赖网络资源；
- 用户 Markdown 使用 GFM 渲染，禁止原始 HTML 并经过白名单清洗；
- 未经清洗的用户内容只通过 `textContent` 注入；
- `--lang zh|en` 显式选择 HTML 语言，省略时按操作系统 locale 选择；
- 默认返回生成文件路径，`--open` 才请求系统浏览器打开。
- 非空的 Cognition `meta` 只在 Cog 节点详情中以格式化 JSON 字典代码块展示，使用 `textContent` 注入；不追加到正文、节点标题或探索产出摘要。

## 7. 外部扩展

未知核心命令仅在 `norns` 实际二进制所在目录查找 `norns-<name>` 外部可执行文件。不读取 `NORNS_PLUGIN_PATH`，不搜索系统 `PATH` 或当前工作目录。主程序通过符号链接启动时，解析链接后使用实际二进制所在目录；无法确定该目录时直接报错，不回退到其他路径。

扩展获得原始 argv、标准输入输出，以及：

- `NORNS_EXECUTABLE`；
- `NORNS_WORKSPACE`；
- `NORNS_PROTOCOL_VERSION`；
- `NORNS_SCHEMA_VERSION`。

核心命令不能被扩展覆盖。扩展若需要修改协议状态，必须调用 `norns`，禁止直接写 SQLite。

`norns extension list` 不依赖已初始化的 Workspace，仅扫描主程序实际二进制所在目录，复用调用扩展时的路径解析与可执行文件检查。每个扩展名显示其绝对路径，按名称排序，排除核心命令名；只检查文件，不执行扩展。输出使用 CLI JSON 信封，`data` 为 `[{"name":"...","path":"..."}]`，没有可发现的扩展时为 `[]`。

仓库提供 `norns-websearch` 参考扩展。它与 Workspace 解耦，未配置代理时使用百度，显式配置 HTTP/SOCKS5/SOCKS5H 代理时使用 Google，并输出稳定的 `title`、`href`、`body` JSON 数组。自带扩展集中在 `internal/custom_extensions/<名称>/`，入口为其中的 `cmd/norns-<名称>/main.go`。发布默认只编译主程序，使用可重复的 `--with-ext <名称>` 显式选择扩展；根发布入口仅向 norns 转发此参数。重复名称去重，非法或不存在的扩展在构建前拒绝，发布会清理目标平台中未选中的旧 `norns-*` 产物。

`norns-deepseeksearch` 提供 DeepSeek Search / Fetch API：默认搜索（也接受 `search` 子命令），`fetch --url` 获取 Markdown 正文。`NORNS_EXTENSION_DEEPSEEKSEARCH_CONFIG` 指向 YAML/JSON 配置文件，字段为必填 `api_key`、可选 `base_url`（默认 `https://api.deepseek.com`）和 `timeout_seconds`（默认 `30`）。客户端分别向 `/v1/tools/search`、`/v1/tools/fetch` 发送 Bearer 鉴权的 JSON POST 请求。按官方限制校验输入，成功保留 API 原生响应结构和排序；搜索无结果是成功的空数组。失败返回 stderr JSON，API 错误包含 HTTP 状态码及可用的上游错误信息、`Retry-After`；不自动重试。此扩展同样不依赖 Workspace，通过 `--with-ext deepseeksearch` 显式加入发布。

每个扩展源码目录提供 `metadata.json`，字段为 `name`、`version`、`description`、`requirements`（字符串数组，无使用要求时为 `[]`）和 `release_files`（附件列表，省略或 `[]` 表示无附件）。发布前严格校验字段、类型、长度及目录名称匹配，按名称排序合并本次所选扩展的元数据，输出二进制同目录的 `norns.exts.meta.json` JSON 数组并纳入校验和；无扩展时为 `[]`，重新发布会替换旧元数据。`extension list` 先发现实际存在的二进制，再按名称匹配元数据，额外显示版本、描述和前提条件。缺失文件或无匹配项时仅显示名称和路径；无效文件以简短 warning 提示并回退，不影响插件执行。元数据读取限制文件大小、拒绝符号链接和非普通文件，不执行代码、不展开变量、不读取配置和凭据，不将说明作为权限或执行规则。

`release_files` 的每个元素包含 `origin_file`、`target_file`：前者为插件目录内的相对普通文件路径，禁止绝对路径、`..` 路径段及符号链接；后者为基础文件名，空字符串时取源文件 basename；最终发布名称统一为 `norns.exts.<插件名>.<基础文件名>`，元数据保留未加前缀的配置值。不同插件的同名附件以此前缀隔离，同一插件内的最终名称不得重复。仅复制显式声明的单文件，不扫描配置内容或引用路径。发布前检查源文件、最终名称重名、加前缀后的长度、跨平台文件名合法性及共享元数据名称冲突；拒绝覆盖不属于旧插件附件的现有文件。附件先复制到 staging，再随二进制、合并元数据一起发布并纳入校验和。旧附件通过上一份已校验的合并元数据确定归属并清理，不递归删除附件目录；旧元数据损坏时阻止发布。根 `release.sh` 在各产品均完成构建后复用同一发布逻辑，确保不会丢失跨产品产物或遗留旧附件。Docker 原有的整个平台 Release 目录复制会自然包含这些附件。

插件使用说明和配置文档分别放在各自扩展目录的 `README.md`，Norns 主 README 只保留扩展列表命令和编写教程。`internal/custom_extensions/` 下的本地扩展默认被 Git 忽略，`websearch/`、`deepseeksearch/` 为明确参与版本管理的例外。

# 第二部分：实现设计

## 8. 技术选择

| 领域 | 实现 |
| --- | --- |
| 语言 | Go 1.25 |
| CLI | Cobra |
| SQLite | `modernc.org/sqlite`，纯 Go、无 CGO |
| YAML | `gopkg.in/yaml.v3` |
| Workspace 锁 | `github.com/gofrs/flock` |
| Markdown | Goldmark GFM + Bluemonday |
| Visual | `html/template` 与内嵌 CSS/JS |
| Websearch | `x/net/html`、`x/net/proxy` |

支持 macOS amd64/arm64、Linux amd64/arm64 和 Windows amd64。核心实现优先使用标准库，不提供供第三方稳定导入的公共 Go package。

## 9. 模块边界

```text
cmd/norns
  -> internal/cli
      -> internal/app
          -> internal/store/sqlite
          -> internal/workspace
          -> internal/domain
      -> internal/output
      -> internal/extension

internal/custom_extensions/websearch/cmd/norns-websearch
  -> internal/custom_extensions/websearch

internal/custom_extensions/deepseeksearch/cmd/norns-deepseeksearch
  -> internal/custom_extensions/deepseeksearch
```

- `domain`：实体、枚举、不变量、ID、Clock；不依赖 CLI、SQLite 或文件系统；
- `store/sqlite`：schema、事务、查询与完整性检查；
- `workspace`：路径、生命周期锁、init/reset 与恢复；
- `app`：组合事务和协议用例，构造三类 View；
- `cli`：参数解析、用例调用与 renderer 选择；
- `output`：JSON/YAML、warning 与 error envelope；
- `extension`：外部命令发现和无 shell 执行；
- `i18n`：仅处理人类文本；
- `version`：产品、协议和 schema 版本；
- `custom_extensions/<名称>`：各命令扩展的独立实现、测试及入口；websearch、deepseeksearch 不依赖 Workspace。

时间、Task UUID、文件 token 和关键文件操作通过小接口注入，使事务时间及 reset 故障点可以确定性测试。Cog / Exp ID 由 SQLite 在实体写事务内分配。

## 10. SQLite schema

数据库标记：

```text
PRAGMA application_id = 0x4E4F524E  # NORN
PRAGMA user_version = <norns.database_schema>
workspace_meta.protocol_version = <norns.support_protocol>
```

尖括号表示由 `versions.json` 对应字段生成的整数，不是可直接执行的 SQL。

物理表：

| 表 | 用途 |
| --- | --- |
| `workspace_meta` | 协议版本、Runtime 占用、Cog / Exp 的最后分配编号、创建时间。 |
| `tasks` | Task 固有字段与关闭状态。 |
| `cognitions` | Cognition 记录、来源的类型化物理列与相关性。 |
| `explorations` | Exploration 定义、`sources` / `hints` ID 数组、执行状态、retry 和结果。 |

`sources` 和 `hints` 直接保存在所属 Exploration 的字段中，不单独建立 `exploration_sources` 或 `exploration_hints` 表。两者都引用 `cognitions` 中的记录，列表中不保存认知正文。也不创建 relations、outputs、assignments 或 global Cognition 镜像表；Graph 关系仍从实体字段派生。

`explorations.explorer_id` 保存 assign 时 CLI `--explorer` 传入的实际 Explorer 不透明标识，使用可空 TEXT 列。Todo 时为空，Running 时必须非空，assign 后不可修改，Finished 后继续保留；retry 的新 Exp 从空值开始。实体、assign 结果、Running 查询和 Visual 数据统一使用 `explorer_id`，不提供旧 `executor_id` 名称兼容。

### 10.1 Cognition 来源映射

保留 `origin`、可空 `task_id` 和类型化来源列，将旧 `origin_cognition_id` 替换为 `origin_task_id`，保留 `origin_exploration_id`。每条 Cog 必须恰好具有一种非空来源引用，对外统一映射为非空 `origin_ref`：

| 场景 | `origin` | `origin_task_id` | `origin_exploration_id` | `task_id` |
| --- | --- | --- | --- | --- |
| Task-init 断言 | `task` | 创建它的 Task | 空 | 创建它的 Task |
| 全局用户断言 | `user` | 添加时的 Open Task | 空 | 空 |
| Exploration 用户断言 | `user` | 空 | 目标 Exp | 添加时的 Open Task |
| Explorer 派生认知 | `exploration` | 空 | 产生它的 Exp | 该 Exp 所属 Task |

断言 / 派生类别由上述组合推导，不新增可与来源冲突的独立持久化类别，也不更改 `cog_` ID 前缀。Task-init 和全局用户认知的相关性必须为 false；用户探索认知的 Task 归属可以与目标 Exp 不同。新认知可以在内容中修正旧认知，存储层仍只追加，不新增替代或失效关系。

`origin_task_id` 建立 Task 外键及查询索引；Task 创建与初始 Cog 的循环引用使用延迟外键，在同一事务提交时检查。Task 复用旧初始 Cog 时只设置新 Task 的 `init_cognition_ref`，不复制或更新旧 Cog。

`cognitions.meta` 使用 `TEXT NOT NULL DEFAULT '{}'` 保存 JSON 对象。数据库 CHECK 验证 JSON 有效且为对象；应用层进一步验证每个值均为字符串，包含 JSON null 的值也拒绝。完整性检查发现非法元信息时返回 `WORKSPACE_STATE_UNTRUSTED`，不得静默修复。Task-init Cog 没有额外的输入参数，创建时使用空对象；其他来源通过 `cognition add --meta` 可选设置。读取后解码为字符串字典供 Visual 使用。

### 10.2 sources 与 hints 数组

`explorations.sources` 和 `explorations.hints` 均使用非空 SQL 值的 `TEXT` 列，保存 JSON 编码的 Cog ID 数组。数据库 CHECK 保证有效 JSON、数组类型，并要求 sources 至少包含一个元素；hints 默认保存 `[]`，不使用 SQL NULL 或 JSON null。数组顺序就是成员顺序，不再需要 ordinal 列。

sources 创建后不可修改，成员必须是存在且不重复的 Cog ID，并保留输入顺序；retry 原样复制该数组。hints 创建时为空，只能在尾部追加新 Cog ID，不能重排、删除或替换既有成员。读取实体时解码为 ID 列表；结构化 View 展开 hints 时按认知创建时间和 ID 稳定排序，不将物理追加顺序作为展示顺序。

SQLite 普通外键不能约束 JSON 数组中的每个元素，因此成员类型、ID 格式、存在性、不重复和归属由事务内的应用校验及完整性检查保证。hints 必须双向核对：每个成员都是 `origin=user` 且 `origin_exploration_id` 等于所属 Exp 的 Cog；每个此类 Cog 也必须恰好出现在该 Exp 的 hints 中。全局用户 Cog、Task-init 和派生认知不能加入 hints。无效数组或引用应作为不可信状态拒绝，不得静默过滤、补齐或修复。

保存用户 Cog、读取目标 Exp 的最新 hints、追加 ID 并更新数组共用一个写事务；写锁取得后再读取旧数组，避免并发追加相互覆盖。更新仅涉及 hints，不覆盖执行状态或其他字段，允许目标是历史或 Finished Exp。任一步失败时 Cog 和数组修改共同回滚。反向来源查找可从同一 snapshot 构建内存索引，必要的 SQL 查询使用 `json_each`，不另存关系副本。

### 10.3 完整性与版本

数据库层通过 CHECK、FOREIGN KEY、唯一索引和应用层完整性检查共同保证：

- 最多一个 Open Task；
- 状态与可空字段组合合法；
- Task-init、全局 Cognition、hint 与 Explorer Cognition 的来源组合正确；
- hints 成员与 Cog 来源双向一致，认知正文保持不可变；
- retry 来源已 Finished 且无环；
- Runtime 占用与 Task 状态一致；
- Graph 引用类型、方向和 DAG 不变量成立。

Graph 校验覆盖全部 Cog，不依赖结构化 View 的投影位置。只从 `sources` 构造 Cog → Exp、从指向 Exp 的 Cog 来源构造 Exp → Cog；用户探索认知也参与后者。Task 来源引用及 `retry_of` 不进入该 DAG；分别校验其引用及 retry 无环。删除 `extends` 生成、遍历和校验，不要求全部节点归于同一初始 Cog。

Cog / Exp 的 ID 分别为 cog_1、cog_2… 和 exp_1、exp_2…；Task 仍为 task_<uuid-v4>。workspace_meta 新增 `last_cognition_number`、`last_exploration_number` 两个 INTEGER NOT NULL DEFAULT 0 列，CHECK 要求非负整数。所有来源的 Cog 共用同一计数器，Exp 新建与 retry 共用另一个计数器。

初始化首个 Task 的 staging 事务，以及普通实体写入的 BEGIN IMMEDIATE 事务，均先通过 UPDATE ... RETURNING 原子递增对应计数，再写实体；计数变更与实体一同提交或回滚。分配操作带 signed 64-bit 上限保护，耗尽返回 INVALID_STATE。不存在独立的进程内 Cog / Exp ID 生成器，也不使用 MAX(id)+1 分配编号。完整性检查要求计数为非负整数且不小于已存在实体的最大数字编号；损坏时拒绝，不自动修复。reset 沿用新建空数据库和既有恢复流程，计数默认归零，不读取历史目录的编号。

ID 校验对 Cog / Exp 仅接受无前导零的规范十进制正整数后缀，范围 1..9223372036854775807，拒绝旧 UUID 格式；Task 保留现有 UUID v4 校验。View、调度查询及 Visual 在同创建时间下按数字后缀排序，保留创建顺序，避免 10 排在 2 前；JavaScript 通过位数和字符串比较完整整数，避免 Number 精度损失。sources / hints 等关系仍保存完整 ID 字符串。本次 schema 从 12 调整到 13，不提供旧数据迁移。

## 11. 连接、事务与锁

数据库先以只读连接检查 application ID、schema 和协议版本，再按操作需要建立读写连接，避免不兼容文件因连接 PRAGMA 或初始化行为被修改。

连接启用 foreign keys、busy timeout 和 WAL。写操作使用 `BEGIN IMMEDIATE` 或等价的单 writer 事务，并通过条件更新防止两个调用方从同一旧状态成功转换。

锁分为两层：

- SQLite 事务处理数据库 reader/writer 并发；
- `.norns.lock` 处理 `.norns/` 的生命周期和目录替换。

普通读写和 View 使用生命周期共享锁；init、reset 和 reset recovery 使用独占锁。文件锁不能代替 SQLite 事务。

依赖已有 Workspace 的操作在加锁前检查 `.norns/` 和数据库文件；缺失时返回 `WORKSPACE_NOT_INITIALIZED`，不产生锁文件。显式 init、首次 task open 和 reset journal 恢复不受此快速检查阻断。数据库验证和读写仍在锁内完成。

最后一个持锁者在独占保护下删除锁文件。共享操作结束后先释放共享锁，再非阻塞获取同一文件的独占锁以尝试清理；仍有其他持锁者时由后续持锁者负责。所有等待者加锁后必须核对打开文件与当前路径的文件身份，不匹配则释放旧锁并重试，防止删除/重建导致互斥失效。正常空闲时不保留 `.norns.lock`；崩溃残留由下一次操作清理。此协议要求同一 Workspace 的并发调用统一使用新版本。

## 12. Workspace 文件布局

```text
<workspace>/
  .norns.lock
  .norns/
    norns.sqlite
    view/
    history/
  .norns-reset.json   # 仅 reset 进行或恢复期间存在
```

init 在同父目录 staging 中建立完整数据库并原子发布。reset 使用带 token 和 phase 的恢复日志，避免崩溃后无法判断旧 Workspace、新 Workspace 和归档各自状态。

View 在持有共享生命周期锁期间读取同一数据库 snapshot，写入唯一临时文件，完成 flush 后原子 rename。多个 View 不共享固定临时文件名。

## 13. 用例事务

所有修改命令先完成可前置的参数校验，再进入单个原子提交边界。典型组合事务包括：

- 首个 Task、首个 Cognition 与 Runtime 占用；
- 后续 Task 与可选的新 Task-init Cognition；
- Exploration 与有序 sources；
- 用户探索 Cog 与目标 `explorations.hints` 数组追加；
- retry Exploration、复制的 sources 与 `retry_of`，不写入 hints 成员；
- Task close、剩余 Exploration 取消与占用清除；
- Task reopen 与占用设置。

批量 close 使用同一个时间写入 Task `closed_at` 和被取消 Exploration 的 `finished_at`。自动生成的取消原因是可读文本，机器判断仍依据 outcome。

向历史或 Finished Exp 追加用户提示时，只记录新 Cog 和 hints 成员，不改变目标的 Task、定义、sources 或执行状态。相关性归属于添加时的 Open Task。提示向实际 Explorer 的投递由 Runtime 在提交后执行，投递失败不回滚认知或成员。

## 14. View 实现

View builder 从 store 的只读 snapshot 加载 typed model，在应用层执行协议投影，不在 renderer 中通过临时 SQL 拼接协议语义。

### 14.1 共享模型与投影

snapshot 加载全部 Task、Cog、Exp、有序 sources 和 hints ID，并构建 Cog/Exp ID 索引及 Exp 的派生认知索引。先验证引用及 hints 归属，再按来源分流。实体模型的 `Hints []string` 与 View 模型的 hints 对象列表分开；两种用户认知投影共用只含 `id`、`content`、`created_at` 的结构。

Task-init 和派生认知进入图内 Cog 投影；全局用户认知进入 `global_hints`；用户探索认知通过所属 Exp 展开为 `hints`。图内 Cog 不输出 `origin`，保留 Task ID 或 Exp ID 形式的 `origin_ref`；用户投影不输出来源、title、Task 归属或相关性字段。`global_hints` 及图内两个节点列表始终输出数组，空 hints 仅在 View 中省略。

Task、Cog、Exp 及展开用户认知按 `created_at ASC, id ASC` 稳定排序，sources 保留输入顺序。所有投影使用同一个 snapshot 和 `generated_at`；相关性判断读取原始 Cog，即使精简投影不展示标记，也不能丢失其 Trace 收录作用。

### 14.2 Status

Status 构造完整当前 Graph 与所选 Task 上下文。选择历史 Task 仅影响 Task 上下文、Todo/Running 摘要和相关性投影，不截断 Workspace 数据。顶层按协议依次输出 `task` 或 `tasks`、`graph`、`global_hints`、`todo_explorations`、`running_explorations`、非空时的 `recent_top10_derived_cognitions`、`generated_at`；没有选中单 Task 时两个调度摘要为空。

`recent_top10_derived_cognitions` 为只含 Cog ID 的摘要。使用同一 snapshot 的所有 `origin=exploration` 认知，以本次 generated_at 为基准，选择创建时间在闭区间 `[generated_at - 30 分钟, generated_at]` 中的记录，按真实时间倒序、同时间 ID 升序选取最多 10 条。该列表不受 Task 选择影响，没有符合条件的认知时省略；YAML 使用单行 flow sequence。认知正文及完整图仍按原规则输出；不增加查看游标或写入状态，也不将此摘要解释为价值、未处理状态或强制探索要求。

单 Task 模式仅对图内属于该 Task 且标记为 true 的 Cog 输出 `task_purpose_relevant`；全部 Task 模式对图内所有 true 标记输出。用户提示仍保持三字段投影。

### 14.3 Trace

Trace 使用目标 Exp 的 Task 作为唯一上下文。收录算法区分“补齐祖先上下文”和“收录本次执行的全部输出”，使用按 ID 去重的 Cog/Exp 集合及分别记录的处理状态，避免把所有祖先 Exp 的派生旁支一起带入：

1. 收录目标 Exp 及从目标开始的完整 `retry_of` 链；对链上每个 Exp 收录全部派生认知、全部用户 hints，并追溯其 sources。
2. 收录目标 Task 中 `task_purpose_relevant=true` 的所有 Cog，按同样的来源规则补齐上下文，包括其所属 Exp 可能来自其他 Task 的用户探索认知。
3. 来源为 Task-init Cog 时收录图内节点并停止；来源为全局用户 Cog 时由顶层全量 `global_hints` 提供并停止，均不沿 Task 来源继续追溯。
4. 来源为派生 Cog 时收录该 Cog、产生它的 Exp 和该 Exp 的全部来源祖先；来源为用户探索 Cog 时收录其所属 Exp，并继续追溯该 Exp 的全部 sources。
5. 任何被纳入的 Exp 都展开全部 hints。仅为祖先或提示承载而收录的 Exp 不自动带入其他派生认知，也不扩展其自身 retry 链；其他派生认知须分别满足来源引用、Purpose 相关性或目标 retry 链产出条件。

同一 Exp 后续从“祖先上下文”升级为“目标 retry 链成员”时仍需补入全部输出，不能因已经访问过而遗漏。所有用户探索认知只出现在所属 Exp.hints，来源 ID 保持原值。最终不输出相关性标记或 Todo 摘要；空子图列表、`global_hints` 和 `running_explorations` 保留为 `[]`。

顶层按协议依次输出 `current_exploration`、`parent_task`、`related_subgraph`、`global_hints`、`running_explorations`、`generated_at`。`current_exploration` 保留目标的 id、title、content；`parent_task` 表达当前探索是父任务拆分出的一个方向，提供目标所属 Task 的 `purpose` 和 `note`，不因收录跨 Task 祖先或用户认知而更换上下文。

`running_explorations` 从同一 snapshot 中选择目标 Exp 所属 Task 的全部 Running Exp，按创建时间及 ID 稳定排序，复用 Status 的 `id`、`title`、`duration_seconds` 投影，并额外输出来自持久记录的非空 `explorer_id`。包含 Running 的目标和子图外并行探索，不因此纳入其来源或产出；历史目标也不改用当前 Open Task。时长使用同一个 generated_at 计算并截断为非负整数秒，空时始终输出 `[]`。Status 的 Running 摘要仍保持三个字段。

英文 `note` 以单行字符串输出：`Complete only current_exploration. Use the parent Task's purpose only as context for understanding this exploration. Do not attempt to complete the entire parent Task.` 中文按相同语义提示只需完成当前探索，父任务目标仅供理解上下文，无需完成整个父任务。

### 14.4 Visual

Visual 从完整持久状态单独构造展示模型。

平面视角将 Task-init 与全局用户认知放在初始输入列，不为时间先后或 Task 来源添加连线；全局用户认知被用作 sources 时，仍显示对应来源连线。探索用户认知显示为所属 Exp 产出侧的 user hint 节点，并保留它作为其他 Exp 来源时的连线。Task-init 与派生节点保留既有内容、Task 和相关性详情。

平面和知识图谱布局都支持多个起点、多个连通分量及孤立节点。知识图谱可以在各分量内选择布局锚点，但不能为了形成单一根而添加认知关系；详情与追溯依据真实的 source/output 关系。删除 extends 连线、图例及其布局分支；retry 继续作为独立的执行沿袭展示，不参与协议 DAG 的分层或祖先收录。上述布局是本实现的选择，不提升为协议约束。

### 14.5 编码与文件发布

Status/Trace 的 YAML、JSON 和 Markdown 使用同一中间模型。应用层通过 ViewFormat 显式选择 yaml、json 或 md，非法值在读取和发布前拒绝。Markdown 渲染复用逻辑投影的 YAML node 和同一次只读事务加载的实体索引，保留收录范围和省略条件；不再次查询数据库或重新计算子图。实体索引仅用于补充标题、来源类别和探索创建时间，以及解析近期 Cog 摘要的标题和创建时间。Golden files 固定三种格式的公开形状。

Markdown 的一级标题为 Status View / Trace View，二级标题沿用原字段名并将首字母大写；graph / related_subgraph 分别替换为 Status / Trace。Task 首先引用显示 ID，随后为 **Purpose** 和引用正文。Tasks 列表中的每个 Task 使用同样排版，其他顶级字段顺序保持原样。

Markdown 在第一个顶级字段之后固定插入第二个二级章节 `## Graph`：Status 的顺序为 Task / Tasks、Graph、Status…；Trace 为 Current_exploration、Graph、Parent_task、Trace…。Graph 是原逻辑图的紧凑索引，内部为 `### Cognitions` 和 `### Explorations`，均使用 text 代码块逐行输出：`cog_id: title <- exp_id` 与 `exp_id: (status) title <- ['cog_id', ...]`。Task-init Cog 仅输出 id 和 title，不把 Task 来源引用画成图边；用户全局认知和探索提示不加入 Cognitions 索引，来源列表仍保留它们的真实 ID。节点顺序来自逻辑投影，sources 保留有序输入；Trace 索引只遍历 related_subgraph，不通过标题索引扩展节点或补入旁支。

Graph 不输出正文、时间、meta 或 retry 关系，空集合在代码块中显示 []。标题的换行和制表符折叠为空格，代码围栏长度根据正文中的反引号动态选择，避免标题改变章节结构。索引仅补充概览，不替代后续完整的 Status / Trace 内容或改变 YAML / JSON。

Status / Trace 章节先在 `### Task-Init Cognitions` 下以 `#### id - title` 保留逻辑投影中的所有 Task-init Cog，非空时才输出该子节。随后按创建时间和 ID 稳定排序展开 Exploration，标题为 `### exp id - title`。Exp 的元数据（含有序 sources、状态、已有的 retry / outcome / finished_at 及补充的 created_at）为一个无序列表，之后显示 **Content** 和引用正文。用一次遍历将原投影图内的派生认知按 origin_ref 分组，逐个在来源 Exp 下输出 `#### cog id - title`、元数据列表及 **Content** 正文，之后以相同形式输出该 Exp 的全部用户提示。两组各自保持认知创建顺序；认知元数据增加 origin 标签以区分 exploration 与 user，Trace 仍不输出 task_purpose_relevant。

Global_hints 保留独立顶级章节，认知同样补充标题并将元数据放在正文之前。Todo_explorations、Running_explorations 使用表格，空时保留表头和分隔行；Trace 的 Running 表保留 explorer_id。Recent_top10_derived_cognitions 从同一快照按已有 ID 顺序解析为 id、title、created_at 三列表格，无记录时省略；YAML / JSON 的近期 ID 列表不改变。其他空列表使用 `[]`；meta 不输出。

正文按文本转义 Markdown 标题、代码围栏、HTML 等符号，保留段落和换行；表格值中的竖线转义、换行归为空格，行内代码按内容选择足够长的反引号定界符，避免输入内容改变文档结构。

Status/Trace 三种格式无论大小都原子发布到 `.norns/view/`，扩展名与文件引用的 format 分别为 yaml、json 或 md。编码结果不超过 6 KiB 时，CLI 直接输出完整 View；超过 6 KiB 时只输出包含文件路径、格式、字节数和说明的 JSON 成功信封，data 字段为 path、format、size_bytes、note。note 按语言环境说明 View 内容较多，完整内容已写入 path 指向的文件而不直接返回全部内容。隐藏的 --output-reference 仍可对任意大小强制返回文件引用，此时小 View 的 note 说明已请求文件引用，不声称内容过大。Visual 始终通过成功信封返回生成的 HTML 路径。View 文件统一使用 `<type>-<UTC timestamp>-<token>.<ext>` 命名，其中 type 为 `status`、`trace` 或 `visual`；UTC timestamp 使用精确到秒的 `YYYYMMDDTHHMMSSZ`，token 为 8 位随机十六进制串，用于区分同一秒内生成的文件。

## 15. 安全与可靠性

- 所有状态写入都通过 CLI 和事务，不允许扩展直接操作数据库；
- Visual 禁止网络依赖、原始 HTML 与未经清洗的 `innerHTML`；
- 外部扩展通过 argv 直接执行，不经过 shell；
- 不受支持的版本在任何写入前失败；
- reset 在无法证明状态可信时拒绝操作；
- 错误 details 不依赖本地化文本表达机器语义。

## 16. 验证基线

测试覆盖以下层次：

- domain 枚举、ID、时间与来源组合；
- SQLite schema、引用、状态转换和并发竞争；
- init/reset 每个故障阶段及恢复；
- Task、Cognition、Exploration 全生命周期矩阵；
- 首个、后续新建和复用 Task-init 的 Task 来源引用，以及全局 / 历史探索用户断言的 Task 归属；
- hints 原子追加、并发追加、双向完整性和 Finished Exp 提示保留；
- sources / hints JSON 数组的有效性、成员类型、顺序、去重和引用检查；
- 所有来源 Cog 的 sources 引用、二部 DAG、多起点和无 extends；
- retry 不复制 hints，Trace 保留完整目标 retry 链及历史用户提示；
- Status/Trace golden files 和 Visual 内容安全；
- 用户认知唯一投影、三字段与空列表约定、查询 ID 数组和 View 展开对象的差异；
- 用户提示来源及 Purpose 相关性的 Trace 祖先闭包，排除无关派生旁支；
- Visual 多连通分量、用户断言来源和引用连线，不伪造单一根或 extends；
- CLI argv、stdout/stderr、exit code 与本地化；
- 外部扩展发现、环境传递和退出码；
- Websearch 后端解析、代理和结果规范化；
- 支持平台上的 race、vet 与真实二进制集成。

`testdata/requirements.json` 是行为覆盖 key 到测试函数的机器清单；CI 校验 key 唯一、格式正确且引用的测试存在。它不再反向解析设计文档的段落编号。
