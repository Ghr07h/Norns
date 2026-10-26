# Norns 协议

状态：正式版  
协议版本：2

## 1. 协议内容

Norns 是一套面向开放任务求解的多智能体协作协议，主要面向由支持多智能体系统（MAS）的 Code Agent 承载的 Runtime。Coordinator 响应协议状态变化和外部事件，基于最新 Graph 与执行状态按需调度 Decider 和 Explorer，持续开展探索并积累可复用的 Cognition，直到当前 Task 形成有依据的结论，或确认在当前条件下无法继续推进。

Norns 将开放任务求解中的共享知识、探索方向及其溯源关系从单次智能体会话上下文中分离，以 Workspace 协议状态持久化，使不同 Runtime 能够观察、恢复和复用任务状态。平台绑定可以在此基础上注入领域知识、Agent 和工具，形成面向特定领域的任务求解套件。

Norns 协议实现即完整支持 Norns 协议的可执行系统。协议不规定协议实现的内部边界、通信方式或部署形式。

本规范定义：

- Norns 协议的基础语义，包括：状态容器 Workspace、目标实体 Subject、状态记录 Graph、状态投影 View 和可选能力扩展 Domain
- Norns 协议的核心实体 Task、Cognition、Exploration 的语义、关系与生命周期
- Norns 协议的 MAS 智能体角色 Coordinator、Decider 与 Explorer 的职责与协作逻辑
- Norns 协议的协议动作与事件驱动的调度和执行过程

本规范不定义承载 Runtime 的具体 Code Agent 或执行平台、数据库实现、文件布局、CLI、HTTP API、具体工具、领域知识内容、部署方式或评测方法。这些内容属于具体实现或平台绑定。

## 2. 规范约定

本文使用以下约束等级：

- 必须（MUST）和禁止（MUST NOT）表示协议不变量；
- 应该（SHOULD）和不应该（SHOULD NOT）表示推荐行为，只有在存在明确理由时才可偏离；
- 可以（MAY）表示可选行为。

协议版本与兼容性：

- 协议使用协议版本管理变更，不承诺协议版本兼容性。
- 协议版本由协议文档配套的 `manifest.json` 的 `protocol_version` 字段声明。
- 协议实现必须准确声明所支持的协议版本，且不得修改协议版本不受支持的 Workspace。
- 协议版本与实现的产品版本、存储 schema 版本相互独立。

协议实体 ID 由协议实现生成，调用方不能指定。格式如下：

- Task：`task_<uuid-v4>`；
- Cognition：`cog_<uuid-v4>`；
- Exploration：`exp_<uuid-v4>`。

其他约定

- UUID 使用小写、带连字符的标准表示。
- 协议时间使用 UTC RFC 3339。

## 3. 核心模型

### 3.1 Norns Workspace 与 Subject

Workspace 是 Norns 协议状态的逻辑容器，一个 Workspace 只描述一个 Subject。Subject 是一组 Task 共同针对的目标实体，不是独立的协议实体或 Graph 节点。Workspace 中持续积累的 Task 与 Graph 共同形成对 Subject 的描述和求解记录。

- 一个 Workspace 中的全部 Task 必须针对同一个 Subject。
- 一个 Workspace 可以先后包含多个 Task，同一时间最多存在一个被打开的 Task。
- Graph 归属于 Workspace，并跨 Task 持续积累和复用。
- 所有协议状态变更必须由协议实现以原子方式完成。

初始化只建立空 Workspace，不创建 Task、Cognition 或 Exploration。结束当前 Subject 并开始新的 Subject，需要由协议实现创建或重置为另一个 Workspace；该物理生命周期不属于协议语义。

例如在数学领域中，Subject 可以是一项待验证的数学命题。多个 Task 可以从证明、反例或适用条件等不同目的推进求解，其 Exploration 与 Cognition 持续积累为同一 Workspace 的 Graph。

### 3.2 Norns Runtime

Norns Runtime 是协议实现针对一个 open Task 形成的动态运行状态及执行上下文。Runtime 可以依托进程、会话或其他智能体平台执行单元存在；具体映射和组件管理方式由协议实现确定。

Runtime 不是 Graph 节点，也不是必须持久化的协议实体，但 Runtime 对 Workspace 的占用属于协议状态。同一 Workspace 同一时间最多由一个 Runtime 占用，一个 open Task 同一时间最多存在一个活动的 Norns 调度过程。

Runtime 存续期间，协议实现负责：

- 承载 Coordinator、Decider 与 Explorer；
- 组织对 Subject 的持续探索过程；
- 调用 Norns 协议动作，对核心实体进行操作。

Runtime 异常退出后，协议实现必须隔离遗留执行单元，并完成协议状态对齐与收尾。

协议实现必须原子维护 Runtime 占用及相关协议状态，并负责启动、观察和停止实际智能体。打开新 Task 或重新打开 closed Task 时必须设置 Runtime 占用；关闭 Task 时必须清除占用。具体职责如何分配给协议实现的内部组件不属于协议。协议不提供脱离 Task 生命周期、单独修改 Runtime 占用的动作。

### 3.3 Task

Task 表示针对 Workspace Subject 的一次具体求解目的。Purpose 描述本次 Task 要达成需求；Subject 描述 Purpose 所针对的目标实体，两者必须保持独立。

| 字段 | 约束 |
| --- | --- |
| `id` | 唯一 Task ID。 |
| `purpose` | 非空的任务目的。 |
| `init_cognition_ref` | 描述本 Task 起始 Subject 上下文的 Task-init Cognition ID。 |
| `max_concurrency` | 正整数，表示额定 Exploration 并发数。 |
| `status` | `open` 或 `closed`。 |
| `outcome` | closed 时的结构化结果；open 时为空。 |
| `outcome_reason` | closed 时非空的结果说明与依据；open 时为空。 |
| `created_at` | 创建时间。 |
| `closed_at` | closed 时的关闭时间；open 时为空。 |

第一个 Task 必须创建一条描述 Subject 的 Task-init Cognition，并通过 `init_cognition_ref` 引用它。后续 Task 可以复用既有 Task-init Cognition，也可以创建新的 Task-init Cognition，并通过 `origin_ref` 引用既有 Task-init Cognition，以补充或扩展 Subject 信息。Task-init Cognition 只能描述 Subject，不能混入 Task Purpose。

`max_concurrency` 由 Runtime 在打开 Task 时传入，表示 Runtime 调度 Explorer 的额定并发数，不是协议状态写入的硬上限。实际 running Exploration 数量超过该值时，协议实现应该产生软警告，但不得因此回滚其他合法状态变更。

Task outcome：

| 值 | 含义 |
| --- | --- |
| `succeeded` | 已达成 Purpose。 |
| `unachievable` | 现有 Graph 证据表明在当前条件下无法达成 Purpose。 |
| `failed` | Runtime、工具或内部异常使 Norns 调度过程无法可靠继续。 |
| `cancelled` | 在形成明确结果前被用户或外部系统终止。 |

Task 生命周期：

```text
             reopen
       +----------------+
       |                |
       v                |
     open ----------> closed
          close(outcome, outcome_reason)
```

Task 必须满足以下约束：

- open Task 的 `outcome`、`outcome_reason` 和 `closed_at` 必须为空，且 Workspace 必须处于 Runtime 占用状态。
- closed Task 的 `outcome`、`outcome_reason` 和 `closed_at` 必须非空，且 Workspace 不得处于 Runtime 占用状态。
- 关闭 Task 时，必须将该 Task 剩余的 todo 和 running Exploration 结束为 `cancelled`；实际 Explorer 必须已经停止或确认不能继续写入。
- 任意 outcome 的 closed Task 都可以在没有其他 open Task 时 reopen。
- reopen 保留 Task 固有字段和现有 Graph，清空关闭字段，不自动恢复或 retry 已结束的 Exploration。
- 对已经是唯一 open Task 且 Runtime 占用状态正确的 Task，reopen 必须幂等成功。

### 3.4 Cognition

Cognition 是写入 Graph 的不可变信息单元，用于表示 Subject 信息、用户提供的事实或提示，以及 Explorer 基于证据形成的发现。Cognition 必须包含可供其他角色复用的明确内容；尚待验证的假说应该表现为 Exploration，而不是 Cognition。

| 字段 | 约束 |
| --- | --- |
| `id` | 唯一 Cognition ID。 |
| `title` | 非空的简短描述。 |
| `content` | 非空的完整信息。 |
| `task_id` | 引入该 Cognition 的 Task ID；Workspace 全局 Cognition 为空。 |
| `origin` | `task`、`user` 或 `exploration`。 |
| `origin_ref` | 按来源类型引用 Cognition、Exploration 或为空。 |
| `task_purpose_relevant` | 是否直接支撑、改变或限制对应 Task Purpose 的结论。 |
| `created_at` | 创建时间。 |

Cognition 来源组合：

| 场景 | `origin` | `origin_ref` | `task_id` |
| --- | --- | --- | --- |
| 首个 Task 的 Task-init Cognition | `task` | 空 | 新 Task ID |
| 后续 Task 的新 Task-init Cognition | `task` | 既有 Task-init Cognition ID | 新 Task ID |
| Workspace 全局用户 Cognition | `user` | 当前 Task 的 `init_cognition_ref` | 空 |
| Exploration 用户提示 | `user` | Exploration ID | 添加提示时的 open Task ID |
| Explorer 发现 | `exploration` | Exploration ID | Exploration 的 Task ID |

Cognition 必须满足以下约束：

- Cognition 创建后不得修改或删除。
- 协议不定义 Cognition 的显式失效、替代、反驳或 supersede 关系；新 Cognition 可以补充或隐含修正旧 Cognition，但旧 Cognition 仍保留在 Graph 中。
- Workspace 全局用户 Cognition 只能在存在 open Task 时创建，并对该 Workspace 的所有 Task 可见。
- Explorer 发现只能写入当前 open Task 中处于 todo 或 running 状态的 Exploration。
- 任意状态的 Exploration 都可以接收用户提示。
- Explorer 发现和 Exploration 用户提示都构成对应 Exploration 的 output Cognition。
- `task_purpose_relevant` 默认为 false，创建后不得修改。
- Task-init Cognition 与 Workspace 全局 Cognition 的 `task_purpose_relevant` 必须为 false。
- 只有直接支撑、改变或限制对应 Task Purpose 结论的 Cognition 才能将 `task_purpose_relevant` 标记为 true。

### 3.5 Exploration

Exploration 是获取新知识或验证假说的可执行探索方向。它以一个或多个 Cognition 作为来源，并可以产生零个或多个 output Cognition。

| 字段 | 约束 |
| --- | --- |
| `id` | 唯一 Exploration ID。 |
| `title` | 非空的简短描述。 |
| `content` | 非空的探索目标与范围。 |
| `task_id` | 创建该 Exploration 的 Task ID。 |
| `source_cognitions` | 有序、不重复且非空的 Cognition ID 列表。 |
| `retry_of` | 被 retry 的 Exploration ID；普通创建时为空。 |
| `status` | `todo`、`running` 或 `finished`。 |
| `executor_id` | 实际执行实例的不透明标识。 |
| `output_ref` | 实际执行输出的引用；若传入值指向可解析的符号链接，assign 保存链接最终目标的绝对路径，否则保留传入值。 |
| `assigned_at` | 分配给执行实例的时间。 |
| `outcome` | finished 时的结构化结果。 |
| `outcome_reason` | finished 时非空的执行结果说明。 |
| `created_at` | 创建时间。 |
| `finished_at` | 完成时间。 |

Exploration outcome：

| 值 | 含义 |
| --- | --- |
| `succeeded` | 已完成探索目标并形成明确结论。 |
| `inconclusive` | 已正常完成探索，但现有信息不足以形成明确结论。 |
| `failed` | 因工具、权限或内部异常未能正常完成。 |
| `cancelled` | 被 Coordinator、用户或外部系统中止。 |

Exploration 生命周期：

```text
add / retry          assign                 finish
     |                  |                      |
     v                  v                      v
   todo ------------> running ------------> finished
     |                                         |
     +---------- finish(cancelled) ----------->+
                                               |
                                               | retry
                                               v
                                          new todo
```

Exploration 必须满足以下约束：

- add 与 retry 创建 todo Exploration；其执行字段和完成字段必须为空。
- assign 将 todo Exploration 转为 running，并写入非空的 `executor_id`、`output_ref` 和 `assigned_at`。
- running Exploration 可以使用任意合法 outcome 转为 finished。
- todo Exploration 只能直接以 `cancelled` outcome 转为 finished，且执行字段必须保持为空。
- finished Exploration 必须具有 `outcome`、非空的 `outcome_reason` 和 `finished_at`。
- Exploration 的定义字段、`source_cognitions` 和已经写入的执行字段不得修改。
- Exploration outcome 只描述本次执行，不改变此前 output Cognition 的协议状态。
- 可复用的探索发现必须在 Exploration 完成前写为 Cognition，不能只存在于 `outcome_reason`。

retry 必须创建具有新 ID 的 todo Exploration，不得修改来源 Exploration。新 Exploration 复制来源 Exploration 的 `title`、`content` 和有序 `source_cognitions`，将 `retry_of` 指向任意历史 Task 中已经 finished 的 Exploration，并归属于 retry 时的 open Task。`retry_of` 引用必须无环；同一来源 Exploration 可以产生多个 retry 分支。

### 3.6 Graph

Graph 是 Workspace 中关于 Subject 的权威共享知识状态，由 Cognition、Exploration 及其有向关系组成：

```text
Cognition --source--> Exploration --output--> Cognition
Cognition --extends-----------------------> Cognition
```

| 关系 | 来源 | 目标 | 唯一事实源 |
| --- | --- | --- | --- |
| `source` | Cognition | Exploration | `Exploration.source_cognitions` |
| `output` | Exploration | Cognition | 指向 Exploration 的 `Cognition.origin_ref` |
| `extends` | Cognition | Cognition | 指向 Cognition 的 `Cognition.origin_ref` |

Graph 关系由节点字段派生，不是独立协议实体。协议实现不得创建需要与节点字段双向同步的关系事实源。

Graph 必须满足以下约束：

- Graph 必须保持有向无环。
- Task 不是 Graph 节点；Task 通过 Cognition 和 Exploration 的字段与 Graph 关联。
- `retry_of` 表示 Exploration 的执行沿袭，不是 Graph 关系。
- Exploration 的 `outcome` 和 `outcome_reason` 是执行结果，不自动形成 output Cognition。
- Agent 对话、私有笔记、工具输出和 Runtime 日志只有按本协议写为 Cognition 或 Exploration 状态后，才能成为共享协议事实。

### 3.7 View

View 是协议实现根据当前 Workspace 状态生成的只读投影，用于向智能体角色或人类提供与其用途相匹配的协议上下文。View 不是持久化协议实体，也不是 Graph 节点；生成 View 不得修改 Workspace 协议状态。

View 表示生成时的当前协议状态，不表示历史快照。选择历史 Task 只改变 View 使用的 Task 上下文，不会将 Graph 回退到历史状态。

Norns 定义三类 View：

- Status View：向 Decider 提供整个 Subject 的当前求解状态。
- Trace View：向 Explorer 提供一个 Exploration 的最小充分上下文。
- Visual View：向人类提供完整 Subject、Task 和 Graph 的可观察投影。

Status View 和 Trace View 是结构化 View，使用英文 `snake_case` 字段和 JSON 兼容值。列表必须稳定排序：Task、Cognition 与 Exploration 默认按 `created_at ASC, id ASC` 排序，`source_cognitions` 保留输入顺序。所有 View 必须包含或关联同一次生成操作的 `generated_at`。

#### 3.7.1 Status View

Status View 供 Decider 获取整个 Subject 的当前状态。

- 显式选择 Task 时，以该 Task 作为 View Task。
- 未显式选择且存在 open Task 时，以该 open Task 作为 View Task。
- 未显式选择且不存在 open Task 时，顶层返回全部 Task，不选择单个 View Task。
- Graph 始终表示 Workspace 当前完整 Graph，不是只属于 View Task 的子图。

Status View 的顶级结构按以下顺序包含：

1. 单个 `task` 或全部 `tasks`；
2. `graph.cognitions` 与 `graph.explorations`；
3. `global_cognitions`；
4. `todo_explorations`；
5. `running_explorations`；
6. `generated_at`。

没有选择单个 View Task 时，`todo_explorations` 和 `running_explorations` 必须为空列表。todo 摘要包含 `id` 和 `title`；running 摘要包含 `id`、`title`，以及基于同一个 `generated_at` 和 `assigned_at` 计算的非负整数 `duration_seconds`。

Status View 使用以下精简投影：

- Task：`id`、`purpose`，以及非空的 `outcome`、`outcome_reason` 和 `closed_at`。
- Cognition：`id`、`content`、非空的 `origin_ref`、条件性的 `task_purpose_relevant` 和 `created_at`；不输出 `origin`。
- Exploration：`id`、`content`、非空的 `retry_of`、`status`、非空的结果字段、`source_cognitions` 和非空的 `finished_at`；不输出执行字段或 output 镜像。
- Workspace 全局 Cognition：`id`、`content` 和 `created_at`。

结构化 View 不得输出值为 false 的 `task_purpose_relevant`。存在单个 View Task 时，只为属于该 Task 且存储值为 true 的 Cognition 输出该字段；返回全部 Task 时，为所有存储值为 true 的非全局 Cognition 输出该字段。

#### 3.7.2 Trace View

Trace View 供 Explorer 获取一个 Exploration 的最小充分上下文。其 Task 上下文始终由目标 Exploration 的 `task_id` 决定，不接受独立的 Task 选择。

Trace View 的顶级结构按以下顺序包含：

1. `current_exploration`：目标 Exploration 的 `id`、`title` 和 `content`；
2. `origin_task`：目标 Exploration 所属 Task 的 Purpose，以及当前只需完成该 Exploration 的提示；
3. `related_subgraph`：与目标 Exploration 相关的 Cognition 与 Exploration；
4. `global_cognitions`：全部 Workspace 全局 Cognition；
5. `generated_at`。

`related_subgraph` 必须包含：

- 目标 Exploration、其全部 source Cognition 与全部 output Cognition；
- 追溯 source Cognition 所需的最小祖先子图；
- 目标 Exploration 的用户提示；
- 属于目标 Exploration 所属 Task 且 `task_purpose_relevant=true` 的额外 Cognition；
- 完整的 `retry_of` 链，以及链上每次执行的全部 output Cognition。

最小祖先子图从每个 source Cognition 反向遍历：沿 `extends` 关系继续追溯；遇到 `output` 关系时，加入来源 Exploration 及其全部 source Cognition，再继续追溯。不得加入与证明当前 source Cognition 无关的旁支 output；`retry_of` 链是例外，必须保留链上每次执行的全部 output Cognition。

Trace View 对 Cognition、Exploration 和 Workspace 全局 Cognition 使用与 Status View 相同的精简投影，但不输出任何 `task_purpose_relevant` 字段，也不包含 `todo_explorations` 或 `running_explorations`。

#### 3.7.3 Visual View

Visual View 供人类观察完整 Subject。它不接受 Task 选择，必须表达全部 Task、当前完整 Graph、Graph 关系、Exploration 状态、Runtime 占用状态，以及所有 Task 的 Purpose 相关 Cognition。

Visual View 的布局、交互、视觉编码和传输格式不属于协议。协议实现必须保证 Visual View 的信息来源与生成时的当前协议状态一致。

### 3.8 Domain

Domain 是对 Norns 基础协议能力的可选扩展，用于为决策和探索提供特定领域的知识与工具。Domain 不是持久化协议实体或 Graph 节点，不构成 Workspace 协议状态，也不引入新的协议动作；Runtime 未启用 Domain 时仍可以完整运行 Norns 协议。

Domain 可以提供领域化的 Decider 和 Explorer。领域化角色仍分别承担第 5.3 节和第 5.4 节定义的逻辑角色，必须遵守相同的职责边界、协议动作和状态约束。Domain 不扩展或替换 Coordinator，也不得改变 Coordinator 的职责、调度语义或权限。

Domain 提供的知识、指令和工具输出本身不是共享协议事实。只有按照本协议写为 Cognition 或实体状态的信息，才能进入 Graph 并供其他角色使用。Domain 的名称、内容结构、工具接口、选择方式、配置和注入机制属于平台绑定，不由协议规定。

## 4. 协议动作

协议动作是协议实现必须提供的抽象能力。本节只定义动作的语义、前置条件和结果，不规定 CLI 命令、API、通信方式或内部调用边界。

### 4.1 通用动作语义

- 每个修改动作对 Workspace 协议状态的变更必须是原子的。
- 停止实际执行单元、投递消息等外部操作不属于协议状态事务；动作定义要求这些操作先完成时，协议实现必须在确认其完成或已经隔离后再提交状态变更。
- 被拒绝的动作不得留下部分协议状态。
- 未知实体、非法状态或引用、无效输入、协议版本不受支持等失败必须能够被调用方区分；具体错误编码和传输形式不属于协议。
- 动作产生的实体 ID 和协议时间必须由协议实现按照第 2 节生成。
- 修改动作完成后，如果当前 Task 的 running Exploration 数量超过 `max_concurrency`，协议实现应该返回软警告，但不得因此回滚其他合法状态变更。

### 4.2 Workspace 与 Task 动作

#### `workspace.initialize`

建立一个不包含 Task、Cognition 或 Exploration 的空 Workspace。对已经存在且协议版本受支持、状态合法的 Workspace 重复执行时，必须幂等成功；不得以初始化覆盖或修复未知、不兼容或状态不可信的 Workspace。

#### `task.open`

打开新 Task，并在同一原子变更中完成以下操作：

1. 创建 open Task，写入非空 `purpose`、正整数 `max_concurrency` 和生成的 `created_at`；
2. 创建或选择该 Task 的 Task-init Cognition，并写入 `init_cognition_ref`；
3. 设置 Workspace 的 Runtime 占用状态。

首个 Task 必须接收新的 Subject 信息并创建 Task-init Cognition。后续 Task：

- 未提供新的 Subject 信息时，复用最近关闭 Task 的 `init_cognition_ref`；最近关闭 Task 按 `closed_at DESC, id ASC` 确定。
- 提供新的 Subject 信息时，创建新的 Task-init Cognition；如果已经存在 Task-init Cognition，其 `origin_ref` 必须指向最新一条，最新记录按 `created_at DESC, id ASC` 确定。

存在 open Task，或 Workspace 已被 Runtime 占用但不存在 open Task 时，动作必须失败。

#### `task.close`

关闭当前 open Task。调用方必须先停止或隔离该 Task 的实际 Explorer，保证其不能继续写入，再提交本动作。本动作必须在同一原子变更中：

- 将该 Task 剩余的 todo 和 running Exploration 以 `cancelled` 结束；
- 写入 Task 的 `outcome`、非空 `outcome_reason` 和 `closed_at`；
- 清除 Workspace 的 Runtime 占用状态。

不存在 open Task 时，动作必须失败。

#### `task.reopen`

在不存在其他 open Task 时重新打开指定的 closed Task，并设置 Runtime 占用。动作必须保留 Task 的 `purpose`、`init_cognition_ref`、`max_concurrency` 和既有 Graph，清空 `outcome`、`outcome_reason` 与 `closed_at`，且不得自动恢复或 retry 已结束的 Exploration。

目标已经是唯一 open Task 且 Runtime 占用状态正确时，动作必须幂等成功。目标为 open 但占用状态不正确，或存在其他 open Task 时，动作必须失败。

### 4.3 Cognition 动作

#### `cognition.add`

创建不可变 Cognition。该动作支持以下来源：

- Workspace 全局用户 Cognition：要求存在 open Task；`origin=user`，`origin_ref` 指向当前 Task 的 `init_cognition_ref`，`task_id` 为空。
- Exploration 用户提示：`origin=user`，`origin_ref` 指向目标 Exploration，`task_id` 为添加提示时的 open Task ID；任意状态的 Exploration 都可以接收提示。
- Explorer 发现：`origin=exploration`，`origin_ref` 指向目标 Exploration，`task_id` 为该 Exploration 的 Task ID；目标必须属于当前 open Task 且处于 todo 或 running 状态。

Task-init Cognition 只能作为 `task.open` 的一部分创建，不能通过本动作单独创建。协议实现向实际 Explorer 投递已经持久化的用户提示属于事务外行为；投递失败不得回滚 Cognition，也不得使其失去协议事实地位。

### 4.4 Exploration 动作

#### `exploration.add`

在当前 open Task 中创建 todo Exploration。`title` 和 `content` 必须非空，`source_cognitions` 必须是有序、不重复且均存在的 Cognition ID 列表；未显式提供来源时，使用当前 Task 的 `init_cognition_ref` 作为唯一来源。

#### `exploration.assign`

将当前 open Task 中指定的 todo Exploration 转为 running，并写入非空的 `executor_id`、`output_ref` 和生成的 `assigned_at`。如果 `output_ref` 指向可解析的符号链接，写入链接最终目标的绝对路径；普通文件、非路径引用、已断开的链接或解析失败的路径保留传入值。目标不属于当前 open Task 或不处于 todo 状态时，动作必须失败。

#### `exploration.finish`

结束当前 open Task 中指定的 Exploration，并写入合法 `outcome`、非空 `outcome_reason` 和生成的 `finished_at`。running Exploration 可以使用任意合法 Exploration outcome；todo Exploration 只能以 `cancelled` 结束。finished Exploration 不得再次结束。

#### `exploration.retry`

以任意历史 Task 中指定的 finished Exploration 为来源，在当前 open Task 中创建新的 todo Exploration。新 Exploration 必须复制来源的 `title`、`content` 和有序 `source_cognitions`，并将 `retry_of` 指向来源 Exploration。该动作不得修改或重新运行来源 Exploration。

#### `exploration.list_todo` 与 `exploration.list_running`

两个动作分别查询当前 open Task 的 todo 和 running Exploration，用于恢复和驱动 Exploration 调度。它们是 Exploration 集合查询，不是第 3.7 节定义的 View。不存在 open Task 时，动作必须返回可区分的失败。

查询结果必须包含：

- 当前 Task 的 `task_id` 和 `max_concurrency`；
- 按 `created_at ASC, id ASC` 稳定排序的 `explorations`；
- 每条 Exploration 的 `id`、`title`、`content`、`source_cognitions`、非空的 `retry_of` 和 `created_at`；
- running Exploration 额外包含 `executor_id`、`output_ref`、`assigned_at`，以及基于同一次查询时间计算的非负整数 `duration_seconds`。

### 4.5 View 动作

所有 View 动作均为只读动作，不得修改 Workspace 协议状态。

#### `view.status`

按照第 3.7.1 节生成 Status View，可以显式选择 Task。

#### `view.trace`

按照第 3.7.2 节为指定 Exploration 生成 Trace View。目标 Exploration 必须存在，且不接受独立的 Task 选择。

#### `view.visual`

按照第 3.7.3 节生成 Visual View，不接受 Task 选择。

## 5. 协议角色

Norns 定义 Coordinator、Decider 和 Explorer 三种逻辑角色。角色不是持久化协议实体或 Graph 节点，也不要求与模型、进程、会话或智能体实例一一对应；协议实现可以采用任意内部结构承载角色，但必须保持本节定义的职责边界。

### 5.1 通用角色约束

- 角色必须通过协议动作读取或修改 Workspace 协议状态。
- 角色的对话、私有推理、临时笔记和工具输出不是共享协议事实；只有写入 Graph 或实体状态的信息才能被其他角色作为事实使用。
- 角色必须基于动作执行时的最新协议状态工作，不得假定先前读取的 View 或查询结果仍然有效。
- 角色之间可以由协议实现直接调用，也可以通过消息或其他平台机制协作；具体承载和通信格式不属于协议。

### 5.2 Coordinator

Coordinator 负责一个 open Task 的整体调度和生命周期协调。Runtime 中必须存在承担该职责的逻辑单元。

Coordinator 必须：

- 建立或恢复 Task，并始终以 Workspace 的当前协议状态作为调度依据；
- 根据 Task 的 `max_concurrency`、todo 和 running Exploration，以及实际执行状态计算可用调度能力；
- 按需调用 Decider，并保证同一 Task 同一时间最多存在一个活动的 Decider；
- 为 todo Exploration 启动 Explorer，并通过 `exploration.assign` 登记实际执行引用；
- 对齐 running Exploration 与实际 Explorer，处理完成、失联、失败或取消；
- 将已经持久化的 Exploration 用户提示转交仍在运行的对应 Explorer；
- 在关闭 Task 前停止或隔离全部相关 Explorer，再通过 `task.close` 提交最终状态。

Coordinator 不应亲自执行 Exploration，也不得通过 `exploration.add` 或 `exploration.retry` 自行创建 Exploration，不得以未写入 Graph 的信息作为 Task 完成依据。Explorer 的单次失败不必然导致 Task 失败；Coordinator 必须结束或隔离异常执行单元，并通过重新调用 Decider 取得后续探索决策。

### 5.3 Decider

Decider 基于最新 Status View 评估当前 Task 的求解状态，并给出以下一种逻辑决策：

- `explore`：创建一个或多个新的 todo Exploration；
- `wait`：当前没有必要创建新的 Exploration，等待正在进行的工作或明确的外部事件；
- `complete`：现有 Graph 已足以支持 Task 达成或不可达的结论。

Decider 必须满足以下约束：

- 只负责选择和创建 Exploration，不执行 Exploration；
- 创建 Exploration 可以通过 `exploration.add` 创建，也可以通过 `exploration.retry` 复制已存在的 finished Exploration；创建的 Exploration 将随这两类操作写入 Task；创建的 Exploration 必须具有明确、相互可区分的目标；
- 创建的 Exploration 数量不得超过 Coordinator 提供的可用调度能力；
- `explore` 可以创建少于可用调度能力的 Exploration；这表示 Decider 认为在当前信息状态下没有更多值得立即执行的高价值方向，剩余调度能力不要求被填满；
- 存在尚未完成的 Exploration 时仍可以基于最新状态再次决策，不得将一组 Exploration 视为必须整体完成的固定轮次；
- `wait` 只能用于存在正在进行的工作或明确外部状态变化来源的情况；
- `complete` 只能建议 `succeeded` 或 `unachievable`，且结论与依据必须能够由当前 Graph 支持。

Decider 不得直接关闭 Task。Coordinator 必须在重新核对当前协议状态并完成执行单元收尾后决定是否接受完成建议。

### 5.4 Explorer

Explorer 每次只执行一个指定的 Exploration，并以该 Exploration 的 Trace View 作为共享协议上下文。

Explorer 必须：

- 只以 `current_exploration` 描述的目标和范围开展工作；
- 将可靠、可复用的发现及时通过 `cognition.add` 写为 Explorer Cognition；
- 在执行结束时通过 `exploration.finish` 报告 `succeeded`、`inconclusive` 或 `failed`，并提供非空的执行过程与结果说明；
- 将支撑 Task Purpose 的发现按第 3.4 节准确标记为 `task_purpose_relevant`。

Explorer 不得创建新的 Exploration、决定 Task outcome 或关闭 Task。`outcome_reason` 只描述本次 Exploration 的执行结果，不能替代应写入 Graph 的 Cognition。

## 6. 事件驱动调度与恢复

Norns 的调度发生在 Runtime 中，由 Coordinator 响应协议状态变化和外部执行事件进行。调度过程不是持久化协议实体，也不形成额外的 Graph 状态。

### 6.1 调度原则

- Norns 不划分固定的 Decide/Explore 轮次，也不设置等待一组 Exploration 全部完成的同步屏障。
- 一次 Decider 调用创建的多个 Exploration 不构成协议批次，每个 Exploration 独立进入 todo、running 和 finished 状态。
- 调用下一次 Decider 不以前次创建的全部 Exploration 都已 finished 为前提；其他 Explorer 仍在运行时，Coordinator 可以基于最新状态再次调用 Decider。
- Decider 返回的 Exploration 少于可用调度能力时，Coordinator 不得仅因仍有空闲能力就在相同信息状态下再次调用 Decider；再次调用前必须出现新的 Cognition、Exploration 完成状态或与当前 Task 相关的外部输入。
- 每次决策与调度都必须以当前协议状态和实际执行状态为依据，不得依赖已失效的 View、缓存计数或先前事件顺序。
- `max_concurrency` 是 Explorer 的额定并发数。Coordinator 应该以其约束主动调度，但超过该值只触发第 3.3 节定义的软警告，不得自动取消合法的 running Exploration。
- open Task 不得仅因暂时没有新事件、没有 todo Exploration 或 Decider 返回 `wait` 而自动关闭。

### 6.2 调度事件与重新评估

以下情况必须使 Coordinator 重新评估调度：

- Runtime 启动、恢复或重新取得当前 Task 的控制权；
- Task 被打开或重新打开；
- Cognition 被创建，尤其是 Explorer 发现或 Exploration 用户提示；
- Exploration 被创建、分配、完成，或其实际 Explorer 的状态发生变化；
- Decider 返回 `explore`、`wait` 或 `complete`；
- 用户或外部系统提供与当前 Task 有关的新输入或终止请求；
- 协议实现发现协议状态与实际执行状态不一致。

协议实现可以合并重复事件，也可以采用通知、轮询或其他机制发现事件，但不得因为事件丢失或乱序而跳过对最新状态的校验。一次重新评估不要求产生新的协议状态；没有可执行工作时，Coordinator 应进入事件驱动等待。

### 6.3 调度与执行过程

每次重新评估时，Coordinator 必须先对齐 running Exploration 与实际 Explorer，再根据 `max_concurrency`、todo 和 running Exploration 以及实际执行状态计算可用调度能力。

Coordinator 按以下语义推进 Task，各步骤不构成必须完整执行一遍的固定循环：

1. 在可用调度能力内，为已有 todo Exploration 启动 Explorer。
2. 实际 Explorer 的 `executor_id` 和 `output_ref` 可用后，立即通过 `exploration.assign` 将对应 Exploration 登记为 running；如果 assign 失败，必须停止或隔离该 Explorer。
3. 仍有可用调度能力且没有活动 Decider，并且是首次决策或满足第 6.1 节的再次调用条件时，可以使用最新 Status View 调用 Decider。Decider 返回 `explore` 时，新增或 retry 的 Exploration 进入 todo，并参与后续调度。
4. Explorer 产生可靠发现时，必须先将其写为 Cognition；执行结束时，再通过 `exploration.finish` 提交本次执行结果。
5. Decider 返回 `wait` 时，Coordinator 不创建无依据的 Exploration，也不关闭 Task，而是等待其声明的进行中工作或外部状态变化。
6. Decider 返回 `complete` 时，Coordinator 必须重新核对当前协议状态和结论依据，停止或隔离全部实际 Explorer，再通过 `task.close` 关闭 Task；该动作会取消仍为 todo 或 running 的 Exploration。

用户或外部系统明确终止 Task 时，Coordinator 必须在完成相同的执行单元收尾后以 `cancelled` 关闭 Task。Runtime、工具或协议实现发生不可恢复且使调度无法可靠继续的异常时，必须完成收尾并以 `failed` 关闭 Task。这两类关闭不需要 Decider 的 `complete` 建议。

同一 Exploration 同一时间最多由一个实际 Explorer 执行。Explorer 失联、超时或异常结束后，Coordinator 必须先确认其已经停止或不能继续写入，再将对应 running Exploration 结束为 `failed` 或 `cancelled`，并将该状态变化作为重新评估事件。仍有可用调度能力时，Coordinator 可以调用 Decider；是否继续该方向以及是否通过 `exploration.retry` 创建新的 todo Exploration，由 Decider 基于最新协议状态决定。任何角色都不得原地重新 assign 已结束的 Exploration。

用户向 Exploration 提供提示时，必须先通过 `cognition.add` 持久化，再尝试投递给实际 Explorer。投递成功或失败都构成重新评估事件；投递失败不得删除或回滚已经创建的 Cognition。

### 6.4 Runtime 对齐与恢复

协议中的 Runtime 占用和 running Exploration 表示逻辑状态，不单独证明原 Runtime 或实际 Explorer 仍然存活。Runtime 启动或恢复时，协议实现必须先确认不存在仍能并发修改该 Workspace 的旧 Runtime，并将协议记录与实际执行单元逐项对齐。

对齐必须遵循以下规则：

- todo Exploration 没有已登记的实际执行实例；发现与其对应但尚未成功 assign 的 Explorer 时，必须停止或隔离该实例，再重新调度。
- running Exploration 存在唯一、可验证且可继续控制的实际 Explorer 时，可以恢复观察与调度。
- running Exploration 不存在可恢复的实际 Explorer 时，Coordinator 必须在确认原实例不能继续写入后，将其结束为 `failed` 或 `cancelled`，再重新评估调度并按需调用 Decider；是否另行 retry 由 Decider 决定。
- 同一 Exploration 对应多个实际 Explorer 时，必须停止或隔离多余实例；无法可靠确定唯一实例时，必须全部停止或隔离，并结束原 Exploration。
- closed Task 或 finished Exploration 对应的实际执行实例不得继续运行，必须停止或隔离。

协议实现能够安全建立唯一控制权并完成上述对齐时，可以继续当前 open Task。无法可靠恢复 Runtime 或保证遗留执行单元不再写入时，必须在完成隔离后以 `failed` 或 `cancelled` 关闭 Task。

继续一个已经关闭的相同 Purpose 时，可以通过 `task.reopen` 恢复原 Task；需要重新执行 finished Exploration 时，必须由 Decider 显式 retry，reopen 本身不得恢复或重新执行任何 Exploration。
