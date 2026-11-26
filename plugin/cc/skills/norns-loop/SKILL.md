---
name: norns-loop
description: 遵循 Norns Protocol 的开放任务处理机制，基于 Cognition/Exploration 状态持续驱动多智能体系统的认知和探索。
---

# Norns Loop

## 实体语义

- **Subject**：持续认知和探索的目标对象，可以是抽象主题或具象实体，也是 Workspace 中信息描述的逻辑对象。
- **Workspace**：承载对 Subject 的认知和探索的 Norns 协议状态的容器。Workspace 以文件系统的形式实现，启动 Runtime 的当前工作目录即当前 Subject 的 Workspace。
- **Task**：针对 Subject 的一次具体求解目的。Task Purpose 描述本次任务要达成的目标；Task-init Cognition 提供任务开始时关于 Subject 的初始断言认知，两者无直接关联。Workspace 的首个 Task 必须提供 Task-init Cognition，后续 Task 可以根据新的任务创建，也可以复用已有初始认知。一个 Workspace 可以先后包含多个 Task，但是同一时间最多存在一个 Open Task。
- **Runtime**：承载实际求解过程的多智能体运行时，包含多智能体系统、智能体协作过程及其执行上下文。一个 Workspace 同一时间最多被一个 Runtime 占用。
- **Cognition**：Workspace 保存的关于 Subject 的不可变认知记录，用于描述 Subject 的属性、行为、关系或其他特性。Cognition 可以分为两类：来源于外部输入的断言认知（asserted cognition），包括任务初始信息、约束、建议和期望探索方向等，断言认知录入不代表已经验证真实性；Runtime 探索中通过观察、实验、分析或推到形成的派生认知（derived cognition），必须有证据支持并保留适用范围。Cognition 创建后不可修改或删除，但新 Cognition 可以在内容上补充、确认、修正或反驳既有 Cognition。
- **Exploration**：获取新知识或验证假说的可执行探索方向，以一个或多个 Cognition 为决策依据，由 Runtime 基于当前 Workspace 记录的 Cognition/Exploration 状态创建和执行。
- **Cognition/Exploration Status**：Workspace 中 Cognition、Exploration 及其关系构成的权威共享协议状态，跨 Task 持续积累。
- **View**：根据当前 Workspace 的 Cognition/Exploration Status 生成的只读投影。View 常以有向无环二部图的形式提供，可选 YAML、JSON、MD 等多种形式输出。Status View 提供整个 Subject 的当前求解状态，供方向决策使用；Trace View 提供指定 Exploration 的最小充分上下文，供探索执行使用；Visual View 输出人类可读的 HTML 任务快照。View 的结构与输出形式与 Norns CLI 与模式的具体实现有关。
- **Domain**：可选提供的领域增强能力，可能包含知识、工具和角色扩展。由 Runtime 根据用户配置提供，供 Agent 按需使用。Domain 不参与 Norns 协议状态，也不改变 Norns 角色的职责边界。

## 角色定义

角色为 Norns Runtime 中的多智能体系统的职责分工。

所有角色都必须遵循协议工作，并且只承担自身角色拥有的协议职责。

角色可以在不更改职责边界的情况下提供领域增强变体，选择合适的调度角色也是 Coordinator 的职责范围。

### Coordinator

Coordinator 是 Norns Runtime 中的中央协调者，负责用户交互处理、Task 生命周期管理、Norns 角色调度。

- 建立或恢复 Task，并管理 Task 生命周期。
- 根据 Max Concurrency、Todo&Running Exploration 和实际 Norns 协议状态，按事件驱动方式调度 Decider 与 Explorer。
- 根据当前 Task 与可调度角色范围，选择通用或领域角色进行对应行为。
- 将用户提供的事实或提示持久化为 Cognition，并把 Exploration 用户提示转交仍在运行的对应 Explorer。

### Decider

Decider 是 Norns Runtime 中的决策者，负责基于最新的 Cognition/Exploration Status 给出下一步的任务推进方向。

- 读取最新 Status View，基于当前协议状态决定 `explore`、`wait` 或 `complete` 三类决策结果。
- 决策 `explore` 通过新增或重试来创建新的、不超过可用调度能力、高价值的 Exploration。
- 决策 `wait` 将要求 Coordinator 等待正在进行的工作结束或明确的外部变化。
- 决策 `complete` 将在有明确证据支持下提供结论并结束任务，可以决策的结论状态有 `succeeded` 或 `unachievable`。

### Explorer

Explorer 是 Norns Runtime 中一个 Exploration 的执行者，负责落地一个待执行的 Exploration 并产出认知、提供结果。

- 根据指定的 Exploration，自主决策和使用工具，完成此探索方向的落地。
- 探索过程中如果存在有证据支持、明确且可复用的发现，立刻记录为 `origin=exploration` 和 `exploration=<EXP_ID>` 的派生认知供后续使用。
- 探索完成后记录探索的结果取值（`succeeded`|`inconclusive`|`failed`）与结果描述（如探索过程、结果概括等），注意探索结果并不能代替认知。

## Agent 动作

Agent 动作可以分为两类：Runtime 动作和协议动作。

Runtime 动作即 Agent 的流程调用动作，如 `Agent`、`TaskStop`、`TaskOutput`、`SendMessage` 等负责真正启动、终止、观察执行状态、通信等操作；协议动作即通过调用 Norns CLI 来声明 Workspace 中的逻辑变化以维护 Cognition/Exploration Status 的操作。工具名和参数以当前 Runtime 提供的定义为准

Runtime 角色必须显式执行两侧动作，两类动作不会自动替代或同步彼此。`exploration assign` 不会启动 Explorer，需要在 `Agent` 工具调用后根据返回信息再执行；`exploration finish` 也不会终止 Explorer，需要 Explorer 在返回前自行调用协议动作或 Coordinator 使用 `TaskStop` 工具强制终止后调用来声明结束。

### 协议动作

- `workspace.init`：新建空 Workspace；不得在已有 Workspace 且仍使用时随意覆盖，也不得覆盖覆盖未知、不兼容或状态不可信的 Workspace。
- `task.open`：提供 Task Purpose 以创建一个 Open Task 并设置 Runtime 占用。首个 Task 必须提供 Subject 信息来创建任务初始认知，后续 Task 按需提供。
- `task.reopen`：重新打开指定的 Closed Task，保留既有协议状态，但不恢复已取消的 Exploration。
- `task.close`：关闭 Open Task、取消剩余 Todo&Running Exploration，并取消 Runtime 占用。
- `cognition.add`：创建不可变 Cognition。可以由 Coordinator 和 Explorer 调用：Coordinator 根据用户输入创建全局断言认知或探索断言认知，用于记录用户提示或要求；Explorer 根据探索过程创建探索派生认知，记录可复用信息。
- `cognition.get`：获取指定的 Cognition 的详细信息。
- `exploration.add`：在需要进行新的探索时创建一个新探索方向。
- `exploration.retry`：在需要重试已有探索时，复制指定探索的标题内容与依据来源创建一个新探索方向。此探索与原探索可被 Trace View 通过 retry 链关联用户探索提示与派生认知。
- `exploration.assign`：把 Todo Exploration 登记为 Running，并记录 Explorer 的 SubAgent ID `explorer_id` 和 Transcript 路径 `output_ref`；该动作只登记协议状态不实际启动 Explorer，应在 SubAgent 创建后根据返回结果进行登记
- `exploration.finish`：将指定的 Exploration 登记为完成；Todo Exploration 登记后将以 `cancelled` 结束。可以由 Coordinator 和 Explorer 调用：Explorer 在返回前登记执行结果；Coordinator 在外部终止 Explorer SubAgent 后登记取消结果。此动作只登记协议状态不实际停止 Explorer。
- `exploration.get`：获取指定的 Exploration 的详细信息。
- `exploration.todo`：列出当前未分配 Explorer 的 Todo Explorations。
- `exploration.running`：列出当前执行中的 Running Explorations 及其对应的 SubAgent ID。
- `view.status`：生成当前 Subject 的全局视图，一般由 Decider 调用。
- `view.trace`：生成指定 Exploration 的最小化上下文，一般由对应的 Explorer 调用。
- `view.visual`：生成当前 Subject 的可视化快照 HTML，一般由用户在 Runtime 外调用。

### Norns CLI Usage

Norns CLI 命令需要在 Subject 目录（即 Workspace 目录）下调用对应协议动作：

```sh
# 检查当前目录状态，是否是 Workspace，是否存在任务
norns workspace check

# 初始化一个当前目录为空 Workspace
norns workspace init

# 将已存在的无 Open Task 的 Workspace 存档到 .norns/history/ 下并创建新的空 Workspace
## 归档仅归档 norns 协议数据，不复制 Subject 下的其他文件
norns workspace reset

# 释放当前工作区被其他 runtime 的占用，并关闭正在进行的 Task
## 由用户在 runtime 外手动调用异常占用的工作区
norns workspace free

# 打开一个新 Task
## --purpose 只描述本次 Task 的目标；--concurrency 指定额定 Explorer 并发数
## --info 描述 Subject 初始信息；首个 Task 必须提供，后续 Task 若省略将复用最近关闭 Task 的 Task-init Cognition
norns task open --purpose TEXT --concurrency N [--info TEXT]

# 重新打开 closed Task
## --task-id 省略时，如已有 Open Task 则返回 Task ID，如无则重新打开最近一次 Closed Task
norns task reopen [--task-id TASK_ID]

# 关闭当前 Open Task
## 执行前必须先终止所有实际 Explorer SubAgent 运行，无须配套执行多次 `exploration.finish`，`task.close` 调用的同时会直接将当前的 Todo&Running Explorations 登记为 `cancelled`
## outcome 参数使用枚举值：succeeded 表示已达成 Purpose；unachievable 表示现有 Graph 证明当前条件下无法达成；failed 表示 Runtime 层发生异常导致无法可靠继续；cancelled 表示被用户或外部系统提前终止。
## 因 Decider 的 `complete` 决策关闭任务时，Coordinator 仅可登记 `succeeded`、`unachievable` 两类 outcome；因用户终止任务要求关闭任务时，Coordinator 仅可登记 `cancelled` 值。其他 outcome 由 Runtime 外部控制。
norns task close --outcome succeeded|unachievable|failed|cancelled --outcome-reason TEXT

# 将用户提供的信息登记为全局用户断言认知或探索断言认知
## 由 Coordinator 调用
## 不带 --exploration 时创建 Workspace 全局用户 Cognition；带该参数时创建为指定 Exploration 的用户提示 Cognition
norns cognition add --cognition-title TEXT --cognition-content TEXT --origin user [--exploration EXP_ID] [--meta JSON]

# 添加 Exploration 过程中形成的可靠发现
## 由 Explorer 调用
## 仅当此 Cognition 发现直接支撑、改变或限制 Task Purpose 时增加 --task-purpose-relevant 标记
## --meta 为可选的元信息记录，由领域增强角色在需要时规定格式和内容，未提及时忽略。传入 JSON 字符串，例如 '{"type":"hint","scope":"geometry"}'
norns cognition add --cognition-title TEXT --cognition-content TEXT --origin exploration --exploration EXP_ID [--task-purpose-relevant] [--meta JSON]

# 读取任意 Cog 的完整详情
norns cognition get --cognition COG_ID

# 新创建 Todo Exploration
## 由 Decider 调用
## --source-cognitions 可重复传入
## --source-cognitions 省略时使用当前 Task 的 Task-init Cognition
norns exploration add --exploration-title TEXT --exploration-content TEXT [--source-cognitions COG_ID]...
# 复制已有 Exp 以创建 Todo Exploration
## 由 Decider 调用
# 从一个 Finished Exploration 创建具有新 ID 但标题、内容和依据认知相同的 Todo Exploration；不会修改或重新运行来源记录
norns exploration retry --exploration EXP_ID

# 实际 Explorer 启动后，登记 Explorer ID 与输出引用并将 todo Exploration 转为 running
# 登记指定的 Exploration 为运行中并记录 SubAgent ID 和 Transcript 文件
## 此命令只登记协议状态不实际启动 Explorer，应在 SubAgent 创建后根据返回结果进行登记
## `explorer_id` 和 `output_ref` 由创建 SubAgent 的工具返回决定
norns exploration assign --exploration EXP_ID --explorer EXPLORER_ID --output OUTPUT_REF

# 结束 Exploration 并记录执行结果
## outcome 参数使用枚举值：`succeeded` 表示成功完成探索；`inconclusive` 表示探索无有效结果；`failed` 表示探索因异常报错失败；`cancelled` 表示探索因外部原因取消
## Explorer 可以登记 `succeeded`、`inconclusive`、`failed` 三类 outcome；Coordinator 仅可登记 `cancelled` 值。
## outcome-reason 参数仅作为过程和结果记录，不能代替派生 Exp 记录有价值认知
## running Exploration 支持全部 outcome；todo Exploration 只能以 cancelled 结束
## 此命令只登记协议状态不实际停止 Explorer
norns exploration finish --exploration EXP_ID --outcome succeeded|inconclusive|failed|cancelled --outcome-reason TEXT

# 查询当前 Open Task 中的 Todo Exploration 和额定并发数
norns exploration todo

# 查询当前 open Task 中的 Running Exploration、Explorer ID、运行时长和额定并发数
norns exploration running

# 读取指定 Exp 的完整详情
norns exploration get --exploration EXP_ID

# 生成当前 Subject 的全局 Cog/Exp 状态视图 Status View
## 一般由 Decider 调用
## --task-id 可显式选择 Task 上下文
## 默认输出 YAML，可选 --json 或 --md
norns view status [--task-id TASK_ID] [--json | --md]

# 生成指定 Exploration 的向前追溯子视图 Trace View
## 一般由 Explorer 调用
## 默认输出 YAML；可选 --json 或 --md，两者互斥
norns view trace --exploration EXP_ID [--json | --md]

# 生成供人类可读的 Subject 快照 HTML
## 一般由用户在 Runtime 外部调用
## --lang 可指定中英文；--open 同时使用系统默认浏览器打开生成文件
norns view visual [--lang zh|en] [--open]

# 查看 Norns CLI 可选增加的扩展插件
## 如因各类任务场景预置可用插件，会在提示词中通知，未获得通知时默认未扩展插件能力
## 外置插件的调用方式为 `norns <extension_name> <extension_args>`
norns extension list
```

注意事项：

- Norns CLI 需要在 Workspace 目录下执行才有效。注意：Workspace 目录指 Norns 协议的 Workspace 语义，即当前 Runtime 启动的工作目录，并非固定或惯例目录名，Workspace 的绝对路径应通过 `pwd` 等命令获取。
- 使用 `norns` CLI 操作 Workspace 目录中的协议状态，不直接修改 `.norns/` 下维护协议状态的文件。
- 严格以 CLI 返回的最新结构化结果作为协议状态事实源，不假定先前的 View、查询或计数仍然有效。
- 对话、私有推理、临时笔记、Agent 返回文本、Runtime 工具输出以及领域知识本身都不是共享事实；只有写为 Cognition 或实体状态的信息才能被其他角色依赖。
- **cognition.add**、**exploration.add**、**exploration.finish** 传入的 `content` 或 `outcome_reason` 参数中如果包含 Shell 可解析语法（如 markdown 反引号语法、`$` 表达式、单双引号等）会导致传入字符串先被 Shell 解析，使记录内容出现遗漏或变更。如果记录内容包含类似语法，需要以 `<<'EOF'` 形式传入参数值，防止正文中的 Shell 展开。
- 修改命令成功时返回 `{"ok":true,"data":...,"warnings":[],"message":"..."}`。检查 `ok`、结构化 `data` 和 `warnings`，不要解析本地化 `message`。失败时 stderr 返回 `{"ok":false,"error":{"code":"...","message":"...","details":...}}`。按 `error.code` 处理，不得把失败动作视为已生效。
- Cognition 的 origin 取值有 task、user、exploration 三种，分别代表 Task-init 断言认知、用户输入产生的全局或探索断言认知、探索派生认知；类似的，Cognition 的 origin_ref 对应引用分别为创建 Task-init 断言认知的 Task ID、全局用户断言认知引用添加时的 Open Task ID 或探索用户断言认知针对的 Exploration ID、产生该派生认知的 Exploration ID。
- View 输出结构特性
  - Status View 为覆盖 Workspace 当前完整视图；选择历史 Task 只改变 Task 上下文，不回退到历史快照。
  - Trace 收录目标探索、所需来源祖先、完整 retry 链、父任务 Purpose 相关认知和全局用户认知。其 `parent_task.purpose` 只提供原始父任务上下文，不要求完成整个父任务；类似的，其 `running_explorations` 还会列出父任务中子图外的并行探索，只用于了解并行工作，不扩大本次职责。
  - Status/Trace View 中的 Graph 字段为有向无环二部图。Status/Trace View 中的 Cog 分布如下：Task-init 与派生认知放在 `graph.cognitions`（Trace 为 `related_subgraph.cognitions`）；全局用户认知独立放在同级 `global_hints`，探索用户认知只放在所属 Exp 的非空 `hints`。认知的 `origin_ref` 为 Task ID 时是初始断言，为 Exp ID 时是派生认知。认知的 `meta` 字段是领域可选自定义的元信息字段，Status / Trace 都不输出 `meta`，需要时使用 `cognition get` 获取完整信息获取。
  - Status View 的 `recent_top10_derived_cognitions` 字段记录近 30 分钟内最多 10 条的派生认知 ID，作为近期发现的 Top10 （时间倒序）参考，不代表价值排名、未处理状态或必须继续探索的方向。字段内容为空时此字段不输出。
  - Status/Trace View 无论大小都会将完整内容写入 `.norns/view/`。UTF-8 编码后的正文不超过 6 KiB 时直接完整内容；超过则返回 view 文件路径，`data` 包含 `path`、`format`、`size_bytes` 和 `note`。必须读取 `data.path` 的完整文件后再决策或执行，不把文件引用当成完整 View。

## 调度逻辑

Norns Coordinator 基于事件驱动调度，不划分固定的 Decide/Explore 轮次，也不等待一组 Exploration 全部结束再进行下一步操作。

- Runtime 首次启动时，Coordinator 应检查当前目录状态，确认是否存在工作区、是否有正在进行中的任务，但不要进行修改和创建，等待下一步指示。
- 根据当前任务状态和用户指令，进入对应的启动流程：
  - 不存在工作区
    - 用户提供明确的任务描述后，初始化工作区并创建任务，进入 Norns Loop。
  - 存在工作区
    - 不存在任务的情况下，在用户提供明确的任务描述后创建任务，进入 Norns Loop。
    - 存在任务但无 Open Task 的情况下，确认继续 Subject 还是重置 Subject 与工作区。前者请用户提供新的任务或重新打开已有任务，后者重置工作区后等待用户提供新 Subject 信息和任务意图。用户提供明确的任务描述后创建任务，进入 Norns Loop。
    - 存在任务且存在 Open Task 的情况下，确认是否继续此任务。如果继续任务则直接进入 Norns Loop，如果终止任务则关闭当前 Open Task 并按存在任务但无 Open Task 的情况推进。
- Norns Loop 调度
  - Coordinator 检查当前并发 Explorer 槽位与 Todo&Running Exploration 情况，进行进一步调度。
  - 若 Running Exploration 小于并发 Explorer 槽位且存在 Todo Exploration 时，选择合适的 Explorer 来启动 Explorer SubAgent 并登记 Exploration 为执行中。
  - 当并发 Explorer 槽位空闲且当前无进行中的 Decider 调用时，Coordinator 需调用 Decider 并给出欠缺 Exploration 的个数，由 Decider 进行下一步决策。
    - Decider 可能会返回有价值的探索方向（`explore` 决策），Coordinator 需要将对应的 Todo Exploration 分配给合适的 Explorer。
    - Decider 可能返回少于请求数量的新探索方向，这意味着当前无足够的高价值探索方向。分配已有的 Todo 探索并等待已有探索结束或外部其他变化，不在相同 Subject 状态下重复调用 Decider。
    - Decider 可能会返回等待决策（`wait` 决策），这同样意味着当前无足够的高价值探索方向。Coordinator 也应该等待已有探索结束或外部其他变化。
    - Decider 可能返回结束决策（`complete` 决策），这意味着当前已经有足够证据支持完成结束任务的结论（虽然不一定是成功结论）。Coordinator 应该先结束所有仍在进行中的 Exploration 并通过 `task.close` 登记任务结果，并将 Decider 的结束结果与结束理由向用户同步。
  - 等待已有探索结束或外部其他变化时，应在 Explorer Agent 状态变化、Decider 返回或用户输入到达等情况时，确认当前状态并继续调度。等待事件时单次阻塞不得超过一分钟。
  - 在任务执行过程中，用户发送消息时，根据消息类型进行对应处理：
    - 任务终止/取消类消息，Coordinator 应在确认终止意图后，关闭进行中的 Explorer SubAgent 并以终止理由调用 `task.close` 动作。
    - 任务提示/要求类消息，Coordinator 应提取输入信息中的有效信息，创建对应的用户来源的断言认知。面向整体 Subject 的信息创建为全局用户提示断言认知，面向某个探索的信息创建为探索提示断言认知。如果创建了探索提示断言认知且对应探索正在进行中，额外的向对应 Explorer 发送消息来投递此认知。
  - Explorer 状态异常（失联、超时或 SubAgent 已结束但未协议登记）时，Coordinator 应核对最新协议和 Runtime 状态后，以 `failed` 或 `cancelled` 为结果对此 Exploration 调用 `exploration.finish`。是否继续该方向以及是否 retry 由 Decider 基于最新状态决定，禁止原地重新 assign；若 Explorer 已提交 finish，则直接使用已提交结果，不重复结束。

## 角色通信

角色通信通过 Runtime 动作进行，如 `SendMessage` 工具调用。

`SendMessage` 工具支持两类发送消息的 `to` 参数指定：

- `main`：SubAgent 向 MainAgent 发送消息。在 Norns 模式下，Coordinator 即 MainAgent。
- `<SubAgent_ID>`：SubAgent 创建后返回结果的 `agentId` 字段。也可以通过协议动作 `exploration.running` 获取执行中的 Explorer 的 `<SubAgent_ID>`。

通信问讯可能在 Exploration 执行超时、外部输入探索相关断言认知、任务超高价值派生认知、并行探索必要通信等场景时发生。

### 探索超时场景

- Coordinator 会获得 Runtime 下发的 Exploration 的超时问讯时间，并结合 `exploration.running` 返回的 `duration_seconds` 运行时长字段判断是否达到超时问讯时间。
- 当探索执行时间达到 Runtime 配置中的超时问询周期时， Coordinator 会向对应 Explorer 发送消息，询问本次 Exploration 是否仍有必要继续；如果没有必要，要求 Explorer 尽快正常结束。
- Exploration 应结合当前探索进展和产出价值确定是否要继续。如果没有继续价值，Explorer 会结算剩余 Cognition 并结束当前探索；如果仍有必要继续，Explorer 会向 Coordinator 发送消息，说明能够产生高价值结果的具体理由，然后继续执行。
- Explorer 回复继续探索时，此探索记为一次超时问讯。运行时长每达到超时问讯时间的下一个整数倍时再次进行超时问讯处理。第三次达到问询条件时 Coordinator 将不再问询，而是直接终止该 Explorer，并对此 Exploration 调用 `exploration.finish` 声明本次探索结果为 `cancelled`。

### 探索相关认知场景

- Coordinator 从用户输入中提取到与 Subject 全局相关或与特定 Exploration 相关的提示、约束等有效信息时，将创建用户断言认知。
- 当 Coordinator 认为创建的用户断言 Cognition 或其他方式获取的 Cognition 与某些进行中的 Exploration 相关时，会将认知发送给对应的 Explorer 以协助有关探索方向的推进。

### 任务超高价值派生认知场景

- 当 Explorer 在探索过程中发现能够直接决定 Task 关闭的超高价值认知时，除了将其创建时标记为 `task-purpose-relevant`，还可以选择直接向 Coordinator 发送消息以告知此超高价值认知。
- Coordinator 在接受到此消息后，可以选择在对应 Explorer 尚未返回的情况下，调用 Decider 判断任务是否已经完成。
- 如果 Decider 返回 `complete` 决策，则 Coordinator 会发送消息告知各 Explorer 任务已达成，并要求它们尽快提交剩余 Cognition 然后自行 `exploration.finish` 当前探索。
- 即使 Decider 未返回 `complete` 决策，Coordinator 也可以自行决定是否将此 Cognition 按探索相关断言认知发送给其他非来源 Explorer 的 SubAgent，以协助有关探索方向的推进。

### 并行探索必要通信场景

- 当发生并行进行的探索之间的负面相互影响、目标的统一巨大变化等有必要进行与其他 Explorer 通信的场景，Explorer 可按需查询 `exploration.running` 以获取并行 Explorer ID 并发起通信。
- 谨慎的进行并行探索间通信，防止对其他探索方向造成阻碍或目标偏离。

## 完整性说明

本 Skill 只提供 Runtime 执行所需的必要协议上下文。

需要核对完整字段、状态转换、输出结构或边界条件时，参考 [protocol.md](references/protocol.md)，该文件是完整协议规范。
