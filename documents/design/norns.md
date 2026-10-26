# Norns CLI 设计

状态：v1 当前设计  
适用协议：Norns protocol v2  
数据库 schema：v9

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

| 版本 | 当前值 | 含义 |
| --- | --- | --- |
| 协议版本 | `2` | 对外可观察的 Norns 语义。 |
| SQLite schema | `9` | 本实现的物理存储格式。 |
| 产品版本 | 构建注入 | `norns` 可执行文件版本。 |

CLI 只接受协议 v2、schema v9。版本检查必须发生在任何持久化修改之前；`workspace reset` 也不能绕过版本检查。

## 3. Workspace 行为

只有 `<workspace>/.norns/norns.sqlite` 存在时才视为已初始化。仅存在 `.norns/`、`history/` 或锁文件不代表初始化完成。

`workspace init` 创建空 Workspace。重复执行时必须先校验 application ID、协议版本、schema 版本和关键状态，只有合法 Workspace 才幂等成功。

未初始化时，只有 init 和 `task open` 可以执行。首个 `task open` 可以自动初始化，但参数校验、数据库创建、首个 Task、首个 Task-init Cognition 与 Runtime 占用必须共同成功或共同回滚。

`workspace reset` 用于结束当前 Subject：

- 只允许在没有 open Task 且状态可信时执行；
- 将 `.norns/` 下除 `history/` 外的内容归档到唯一的 `.norns/history/<timestamp>/`；
- 创建全新的空 Workspace，不继承旧 Task 或 Graph；
- 使用恢复日志保证目录移动中断后可以确定性回滚或前滚；
- 不兼容版本或无法确认 Task 状态时拒绝执行且不移动内容。

## 4. CLI 操作面

公开命令分为：

```text
norns version [--json]
norns workspace init|reset
norns task open|close|reopen
norns cognition add
norns exploration add|retry|assign|finish|todo|running
norns view status|trace|visual
norns <external-extension> ...
```

命令参数和示例见 [`../../norns/README.md`](../../norns/README.md)。核心行为遵循以下约束：

- 首个 Task 必须提供 Subject 信息；后续 Task 可以复用最新 closed Task 的 origin，也可以扩展 Subject 信息；
- `task close` 原子取消剩余 todo/running Exploration，并清除 Runtime 占用；
- `task reopen` 保留已有 Graph，不自动重试历史 Exploration；
- Exploration 默认 source 是当前 Task 的 Task-init Cognition；显式 sources 有序且不得重复；
- assign 必须同时提供非空 executor 与 output reference；
- todo 只能直接 finish 为 cancelled；
- retry 从任意历史 Task 的 finished Exploration 创建当前 Task 的新 todo；
- todo/running 查询只面向当前 open Task，没有 open Task 时返回 `NO_OPEN_TASK`。

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

`max_concurrency` 是软限制。合法修改提交后若仍超额，返回 `CONCURRENCY_LIMIT_EXCEEDED` warning，并包含 `operation_applied=true`、实际 running 数和额度。

人类提示支持中文和英文，按主机 locale 选择；机器字段、枚举、ID、时间、warning code 和 error code 不本地化。

## 6. View 绑定

Status 与 Trace 默认输出 UTF-8 YAML，并提供等价 JSON。输出同时原子发布到 `.norns/view/`。字段、投影、顺序和最小子图规则以正式协议为准。

Visual View 是自包含 HTML：

- 展示全部 Task、完整 Graph、状态、Purpose 相关节点和 retry 连线；
- 不依赖网络资源；
- 用户 Markdown 使用 GFM 渲染，禁止原始 HTML 并经过白名单清洗；
- 未经清洗的用户内容只通过 `textContent` 注入；
- `--lang zh|en` 显式选择 HTML 语言，省略时按操作系统 locale 选择；
- 默认返回生成文件路径，`--open` 才请求系统浏览器打开。

## 7. 外部扩展

未知核心命令按 `norns-<name>` 外部可执行文件发现。顺序为：

1. `norns` 可执行文件同目录；
2. `NORNS_PLUGIN_PATH` 中的目录；
3. 当前 `PATH`。

扩展获得原始 argv、标准输入输出，以及：

- `NORNS_EXECUTABLE`；
- `NORNS_WORKSPACE`；
- `NORNS_PROTOCOL_VERSION`；
- `NORNS_SCHEMA_VERSION`。

核心命令不能被扩展覆盖。扩展若需要修改协议状态，必须调用 `norns`，禁止直接写 SQLite。

仓库提供 `norns-websearch` 参考扩展。它与 Workspace 解耦，未配置代理时使用百度，显式配置 HTTP/SOCKS5/SOCKS5H 代理时使用 Google，并输出稳定的 `title`、`href`、`body` JSON 数组。

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

cmd/norns-websearch -> internal/websearch
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
- `websearch`：独立搜索实现，不依赖 Workspace。

时间、ID 和关键文件操作通过小接口注入，使事务时间、UUID 和 reset 故障点可以确定性测试。

## 10. SQLite v9

数据库标记：

```text
PRAGMA application_id = 0x4E4F524E  # NORN
PRAGMA user_version = 9
workspace_meta.protocol_version = 2
```

物理表：

| 表 | 用途 |
| --- | --- |
| `workspace_meta` | 协议版本、Runtime 占用、创建时间。 |
| `tasks` | Task 固有字段与关闭状态。 |
| `cognitions` | Cognition、来源的类型化物理列与相关性。 |
| `explorations` | Exploration 定义、执行状态、retry 和结果。 |
| `exploration_sources` | 有序 `source_cognitions` 的物理表示。 |

不创建 relations、outputs、assignments 或 global Cognition 镜像表。Graph 关系始终从实体字段派生。

数据库层通过 CHECK、FOREIGN KEY、唯一索引和应用层完整性检查共同保证：

- 最多一个 open Task；
- 状态与可空字段组合合法；
- Task-init、全局 Cognition、hint 与 Explorer Cognition 的来源组合正确；
- retry 来源已 finished 且无环；
- Runtime 占用与 Task 状态一致；
- Graph 引用类型、方向和 DAG 不变量成立。

## 11. 连接、事务与锁

数据库先以只读连接检查 application ID、schema 和协议版本，再按操作需要建立读写连接，避免不兼容文件因连接 PRAGMA 或初始化行为被修改。

连接启用 foreign keys、busy timeout 和 WAL。写操作使用 `BEGIN IMMEDIATE` 或等价的单 writer 事务，并通过条件更新防止两个调用方从同一旧状态成功转换。

锁分为两层：

- SQLite 事务处理数据库 reader/writer 并发；
- `.norns.lock` 处理 `.norns/` 的生命周期和目录替换。

普通读写和 View 使用生命周期共享锁；init、reset 和 reset recovery 使用独占锁。文件锁不能代替 SQLite 事务。

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
- retry Exploration、复制的 sources 与 `retry_of`；
- Task close、剩余 Exploration 取消与占用清除；
- Task reopen 与占用设置。

批量 close 使用同一个时间写入 Task `closed_at` 和被取消 Exploration 的 `finished_at`。自动生成的取消原因是可读文本，机器判断仍依据 outcome。

## 14. View 实现

View builder 从 store 的只读 snapshot 加载 typed model，在应用层执行协议投影，不在 renderer 中通过临时 SQL 拼接协议语义。

Status 构造完整当前 Graph 与 Task 上下文；Trace 通过反向遍历派生关系构造最小祖先子图，并额外合并 Purpose 相关 Cognition、全局 Cognition 和完整 retry 链；Visual 使用完整持久状态生成页面。

Status/Trace 的 YAML 和 JSON 使用同一中间模型，字段顺序由明确的 YAML node/结构定义固定。Golden files 固定两种编码的公开形状。

Status/Trace 无论大小都原子发布到 `.norns/view/`。编码结果不超过 6 KiB 时，CLI 直接输出完整 View；超过 6 KiB 时只输出包含文件路径、格式和字节数的 JSON 成功信封。Visual 始终通过成功信封返回生成的 HTML 路径。View 文件统一使用 `<type>-<UTC timestamp>-<token>.<ext>` 命名，其中 type 为 `status`、`trace` 或 `visual`。

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
- Status/Trace golden files 和 Visual 内容安全；
- CLI argv、stdout/stderr、exit code 与本地化；
- 外部扩展发现、环境传递和退出码；
- Websearch 后端解析、代理和结果规范化；
- 支持平台上的 race、vet 与真实二进制集成。

`testdata/requirements.json` 是行为覆盖 key 到测试函数的机器清单；CI 校验 key 唯一、格式正确且引用的测试存在。它不再反向解析设计文档的段落编号。
