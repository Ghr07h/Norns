# Norns CLI

`norns` 是 Norns protocol 的本地 Workspace CLI。它使用 SQLite 持久化 Task、Cognition、Exploration 及其关系，执行协议状态转换，并生成 Status、Trace 和 Visual 三类 View。

当前版本边界：

- 支持的 protocol version：根目录 `versions.json` 的 `norns.support_protocol`
- database schema version：根目录 `versions.json` 的 `norns.database_schema`
- Go：`1.25`
- SQLite 驱动：纯 Go，无 CGO 依赖

CLI 只管理协议状态，不启动、监控或停止智能体。Runtime 必须在关闭 Task 前停止并确认相关 Explorer 不再写入。

协议规范版本由 `versions.json` 的 `protocol` 声明，产品版本由 `norns.version` 声明。已安装产物的实际产品、协议和数据库版本使用 `norns version --json` 查看。

## 构建

从完整仓库构建：产品版本、支持的协议版本和数据库 schema 来自根目录 `versions.json`，无需额外的运行时版本文件。推荐从项目根目录执行 `./release.sh norns`，它会先刷新版本代码，再构建当前平台的 CLI，默认不编译命令扩展。需要扩展时使用 `--with-ext <名称>` 显式选择，可重复传入以选择多个扩展；编写和构建方式见下方“外部扩展”。六平台产物增加 `--all-platforms`。

GitHub Release 打包同样支持 `./release.sh --gitrelease --with-ext <名称>`，可重复选择多个扩展。对应平台的 Norns ZIP 会包含所选扩展二进制、合并元数据和声明的发布附件；不传时不包含扩展。

也可以在本目录执行 `go run ./scripts/release.go`，独立完成版本生成、CLI 编译、产物整理和校验和生成，不依赖 nornsd 或 Well 的构建实现。同样支持可重复的 `--with-ext <名称>`，以及 `--all-platforms`、可重复的 `--target <os>/<arch>` 与 `--output <directory>`，默认产物写入 `../release/`。每次发布按本次选择替换当前平台的 norns 产物，移除旧的未选中 `norns-*` 扩展；构建脚本测试随 `go test ./...` 运行。

下列命令可用于模块内开发构建；版本使用已提交的 `internal/version/versions_generated.go` 派生代码。修改根版本清单后应先运行统一或本目录构建入口，直接 `go build` 不会自动刷新版本。

~~~sh
cd norns
go build -o ../bin/norns ./cmd/norns
../bin/norns version
../bin/norns extension list
~~~

将主程序安装到 Go bin 目录：

~~~sh
go install ./cmd/norns
norns version --json
~~~

交叉构建示例：

~~~sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../bin/norns-linux-amd64 ./cmd/norns
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ../bin/norns-windows-amd64.exe ./cmd/norns
~~~

支持的发布目标为 macOS amd64/arm64、Linux amd64/arm64 和 Windows amd64/arm64。

## 快速开始

在需要管理的 Workspace 根目录执行：

~~~sh
# 可选；首个 task open 也会自动初始化
norns workspace init

# 空 Workspace 的首个 Task 必须提供 Subject 信息
norns task open \
  --purpose "确认项目是否适合发布 v1" \
  --concurrency 3 \
  --info "这是 Norns 项目及其当前代码库"

# 创建待探索事项；未指定 source 时使用当前 Task 的 Task-init Cognition
norns exploration add \
  --exploration-title "验证交叉平台锁" \
  --exploration-content "确认目标平台上的 SQLite 和生命周期锁行为"

norns exploration todo

# Runtime 获得实际执行入口和输出引用后登记 assign
norns exploration assign \
  --exploration exp_123e4567-e89b-42d3-a456-426614174001 \
  --explorer "claude-session-7" \
  --output "claude://sessions/7/output"

# Explorer 写入可复用的发现
norns cognition add \
  --cognition-title "SQLite 结论" \
  --cognition-content "WAL 允许并发 reader，writer 通过事务串行化" \
  --origin exploration \
  --exploration exp_123e4567-e89b-42d3-a456-426614174001 \
  --task-purpose-relevant

norns exploration finish \
  --exploration exp_123e4567-e89b-42d3-a456-426614174001 \
  --outcome succeeded \
  --outcome-reason "目标平台行为已经验证"

norns view status
norns view visual --lang zh --open

# Runtime 停止全部 Explorer 后关闭 Task
norns task close \
  --outcome succeeded \
  --outcome-reason "已有证据足以支持发布结论"
~~~

ID 由 CLI 生成，格式分别为 `task_<uuid-v4>`、`cog_<positive-integer>` 和 `exp_<positive-integer>`。示例中的 ID 需要替换为命令实际返回的 ID。

## Workspace

只有 `<workspace>/.norns/norns.sqlite` 存在时才视为已初始化。仅存在 `.norns/` 或 `.norns/history/` 不代表初始化完成。

~~~text
<workspace>/
  .norns.lock                 操作期间的生命周期共享/独占锁，空闲时清理
  .norns/
    norns.sqlite              带协议和 schema 版本检查的数据库
    view/                     原子发布的 Status、Trace 和 Visual View
    history/                  workspace reset 归档
  .norns-reset.json           仅在 reset 进行或恢复期间存在
~~~

不要直接修改 SQLite。所有 Graph 写入必须通过 `norns` 命令完成。

### 初始化和重置

~~~sh
norns workspace init
norns workspace check
norns workspace free
norns workspace reset
~~~

`workspace init` 可以重复调用，但会先校验协议版本、schema 版本和 Workspace 状态。

`workspace check` 校验当前目录的 Workspace，返回 JSON 成功信封，data 包含 `valid: true`、`state`、`runtime_occupied` 和 `summary`：

- `state: open_task`：返回当前 `task`，包含 ID、purpose、初始认知引用、额定并发数、状态和创建时间。
- `state: no_open_task`：返回按创建时间升序排列的全部 `tasks`，包含 Task 基本字段及关闭结果、理由和时间。
- `state: empty`：返回 `tasks: []`，提示当前为空 Workspace。

`runtime_occupied` 为布尔值，true 表示协议记录的 Runtime 占用，false 表示未占用，始终显式输出；它不证明实际 Runtime 进程或 Subagent 存活。`summary` 包含整个 Workspace 的 `task_count`、`cognition_count` 和 `exploration_count`。任务信息、占用状态与数量来自同一次读取快照。该命令不生成 View、不自动初始化工作区；未初始化、版本不兼容或状态损坏时返回结构化错误和非零退出码。它只检查当前目录，不向上查找父目录。

`workspace free` 解除 Runtime 占用：将当前 open Task 的 todo/running Exploration 以 `cancelled` 结束，将 open Task 以 `cancelled` 关闭，并清除占用标记。取消理由和结束时间自动记录；已经完成的结果、认知、来源关系及编号计数保留，不归档或重置工作区。无 open Task 时幂等成功。JSON 成功信封的 data 包含 `changed`、`runtime_occupied: false`、非空的 `closed_task_id`，以及 `cancelled_todo`、`cancelled_running` 数量。命令仍校验版本和状态完整性，调用前须停止或隔离相关实际 Agent；CLI 不负责停止进程。

`workspace reset` 仅在不存在 open Task、版本兼容且状态可信时执行。它把旧数据库、View 和其他 Workspace 资源归档到 `.norns/history/<timestamp>/`，然后创建空 Workspace。reset 后的首个 Task 必须重新提供 `--info`。

## Task

~~~sh
norns task list [--status open|closed]
norns task open --purpose TEXT --concurrency N [--info TEXT]
norns task close --outcome OUTCOME --outcome-reason TEXT
norns task reopen [--task-id TASK_ID]
~~~

`task list` 是只读查询，默认返回当前 Workspace 的全部 Task；`--status open` / `--status closed` 只返回对应状态。JSON 成功信封的 `data.tasks` 始终是数组，无匹配项时为 `[]`。每项包含 `id`、`purpose`、`init_cognition_ref`、`max_concurrency`、`status`、`created_at`，Closed Task 还包含 `outcome`、`outcome_reason`、`closed_at`。按创建时间升序排列，同一时间按 ID 升序。命令保留工作区版本和完整性校验，不加载 Cog/Exp 实体、不构造 Graph、不生成 View 文件，也不适用 View 的 6 KiB 文件引用阈值。尚未初始化时返回 `WORKSPACE_NOT_INITIALIZED`；非法状态参数返回 `INVALID_ARGUMENT`。

Task outcome：

- `succeeded`
- `unachievable`
- `failed`
- `cancelled`

Workspace 最多存在一个 open Task。首个 Task 必须提供 `--info`；后续 Task 可以省略它并复用最近关闭 Task 的 Subject Cognition，也可以提供新信息并创建独立的 Task-init Cognition。新建初始认知的 `origin_ref` 和 `task_id` 均为创建它的 Task ID；复用不改变旧认知，不建立 Cog 之间的来源链。

`task close` 在同一事务中取消当前 Task 剩余的 todo/running Exploration、关闭 Task，并清除 Runtime 占用。`task reopen` 保留已有 Graph 和 Exploration，不会自动恢复或 retry 历史执行。

Cog 和 Exp 分别使用 `cog_1`、`cog_2`… 和 `exp_1`、`exp_2`…，在单个 Workspace 内各自递增、不补零；所有认知来源共用 Cog 编号，新建与 retry 共用 Exp 编号。跨 Task 及进程重启不重置，通过数据库事务保证并发唯一和失败回滚。reset 创建新 Workspace 后两类编号重新从 1 开始。Task ID 仍为 `task_<uuid-v4>`。编号仅在所属 Workspace 内有效；同创建时间下按数字编号排序。

## Cognition

~~~sh
norns cognition add \
  --cognition-title TEXT \
  --cognition-content TEXT \
  --origin user|exploration \
  [--exploration EXP_ID] \
  [--task-purpose-relevant[=BOOL]] \
  [--meta JSON]

norns cognition get --cognition COG_ID
~~~

支持三种调用场景：

- 全局用户断言认知：`--origin user`，不指定 Exploration；要求存在 open Task，`origin_ref` 为该 Task ID，`task_id` 为空。
- Exploration 用户断言认知：`--origin user --exploration EXP_ID`；目标可以来自任意 Task、处于任意状态。认知保存和目标 `hints` ID 追加共同提交，`task_id` 记录添加时的 open Task。
- Explorer 派生认知：`--origin exploration --exploration EXP_ID`；目标必须是当前 open Task 的 todo 或 running Exploration，`task_id` 为目标所属 Task。

Task 初始信息也属于断言认知。所有认知统一使用工作区内递增的 `cog_1`、`cog_2` 等 ID；断言的录入不表示内容已经验证，派生认知也应保留证据、适用范围和不确定性。

`cognition add` 可选传入 `--meta`，用于保存 Domain 约定的类别、结论等元信息。参数必须是 JSON 对象，且每个值都是字符串，例如 `--meta '{"type":"theorem","scope":"geometry"}'`。省略时保存空对象 `{}`；非法 JSON、null、数组、数字、布尔值或嵌套值会被拒绝。meta 与认知一同创建且不可修改，不改变认知来源或图关系。

~~~sh
norns cognition add \
  --origin user \
  --cognition-title '勾股定理' \
  --cognition-content '在任何一个直角三角形中，两条直角边的平方和等于斜边的平方。' \
  --meta '{"type":"theorem","scope":"geometry"}'
~~~

Status / Trace 不输出 meta；非空 meta 在 Visual 的 Cog 节点详情中以格式化 JSON 代码块显示。`cognition get` 返回完整认知实体，包括非空 meta、title、content、origin、origin_ref、task_purpose_relevant 和 created_at；为空的 task_id、meta 省略。可查询任意来源及历史 Task 的认知，不要求存在 open Task。

全局 Cognition 不能设置 `--task-purpose-relevant`。Cognition 创建后不可修改或失效；新的 Cognition 可以补充或修正旧认知，但不会删除旧节点。

## Exploration

~~~sh
norns exploration add \
  --exploration-title TEXT \
  --exploration-content TEXT \
  [--source-cognitions COG_ID]...

norns exploration get --exploration EXP_ID
norns exploration retry --exploration EXP_ID
norns exploration assign --exploration EXP_ID --explorer EXECUTOR_ID --output OUTPUT_REF
norns exploration finish --exploration EXP_ID --outcome OUTCOME --outcome-reason TEXT
norns exploration todo
norns exploration running
~~~

`exploration running` 查询当前 open Task 中正在执行的探索，包含 `explorer_id`、`assigned_at` 和 `duration_seconds`，不输出 `output_ref`；需要实际执行输出引用时使用 `exploration get`。

`exploration get` 返回完整探索实体，包含定义、task_id、状态、创建时间，以及已有的 retry、执行和结束字段。sources / hints 为有序 Cog ID 列表，sources 保持非空，空 hints 为 []，可使用 `cognition get` 读取对应认知；为空的可选字段省略。可查询任意状态及历史 Task 的探索，不要求存在 open Task。两个 get 命令均直接返回 JSON 成功信封，不应用 View 的长度阈值，也不生成 View 文件。

Exploration 状态流转：

~~~text
add ──> todo ── assign ──> running ── finish ──> finished
          │
          └── finish(outcome=cancelled) ─────────> finished

finished ── retry ──> 新 ID 的 todo Exploration
~~~

running Exploration outcome：

- `succeeded`
- `inconclusive`
- `failed`
- `cancelled`

todo 只能直接 finish 为 `cancelled`。assign 时，如果 `output_ref` 指向可解析的符号链接，Norns 会记录链接最终目标的绝对路径；普通文件、非路径引用、已断开的链接或解析失败的路径仍按传入值记录。assign 后的 `explorer_id` 和 `output_ref` 不可修改；CLI 不提供 reassign 或 executor reconnect。retry 可以使用任意历史 Task 的 finished Exploration，并为当前 open Task 创建新执行记录。

来源参数保留 `--source-cognitions`，成功结果和查询中的字段使用 `sources`。来源可以引用任意已存在的 Cog，包括全局用户认知和其他探索的用户提示，并保留输入顺序。省略参数时使用当前 Task 的初始认知。

`todo` 和 `running` 只返回当前 open Task 的调度列表；没有 open Task 时返回 `NO_OPEN_TASK`。查询中的 `hints` 是用户提示 Cog ID 数组，空时返回 `[]`。retry 复制定义与 `sources`，新 `hints` 为空；旧提示通过 Trace 的 retry 链读取，不复制到新探索。

## View

~~~sh
norns view status [--task-id TASK_ID] [--json | --md]
norns view trace --exploration EXP_ID [--json | --md]
norns view visual [--lang zh|en] [--open]
~~~

`--md` 输出 Markdown，和 `--json` 互斥。一级标题为 `# Status View` / `# Trace View`，二级标题首字母大写。Task 先用引用段落显示 ID，再显示 **Purpose** 和引用正文。

第二个二级章节固定为 `## Graph`，通过 `### Cognitions` / `### Explorations` 下的文本代码块提供紧凑关系索引，例如：

~~~text
cog_4: Finding <- exp_1
exp_1: (running) Inspect <- ['cog_1', 'cog_2']
~~~

Task 初始认知只显示 ID 和标题，不追加 Task 来源箭头；摘要不包含正文和时间。节点范围与 YAML / JSON 的逻辑图一致，用户提示仍在后面的对应位置展开，Trace 不收录额外节点。后面的详细列表继续保留。

`## Status` / `## Trace` 按创建顺序以 `### exp id - title` 展开探索，每个探索先显示元数据无序列表，再显示 **Content** 和引用正文。其下先按 `#### cog id - title` 显示已收录的派生认知，再以同样形式显示用户提示；每条认知同样先元数据、后正文，Origin 标识来源。Task 初始认知在该章节开头的 `### Task-Init Cognitions` 中以四级标题保留。全局用户认知仍在 `## Global_hints` 独立展示。

Todo_explorations、Running_explorations、Recent_top10_derived_cognitions 均使用表格；近期认知表为 id、title、created_at 三列，保留原最近 30 分钟内最多 10 条的倒序。空 todo / running 保留空表，空近期摘要仍省略。Markdown 从同一快照补充标题、来源类别和探索创建时间，节点范围与 YAML / JSON 一致；YAML / JSON 结构不变，meta 不输出。

~~~sh
norns view status --md
norns view trace --exploration EXP_ID --md
~~~

### Status

展示当前完整 Graph，并以 open Task 或显式 `--task-id` 作为 Task 上下文。没有 open Task 且未指定 Task 时，顶层返回全部历史 Tasks。

Task 初始认知和派生认知保留在 `graph.cognitions`；全局用户认知位于顶层 `global_hints`，用户探索认知只在所属 Exploration 的 `hints` 中展开。两种用户认知列表项均只有 `id`、`content`、`created_at`。空 `global_hints` 返回 `[]`，空 Exploration `hints` 省略。`sources` 仍引用原 Cog ID，不因认知投影位置改变而替换 ID 或复制图内节点。

Status / Trace 的 YAML 中，`sources` 使用单行数组形式，例如 `sources: [cog_1, cog_2]`。

Status 在 `generated_at` 前提供可选的 `recent_top10_derived_cognitions: [cog_3, cog_2, cog_1]`，仅列出整个 Workspace 在生成时间前 30 分钟内新增的最多 10 条派生认知 ID，时间倒序、同时间按 ID 升序。时间窗口包含两端，不含未来时间；不足 10 条时按实际条数输出，没有符合条件的记录时省略字段。Task 选择不改变此列表，正文仍从 graph.cognitions 读取。该摘要用于提醒近期发现，不记录已读或已处理状态。

### Trace

展示目标 Exploration 的来源祖先、相关 Cognition、retry 链、链上 outputs、全局 Cognition 和必要的 Purpose 上下文。Trace 的 Task 上下文始终由目标 Exploration 决定。

`parent_task` 提供当前探索作为拆分方向所归属的父任务目标 `purpose` 和职责提示 `note`：仅完成当前探索，父任务目标只供理解上下文，无需完成整个父任务。英文 note 使用单行字符串。

`running_explorations` 列出该父任务的全部 running 探索，包括目标自身及子图外的并行探索，每项为 `id`、`title`、`duration_seconds`、`explorer_id`；最后一个字段是 assign 时通过 `--explorer` 登记的实际 Explorer 标识。空时输出 `[]`。摘要不扩大 related_subgraph，历史目标也不改用当前 open Task 的列表。实体结果与 running 查询也统一使用 `explorer_id`。

Trace 沿 `sources` 和指向 Exp 的认知来源追溯；Task 来源引用不形成图边。用户认知采用与 Status 相同的投影位置。引用某个 Exp 的用户提示时，纳入该 Exp、其全部 hints 和来源祖先，但不自动纳入无关派生认知。目标完整 retry 链上的派生认知和用户提示都会保留。

### Visual

生成包含完整 Subject、全部 Task 和 Graph 的自包含 HTML。`--lang` 可显式选择中文或英文；省略时按操作系统的 `LC_ALL`、`LC_MESSAGES`、`LANG` 选择。默认只返回文件路径；`--open` 会尝试使用系统浏览器打开。HTML 不依赖网络资源。

Graph 默认显示平面视图，也可切换为知识图谱。两者采用相同的浅灰蓝背景色，分别使用固定屏幕间距的网格和点阵，不随缩放变化。平面视图左上角的图例说明认知类型与探索状态的颜色；Task 初始认知与全局用户认知位于输入列，探索用户提示显示在所属 Exp 的产出侧，并标识为用户断言。知识图谱使用圆形表示 Cognition、六边形表示 Exploration，按真实关系形成主干和分支，支持多个起点及孤立节点，不为独立认知补造连接。局部分叉和关联密集处自动增加间距，跨链关系完整保留，并使用平滑曲线避让节点。retry 以独立的执行沿袭连线展示，不参与知识关系的分层。

标题独立于节点显示，缩放时保持字号并在屏幕空间中自动避让。概览优先保留初始认知、分支入口与内部节点的标题，其余可用位置也会显示标题；悬停、聚焦或选中时展开更多内容。悬停或键盘聚焦可高亮直接关联，点击或按 Enter / Space 查看详情。两种视图的详情面板均可关闭，不预留空白侧栏。知识图谱详情按钮第二行提供跨链关系显示切换和定位初始认知；底部提供方向筛选、缩放、全图、搜索及视图切换。

平面图例复用节点的金属渐变与空心探索状态边框。知识图谱的认知边环复用铜、金、银金属渐变，图例对应认知类型和探索状态；连线、箭头与柔光珠均跟随目标节点的主题色，目标为认知时使用对应的纯色金属色，重试关系保留虚线。知识图谱以柔光珠沿主干流动：同色亮核配轻柔光晕，悬停或选中的关联线略微提亮，跨链关系仅在强调或关联时显示动效。光珠尺寸与移动速度保持屏幕像素尺度，不随缩放变粗或变快；离屏、后台、平面视图或没有可见关联时停止动画调度，并遵循系统的减少动态效果设置。

Status 和 Trace 默认生成 YAML，`--json` 生成等价 JSON，`--md` 生成 Markdown。编码后的 View 不超过 6 KiB 时直接向 stdout 输出；超过 6 KiB 时，stdout 改为返回包含 `path`、`format`、`size_bytes` 和 `note` 的 JSON 成功信封，完整 View 只从 `path` 指向的文件读取。`note` 按语言环境说明 View 输出内容较多，完整内容已写入文件而不直接返回全部内容。阈值按 UTF-8 编码后的字节数计算。三种格式无论大小都会将完整内容原子发布到 `.norns/view/`；Markdown 文件扩展名和引用中的 `format` 都是 `md`。Visual 始终返回 JSON 成功信封。三类文件名类似：

~~~text
.norns/view/status-20261012T063546Z-a1b2c3d4.yaml
.norns/view/trace-20261012T063546Z-e5f6a7b8.json
.norns/view/status-20261012T063546Z-c3d4e5f6.md
.norns/view/visual-20261012T063546Z-bbd20338.html
~~~

## 输出和错误

除 Status、Trace 和人类模式的 `version` 外，核心命令成功时输出单行 JSON：

~~~json
{"ok":true,"data":{},"warnings":[],"message":"Workspace initialized"}
~~~

错误写入 stderr，stdout 保持为空，并使用稳定的机器错误码：

~~~json
{"ok":false,"error":{"code":"NO_OPEN_TASK","message":"Workspace has no open Task","details":{}}}
~~~

调用方应依据 `error.code`、枚举和结构化字段判断结果，不应解析本地化 message。运行数量超过 Task 配置的并发值时，修改仍会成功，并返回 `CONCURRENCY_LIMIT_EXCEEDED` warning。

人类可读文本根据 `LC_ALL`、`LC_MESSAGES`、`LANG` 选择中文或英文；机器字段、错误码、ID、枚举和时间不会本地化。

## 并发和锁

- SQLite 使用 WAL；reader 可以并发。
- writer 使用 `BEGIN IMMEDIATE`，由 SQLite 串行化。
- 普通读写命令和 View 持有 Workspace 生命周期共享锁。
- `workspace init`、`workspace reset` 和 reset recovery 持有生命周期独占锁。
- `.norns.lock` 只保护 `.norns/` 的生命周期和目录替换，不代替 SQLite 事务锁。
- 依赖已有 Workspace 的命令先检查 `.norns/` 和 `norns.sqlite`，不存在时返回 `WORKSPACE_NOT_INITIALIZED`，不会创建锁文件；`workspace init`、首次 `task open` 和待恢复的 reset journal 保留初始化/恢复入口。
- 最后一个持锁操作结束后，在独占保护下删除 `.norns.lock`；其他共享操作尚未结束时保留。等待者获取锁后重新核对文件身份，避免使用已删除的旧锁。进程异常退出可能留下锁文件，下次操作会重新加锁并清理；不要混用新旧版本并发操作同一 Workspace。
- View 在共享生命周期锁内读取 snapshot 并完成原子文件发布，多个 View 使用唯一文件名并发写入。

Runtime 占用保存在 SQLite 的 `workspace_meta.runtime_occupied` 中。`workspace free` 通过取消未完成探索并关闭 open Task 来清除占用；CLI 不提供保留 open Task、单独修改占用标记的命令。

## 外部扩展

### 查看可发现的扩展

~~~sh
norns extension list
~~~

无需初始化 Workspace。输出沿用 CLI 的 JSON 信封，`data` 为按名称排序的数组，每项包含扩展名称 `name` 和实际选中的可执行文件绝对路径 `path`；没有可发现的扩展时输出 `data: []`。如果同目录的 `norns.exts.meta.json` 中存在匹配记录，额外输出 `version`、`description`、`requirements`、`release_files`；`requirements` 为字符串数组，无使用前提时为 `[]`。

~~~json
{"ok":true,"data":[{"name":"lookup","path":"/opt/norns/norns-lookup"}],"warnings":[],"message":"Extensions listed"}
~~~

发现范围与执行扩展时一致：仅检查 `norns` 实际二进制所在目录，不读取 `NORNS_PLUGIN_PATH`，不搜索系统 `PATH` 或当前工作目录。通过符号链接启动主程序时，以链接目标二进制所在目录为准。忽略非法名称、目录、不可执行文件及与核心命令同名的扩展。仅检查文件，不启动扩展，也不验证其配置或远端服务是否可用。各插件的使用方法与配置写在各自目录的 `README.md` 中。元数据只作为静态说明，不读取环境变量值或插件配置，不检查凭据，也不代表插件通过安全认证。合并文件缺失或没有匹配项时，仅保留 `name`、`path`；合并文件不合法、过大、为符号链接或无法读取时，返回简短 warning 并回退，不输出文件内容。

### 编写扩展

扩展是独立的 `norns-<名称>` 可执行程序，Windows 使用 `norns-<名称>.exe`。名称必须以小写字母开头，仅包含小写字母、数字和连字符。未知核心命令会按扩展分发，例如 `norns lookup --query example` 会查找并执行 `norns-lookup --query example`；核心命令名（包括 `extension`）不能被覆盖。

仓库中的 Go 扩展使用以下布局：

~~~text
norns/internal/custom_extensions/lookup/
  README.md
  metadata.json
  cmd/norns-lookup/
    main.go
  ...实现与测试文件
~~~

每个扩展必须提供 `metadata.json`，包含以下字段，`name` 必须与扩展目录名及命令名称一致：

~~~json
{
  "name": "lookup",
  "version": "0.1.0",
  "description": "查询并返回数据。",
  "requirements": [],
  "release_files": []
}
~~~

`version`、`description` 为非空字符串；`requirements` 为字符串数组，只描述使用前提，无要求时为 `[]`，可选行为写在 `description` 或插件 README 中。前提条件应写变量名称、配置要求等公开说明，不要填写真实密钥、环境变量值、个人路径或运行数据。元数据文件限制为 16 KiB，未知字段、重复字段和名称不匹配均在构建前拒绝。

`release_files` 声明随插件一起发布的配置模板或其他普通单文件，可省略或使用 `[]` 表示无附件：

~~~json
"release_files": [
  {"origin_file": "configs/default.yaml", "target_file": "example.yaml"},
  {"origin_file": "usage.txt", "target_file": ""}
]
~~~

`origin_file` 相对于当前插件源码目录，可以指向子目录中的文件，不允许绝对路径、`..` 路径段、反斜杠或符号链接（包括中间目录的符号链接）。`target_file` 指定基础文件名，不允许目录；空字符串时使用源文件的 basename。发布时统一在其前面加上 `norns.exts.<插件名>.` 前缀，例如上述 `lookup` 插件生成 `norns.exts.lookup.example.yaml` 和 `norns.exts.lookup.usage.txt`。元数据中的 `target_file` 保留原始配置值，无需自行填写前缀。附件必须显式列出，不扫描目录，不展开环境变量或读取配置中的引用。只应声明可公开发布的模板和资料，不应声明真实凭据文件。

发布前拒绝缺失源文件、目录、符号链接、不安全名称、最终目标重名（不区分大小写）、加上前缀后文件名过长，以及覆盖共享元数据文件。目标文件已存在但不属于旧元数据声明的附件时，也会拒绝覆盖。不同插件可以声明相同的基础文件名，前缀会将它们隔离；同一插件内不能声明重名附件。每个平台附带相同文件并纳入 SHA256SUMS；重新发布时清理旧元数据中已声明、此次不再发布的附件。损坏的旧元数据会阻止发布，以免误判附件归属。

在 `main.go` 中实现自己的参数解析和输出，例如：

~~~go
package main

import (
    "encoding/json"
    "flag"
    "os"
)

func main() {
    query := flag.String("query", "", "query to look up")
    flag.Parse()
    if *query == "" || flag.NArg() != 0 {
        _ = json.NewEncoder(os.Stderr).Encode(map[string]any{
            "error": map[string]string{"code": "invalid_input", "message": "--query is required; positional arguments are not supported"},
        })
        os.Exit(2)
    }
    if err := json.NewEncoder(os.Stdout).Encode(map[string]string{"query": *query}); err != nil {
        os.Exit(1)
    }
}
~~~

从仓库根目录显式选择要编译的扩展：

~~~sh
./release.sh norns --with-ext lookup
# --with-ext 可重复指定；不传时只构建主程序。
~~~

入口必须为 `internal/custom_extensions/<名称>/cmd/norns-<名称>/main.go`。发布时按名称排序、去重，将本次选中扩展的 `metadata.json` 合并为每个平台二进制同目录下的 `norns.exts.meta.json`（JSON 数组），并纳入 SHA256SUMS；不选择扩展时写入 `[]`。重复选择只编译一次，不存在的扩展在构建前报错。发布按本次选择替换当前平台的 norns 产物，移除旧的未选中 `norns-*` 扩展及旧元数据声明的附件。

只编译扩展用于开发时，在 `norns/` 模块中执行：

~~~sh
go build -o ../bin/norns-lookup ./internal/custom_extensions/lookup/cmd/norns-lookup
~~~

将产物与 `norns` 实际二进制放在同一目录。手动 `go build` 不生成合并元数据；需要显示完整元信息时，使用发布入口，或将各源码元数据对象组成 JSON 数组保存为同目录的 `norns.exts.meta.json`。扩展发现仅限此目录，环境变量和系统 `PATH` 不能用于增加搜索路径。通过 `norns extension list` 检查实际发现的文件，再执行 `norns lookup --query example`。

扩展收到原始参数、stdin/stdout/stderr，工作目录为调用 `norns` 时的当前目录，并继承环境变量；Norns 额外提供：

- `NORNS_EXECUTABLE`：当前 Norns 主程序路径；
- `NORNS_WORKSPACE`：调用时的工作目录；
- `NORNS_PROTOCOL_VERSION`：当前支持的协议版本；
- `NORNS_SCHEMA_VERSION`：当前数据库 schema 版本。

扩展退出码和输出原样传递，核心不会为扩展输出自动添加 JSON 信封。建议成功数据写 stdout，错误写 stderr，并提供不依赖配置的 `--help`。需要配置文件时，可自行约定 `NORNS_EXTENSION_<名称大写>_CONFIG` 环境变量，并在插件自己的 README 中说明。扩展若要修改协议状态，必须调用 `norns`，不能直接写 SQLite。

`internal/custom_extensions/` 下新增的本地扩展默认被 `.gitignore` 忽略，仅仓库明确列入白名单的扩展参与版本管理。需要共享新扩展时，再调整根目录的忽略规则。

## 测试

~~~sh
go test ./...
go test -race ./...
go vet ./...
~~~

真实构建产物的集成测试需要显式启用：

~~~sh
NORNS_RUN_BINARY_INTEGRATION=1 go test ./... -run Integration
~~~

`testdata/requirements.json` 保存行为覆盖 key 与自动化测试的映射；CI 会检查 key、来源标识和测试名有效且唯一。Status/Trace 的稳定输出由 `testdata/views/` 下的 golden files 固定。

## 文档

- [Norns CLI 设计](../documents/design/norns.md)
- [Norns 协议](../documents/protocol/protocol.md)
- [第三方声明](THIRD_PARTY_NOTICES.md)
