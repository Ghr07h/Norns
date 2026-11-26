# Well 设计

本文记录 Well 的长期产品设计与当前实现设计。服务端资源和接口契约以 [`nornsd.md`](nornsd.md) 的 HTTP API 章节为准。

# 第一部分：原始设计

## 1. 定位与边界

Well 是本地桌面 GUI，用于连接和管理多个 nornsd Server，浏览 Subject、Runtime、Queue、Event 与 Norns 信息。

当前提供：

- 保存、编辑、连接和移除多个 Server；
- 汇总 Server 可达性、身份、容量和能力；
- 浏览和筛选 Subject；
- 创建 Subject，可选同时提交首条 instruction；
- 查看当前 Runtime、历史 Runtime 和全局 Queue；
- 向 Subject 提交 instruction；
- 请求 graceful 或 force stop；
- 增量读取并缓存 readable Event；
- 查看 Norns Status、Trace、Visual View 与 Cog/Exp 详情；
- 下载整个 Root、Subject 工作目录或分级运行数据的 tar.gz 归档；
- 展示服务端返回的 Workspace 与 Runtime artifact 路径。

Well 不负责：

- 安装、启动、更新或配置 nornsd；
- 直接访问 Subject 文件系统、SQLite、raw log 或 transcript；
- 解释 Claude Code stream-json；
- 实现 API 不提供的重命名、删除或迁移；
- 将 nornsd Subject 等同于 Norns Task；
- 提供账户、中心服务或云同步。

## 2. Server 身份

Server 是 Well 本地连接记录，不是 nornsd 资源：

| 字段 | 含义 |
| --- | --- |
| `id` | Well 生成的本地 UUID。 |
| `name` | 用户设置的非空显示名称。 |
| `base_url` | 规范化服务根 URL，只包含 origin，不包含 `/api/` 或其他路径。 |
| `remote_server_id` | nornsd root 的稳定 UUID。 |
| `remember_password` | 是否持久保存 Token；字段名为 v1 存储兼容名称。 |
| `password` | 持久或 session-only Token；字段名为 v1 存储兼容名称。 |
| 时间字段 | 创建、更新与最近成功连接时间。 |

规范化 `base_url` 在本地 registry 中唯一。修改 URL 时保留本地记录 ID，但清除其远端 identity、query cache、Event/View cache 和当前选择；只修改名称或 Token 不清理业务缓存。

首次成功连接后绑定 `remote_server_id`。同一 URL 后续返回不同 ID 时进入 `identity_changed`，禁止继续远端修改，直到用户确认并清空本地关联数据。多条连接记录可以绑定同一远端 ID，用于同一 root 的多入口访问、迁移恢复或独立副本；Well 显示“识别到重复的 Server Root 记录”告警，但连接保持在线且不阻止工作区操作。

跨 Server 身份必须组合表示：

```text
Subject = (local_server_id, subject_id)
Runtime = (local_server_id, subject_id, runtime_id)
```

## 3. 连接状态

| 状态 | 含义 |
| --- | --- |
| `unknown` | 尚未检查。 |
| `checking` | 正在读取服务信息/health。 |
| `retrying` | 连接或超时失败，正在尝试本轮的剩余重试。 |
| `online` | 认证、服务类型、API 版本和 health 均有效。 |
| `offline` | 网络、超时或服务不可用。 |
| `unauthorized` | Bearer 认证失败。 |
| `incompatible` | 非 nornsd 或 API 版本不支持。 |
| `identity_changed` | URL 后的远端 root 已改变。 |

连接状态是客户端观察，不写回服务端。短暂失败不能删除 Server 或缓存。进入
Servers 页面时，每条具有 Token 的记录立即开始一轮连接探测；单次探测最长
3 秒，只有连接或超时错误才重试，最多共尝试 3 次，重试间隔为 1 秒。三次均失败后
进入 `offline`，不再后台自动重连；重新启动 Well、从工作区返回 Servers 页面、手动刷新
或修改连接配置会开始新的探测。`401` 直接进入 `unauthorized`，显示 Token 不匹配，
不参与自动重试。

## 4. 本地凭据与数据

新增 Server 默认勾选“记住 Token”。勾选时 Token 进入本地 registry；取消后立即删除持久值，仅保留当前 session 输入。主动断开只清 session Token，移除 Server 则删除其全部凭据和缓存。

Well 当前没有用于加密该存储的主密码。记住的 Token 由 WebView 本地存储保护，其安全级别取决于操作系统账户和应用数据目录；README 必须明确这一点。

本地存储分层：

- `localStorage`：带 schema version 的 Server registry 和界面偏好；
- session memory：不记住的 Token、当前选择和临时加载的旧 Event；
- IndexedDB：可增长 Event cache。

Event cache 以 `(server_id, subject_id, seq)` 唯一定位。每个 Subject 跨重启保留最近 5,000 条；用户本次显式加载的更早历史保留在面板 session buffer 中，切换 Subject 或销毁面板时可以释放。

## 5. 信息架构

应用启动直接进入 Servers 页面，不需要登录页。

### 5.1 Servers overview

以可扫描列表展示所有本地 Server 的名称、URL、连接状态、远端 identity、协议版本、容量与 Queue。支持新增、编辑、连接、断开和移除。

多个 Server 的检查独立并发；单个失败不得阻塞其他 Server。`identity_changed` 必须显式阻止进入工作区与修改操作。相同 `remote_server_id` 只产生非阻断告警；各连接的 query、Event/View 缓存和工作区选择继续按本地 Server ID 隔离。

### 5.2 Server workspace

单 Server 工作区包含：

- Subject 列表和状态筛选；
- 全局 Runtime Queue；
- 创建 Subject 操作；
- 右上方顶栏的归档下载入口；
- 当前 Subject 详情面板。

Subject 列表使用服务端 cursor 分页，不在客户端推导 total。切换筛选条件时清除当前分页链和不再有效的选择。

### 5.3 Subject 详情

详情使用适合紧凑工作流的 tabs：

- 实体概览：按顺序展示实体信息、Workspace 状态与 Runtime 占用；
- 运行时：提交与终止操作、可折叠的 Runtime 历史，以及当前 Runtime 信息；
- Agent 消息：选择当前或历史 Runtime，左侧浏览 Agent 实例，右侧阅读该实例的有序事件消息；
- 事件记录：缓存优先、增量同步与向前加载历史；
- Norns 信息：Visual、Status、Trace、Cognition 和 Exploration 查询，默认始终开放，由服务端校验实际可用性。

关闭详情后 Subject 列表选择仍应稳定；Subject 从筛选结果消失时清除选择。

事件记录提供内容搜索，以及 Runtime ID、事件类型、来源智能体角色和来源智能体 ID 四项独立筛选，多个条件取交集。ID 支持不区分大小写的片段匹配；事件类型精确选择，角色既可选择主/子智能体或归属未知，也可选择具体角色名称。智能体 ID 使用与事件标签相同的实例关联，可根据同页晚到的实际 agentId 匹配此前输出，并保留调用/task ID 的查询能力；以事件来源/工具调用者为准，关联的 spawned_agent / target_agent 不作为该事件的来源。

筛选仅作用于当前最多 200 条事件的页面，显示当前页匹配数量及无匹配提示，不自动扫描全部历史。翻页保留条件；切换 Server/Subject 时重置；支持一键清空。跳转到同页关联的工具调用/结果时清空文本、类型和来源筛选，保留 Agent Only 开关，保证目标事件可见。

事件记录和 Agent 消息各自提供 `Agent Only` Switch，默认关闭；开启后使用同一白名单，仅显示 `agent.text`、`agent.tool_call`、`agent.tool_result`、`agent.interrupted`、`agent.result`。该开关仅控制展示，不丢弃缓存或参与 Agent 身份和状态的计算。事件记录中与其他筛选取交集，作用于当前事件页，翻页保留，清空筛选时关闭；Agent 消息先筛选再按 200 条分页，切换开关回到当前 Agent 的最新消息页，左侧事件数量同步显示筛选后的数量，Agent 顺序和生命周期仍由完整事件决定。

Agent 消息默认选择当前 Runtime，没有当前 Runtime 时选择最近的一条；选择后保持，直到用户切换。支持分页加载更早的 Runtime。左侧固定 Main Agent，其后先进行中、再已结束子智能体，各组按首次出现时间升序（相同时按事件 seq 排序）；角色名相同的实例通过明确 ID 分开。状态未知及归属未知排在最后，不根据名称合并实例。右侧仅渲染选定 Runtime、选定来源实例的 `agent.*` 事件，按 seq 升序复用 EventRow、原始 payload 展开和工具调用/结果跳转。关联子实例元数据不改变调用者的消息归属。

Agent 列表使用完整历史建立，不受事件页 200 条限制。首先读取本地所选 Runtime 的 Agent 事件，并提取同 Subject 历史的精简身份元数据与 runtime.session_resumed 关系，依次补齐 Subject 历史游标链并写入共享缓存；补齐期间明确提示列表与状态仍会更新。使用增量游标同步最新事件，活跃 Runtime 且窗口可见时每 30 秒检查，也支持手动刷新；Runtime 状态来自历史列表 API，Subject 活跃状态改变时刷新列表。切换 Runtime/Subject/Server 后忽略旧响应。游标失效时清理该 Subject 缓存并重试一次；历史加载失败保留已有结果并支持重试。消息 DOM 每页最多 200 条，切换 Agent 显示最新一页；用户在最新页底部时跟随新消息，翻旧页时保持位置。

恢复会话后，CC 可能在确认恢复之前发送旧任务的 task_notification，关闭强制终止时遗留的子任务。当前 Runtime 缺少该 task ID 的明确来源时，Well 沿唯一的 runtime.session_resumed.resume_session_id 链查找历史任务身份；支持连续恢复，拒绝按角色名、无关 Runtime 或相互矛盾的恢复关系猜测归属。当前 Runtime 的明确身份优先。历史上下文仅保存角色及实例/调用/任务/Agent ID 等元数据，按内容去重，不持有历史正文；消息列表仍只展示所选 Runtime 的事件，原始 payload 不改写。已明确关联 Agent 自身 task_id 的终态通知，即使当前 Runtime 没有对应 task_started，也将该实例标为已结束；关联 Bash 子任务的通知不结束其 Agent。

子智能体状态以明确执行结果判断：Agent 工具调用开始，同步结果 `subagent_status=completed/failed/stopped/cancelled` 结束；后台结果 `subagent_status=running` 仅表示启动，随后关联的 `local_agent` task 通知或 patch 终态才结束；明确恢复结果令同一实例重新进行中。`local_bash` task 的结束不改变所属 Agent 的状态。旧工具结果没有明确状态且无活动 Agent task 依据时显示状态未知。Main 的 `agent.result` 仅为一轮处理结果，不结束 Main/Runtime；Runtime API 返回终态时结束所有已知实例。

## 6. 修改操作

创建 Subject 表单包含可选名称和可选首条 instruction；创建首个 Runtime 时可附加 AGENT_* JSON 配置覆盖。请求发送期间禁用重复提交；响应不确定时不得自动重试，提示用户先刷新列表确认。

提交 instruction 适用于 idle、queued、running 或 waiting Subject，具体接受性由服务端决定。成功 `202` 后立即刷新 Subject、Runtime、Queue 和 Event，但不显示为“已处理”。

启动新 Runtime 可提交可选 `config` 与 `resume_session`；已存在活跃 Runtime 时仅追加 instruction，不显示或提交这两个参数。config 必须是 JSON 对象，仅允许 AGENT_* 字段，不允许 null 字段值；空文本省略覆盖。服务端完成字段、路径及组合规则校验。

Profile 属于 Runtime，Subject 与 Server 没有固定 Profile。历史 Runtime 展示自己的 Profile；排队期间允许 agent/Profile 为 null，不从当前服务模板推断。

停止操作必须明确选择：

- graceful：允许收尾；
- force：立即终止，使用更强确认文案。

请求接受后刷新相关 query；直到服务端状态变化前，界面显示正在停止，禁止重复发送。

## 7. 轮询模型

只轮询当前路由需要观察的数据：

- Servers overview：只有 `online` Server 每 10 秒刷新 service info 和 health；已进入
  `offline`、`unauthorized`、`incompatible` 或身份冲突的记录不自动重连；
- Server workspace：Subject 列表在存在 `using` 或 `queuing` 时每 10 秒刷新，
  全部空闲时每 30 秒刷新；Queue 只在 Queue 对话框打开时每 10 秒刷新；
- 选中 Subject：基础详情在选择、回到前台或相关修改后刷新；仅打开运行时面板时每
  10 秒查询当前 Runtime；
- Event：打开面板时先读本地缓存并与服务端同步；当前 Runtime 活跃且窗口可见时，
  每 30 秒先检查游标后是否有更新，有更新时再增量读取；
- Norns View：不自动轮询，由用户点击生成或重新生成，结果按 Server、Subject 和
  View 类型、输出格式和查询参数状态缓存。

窗口失焦、最小化或不可见时停止周期轮询。恢复焦点后立即刷新当前活跃的业务查询，
Servers overview 只重新探测上次成功的在线 Server，不唤醒已离线记录。mutation 成功
后立即刷新 Subject、Runtime、历史、Queue 和 Server 容量中受影响的部分，不等待下一周期。

同一资源同一时刻只允许一个请求；慢请求完成后若已过期，最多合并一次补偿刷新。切换 Server/Subject 时取消或忽略旧请求，禁止旧响应覆盖新选择。

## 8. 错误和内容边界

常见 API error code 映射为本地化文案；原始 `message` 仅用于诊断。网络、超时、认证、身份冲突、能力缺失和服务端错误使用不同 UI 状态，不归并为一个模糊错误。

JSON endpoint 必须验证 JSON Content-Type；Status/Trace 根据所选格式验证 Markdown、YAML 或 JSON MIME 和 UTF-8；Visual endpoint 必须验证 HTML。错误响应只读取受限文本，不能注入页面。

Visual View 使用 sandboxed iframe `srcdoc`，不授予 `allow-same-origin`、导航、下载、弹窗或外部网络权限。服务端内容不能访问 Well DOM、storage 或 Tauri bridge。

## 9. 本地化与平台

支持 macOS、Windows 和 Linux。界面提供中文和英文：locale 以 `zh` 开头时使用中文，其他使用英文。API 字段、错误码、枚举、ID、路径与原始 payload 不本地化；时间按本地时区展示，并可复制原始 UTC。

# 第二部分：实现设计

## 10. 技术选择

| 层 | 实现 |
| --- | --- |
| 桌面容器 | Tauri 2 |
| Rust | edition 2021，`reqwest` + Rustls |
| 前端 | React + TypeScript + Vite |
| 路由 | React Router Hash Router |
| 远端状态 | TanStack Query |
| Event 存储 | Dexie / IndexedDB |
| 本地化 | i18next + react-i18next |
| 图标 | Lucide React |
| 测试 | Vitest、Testing Library、Playwright、Cargo test |

依赖精确版本由 lockfile 固定。当前不引入大型 UI 组件库、CSS-in-JS runtime 或通用本地数据库。

## 11. 工程边界

```text
src/
  app/          路由、Provider、Error Boundary
  api/          bridge、DTO、query、mutation、pagination
  storage/      registry、session Token、Event DB、preferences
  polling/      focus 和 refresh 协调
  routes/       Servers 与 Server workspace
  components/   server、subject、runtime、events、norns
  i18n/         中英文资源
  styles/       tokens 与全局样式

src-tauri/src/
  api/          Rust client、commands、DTO、URL、error
  state.rs      共享 reqwest Client
```

页面组合、远端 query、本地持久化和展示组件分别拥有状态边界，禁止形成单个全局可变 State 或超大入口组件。

## 12. Rust 网络层

前端不能直接访问 nornsd，也不能获得任意 URL/method/header 的通用 Tauri command。Rust 层只暴露白名单 typed commands，并负责：

- 规范化 base URL，禁止 userinfo、fragment 和不支持的 scheme；
- 拼接固定 `/api/` 与 URL-encoded path segment，服务信息直接请求 `/api/`，保留末尾斜杠；
- 注入 Bearer header；
- 完整连接探测超时 3 秒，底层 TCP connect timeout 8 秒，普通请求 15 秒，
  Norns View 30 秒，归档独立使用 600 秒；
- 限制响应体大小；
- 校验 status 与 Content-Type；
- 解析 nornsd error envelope；
- 反序列化 typed DTO；
- 返回受限的视图正文与源文件名；
- 在 Rust 层流式写入归档，通过进度事件通知前端，支持取消。

AppState 保存可复用 `reqwest::Client` 和归档下载取消句柄。Token 作为 command 参数或 session 状态使用，不进入 URL、日志、error display 或 query key。

## 13. DTO 与兼容性

Rust DTO 与 TypeScript DTO 逐字段对应 nornsd API。时间先保持 RFC 3339 string，到展示边界解析；可能超过 JavaScript 安全整数的计数使用 string 或显式转换策略。

响应允许服务端增加未知字段，但稳定枚举出现未知值时以 `unknown` 展示并保留原值，不应导致整个页面崩溃。API schema 必须匹配。Norns 信息不按 capability 或 Profile 隐藏、禁用；工作区和当前服务可用性以具体接口返回为准。

根目录 `versions.json` 是 Well 唯一人工维护的版本来源。`well.version` 用于前端版本展示、Cargo 产品版本、Tauri 运行时和安装包版本；`well.support_api_schema` 编译进客户端，用于与 `/api/` 返回的整数 `api_schema` 严格匹配。产品版本不参与 API 兼容性判断，schema 也不参与 URL 拼接。字段缺失、格式错误或 schema 不匹配均视为协议不兼容。

Well 的 npm 开发、构建和 Tauri 入口自动执行轻量版本同步脚本，仅同步 Cargo manifest 与 lockfile 的派生产品版本。Tauri 不单独声明版本，继承 Cargo；直接运行 Cargo 前需执行 `npm run prepare:versions`，Rust 构建检查元数据与根文件一致。

契约 fixtures 来自 nornsd HTTP tests，固定 service info、Subject、Runtime、Queue、Event、错误和 Norns View 的共同形状。

## 14. Server Registry

registry 是 versioned `localStorage` document。Repository 层负责 schema 校验、URL 唯一性、时间更新和迁移。单条损坏记录不能使应用白屏；保留可解析记录并显示本地配置错误。

Token 分为 persisted 与 session map。UI 不直接从通用 registry object 读取 Token；网络 command 调用前由 credential resolver 明确取得。

修改 URL 或确认远端 identity 变化时，通过一个清理协调器同时完成 registry 更新、session Token、TanStack Query cache、IndexedDB cache、selection 与 preference 清理，避免半清理状态。

## 15. Event IndexedDB

Event 表复合键为 `[serverId+subjectId+seq]`，seq 使用补齐到 20 位的字符串排序键，支持完整 uint64 范围；提供按 Subject/seq 的索引。写入一页 Event 使用单事务 upsert。保留当前 Subject 的历史以供 Agent 列表建立，缓存总量超过 256 MiB 时按最近访问顺序清理其他 Subject 的完整缓存与游标，不单独截断当前 Subject。另按
Server/Subject 保存服务端返回的增量 `cursor` 和历史 `next_cursor` 链。

增量同步只能使用上次响应中的不透明 `cursor` 作为 `after`；历史加载使用响应中的
`next_cursor` 作为 `cursor`。`seq` 仅用于本地排序和去重，不能从最大/最小 seq 生成
服务端游标，也不能用本地记录数量推导服务端位置。相同 key 的新响应覆盖旧缓存，允许
服务端修正 readable projection。

## 16. React 状态归属

| 状态 | 所有者 |
| --- | --- |
| Server registry、preferences | storage repository + React context/query |
| session Token | 内存 credential store |
| 远端 service/Subject/Runtime/Queue/View | TanStack Query |
| 持久 Event | Dexie |
| 当前历史 Event buffer | Events panel local state |
| 当前 Server、Subject、tab | Hash Router URL |
| 表单草稿和 modal | 局部组件 state |

远端 server state 不复制进全局 React context。Query key 始终包含 local Server ID，Subject/Runtime query 再包含各自稳定 ID。

## 17. Query 与轮询

分页 helper 顺序消费 cursor 并去重稳定 ID，不自行生成 cursor。上一页由客户端保存已经取得的 cursor/data 实现。

全局 focus manager 结合 Tauri window focus、窗口可见性和浏览器 visibility。远端 query 由所属
路由、Server online 状态、身份有效性和当前面板共同决定是否启用；周期轮询再由 focus
manager 统一暂停。Server probe 在连接或超时失败时由 TanStack Query 最多重试两次，其他错误
不重试；只对成功 probe 建立 10 秒的后续轮询。

mutation 使用明确的 invalidation 集：

- create Subject：Server probe/capacity、subjects 和 queue；
- submit instruction：Server probe/capacity、subjects、Subject 详情、current Runtime、Runtime history
  和 queue；
- graceful/force stop：与 submit instruction 相同的 Runtime 相关集合。

同资源刷新通过 Query 层去重。Event 使用自己的本地缓存、不透明游标和单个防重入的
30 秒 interval；它不复制 Subject/Runtime query 的通用轮询。

## 18. 页面与组件

Servers overview 使用密集、可扫描的行式布局，不使用营销页或装饰性大卡片。状态以图标、文本和必要的色彩共同表达。

Server workspace 使用固定导航与主工作区；Subject 列表、Queue 和详情在桌面宽度下并排或通过明确 tabs 切换，在最小窗口宽度下折叠但不重叠。

命令按钮使用 Lucide 图标并提供 tooltip；二元设置使用 checkbox/toggle；状态筛选使用 segmented control 或 menu；危险 force stop 必须与普通 graceful stop 视觉区分并确认。

所有动态文本、路径、UUID 和错误必须允许换行或截断并可复制，不能撑破 toolbar、table 或 dialog。

## 19. Norns View 实现

Norns 信息合并 Visual、Status、Trace、Cognition 和 Exploration 标签。Status/Trace 默认请求 `format=md`，下拉框切换 md/yaml/json；Markdown 用 ReactMarkdown + GFM 渲染，YAML 解析后以缩进文本格式化，JSON 使用可折叠 tree。复制、缓存与导出保留完整原始正文，客户端不重新组织协议 Graph。格式切换或 Subject/Server 切换时，忽略旧请求结果。

`global_hints`、Exp.hints、sources、Task-init 和派生认知的组织均由 Norns CLI 决定。来源 ID 可以指向图外全局提示或 Exp.hints 内的 Cog；不重建 extends、单一根或 retry hints。Cog/Exp 详情输入 `cog_N` / `exp_N`，调用各自的详情接口，使用 JSON 预览并支持复制、保存。

Norns 信息入口始终开放。服务端依次检查 Subject `.norns/` 目录和 `.norns/norns.sqlite` 常规文件，再检查当前服务的兼容 Norns CLI：缺少工作区返回 `WORKSPACE_NOT_INITIALIZED`，工作区存在但当前服务不能提供 Norns 返回 `NORNS_UNAVAILABLE`。Well 分别显示“非 Norns 模式的工作区”和“当前服务无法提供 Norns”提示；不根据最新 Runtime 或配置槽位提前判断。

Trace 要求输入 Exploration ID。Visual HTML 只进入带严格 sandbox 的 iframe；切换 Subject、Server 或 identity 时销毁旧 iframe。HTML 内图布局、用户断言与派生认知标识及 retry 展示由 CLI 负责。

保存视图使用系统对话框。Visual 导出 HTML，Status/Trace 导出当前 md/yaml/json 原文，Cog/Exp 导出格式化 JSON。源 View 文件名保留并增加 `well-` 前缀；没有源文件名时生成回退名称。

### 19.1 归档下载

进入 Server 后，右上方顶栏提供归档入口，映射 `/api/archive` 的五种范围：

| 界面范围 | scope | subject_id | runtime_id |
| --- | --- | --- | --- |
| 整个 Root（包含 .nornsd/） | subject | 省略 | 省略 |
| Subject 工作目录 | subject | 必填 | 省略 |
| 全部运行数据 .nornsd/ | rundata | 省略 | 省略 |
| Subject 运行数据 | rundata | 必填 | 省略 |
| Runtime 运行数据 | rundata | 必填 | 必填 |

Subject 可从当前列表选择，Runtime 可从所选 Subject 的最近历史选择或输入 UUID。界面提示空闲限制，最终由服务端锁定和校验；所有范围共用服务端单任务额度，Well 本地也限制一次下载。不会自动重试。

Rust 弹出本地保存对话框后才发起请求，归档 HTTP 独立超时 600 秒。大文件按块写入目标同目录临时文件，不经过前端内存；完成 flush/sync 后原子发布。失败、超时或取消清理临时文件，保留原目标文件。前端显示接收字节数，服务端没有 Content-Length 时不显示虚假的百分比。

### 19.2 Runtime 诊断与事件

Runtime 历史展示自身 Profile、memory_samples_path，以及 usage、error、resources；usage 是服务端快照，不能累加为额外账单。输出和错误仅作为诊断，不从某条 `agent.result` 推导 Runtime 结束。事件记录和 Agent 消息复用同一个 EventRow 展示入口。instruction 的 accepted/replayed/settled 展示输入 ID、指令编号、结果 ID 与结算状态；API retry 展示次数、上限、等待时间、HTTP 状态和错误；task 生命周期展示任务 ID、类型（子智能体任务或 Bash 后台任务）、描述、摘要、输出文件及用量，task_updated 读取 patch 中的状态、结束时间和 is_backgrounded。status 翻译已知状态，null 仅表示状态标记清除，不据此推断 Runtime waiting；interrupted 区分一般中断与工具使用期间中断；user_message 作为会话输入或注入上下文展示，不能一概认作人类消息。当前 Runtime 状态始终来自 Runtime API。

agent.result 展示正文、错误列表、停止与本次处理结束原因、轮数和本事件用量；model_usage 明确标为进程累计用量快照，不累加到事件用量。事件记录和 Agent 消息不解析展示 total_cost_usd（外部模型价格可能不准确）及缓存写入 Token（cache_creation_input_tokens / cacheCreationInputTokens），保留输入、输出和缓存读取 Token 等其他用量；完整值仍可在原始 payload 中查看。agent.tool_result 独立展示 subagent_status，工具返回成功不等于后台子智能体已完成。runtime.error 与 diagnostic 提取错误码、说明及恢复情况。已知元数据使用语义字段展示，消息正文使用 Markdown；正文自身为完整 JSON 时格式化展示，混合文本保持原文。未知字段和完整事件 payload 始终可通过“数据”展开查看。

事件来源显示角色及实例 ID，优先采用 CC 实际 `agent_id`，返回前使用明确的 Agent 调用 ID / task ID；同名 worker 可据此区分。事件窗口通过 Runtime 内的显式实例/调用/任务/Agent ID 关联，把晚到的 `spawned_agent.agent_id` / `target_agent.agent_id` 用于同窗口此前输出的标签。不会按角色名合并实例；跨 Runtime 仅允许沿明确恢复会话关系为 task 事件恢复来源，不连接无关 Runtime。没有实例依据的子角色显示“实例未知”，未知后台任务显示归属未知。Agent 工具调用/结果同时显示调用者与关联子智能体，避免把子智能体的完成结果误当成父实例的身份。完整标识保留在标签、title 和原始 payload 中，不截断成可能碰撞的前缀。


## 20. 安全与隐私

- Token 不进入 URL、query key、日志、错误或 analytics；
- 没有通用 HTTP/Tauri command；
- 所有远端 HTML 视为不可信；
- 移除 Server 与身份重置删除其 Event/View cache；
- 应用数据可能包含 instruction、Agent 输出、工具结果和路径，按敏感本地数据处理；
- Well 不把服务端路径当成本机可访问路径，只提供展示和复制。

## 21. 验证基线

验证覆盖：

- Rust URL、认证、Content-Type、body limit、错误和 DTO；
- registry schema、URL/identity 唯一性与凭据生命周期；
- IndexedDB upsert、5,000 条窗口、增量和历史游标；
- Query key、cursor、focus、request race 与 mutation invalidation；
- 多 Server/Subject 选择稳定性；
- 创建、instruction、stop 与不确定响应；
- Event cache-first 和历史加载；
- Norns 入口常开、两类服务端错误、md/yaml/json 与 sandboxed Visual；
- Runtime config/resume、Runtime Profile 和 nullable agent/Profile；
- 五种归档范围、流式写入、失败/取消清理和原目标保护；
- 新版 Status/Trace 的 global_hints、Exp.hints、sources 及空值约定在展示、复制、缓存和保存中完整保留；
- Task-init 保留图内、用户认知不重复、包含历史 retry hints 的 Trace 能按原始结构展示；
- 中英文、窄窗口、键盘导航和无障碍名称；
- macOS、Windows、Linux 的 Tauri 构建与真实 nornsd smoke test。
