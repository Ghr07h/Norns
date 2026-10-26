# Norns CLI

`norns` 是 Norns protocol v2 的本地 Workspace CLI。它使用 SQLite 持久化 Task、Cognition、Exploration 及其关系，执行协议状态转换，并生成 Status、Trace 和 Visual 三类 View。

当前版本边界：

- protocol version：`2`
- database schema version：`9`
- Go：`1.25`
- SQLite 驱动：纯 Go，无 CGO 依赖

CLI 只管理协议状态，不启动、监控或停止智能体。Runtime 必须在关闭 Task 前停止并确认相关 Explorer 不再写入。

## 构建

~~~sh
cd norns
go build -o ./bin/norns ./cmd/norns
go build -o ./bin/norns-websearch ./cmd/norns-websearch
./bin/norns version
./bin/norns websearch --help
~~~

将两个可执行文件安装到 Go bin 目录：

~~~sh
go install ./cmd/norns ./cmd/norns-websearch
norns version --json
~~~

交叉构建示例：

~~~sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/norns-linux-amd64 ./cmd/norns
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/norns-websearch-linux-amd64 ./cmd/norns-websearch
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ./bin/norns-windows-amd64.exe ./cmd/norns
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ./bin/norns-websearch-windows-amd64.exe ./cmd/norns-websearch
~~~

支持的发布目标为 macOS amd64/arm64、Linux amd64/arm64 和 Windows amd64。

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

ID 由 CLI 生成，格式分别为 `task_<uuid-v4>`、`cog_<uuid-v4>` 和 `exp_<uuid-v4>`。示例中的 ID 需要替换为命令实际返回的 ID。

## Workspace

只有 `<workspace>/.norns/norns.sqlite` 存在时才视为已初始化。仅存在 `.norns/` 或 `.norns/history/` 不代表初始化完成。

~~~text
<workspace>/
  .norns.lock                 生命周期共享/独占锁
  .norns/
    norns.sqlite              protocol v2 / schema v9 数据库
    view/                     原子发布的 Status、Trace 和 Visual View
    history/                  workspace reset 归档
  .norns-reset.json           仅在 reset 进行或恢复期间存在
~~~

不要直接修改 SQLite。所有 Graph 写入必须通过 `norns` 命令完成。

### 初始化和重置

~~~sh
norns workspace init
norns workspace reset
~~~

`workspace init` 可以重复调用，但会先校验协议版本、schema 版本和 Workspace 状态。

`workspace reset` 仅在不存在 open Task、版本兼容且状态可信时执行。它把旧数据库、View 和其他 Workspace 资源归档到 `.norns/history/<timestamp>/`，然后创建空 Workspace。reset 后的首个 Task 必须重新提供 `--info`。

## Task

~~~sh
norns task open --purpose TEXT --concurrency N [--info TEXT]
norns task close --outcome OUTCOME --outcome-reason TEXT
norns task reopen [--task-id TASK_ID]
~~~

Task outcome：

- `succeeded`
- `unachievable`
- `failed`
- `cancelled`

Workspace 最多存在一个 open Task。首个 Task 必须提供 `--info`；后续 Task 可以省略它并复用最新 closed Task 的 Subject Cognition，也可以提供新信息以扩展 Subject。

`task close` 在同一事务中取消当前 Task 剩余的 todo/running Exploration、关闭 Task，并清除 Runtime 占用。`task reopen` 保留已有 Graph 和 Exploration，不会自动恢复或 retry 历史执行。

## Cognition

~~~sh
norns cognition add \
  --cognition-title TEXT \
  --cognition-content TEXT \
  --origin user|exploration \
  [--exploration EXP_ID] \
  [--task-purpose-relevant[=BOOL]]
~~~

支持三种调用场景：

- 全局用户 Cognition：`--origin user`，不指定 Exploration；要求存在 open Task。
- Exploration hint：`--origin user --exploration EXP_ID`；目标可以来自任意 Task、处于任意状态。
- Explorer output：`--origin exploration --exploration EXP_ID`；目标必须是当前 open Task 的 todo 或 running Exploration。

全局 Cognition 不能设置 `--task-purpose-relevant`。Cognition 创建后不可修改或失效；新的 Cognition 可以补充或修正旧认知，但不会删除旧节点。

## Exploration

~~~sh
norns exploration add \
  --exploration-title TEXT \
  --exploration-content TEXT \
  [--source-cognitions COG_ID]...

norns exploration retry --exploration EXP_ID
norns exploration assign --exploration EXP_ID --explorer EXECUTOR_ID --output OUTPUT_REF
norns exploration finish --exploration EXP_ID --outcome OUTCOME --outcome-reason TEXT
norns exploration todo
norns exploration running
~~~

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

todo 只能直接 finish 为 `cancelled`。assign 时，如果 `output_ref` 指向可解析的符号链接，Norns 会记录链接最终目标的绝对路径；普通文件、非路径引用、已断开的链接或解析失败的路径仍按传入值记录。assign 后的 `executor_id` 和 `output_ref` 不可修改；CLI 不提供 reassign 或 executor reconnect。retry 可以使用任意历史 Task 的 finished Exploration，并为当前 open Task 创建新执行记录。

`todo` 和 `running` 只返回当前 open Task 的调度列表；没有 open Task 时返回 `NO_OPEN_TASK`。

## View

~~~sh
norns view status [--task-id TASK_ID] [--json]
norns view trace --exploration EXP_ID [--json]
norns view visual [--lang zh|en] [--open]
~~~

### Status

展示当前完整 Graph，并以 open Task 或显式 `--task-id` 作为 Task 上下文。没有 open Task 且未指定 Task 时，顶层返回全部历史 Tasks。

### Trace

展示目标 Exploration 的来源祖先、相关 Cognition、retry 链、链上 outputs、全局 Cognition 和必要的 Purpose 上下文。Trace 的 Task 上下文始终由目标 Exploration 决定。

### Visual

生成包含完整 Subject、全部 Task 和 Graph 的自包含 HTML。`--lang` 可显式选择中文或英文；省略时按操作系统的 `LC_ALL`、`LC_MESSAGES`、`LANG` 选择。默认只返回文件路径；`--open` 会尝试使用系统浏览器打开。HTML 不依赖网络资源。

Status 和 Trace 默认生成 YAML，`--json` 生成等价 JSON。编码后的 View 不超过 6 KiB 时直接向 stdout 输出；超过 6 KiB 时，stdout 改为返回包含 `path`、`format` 和 `size_bytes` 的 JSON 成功信封，完整 View 只从 `path` 指向的文件读取。阈值按 UTF-8 编码后的字节数计算。两者无论大小都会将完整内容原子发布到 `.norns/view/`。Visual 始终返回 JSON 成功信封。三类文件名类似：

~~~text
.norns/view/status-20261012T063546.749558000Z-a1b2c3d4.yaml
.norns/view/trace-20261012T063546.749558000Z-e5f6a7b8.json
.norns/view/visual-20261012T063546.749558000Z-bbd20338.html
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
- View 在共享生命周期锁内读取 snapshot 并完成原子文件发布，多个 View 使用唯一文件名并发写入。

Runtime 占用保存在 SQLite 的 `workspace_meta.runtime_occupied` 中。CLI 不提供单独设置或清除该值的命令。

## 外部扩展

未知核心命令会按外部扩展处理：

~~~sh
norns websearch --query "SQLite WAL shared reader behavior"
~~~

CLI 依次查找：

1. `norns` 可执行文件所在目录中的 `norns-websearch`；
2. `NORNS_PLUGIN_PATH` 中的目录；
3. 当前 `PATH`。

扩展收到原始参数、stdin/stdout/stderr 和以下环境变量：

- `NORNS_EXECUTABLE`
- `NORNS_WORKSPACE`
- `NORNS_PROTOCOL_VERSION`
- `NORNS_SCHEMA_VERSION`

扩展退出码和输出原样传递。核心命令名不能被扩展覆盖。扩展若要修改协议状态，必须调用 `norns`，不能直接写 SQLite。

### Websearch 工具

仓库提供独立的 Go 工具 `norns-websearch`，用于补充不具备 WebSearch 能力的智能体平台。它不访问 Norns Workspace，也不依赖 Python 或 DDGS 运行时。

~~~sh
norns websearch --query "SQLite WAL shared reader behavior"
norns websearch --query "Norns protocol" --max-results 5
norns websearch --query "Norns protocol" --proxy socks5h://127.0.0.1:1080
~~~

参数与 MVP 保持一致：

- `--query`：必填搜索词；
- `-m`、`--max-results`：最大结果数，默认 `10`；
- `--proxy`：显式 HTTP、SOCKS5 或 SOCKS5H 代理地址，必须包含端口。

未指定代理时使用百度后端；显式指定代理时使用 Google 后端。两个后端分别维护请求参数和 HTML 解析器，公共层负责文本与 URL 规范化、按链接去重、简单相关性排序及结果截断。

成功时直接输出 JSON 数组，每项包含稳定字段 `title`、`href` 和 `body`。参数错误和搜索错误分别使用 `invalid_input`、`search_error`，写入 stderr；扩展输出不套核心 CLI 的 `ok/data/warnings/message` 信封。

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
