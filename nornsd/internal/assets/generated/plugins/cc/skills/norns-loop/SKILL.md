---
name: norns-loop
description: 使用 Norns Protocol 的 Cognition/Exploration Graph 组织开放任务；在建立、恢复、决策、探索、调度或关闭 Norns Task 时使用。
---

# Norns Loop

## 实体语义

- **Workspace**：一个 Subject 的协议状态容器。当前工作目录对应当前 Workspace；一个 Workspace 同一时间最多存在一个 open Task。
- **Subject**：Workspace 持续认识的目标对象，不是独立协议实体或 Graph 节点。
- **Runtime**：承载 open Task 及其 Agent 的执行上下文，不是 Graph 节点。同一 Workspace 同一时间最多由一个 Runtime 占用。
- **Task**：针对 Subject 的一次具体求解目的。`purpose` 只描述本次目标，Task-init Cognition 描述 Subject 信息。Task outcome 为 `succeeded`、`unachievable`、`failed` 或 `cancelled`。
- **Cognition**：写入 Graph 的不可变信息单元。可靠事实、用户提示和可复用发现应写为 Cognition；尚待验证的假说应表现为 Exploration。
- **Exploration**：获取新知识或验证假说的可执行方向，状态依次为 `todo`、`running` 和 `finished`。执行结果为 `succeeded`、`inconclusive`、`failed` 或 `cancelled`。
- **Graph**：Workspace 中 Cognition、Exploration 及其关系构成的权威共享知识状态，跨 Task 持续积累。
- **View**：根据当前 Workspace 生成的只读投影。Status View 用于决策，Trace View 用于执行指定 Exploration，Visual View 用于人类观察。
- **Domain**：可选的领域知识、工具和角色扩展，不是协议状态，不引入协议动作，也不改变通用角色边界。具体 Domain Agent 的选择方式属于 Runtime 行为，不由协议或本 Skill 规定。

## 角色定义

所有角色都必须基于动作执行时的最新协议状态工作，并且只承担自身角色拥有的协议职责。

### Coordinator

- 建立或恢复 Task，按事件驱动方式调度 Decider 与 Explorer，并管理 Task 生命周期。
- 根据 `max_concurrency`、todo、running 和实际 Runtime Agent 状态计算可用调度能力。
- 启动、联系、观察或停止 Runtime Agent，并通过协议动作登记相应的逻辑状态。
- 将用户提供的事实或提示持久化为 Cognition，并把 Exploration 用户提示转交仍在运行的对应 Explorer。
- 不亲自执行 Exploration，不代替 Explorer 形成探索结论，也不通过 add 或 retry 创建 Exploration。

### Decider

- 读取最新 Status View，基于当前 Graph 决定 `explore`、`wait` 或 `complete`。
- `explore` 时通过 add 或 retry 创建不超过可用调度能力的高价值 Exploration；可以少于请求数量。
- `wait` 只用于等待正在进行的工作或明确的外部变化。
- `complete` 只能建议 `succeeded` 或 `unachievable`，结论必须由当前 Graph 支持。
- 不执行 Exploration，不创建 Cognition，不 assign 或 finish Exploration，也不关闭 Task。

### Explorer

- 每次只执行一个指定的 Exploration，并先读取该 Exploration 的最新 Trace View。
- 只完成 `current_exploration` 描述的目标和范围；Trace 中的其他信息仅用于理解上下文。
- 将可靠、明确且可复用的发现及时写为 `origin=exploration` 的 Cognition。
- 结束前写完所有有价值 Cognition，再以 `succeeded`、`inconclusive` 或 `failed` finish Exploration。
- 不创建 Exploration，不决定 Task outcome，也不关闭 Task。外部取消由 Coordinator 处理并登记为 `cancelled`。

## 协议动作

协议动作只读取或修改 Workspace 的逻辑状态。Runtime Agent 动作负责真正启动、联系、观察或停止 Agent；两类动作不会自动替代或同步彼此。

- `workspace.initialize`：建立空 Workspace；不得覆盖未知、不兼容或状态不可信的 Workspace。
- `task.open`：创建 open Task、创建或选择 Task-init Cognition，并设置 Runtime 占用。首个 Task 必须提供 Subject 信息。
- `task.reopen`：重新打开 closed Task，保留既有 Graph，但不恢复或 retry 已结束的 Exploration。
- `task.close`：关闭 open Task、取消剩余 todo 和 running Exploration，并清除 Runtime 占用。调用前必须先停止或隔离相关 Runtime Agent。
- `cognition.add`：创建不可变 Cognition。Coordinator 写入用户 Cognition；Explorer 写入指定 Exploration 的可靠发现。
- `exploration.add`：在当前 open Task 中创建 todo Exploration；没有显式来源时使用 Task-init Cognition。
- `exploration.retry`：从 finished Exploration 创建具有新 ID 的 todo Exploration，不修改或重新 assign 原记录。
- `exploration.assign`：把 todo Exploration 登记为 running，并记录实际 Explorer 的 `executor_id` 和 `output_ref`；该动作本身不启动 Explorer。
- `exploration.finish`：结束 todo 或 running Exploration；todo 只能以 `cancelled` 结束。Explorer 登记正常执行结果；Coordinator 在外部取消、失联或异常恢复时登记相应结果。该动作本身不停止 Explorer。
- `exploration.list_todo` 与 `exploration.list_running`：查询当前 open Task 的调度集合，不是 View。
- `view.status`、`view.trace` 与 `view.visual`：生成只读 View，不修改 Workspace。

修改动作必须原子执行。失败动作不得留下部分状态；软 warning 表示动作已经提交，不能据此回滚或重复执行。

## CLI Usage

使用以下 CLI 命令调用对应协议动作：

```sh
# 初始化当前工作目录中的空 Workspace；重复初始化合法 Workspace 时幂等成功
norns workspace init

# 打开一个新 Task
## --purpose 只描述本次 Task 的目标；--concurrency 指定额定 Explorer 并发数
## --info 描述 Subject 信息；首个 Task 必须提供，后续 Task 省略时复用最近的 Task-init Cognition
norns task open --purpose TEXT --concurrency N [--info TEXT]

# 重新打开 closed Task，保留原 Task 和 Graph，但不恢复或 retry 已结束的 Exploration
## --task-id 省略时优先幂等选择当前 open Task，否则选择最近创建的 closed Task
norns task reopen [--task-id TASK_ID]

# 关闭当前 open Task；执行前必须先停止或隔离所有实际 Explorer
## succeeded 表示已达成 Purpose；unachievable 表示现有 Graph 证明当前条件下无法达成
## failed 表示 Runtime、工具或内部异常导致无法可靠继续；cancelled 表示被用户或外部系统提前终止
norns task close --outcome succeeded|unachievable|failed|cancelled --outcome-reason TEXT

# 添加用户提供的事实或提示
## 不带 --exploration 时创建 Workspace 全局 Cognition；带该参数时创建指定 Exploration 的用户提示
norns cognition add --cognition-title TEXT --cognition-content TEXT --origin user [--exploration EXP_ID]

# 添加 Explorer 在指定 Exploration 中形成的可靠发现
## 仅当发现直接支撑、改变或限制 Task Purpose 时增加 --task-purpose-relevant
norns cognition add --cognition-title TEXT --cognition-content TEXT --origin exploration --exploration EXP_ID [--task-purpose-relevant]

# 创建 todo Exploration
## --source-cognitions 可重复传入并保留顺序；省略时使用当前 Task 的 Task-init Cognition
norns exploration add --exploration-title TEXT --exploration-content TEXT [--source-cognitions COG_ID]...

# 从一个 finished Exploration 创建具有新 ID 的 todo Exploration；不会修改或重新运行来源记录
norns exploration retry --exploration EXP_ID

# 实际 Explorer 启动后，登记 executor ID 与输出引用并将 todo Exploration 转为 running
## 此命令只登记协议状态，不会启动 Explorer
norns exploration assign --exploration EXP_ID --explorer EXECUTOR_ID --output OUTPUT_REF

# 结束 Exploration 并记录执行结果；所有可复用发现必须先写为 Cognition
## running Exploration 支持全部 outcome；todo Exploration 只能以 cancelled 结束
## 此命令只登记协议状态，不会停止 Explorer
norns exploration finish --exploration EXP_ID --outcome succeeded|inconclusive|failed|cancelled --outcome-reason TEXT

# 查询当前 open Task 中按创建顺序排列的 todo Exploration 和额定并发数
norns exploration todo

# 查询当前 open Task 中的 running Exploration、实际执行引用、运行时长和额定并发数
norns exploration running

# 生成供 Decider 使用的 Status View；--task-id 可显式选择 Task 上下文
## 默认输出 YAML；--json 改为输出 JSON
norns view status [--task-id TASK_ID] [--json]

# 生成供 Explorer 使用的指定 Exploration 的 Trace View
## 默认输出 YAML；--json 改为输出 JSON
norns view trace --exploration EXP_ID [--json]

# 生成供人类观察完整 Subject、Task 和 Graph 的 HTML Visual View
## --open 同时使用系统默认浏览器打开生成文件
norns view visual [--open]
```

- 注意：Norns CLI 需要在当前 Subject 目录下执行。
- 修改命令成功时返回 `{"ok":true,"data":...,"warnings":[],"message":"..."}`。检查 `ok`、结构化 `data` 和 `warnings`，不要解析本地化 `message`。
- 失败时 stderr 返回 `{"ok":false,"error":{"code":"...","message":"...","details":...}}`。按 `error.code` 处理，不得把失败动作视为已生效。
- Status 和 Trace 默认输出 YAML，只有显式传入 `--json` 时才输出 JSON；`exploration todo` 与 `exploration running` 是调度查询，不是 View。
- Status 或 Trace 文本超过 6 KiB 时，命令返回 JSON 成功信封而不内联 View；从 `data.path` 指向的 `.norns/view/` 文件读取完整内容。

## 调度逻辑

Norns 使用事件驱动调度，不划分固定的 Decide/Explore 轮次，也不等待一组 Exploration 全部结束。

1. Runtime 启动、恢复或重新取得控制权时，先读取最新 todo、running 和 Runtime Agent 状态，确认不存在仍能并发写入的旧 Runtime，并对齐协议记录与实际执行单元。
2. 没有 open Task 时，根据用户意图 open 新 Task 或 reopen 既有 Task；已有 open Task 时继续该 Task，不重复创建。
3. 每次重新评估先对齐 running Exploration 与实际 Explorer，再根据 `max_concurrency`、todo、running 和实际执行状态计算空闲槽位。
4. 在空闲槽位内优先启动已有 todo Exploration。取得 `executor_id` 和 `output_ref` 后立即 assign；assign 失败时停止或隔离刚启动的 Explorer。
5. 仍有空闲槽位、todo 不足且没有活动 Decider 时，可以请求不超过空闲槽位数量的新方向。Decider 返回少于请求数量的方向后，不在相同信息状态下重复调用。
6. Cognition 创建、Exploration 状态变化、Runtime Agent 状态变化、Decider 返回或用户输入到达时，重新读取必要状态并评估调度。等待事件时单次阻塞不得超过一分钟。
7. 用户针对 Exploration 提供提示时，先通过 `cognition.add` 持久化，再投递给实际 Explorer；投递失败不得回滚 Cognition，并触发重新评估。
8. Explorer 失联、超时或异常结束时，先确认旧实例已经停止或不能继续写入，再 finish 原 Exploration。是否继续该方向以及是否 retry 由 Decider 基于最新状态决定，禁止原地重新 assign。
9. Decider 返回 `wait`、暂时没有 todo 或暂时没有新事件时，保持 Task open 并等待新的事件。
10. Decider 返回 `complete` 时，Coordinator 重新核对最新 Graph 和结论依据，停止或隔离全部实际 Explorer，再执行 `task.close`。用户终止或不可恢复异常可以分别以 `cancelled` 或 `failed` 完成同样的关闭流程。

同一 Task 同一时间最多存在一个活动 Decider；同一 Exploration 同一时间最多由一个实际 Explorer 执行。

## 总约束

- 使用 `norns` CLI 操作当前目录中的 Workspace，不直接修改 `.norns/`。
- 严格以 CLI 返回的最新结构化结果作为协议状态事实源，不假定先前的 View、查询或计数仍然有效。
- 对话、私有推理、临时笔记、Agent 返回文本、Runtime 工具输出以及领域知识本身都不是共享事实；只有写为 Cognition 或实体状态的信息才能被其他角色依赖。
- Runtime Agent 动作与协议动作必须分别执行并持续核对：`Agent`、`SendMessage`、`TaskStop` 和 `TaskOutput` 等 Runtime 工具控制实际 Agent，Norns CLI 只登记逻辑状态。
- Agent 必须围绕当前角色工作，不代行其他角色的协议职责。Domain Agent 只补充领域知识和工具，不改变通用角色边界。
- 需要核对完整字段、状态转换、View 语义或边界条件时，读取 [protocol.md](references/protocol.md)；该文件是完整规范，本 Skill 只提供 Runtime 执行所需的协议摘要和 CLI 绑定。
