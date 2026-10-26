# Well 设计

状态：v1 当前设计  
支持服务：nornsd HTTP API v1

本文记录 Well 的长期产品设计与当前实现设计。服务端资源和接口契约以 [`nornsd.md`](nornsd.md) 的 HTTP API 章节为准。

# 第一部分：原始设计

## 1. 定位与边界

Well 是本地桌面 GUI，用于连接和管理多个 nornsd Server，浏览 Subject、Runtime、Queue、Event 与可选 Norns View。

v1 提供：

- 保存、编辑、连接和移除多个 Server；
- 汇总 Server 可达性、身份、容量、Profile 和能力；
- 浏览和筛选 Subject；
- 创建 Subject，可选同时提交首条 instruction；
- 查看当前 Runtime、历史 Runtime 和全局 Queue；
- 向 Subject 提交 instruction；
- 请求 graceful 或 force stop；
- 增量读取并缓存 readable Event；
- 查看 Norns Status、Trace 和 Visual View；
- 展示服务端返回的 Workspace 与 Runtime artifact 路径。

Well 不负责：

- 安装、启动、更新或配置 nornsd；
- 直接访问 Subject 文件系统、SQLite、raw log 或 transcript；
- 解释 Claude Code stream-json；
- 实现 API 不提供的重命名、删除、归档或迁移；
- 将 nornsd Subject 等同于 Norns Task；
- 提供账户、中心服务或云同步。

## 2. Server 身份

Server 是 Well 本地连接记录，不是 nornsd 资源：

| 字段 | 含义 |
| --- | --- |
| `id` | Well 生成的本地 UUID。 |
| `name` | 用户设置的非空显示名称。 |
| `base_url` | 规范化服务根 URL，不包含 `/api/v1`。 |
| `remote_server_id` | nornsd root 的稳定 UUID。 |
| `remember_password` | 是否持久保存 Token；字段名为 v1 存储兼容名称。 |
| `password` | 持久或 session-only Token；字段名为 v1 存储兼容名称。 |
| 时间字段 | 创建、更新与最近成功连接时间。 |

规范化 `base_url` 在本地 registry 中唯一。修改 URL 时保留本地记录 ID，但清除其远端 identity、query cache、Event/View cache 和当前选择；只修改名称或 Token 不清理业务缓存。

首次成功连接后绑定 `remote_server_id`。同一 URL 后续返回不同 ID 时进入 `identity_changed`，禁止继续远端修改，直到用户确认并清空本地关联数据。两条连接记录返回同一远端 ID 时，后连接记录进入 `duplicate_identity`，避免通过不同 URL 重复操作同一个 root。

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
| `duplicate_identity` | 另一条记录已绑定同一 root。 |

连接状态是客户端观察，不写回服务端。短暂失败不能删除 Server 或缓存。进入
Servers 页面时，每条具有 Token 的记录立即开始一轮连接探测；单次探测最长
3 秒，只有连接或超时错误才重试，最多共尝试 3 次，重试间隔为 1 秒。三次均失败后
进入 `offline`，不再后台自动重连；重新启动 Well、从工作区返回 Servers 页面、手动刷新
或修改连接配置会开始新的探测。`401` 直接进入 `unauthorized`，显示 Token 不匹配，
不参与自动重试。

## 4. 本地凭据与数据

新增 Server 默认勾选“记住 Token”。勾选时 Token 进入本地 registry；取消后立即删除持久值，仅保留当前 session 输入。主动断开只清 session Token，移除 Server 则删除其全部凭据和缓存。

Well v1 没有用于加密该存储的主密码。记住的 Token 由 WebView 本地存储保护，其安全级别取决于操作系统账户和应用数据目录；README 必须明确这一点。

本地存储分层：

- `localStorage`：带 schema version 的 Server registry 和界面偏好；
- session memory：不记住的 Token、当前选择和临时加载的旧 Event；
- IndexedDB：可增长 Event cache。

Event cache 以 `(server_id, subject_id, seq)` 唯一定位。每个 Subject 跨重启保留最近 5,000 条；用户本次显式加载的更早历史保留在面板 session buffer 中，切换 Subject 或销毁面板时可以释放。

## 5. 信息架构

应用启动直接进入 Servers 页面，不需要登录页。

### 5.1 Servers overview

以可扫描列表展示所有本地 Server 的名称、URL、连接状态、远端 identity、Profile、容量与 Queue。支持新增、编辑、连接、断开和移除。

多个 Server 的检查独立并发；单个失败不得阻塞其他 Server。identity_changed 和 duplicate_identity 必须显式阻止进入工作区与修改操作。

### 5.2 Server workspace

单 Server 工作区包含：

- Subject 列表和状态筛选；
- 全局 Runtime Queue；
- 创建 Subject 操作；
- 当前 Subject 详情面板。

Subject 列表使用服务端 cursor 分页，不在客户端推导 total。切换筛选条件时清除当前分页链和不再有效的选择。

### 5.3 Subject 详情

详情使用适合紧凑工作流的 tabs：

- 实体概览：按顺序展示实体信息、Workspace 状态与 Runtime 占用；
- 运行时：提交与终止操作、可折叠的 Runtime 历史，以及当前 Runtime 信息；
- 事件记录：缓存优先、增量同步与向前加载历史；
- Norns 视图：Visual、全局 Status 与 Trace，仅在 capability 可用时显示。

关闭详情后 Subject 列表选择仍应稳定；Subject 从筛选结果消失时清除选择。

## 6. 修改操作

创建 Subject 表单包含可选名称和可选首条 instruction。请求发送期间禁用重复提交；响应不确定时不得自动重试，提示用户先刷新列表确认。

提交 instruction 适用于 idle、queued、running 或 waiting Subject，具体接受性由服务端决定。成功 `202` 后立即刷新 Subject、Runtime、Queue 和 Event，但不显示为“已处理”。

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
  View 类型缓存。

窗口失焦、最小化或不可见时停止周期轮询。恢复焦点后立即刷新当前活跃的业务查询，
Servers overview 只重新探测上次成功的在线 Server，不唤醒已离线记录。mutation 成功
后立即刷新 Subject、Runtime、历史、Queue 和 Server 容量中受影响的部分，不等待下一周期。

同一资源同一时刻只允许一个请求；慢请求完成后若已过期，最多合并一次补偿刷新。切换 Server/Subject 时取消或忽略旧请求，禁止旧响应覆盖新选择。

## 8. 错误和内容边界

常见 API error code 映射为本地化文案；原始 `message` 仅用于诊断。网络、超时、认证、身份冲突、能力缺失和服务端错误使用不同 UI 状态，不归并为一个模糊错误。

JSON endpoint 必须验证 JSON Content-Type；Visual endpoint 必须验证 HTML。错误响应只读取受限文本，不能注入页面。

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

依赖精确版本由 lockfile 固定。v1 不引入大型 UI 组件库、CSS-in-JS runtime 或通用本地数据库。

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
- 拼接固定 `/api/v1` 与 URL-encoded path segment；
- 注入 Bearer header；
- 完整连接探测超时 3 秒，底层 TCP connect timeout 8 秒，普通请求 15 秒，
  Visual 30 秒；
- 限制响应体大小；
- 校验 status 与 Content-Type；
- 解析 nornsd error envelope；
- 反序列化 typed DTO；
- 只把受限 Visual HTML 字符串返回前端。

AppState 只保存可复用 `reqwest::Client`。Token 作为 command 参数或 session 状态使用，不进入 URL、日志、error display 或 query key。

## 13. DTO 与兼容性

Rust DTO 与 TypeScript DTO 逐字段对应 nornsd API。时间先保持 RFC 3339 string，到展示边界解析；可能超过 JavaScript 安全整数的计数使用 string 或显式转换策略。

响应允许服务端增加未知字段，但稳定枚举出现未知值时以 `unknown` 展示并保留原值，不应导致整个页面崩溃。修改操作只在明确 capability 和 API version 支持时启用。

契约 fixtures 来自 nornsd HTTP tests，固定 service info、Subject、Runtime、Queue、Event、错误和 Norns View 的共同形状。

## 14. Server Registry

registry 是 versioned `localStorage` document。Repository 层负责 schema 校验、URL 唯一性、时间更新和迁移。单条损坏记录不能使应用白屏；保留可解析记录并显示本地配置错误。

Token 分为 persisted 与 session map。UI 不直接从通用 registry object 读取 Token；网络 command 调用前由 credential resolver 明确取得。

修改 URL 或确认远端 identity 变化时，通过一个清理协调器同时完成 registry 更新、session Token、TanStack Query cache、IndexedDB cache、selection 与 preference 清理，避免半清理状态。

## 15. Event IndexedDB

Event 表复合键为 `[serverId+subjectId+seq]`，并提供按 Subject/seq 的索引。写入一页 Event 使用单事务 upsert，随后按 seq 删除超出 5,000 条的最旧持久记录。另按
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

Status/Trace 使用可折叠 JSON tree，同时保留复制原始 JSON 的入口，不在客户端重新解释协议 Graph。

Trace 要求用户输入或选择 Exploration ID。Visual HTML 只进入带严格 sandbox 的 iframe；切换 Subject、Server 或 identity 时销毁旧 iframe，避免保留跨域内容。

生成 View 后提供“保存视图”。Visual 按 HTML 保存，Status/Trace 按格式化 JSON 保存。nornsd 提供源 View 文件名时，Well 保留其 View 类型、时间戳和随机后缀，仅增加 `well-` 前缀；旧服务端未提供文件名时，Well 使用相同时间戳形状生成回退名称。文件只能写入用户通过系统保存对话框选择的位置。

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
- Norns capability、JSON 与 sandboxed Visual；
- 中英文、窄窗口、键盘导航和无障碍名称；
- macOS、Windows、Linux 的 Tauri 构建与真实 nornsd smoke test。
