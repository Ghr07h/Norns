# Norns 协议

## 1. 协议内容

Norns 是一套面向开放任务求解的多智能体协作协议，主要面向由支持多智能体系统（MAS）的 Code Agent 承载的 Runtime。Coordinator 响应协议状态变化和外部事件，基于最新 Cognition/Exploration Status 与实际执行状态按需调度 Decider 和 Explorer，持续执行 Exploration 并积累可复用的 Cognition，直到当前 Task 形成有依据的结论或确认在当前条件下无法继续推进。

Norns 将开放任务求解中的共享知识、探索方向及其溯源关系从单次智能体会话上下文中分离，以 Workspace 协议状态持久化，使不同 Runtime 能够观察、恢复和复用任务状态。平台绑定可以在此基础上注入领域知识、Agent 和工具，形成面向特定领域的任务求解套件。

Norns 协议实现即完整支持 Norns 协议的可执行系统。协议不规定协议实现的内部边界、通信方式或部署形式。

本规范定义：

- Norns 协议的基础语义，包括：状态容器 Workspace、目标实体 Subject、共享协议状态 Cognition/Exploration Status、关系图模型 Graph、状态投影 View 和可选能力扩展 Domain
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
- 协议规范版本由仓库根目录 `versions.json` 的整数 `protocol` 字段声明。
- 协议实现必须准确声明所支持的协议版本，且不得修改协议版本不受支持的 Workspace。
- 协议版本与实现的产品版本、存储 schema 版本相互独立。

协议实体 ID 由协议实现生成，调用方不能指定。格式如下：

- Task：`task_<uuid-v4>`；
- Cognition：`cog_<positive-integer>`，例如 `cog_1`，断言认知与派生认知共用计数；
- Exploration：`exp_<positive-integer>`，例如 `exp_1`。

其他约定

- Task UUID 使用小写、带连字符的标准表示。
- Cog / Exp 在单个 Workspace 内分别从 1 递增，不补零，跨 Task 不重置；retry 也分配新的 Exp ID。创建实体与编号分配必须原子提交，并发创建不得重复；失败回滚后未提交的编号可复用。
- reset 后是独立的新 Workspace，Cog / Exp 编号重新从 1 开始，不继承归档的计数；编号仅在所属 Workspace 内标识实体。
- 数字编号为规范十进制正整数，范围为 1 到 9223372036854775807；拒绝 0、前导零、符号、空白和超范围值。计数耗尽必须失败，不得溢出或重用已提交的编号。
- 下文 ID 升序对 Cog / Exp 按数字后缀比较（如 cog_2 在 cog_10 前），Task 继续按 UUID 字符串比较。
- 协议时间使用 UTC RFC 3339。

## 3. 核心模型

### 3.1 Norns Workspace 与 Subject

Workspace 是 Norns 协议状态的逻辑容器，一个 Workspace 只描述一个 Subject。Subject 是一组 Task 共同针对的目标实体，不是独立的协议实体或 Graph 节点。Workspace 中持续积累的 Task 与 Cognition/Exploration Status 共同形成对 Subject 的描述和求解记录。

- 一个 Workspace 中的全部 Task 必须针对同一个 Subject。
- 一个 Workspace 可以先后包含多个 Task，同一时间最多存在一个被打开的 Task。
- Cognition/Exploration Status 归属于 Workspace，并跨 Task 持续积累和复用。
- 所有协议状态变更必须由协议实现以原子方式完成。

初始化只建立空 Workspace，不创建 Task、Cognition 或 Exploration。结束当前 Subject 并开始新的 Subject，需要由协议实现创建或重置为另一个 Workspace；该物理生命周期不属于协议语义。

例如在数学领域中，Subject 可以是一项待验证的数学命题。多个 Task 可以从证明、反例或适用条件等不同目的推进求解，其 Exploration、Cognition 及其关系持续积累为同一 Workspace 的 Cognition/Exploration Status。

### 3.2 Norns Runtime

Norns Runtime 是承载实际求解过程的多智能体运行时，包含多智能体系统、智能体协作过程及其执行上下文。Runtime 可以依托进程、会话或其他智能体平台执行单元存在；具体映射和组件管理方式由协议实现确定。Runtime 可以在 Workspace 初始化或 Task 打开之前启动，负责检查当前状态和等待用户指示；只有在存在 Open Task 且已确认推进该 Task 时，才进入探索调度。

Runtime 不是 Graph 节点，也不是必须持久化的协议实体。同一 Workspace 同一时间最多由一个 Runtime 操作，一个 Open Task 同一时间最多存在一个活动的 Norns 调度过程。实际执行的独占由外部控制面或承载平台保证，Coordinator 不负责通过协议占用标记判定占用者身份或实现 Runtime 互斥。

Workspace 的 `runtime_occupied` 是与 Open Task 生命周期绑定的逻辑占用标记，属于协议状态。该标记不记录占用者身份，也不证明任何 Runtime 或进程仍然存活；Runtime 已启动但尚无 Open Task 时，不设置该标记。外部独占保证与协议逻辑占用必须分别维护，不能相互替代。

Runtime 存续期间，协议实现负责：

- 承载 Coordinator、Decider 与 Explorer；
- 组织对 Subject 的持续探索过程；
- 调用 Norns 协议动作，对核心实体进行操作。

Runtime 异常退出后，外部控制面或承载平台必须隔离遗留执行单元，并完成协议状态对齐与收尾。

协议实现必须原子维护 Runtime 占用及相关协议状态，并负责启动、观察和停止实际智能体。打开新 Task 或重新打开 Closed Task 时必须设置 Runtime 占用；关闭 Task 时必须清除占用。具体职责如何分配给协议实现的内部组件不属于协议。协议不提供脱离 Task 生命周期、单独修改 Runtime 占用的动作。

### 3.3 Task

Task 表示针对 Workspace Subject 的一次具体求解目的。Purpose 描述本次 Task 要达成需求；Subject 描述 Purpose 所针对的目标实体，两者必须保持独立。

| 字段 | 约束 |
| --- | --- |
| `id` | 唯一 Task ID。 |
| `purpose` | 非空的任务目的。 |
| `init_cognition_ref` | 描述本 Task 起始 Subject 上下文的 Task-init Cognition ID。 |
| `max_concurrency` | 正整数，表示额定 Exploration 并发数。 |
| `status` | `open` 或 `closed`。 |
| `outcome` | Closed 时的结构化结果；Open 时为空。 |
| `outcome_reason` | Closed 时非空的结果说明与依据；Open 时为空。 |
| `created_at` | 创建时间。 |
| `closed_at` | Closed 时的关闭时间；Open 时为空。 |

第一个 Task 必须创建一条描述 Subject 的 Task-init Cognition，并通过 `init_cognition_ref` 引用它。后续 Task 可以复用既有 Task-init Cognition，也可以创建新的 Task-init Cognition。每条新建 Task-init Cognition 的 `origin_ref` 和 `task_id` 都必须指向创建它的 Task；复用时不得改写该 Cognition 的来源或内容。Task-init Cognition 只能描述 Subject，不能混入 Task Purpose。

`max_concurrency` 由 Runtime 在打开 Task 时传入，表示 Runtime 调度 Explorer 的额定并发数，不是协议状态写入的硬上限。实际 Running Exploration 数量超过该值时，协议实现应该产生软警告，但不得因此回滚其他合法状态变更。

Task outcome：

| 值 | 含义 |
| --- | --- |
| `succeeded` | 已达成 Purpose。 |
| `unachievable` | 当前持久化的认知和探索证据表明在当前条件下无法达成 Purpose。 |
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

- Open Task 的 `outcome`、`outcome_reason` 和 `closed_at` 必须为空。
- Closed Task 的 `outcome`、`outcome_reason` 和 `closed_at` 必须非空。
- Workspace 存在唯一 Open Task，当且仅当 Workspace 处于 Runtime 占用状态；不存在 Open Task 时，Workspace 不得处于 Runtime 占用状态。
- 关闭 Task 时，必须将该 Task 剩余的 Todo 和 Running Exploration 结束为 `cancelled`；实际 Explorer 必须已经停止或确认不能继续写入。
- 任意 outcome 的 Closed Task 都可以在没有其他 Open Task 时 reopen。
- reopen 保留 Task 固有字段和现有 Cognition/Exploration Status，清空关闭字段，不自动恢复或 retry 已结束的 Exploration。
- 对已经是唯一 Open Task 且 Runtime 占用状态正确的 Task，reopen 必须幂等成功。

### 3.4 Cognition

Cognition 是 Workspace 中统一保存的不可变认知信息单元，根据来源和形成方式具有两类语义：

- **断言认知（asserted cognition）**：由 Task 输入或用户直接提供的认知，包括 Task 初始信息、Workspace 全局用户信息和针对 Exploration 的用户提示。记录必须保留其事实陈述、约束、建议或待验证线索等原始性质；写入不表示系统已经验证或认可其真实性。
- **派生认知（derived cognition）**：Explorer 在探索过程中通过观察、实验、分析或推导形成并记录的认知。派生认知必须有证据支持，准确表达结论的适用范围和不确定性，不表示绝对正确与恒定不变。

两类认知都是 Cognition，统一使用 `cog_<positive-integer>`。其语义类别由 `origin`、`origin_ref` 和 `task_id` 的组合确定。所有 Cognition 必须包含可供其他角色复用的明确内容；由 Decider 决策进行但 Explorer 尚待验证的假说应该表现为 Exploration，不得作为已经成立的派生认知写入。

| 字段 | 约束 |
| --- | --- |
| `id` | 唯一 Cognition ID。 |
| `title` | 非空的简短描述。 |
| `content` | 非空的完整信息。 |
| `meta` | 可选的领域元信息，键和值均为字符串的 JSON 对象；省略时为空对象。 |
| `task_id` | 引入该 Cognition 的 Task ID；Workspace 全局 Cognition 为空。 |
| `origin` | `task`、`user` 或 `exploration`。 |
| `origin_ref` | 按来源类型引用 Task 或 Exploration，必须非空。 |
| `task_purpose_relevant` | 是否直接支撑、改变或限制对应 Task Purpose 的结论。 |
| `created_at` | 创建时间。 |

Cognition 来源组合：

| 场景 | 语义类别 | `origin` | `origin_ref` | `task_id` |
| --- | --- | --- | --- | --- |
| Task-init Cognition，首个与后续新建使用同一规则 | 断言认知 | `task` | 创建该 Cognition 的 Task ID | 创建该 Cognition 的 Task ID |
| Workspace 全局用户 Cognition | 断言认知 | `user` | 添加时的 Open Task ID | 空 |
| Exploration 用户提示 | 断言认知 | `user` | 目标 Exploration ID | 添加提示时的 Open Task ID |
| Explorer 发现 | 派生认知 | `exploration` | 来源 Exploration ID | 来源 Exploration 的 Task ID |

Cognition 必须满足以下约束：

- Cognition 创建后不得修改或删除。
- `meta` 与 Cognition 一同创建且不可修改；键的含义由 Domain 约定，用于记录类别、结论等领域元信息。它不改变认知来源、断言 / 派生类别或 Graph 关系，也不表示内容已经验证。
- 协议不定义 Cognition 的显式失效、替代、反驳或 supersede 关系；新 Cognition 可以补充、隐含修正或显式修正旧 Cognition，但旧 Cognition 仍保留在 Workspace 中。
- `origin_ref` 不得引用另一条 Cognition；首个和后续 Task-init Cognition 均不得建立 Cognition 之间的来源链。
- Workspace 全局用户 Cognition 只能在存在 Open Task 时创建，并对该 Workspace 的所有 Task 可见。
- Explorer 发现只能写入当前 Open Task 中处于 Todo 或 Running 状态的 Exploration。
- 任意状态的 Exploration 都可以接收用户提示。
- Exploration 用户提示的 `task_id` 是添加时的 Open Task ID，可以与目标 Exploration 的 Task ID 不同。
- Explorer 发现的认知和 Exploration 用户提示都与对应 Exploration 关联；前者是派生认知，后者是断言认知，不能因关联同一 Exploration 而混淆其来源。
- 创建 Exploration 用户提示时，必须在同一原子变更中保存 Cognition 并将其 ID 追加到目标 Exploration 的 `hints`。
- `task_purpose_relevant` 默认为 false，创建后不得修改。
- Task-init Cognition 与 Workspace 全局 Cognition 的 `task_purpose_relevant` 必须为 false。
- 只有直接支撑、改变或限制对应 Task Purpose 结论的 Cognition 才能将 `task_purpose_relevant` 标记为 true。

### 3.5 Exploration

Exploration 是获取新知识或验证假说的可执行探索方向。它以一个或多个 Cognition 作为来源，可以产生零个或多个派生认知，并通过 `hints` 关联用户针对该探索提供的断言认知。

| 字段 | 约束 |
| --- | --- |
| `id` | 唯一 Exploration ID。 |
| `title` | 非空的简短描述。 |
| `content` | 非空的探索目标与范围。 |
| `task_id` | 创建该 Exploration 的 Task ID。 |
| `sources` | 有序、不重复且非空的 Cognition ID 列表；可以引用任意来源的 Cognition。 |
| `hints` | 用户针对本 Exploration 提供的提示 Cognition ID 列表；创建时为空，仅允许追加。 |
| `retry_of` | 被 retry 的 Exploration ID；普通创建时为空。 |
| `status` | `todo`、`running` 或 `finished`。 |
| `explorer_id` | 实际执行实例的不透明标识。 |
| `output_ref` | 实际执行输出的引用；若传入值指向可解析的符号链接，assign 保存链接最终目标的绝对路径，否则保留传入值。 |
| `assigned_at` | 分配给执行实例的时间。 |
| `outcome` | Finished 时的结构化结果。 |
| `outcome_reason` | Finished 时非空的执行结果说明。 |
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

- add 与 retry 创建 Todo Exploration；其执行字段和完成字段必须为空，`hints` 必须为空列表。
- assign 将 Todo Exploration 转为 Running，并写入非空的 `explorer_id`、`output_ref` 和 `assigned_at`。
- Running Exploration 可以使用任意合法 outcome 转为 Finished。
- Todo Exploration 只能直接以 `cancelled` outcome 转为 Finished，且执行字段必须保持为空。
- Finished Exploration 必须具有 `outcome`、非空的 `outcome_reason` 和 `finished_at`。
- Exploration 的定义字段、`sources` 和已经写入的执行字段不得修改。
- `hints` 必须完整且不重复地关联所有 `origin=user`、`origin_ref` 指向本 Exploration 的 Cognition；不得包含其他来源或其他 Exploration 的 Cognition。
- 追加 `hints` 不得修改既有认知、探索定义、`sources` 或执行状态，也不得使 Finished Exploration 恢复执行。
- Exploration outcome 只描述本次执行，不改变此前 output Cognition 的协议状态。
- 可复用的探索发现必须在 Exploration 完成前写为 Cognition，不能只存在于 `outcome_reason`。

retry 必须创建具有新 ID 的 Todo Exploration，不得修改来源 Exploration。新 Exploration 复制来源 Exploration 的 `title`、`content` 和有序 `sources`，将 `retry_of` 指向任意历史 Task 中已经 Finished 的 Exploration，并归属于 retry 时的 Open Task。新 Exploration 不复制原 `hints` 或其 Cognition；历史用户提示通过 Trace 的 retry 链保留。`sources` 中原本引用的用户 Cognition ID 仍须原样保留。`retry_of` 引用必须无环；同一来源 Exploration 可以产生多个 retry 分支。

### 3.6 Cognition/Exploration Status 与 Graph

Cognition/Exploration Status 是 Workspace 中关于 Subject 的权威共享协议状态，由已持久化的 Cognition、Exploration 及其关系构成，跨 Task 持续积累。该状态的语义不依赖具体存储方式或输出布局。

Graph 是上述状态中认知与探索来源关系的图模型，也是 View 可以采用的投影结构，不是独立保存的协议状态或另一份事实源。协议实现可以用图、列表或其他形式表达同一状态；表示形式的变化不得改变实体身份、来源关系或本节定义的图约束，具体 View 仍须遵守第 3.7 节规定的收录范围和投影规则。Graph 以 Cog 和 Exp 为两侧，构成有向无环二部图：

```text
Cognition --source--> Exploration --output--> Cognition
```

| 关系 | 来源 | 目标 | 唯一事实源 |
| --- | --- | --- | --- |
| `source` | Cognition | Exploration | `Exploration.sources` |
| `output` | Exploration | Cognition | 指向 Exploration 的 `Cognition.origin_ref` |

`output` 关联包括 Explorer 的派生认知和用户针对该 Exploration 提供的断言认知；该关系不代表用户提示由 Explorer 发现。Exploration 的 `hints` 是后者的成员 ID 列表，必须与 Cognition 的来源字段一致，不构成另一份认知内容或独立的关系事实源。Graph 关系由实体字段确定，不是独立协议实体。

Graph 必须满足以下约束：

- Graph 的每条边只能连接 Cog 与 Exp；不得连接两条 Cog 或两个 Exp。
- Graph 必须保持有向无环。
- Task 不是 Graph 节点；Task 通过 Cognition 和 Exploration 的字段与 Graph 关联。
- 指向 Task 的 `origin_ref` 仅表示认知来源，不构成图边。
- 断言认知与派生认知都处于 Cog 一侧；两者在结构化 View 中的投影位置不同，不改变其 Cognition 身份或可引用性。
- `retry_of` 表示 Exploration 的执行沿袭，不是 Graph 关系。
- Exploration 的 `outcome` 和 `outcome_reason` 是执行结果，不自动形成 output Cognition。
- Agent 对话、私有笔记、工具输出和 Runtime 日志只有按本协议写为 Cognition 或 Exploration 状态后，才能成为共享协议事实。

### 3.7 View

View 是协议实现根据当前 Workspace 的 Cognition/Exploration Status 及相关 Task 状态生成的只读投影，用于向智能体角色或人类提供与其用途相匹配的协议上下文。View 不是持久化协议实体，也不是 Graph 节点；生成 View 不得修改 Workspace 协议状态。

View 表示生成时的当前协议状态，不表示历史快照。选择历史 Task 只改变 View 使用的 Task 上下文，不会将 Cognition/Exploration Status 回退到历史状态。

Norns 定义三类 View：

- Status View：向 Decider 提供整个 Subject 的当前求解状态。
- Trace View：向 Explorer 提供一个 Exploration 的最小充分上下文。
- Visual View：向人类提供 Subject、Task 和完整 Cognition/Exploration Status 的可观察投影。

Status View 和 Trace View 是结构化 View，使用英文 `snake_case` 字段和 JSON 兼容值。列表必须稳定排序：Task、Cognition 与 Exploration 默认按 `created_at ASC, id ASC` 排序，`sources` 保留输入顺序。所有 View 必须包含或关联同一次生成操作的 `generated_at`。

Status / Trace 可使用 YAML（默认）、JSON 或 Markdown 表示同一逻辑投影。下文的字段位置与精简字段规则直接适用于 YAML / JSON；Markdown 使用以下阅读结构，不改变认知和探索的收录范围、来源、排序和条件省略语义：

- 一级标题为 `# Status View` / `# Trace View`。二级标题沿用顶级字段名并将首字母大写，但 graph / related_subgraph 分别改为 `## Status` / `## Trace`。
- Markdown 的第二个二级章节固定为 `## Graph`，是同一逻辑图的紧凑索引，包含 `### Cognitions` 和 `### Explorations`，各用文本代码块逐行列出节点。Cog 行为 `cog_id: title`，派生认知追加 ` <- exp_id`；Task-init 不追加 Task 来源箭头。Exp 行为 `exp_id: (status) title <- ['cog_id', ...]`，sources 保留原顺序。该索引仅遍历原 graph / related_subgraph 中的 cognitions 与 explorations，不将图外用户认知复制为图内节点，不扩大 Trace 收录范围；不包含正文、时间、meta 或 retry 连线。空集合显示 []。后面的详细 Status / Trace、全局认知及用户提示继续按原规则展开。
- `## Task` 首先用引用段落记录 Task ID，然后显示加粗的 Purpose 标题和引用正文；无单个 Task 时，Tasks 中的每个 Task 使用相同形式。
- Status / Trace 章节内，先将已收录的 Task-init Cognition 放入 `### Task-Init Cognitions`，逐条以 `#### cog id - title` 展开；没有此类认知时省略该子节。随后按创建顺序逐个显示 `### exp id - title`，先列元数据无序列表，再显示加粗的 Content 标题和引用正文。
- 每个 Exp 下面，先将逻辑投影已收录的派生认知以 `#### cog id - title` 展开，再以同样形式显示该 Exp 的用户提示；两组各自保留创建顺序。认知先列正文之外的元数据，再显示 Content 标题和引用正文，使用 origin 区分探索发现与用户提示。不得为排版而补入 Trace 原未收录的派生产出或祖先之外的探索。
- Markdown 可从同一生成快照补充认知 / 探索标题、来源类别及探索创建时间，供标题和元数据展示；不增加 meta。全局用户认知继续独立放在 Global_hints，探索用户提示继续只在所属 Exp 下出现。
- Todo_explorations、Running_explorations、Recent_top10_derived_cognitions 都使用 Markdown 表格。近期派生认知保留原选取范围和倒序，列为 id、title、created_at；此标题和时间补充仅用于 Markdown，YAML / JSON 仍使用原 ID 列表。空 Todo / Running 输出只有表头的空表，近期摘要为空时继续省略。其他空集合明确表示为空。

Markdown 是阅读表示，不要求能逆向还原为 YAML。

结构化 View 按以下位置组织 Cognition，不得重复投影同一条认知：

| 来源 | 投影位置 |
| --- | --- |
| Task-init 断言认知 | `graph.cognitions` 或 `related_subgraph.cognitions` |
| Workspace 全局用户断言认知 | 与图同级的 `global_hints` 列表 |
| Exploration 用户断言认知 | 所属 Exploration 的 `hints` 列表 |
| Explorer 派生认知 | `graph.cognitions` 或 `related_subgraph.cognitions` |

`global_hints` 仅包含 Workspace 全局用户认知，不包含 Task-init Cognition 或 Exploration 用户提示。所有用户认知的列表项都只包含 `id`、`content` 和 `created_at`。Exploration 的 `hints` 在实体中是 ID 列表，在 View 中必须展开为上述三字段对象；仅在非空时输出，并按 Cognition 的 `created_at ASC, id ASC` 排序。

图的完整含义必须结合图内认知、图外 `global_hints` 和 Exploration 内 `hints` 理解。`sources` 可以引用这些位置中的任意 Cognition ID，不得因投影位置不同而改写来源 ID、将用户认知复制到图内 Cog 列表，或把来源替换为所属 Exploration ID。

#### 3.7.1 Status View

Status View 供 Decider 获取整个 Subject 的当前状态。

- 显式选择 Task 时，以该 Task 作为 View Task。
- 未显式选择且存在 Open Task 时，以该 Open Task 作为 View Task。
- 未显式选择且不存在 Open Task 时，顶层返回全部 Task，不选择单个 View Task。
- View 始终覆盖 Workspace 当前完整 Cognition/Exploration Status，包括图内记录、图外的全局断言和 Exploration 内的用户断言，不是只属于 View Task 的子图。

Status View 的顶级结构按以下顺序包含：

1. 单个 `task` 或全部 `tasks`；
2. `graph.cognitions` 与 `graph.explorations`；
3. `global_hints`；
4. `todo_explorations`；
5. `running_explorations`；
6. 非空时输出 `recent_top10_derived_cognitions`；
7. `generated_at`。

`recent_top10_derived_cognitions` 仅包含派生认知（`origin=exploration`）的 ID，用于提醒近期新增的探索发现。以本次 View 的 `generated_at` 为基准，从整个 Workspace 中选择创建时间位于 `[generated_at - 30 分钟, generated_at]` 的认知，按 `created_at DESC, id ASC` 排序，取最多 10 条；不足 10 条时按实际条数输出，没有符合条件的认知时省略整个字段。该摘要不受 View Task 选择影响，不重复认知正文，不代表价值排序、尚未处理状态或必须继续探索的结论。认知完整记录仍位于 `graph.cognitions`。

没有选择单个 View Task 时，`todo_explorations` 和 `running_explorations` 必须为空列表。Todo 摘要包含 `id` 和 `title`；Running 摘要包含 `id`、`title`，以及基于同一个 `generated_at` 和 `assigned_at` 计算的非负整数 `duration_seconds`。

Status View 使用以下精简投影：

- Task：`id`、`purpose`，以及非空的 `outcome`、`outcome_reason` 和 `closed_at`。
- 图内 Cognition：`id`、`content`、`origin_ref`、条件性的 `task_purpose_relevant` 和 `created_at`；只包含 Task-init 断言认知与 Explorer 派生认知，不输出 `origin`。前者的 `origin_ref` 为 Task ID，后者为 Exploration ID。
- Exploration：`id`、`content`、非空的 `retry_of`、`status`、非空的结果字段、`sources`、非空的 `hints` 和非空的 `finished_at`；不输出执行字段或 output 镜像。
- Workspace 全局用户 Cognition：在 `global_hints` 中输出 `id`、`content` 和 `created_at`；无论选择哪个 Task，均包含全部全局用户认知。
- Exploration 用户 Cognition：在所属 Exploration 的 `hints` 中输出 `id`、`content` 和 `created_at`，不在其他认知列表重复出现。

结构化 View 不得输出值为 false 的 `task_purpose_relevant`。存在单个 View Task 时，只为图内属于该 Task 且存储值为 true 的 Cognition 输出该字段；返回全部 Task 时，为图内所有存储值为 true 的 Cognition 输出该字段。用户认知的精简投影不输出该标记，其存储值及 Trace 收录语义仍然有效。`global_hints`、`graph.cognitions` 和 `graph.explorations` 没有记录时仍须输出空列表。

Status / Trace 的全部认知精简投影均不输出 `meta`，包括图内认知、`global_hints` 与 Exploration.hints。

#### 3.7.2 Trace View

Trace View 供 Explorer 获取一个 Exploration 的最小充分上下文。其 Task 上下文始终由目标 Exploration 的 `task_id` 决定，不接受独立的 Task 选择。

Trace View 的顶级结构按以下顺序包含：

1. `current_exploration`：目标 Exploration 的 `id`、`title` 和 `content`；
2. `parent_task`：当前 Exploration 作为拆分方向所归属的父任务，包含该 Task 的 `purpose` 和职责提示 `note`；提示明确只需完成当前 Exploration，父任务目标仅作为理解上下文，无需完成整个父任务；
3. `related_subgraph`：与目标 Exploration 相关的 Cognition 与 Exploration；
4. `global_hints`：全部 Workspace 全局用户断言认知；
5. `running_explorations`：目标 Exploration 所属父任务中全部 Running Exploration 的摘要；
6. `generated_at`。

`running_explorations` 每项包含 `id`、`title`、`duration_seconds` 和 `explorer_id`，其中 `explorer_id` 为该 Exploration 在 assign 时保存的实际 Explorer 标识，必须非空。列表按 `created_at ASC, id ASC` 排序。运行时长基于本次 View 的同一个 `generated_at` 和 `assigned_at`，以非负整数秒表示。该列表包含不在 `related_subgraph` 中的并行探索，也包含处于 Running 的目标 Exploration；不因列入摘要而扩大子图。没有记录时输出 `[]`；为历史 Exploration 生成 Trace 时仍以其所属 Task 为范围，不改用当前 Open Task。

Trace 必须收录以下信息，并按第 3.7 节的投影位置组织，不能将用户认知一律放入 `related_subgraph.cognitions`：

- 目标 Exploration、其全部 `sources` 所引用的 Cognition，以及该 Exploration 的全部派生认知和用户提示；
- 追溯来源认知所需的最小祖先子图；
- 属于目标 Exploration 所属 Task 且 `task_purpose_relevant=true` 的额外 Cognition；
- 从目标 Exploration 开始的完整 `retry_of` 链，以及链上每次执行的全部派生认知、用户提示及所需来源上下文；
- 全部 Workspace 全局用户认知，统一放在 `global_hints` 中。

最小祖先子图从每条需要收录的来源认知反向追溯：

1. Task-init 断言认知：加入图内 Cog 列表，到此停止；不沿其 Task 来源引用连接其他认知。
2. Workspace 全局用户断言认知：由顶层 `global_hints` 提供，到此停止；不将其复制进子图，也不沿其 Task 来源引用追溯。
3. Explorer 派生认知：加入图内 Cog 列表，纳入产生它的 Exploration，并继续追溯该 Exploration 的全部 `sources`。
4. Exploration 用户断言认知：纳入其所属 Exploration，将认知放入该 Exploration 的 `hints`，并继续追溯该 Exploration 的全部 `sources`。

纳入任意 Exploration 时，都必须在其 `hints` 中展开全部用户提示。额外收录的 Purpose 相关认知同样按其来源应用上述规则；如果是用户探索认知，必须纳入所属 Exploration 及其所需来源上下文。认知的 `task_id` 不改变目标 Trace 的 Task 上下文。

仅为追溯来源或承载用户提示而纳入一个 Exploration 时，不得自动加入它的其他派生认知；这些认知只有满足目标探索产出、来源引用、Purpose 相关性或目标 retry 链等收录条件时才进入子图。完整 retry 链保留历史探索和其中的用户提示，不将旧 `hints` 复制到新 Exploration。

Trace View 使用与 Status View 相同的认知、Exploration、全局断言及 hints 精简投影，但不输出任何 `task_purpose_relevant` 字段，也不包含 `todo_explorations`。没有记录时，`global_hints`、`running_explorations`、`related_subgraph.cognitions` 和 `related_subgraph.explorations` 仍须输出空列表。

#### 3.7.3 Visual View

Visual View 供人类观察完整 Subject。它不接受 Task 选择，必须表达全部 Task、当前完整 Cognition/Exploration Status、其中的 Graph 关系、Runtime 占用状态，以及所有 Task 的 Purpose 相关 Cognition。

非空的 Cognition `meta` 仅在对应认知节点详情中展示。

Visual View 的具体布局、交互、视觉编码和传输格式不属于协议；其展示图元不必与结构化 View 的二部图投影一一对应，但不得改变协议中的关系或认知来源。协议实现必须保证 Visual View 的信息来源与生成时的当前协议状态一致。

### 3.8 Domain

Domain 是对 Norns 基础协议能力的可选扩展，用于为决策和探索提供特定领域的知识与工具。Domain 不是持久化协议实体或 Graph 节点，不构成 Workspace 协议状态，也不引入新的协议动作；Runtime 未启用 Domain 时仍可以完整运行 Norns 协议。

Domain 可以为 Coordinator、Decider 和 Explorer 提供领域增强变体，补充领域知识、指令和工具。领域化角色仍分别承担第 5.2、5.3 和 5.4 节定义的逻辑角色，必须遵守相同的职责边界、协议动作和状态约束；不得改变 Coordinator 的职责、调度语义或权限，也不得将决策、执行或生命周期管理职责转移给其他角色。Coordinator 负责根据当前 Task 和可调度角色范围选择合适的通用或领域增强角色。

Domain 提供的知识、指令和工具输出本身不是共享协议事实。只有按照本协议写为 Cognition 或实体状态的信息，才能作为共享协议信息供其他角色使用。Domain 的名称、内容结构、工具接口、选择方式、配置和注入机制属于平台绑定，不由协议规定。

## 4. 协议动作

协议动作是协议实现必须提供的抽象能力。本节只定义动作的语义、前置条件和结果，不规定 CLI 命令、API、通信方式或内部调用边界。

### 4.1 通用动作语义

- 每个修改动作对 Workspace 协议状态的变更必须是原子的。
- 停止实际执行单元、投递消息等外部操作不属于协议状态事务；动作定义要求这些操作先完成时，协议实现必须在确认其完成或已经隔离后再提交状态变更。
- 被拒绝的动作不得留下部分协议状态。
- 未知实体、非法状态或引用、无效输入、协议版本不受支持等失败必须能够被调用方区分；具体错误编码和传输形式不属于协议。
- 动作产生的实体 ID 和协议时间必须由协议实现按照第 2 节生成。
- 修改动作完成后，如果当前 Task 的 Running Exploration 数量超过 `max_concurrency`，协议实现应该返回软警告，但不得因此回滚其他合法状态变更。

### 4.2 Workspace 与 Task 动作

#### `workspace.initialize`

建立一个不包含 Task、Cognition 或 Exploration 的空 Workspace。对已经存在且协议版本受支持、状态合法的 Workspace 重复执行时，必须幂等成功；不得以初始化覆盖或修复未知、不兼容或状态不可信的 Workspace。

#### `workspace.check`

只读校验当前 Workspace 的协议版本与状态完整性，并返回当前任务概况及布尔字段 `runtime_occupied`，表示协议记录的 Runtime 占用状态，true 和 false 均须显式输出，不表示实际执行进程存活。存在 Open Task 时返回该 Task；不存在 Open Task 时返回按 `created_at ASC, id ASC` 排序的全部 Task 及基本摘要；不存在任何 Task 时明确返回空 Workspace。摘要包含 Workspace 的 Task、Cognition 和 Exploration 总数，并与任务信息和 Runtime 占用状态来自同一次读取快照。

该动作不要求存在 Open Task，不得自动初始化 Workspace、创建 View 或变更实体状态。未初始化、版本不兼容或状态不可信时必须返回可区分的失败，不得将其视为空 Workspace。

#### `workspace.free`

取消当前 Workspace 中尚未完成的工作，释放 Runtime 占用，并保留已有记录。调用方必须先停止或隔离相关实际执行单元，确认其不能继续写入。本动作必须原子完成：

- 将所有 Todo 和 Running Exploration 结束为 `finished`、`outcome=cancelled`，写入非空的取消理由和结束时间；保留已有执行引用。
- 将存在的 Open Task 以 `cancelled` 关闭，写入非空的关闭理由和时间。
- 清除 Runtime 占用，使 Workspace 恢复为没有 Open Task、没有未完成 Exploration 且未被占用的状态。

已经 Finished 的 Exploration 和 Closed 的 Task 保留原结果，Cognition、来源关系和编号计数不得改变；本动作不归档或重置 Workspace。合法 Workspace 已处于可用状态时，动作必须幂等成功。动作仍须校验版本和协议状态完整性，不得自动初始化或绕过不可信状态校验。

#### `task.list`

只读查询当前 Workspace 的 Task 集合，不要求存在 Open Task。默认列出全部 Task，也可以按 `open` 或 `closed` 状态筛选；其他状态值必须拒绝。结果中的 `tasks` 始终为列表，无匹配项时为空列表，按创建时间升序排列，同一时间按 ID 升序。

每项包含 Task 的 `id`、`purpose`、`init_cognition_ref`、`max_concurrency`、`status`、`created_at`，以及已有的 `outcome`、`outcome_reason`、`closed_at`；为空的关闭字段可省略。本动作不展开 Cognition 或 Exploration，不属于 View，不生成 Graph；未初始化、版本不兼容及状态不可信的 Workspace 必须返回可区分的失败。

#### `task.open`

打开新 Task，并在同一原子变更中完成以下操作：

1. 创建 Open Task，写入非空 `purpose`、正整数 `max_concurrency` 和生成的 `created_at`；
2. 创建或选择该 Task 的 Task-init Cognition，并写入 `init_cognition_ref`；
3. 设置 Workspace 的 Runtime 占用状态。

首个 Task 必须接收新的 Subject 信息并创建 Task-init 断言认知，将其 `origin` 设为 `task`，`origin_ref` 和 `task_id` 均设为新 Task ID。后续 Task：

- 未提供新的 Subject 信息时，复用最近关闭 Task 的 `init_cognition_ref`；最近关闭 Task 按 `closed_at DESC, id ASC` 确定。复用不得修改该认知的内容、`origin_ref` 或 `task_id`。
- 提供新的 Subject 信息时，按照首个 Task 相同的来源规则创建新的 Task-init 断言认知，`origin_ref` 和 `task_id` 均为新 Task ID，不引用任何既有 Cog。

存在 Open Task，或 Workspace 已被 Runtime 占用但不存在 Open Task 时，动作必须失败。

#### `task.close`

关闭当前 Open Task。调用方必须先停止或隔离该 Task 的实际 Explorer，保证其不能继续写入，再提交本动作。本动作必须在同一原子变更中：

- 将该 Task 剩余的 Todo 和 Running Exploration 以 `cancelled` 结束；
- 写入 Task 的 `outcome`、非空 `outcome_reason` 和 `closed_at`；
- 清除 Workspace 的 Runtime 占用状态。

不存在 Open Task 时，动作必须失败。Coordinator 接受 Decider 的 `complete` 建议时只能以 `succeeded` 或 `unachievable` 关闭 Task；响应用户或外部系统的明确终止请求时以 `cancelled` 关闭。Task 的 `failed` 由外部控制面或承载平台在无法可靠继续 Runtime 时登记，不由 Coordinator 自行决定。具体收尾规则见第 6.3 节。

#### `task.reopen`

在不存在其他 Open Task 时重新打开指定的 Closed Task，并设置 Runtime 占用。动作必须保留 Task 的 `purpose`、`init_cognition_ref`、`max_concurrency` 和既有 Cognition/Exploration Status，清空 `outcome`、`outcome_reason` 与 `closed_at`，且不得自动恢复或 retry 已结束的 Exploration。

目标已经是唯一 Open Task 且 Runtime 占用状态正确时，动作必须幂等成功。目标为 Open 但占用状态不正确，或存在其他 Open Task 时，动作必须失败。

### 4.3 Cognition 动作

#### `cognition.add`

在存在 Open Task 时创建不可变 Cognition。可选参数 `meta` 接收键和值均为字符串的 JSON 对象，与认知正文及来源共同保存；省略时使用空对象。非法 JSON、非对象或含非字符串值的对象必须拒绝，且不得产生部分写入。该动作支持以下来源：

- Workspace 全局用户断言认知：`origin=user`，`origin_ref` 为当前 Open Task ID，`task_id` 为空。
- Exploration 用户断言认知：`origin=user`，`origin_ref` 指向目标 Exploration，`task_id` 为添加提示时的 Open Task ID；任意状态、任意历史 Task 的 Exploration 都可以接收提示。保存认知和将 Cog ID 追加到目标 Exploration 的 `hints` 必须在同一原子变更中完成，不得复制认知正文或改变目标 Exploration 的执行状态。
- Explorer 派生认知：`origin=exploration`，`origin_ref` 指向目标 Exploration，`task_id` 为该 Exploration 的 Task ID；目标必须属于当前 Open Task 且处于 Todo 或 Running 状态。

Task-init Cognition 只能作为 `task.open` 的一部分创建，不能通过本动作单独创建。协议实现向实际 Explorer 投递已经持久化的用户提示属于事务外行为；投递失败不得回滚 Cognition，也不得使其失去协议事实地位。

#### `cognition.get`

按 Cognition ID 只读查询单个完整认知，适用于所有来源及历史 Task，不要求存在 Open Task。结果使用第 3.4 节的实体字段，包含 title、content、来源、归属、Purpose 相关标记、创建时间及非空的 meta；不使用 View 的精简投影，不暴露实现内部的来源物理列。空 meta 和为空的 task_id 可省略。非法或类型不匹配的 ID 必须拒绝；合法 ID 不存在时返回可区分的 Cognition 不存在错误。

### 4.4 Exploration 动作

#### `exploration.add`

在当前 Open Task 中创建 Todo Exploration，`hints` 初始为空列表。`title` 和 `content` 必须非空，`sources` 必须是非空、有序、不重复且均存在的 Cognition ID 列表；未显式提供来源时，使用当前 Task 的 `init_cognition_ref` 作为唯一来源。来源可以是 Task-init 认知、全局用户认知、其他 Exploration 的用户认知或派生认知，不因认知类别或 View 投影位置而受到额外限制。

#### `exploration.assign`

将当前 Open Task 中指定的 Todo Exploration 转为 Running，并写入非空的 `explorer_id`、`output_ref` 和生成的 `assigned_at`。如果 `output_ref` 指向可解析的符号链接，写入链接最终目标的绝对路径；普通文件、非路径引用、已断开的链接或解析失败的路径保留传入值。目标不属于当前 Open Task 或不处于 Todo 状态时，动作必须失败。

#### `exploration.finish`

结束当前 Open Task 中指定的 Exploration，并写入合法 `outcome`、非空 `outcome_reason` 和生成的 `finished_at`。Running Exploration 可以使用任意合法 Exploration outcome；Todo Exploration 只能以 `cancelled` 结束。Finished Exploration 不得再次结束。

Explorer 可以报告自身执行的合法结果；Coordinator 仅可使用 `cancelled`，不得代替 Explorer 登记 `succeeded`、`inconclusive` 或 `failed`。Coordinator 登记前必须确认实际 Explorer 已停止或不能继续写入，并在 `outcome_reason` 中说明主动终止、失联、超时或未登记结束等原因。Explorer 已提交结果时必须保留，不得重复结束或覆盖。

#### `exploration.retry`

以任意历史 Task 中指定的 Finished Exploration 为来源，在当前 Open Task 中创建新的 Todo Exploration。新 Exploration 必须复制来源的 `title`、`content` 和有序 `sources`，并将 `retry_of` 指向来源 Exploration。新 `hints` 必须为空，不复制旧提示成员或 Cognition；复制的 `sources` 中原本引用的用户认知 ID 仍须保留。该动作不得修改或重新运行来源 Exploration。

#### `exploration.get`

按 Exploration ID 只读查询单个完整探索，适用于 Todo、Running、Finished 及历史 Task，不要求存在 Open Task。结果使用第 3.5 节的实体字段，包含完整定义、Task 归属、有序 sources / hints ID 列表、状态、创建时间及已有的 retry、执行和结束字段。hints 不展开 Cognition 正文，sources 保持有序非空，hints 为空时使用空列表；为空的可选字段可省略。非法或类型不匹配的 ID 必须拒绝；合法 ID 不存在时返回可区分的 Exploration 不存在错误。

两个 get 动作均不得修改协议状态或生成 View 文件；读取时仍须遵守 Workspace 版本与完整性校验。

#### `exploration.todo` 与 `exploration.running`

两个动作分别查询当前 Open Task 的 Todo 和 Running Exploration，用于恢复和驱动 Exploration 调度。它们是 Exploration 集合查询，不是第 3.7 节定义的 View。不存在 Open Task 时，动作必须返回可区分的失败。

查询结果必须包含：

- 当前 Task 的 `task_id` 和 `max_concurrency`；
- 按 `created_at ASC, id ASC` 稳定排序的 `explorations`；
- 每条 Exploration 的 `id`、`title`、`content`、`sources`、`hints`、非空的 `retry_of` 和 `created_at`；其中 `hints` 是实体中的 Cog ID 列表，空时返回空列表，不使用 View 中的对象展开形式；
- Running Exploration 额外包含 `explorer_id`、`assigned_at`，以及基于同一次查询时间计算的非负整数 `duration_seconds`；不输出 `output_ref`，需要实际执行输出引用时通过 `exploration.get` 获取。

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
- 角色的对话、私有推理、临时笔记和工具输出不是共享协议事实；只有按协议写为 Cognition 或实体状态的信息才能被其他角色作为共享协议信息使用。用户认知位于 View 的图外列表或 Exploration 的 `hints`，不影响其协议地位，也不表示其内容已被验证。
- 角色必须基于动作执行时的最新协议状态工作，不得假定先前读取的 View 或查询结果仍然有效。
- 角色之间可以由协议实现直接调用，也可以通过消息或其他平台机制协作；具体承载和通信格式不属于协议。
- 角色可以使用 Domain 提供的领域增强变体，但不得改变本节规定的职责边界、协议动作权限和状态约束。

### 5.2 Coordinator

Coordinator 负责用户交互、Workspace 与 Task 的启动选择，以及 Open Task 的整体调度和生命周期协调。Runtime 中必须存在承担该职责的逻辑单元。

Coordinator 必须：

- 首次启动时只读检查 Workspace 与 Task 状态，按第 6.1 节等待用户指示并选择建立、重新打开或继续 Task，始终以 Workspace 的当前协议状态作为调度依据；
- 根据 Task 的 `max_concurrency`、Todo 和 Running Exploration，以及实际执行状态计算可用调度能力；
- 按需调用 Decider，并保证同一 Task 同一时间最多存在一个活动的 Decider；
- 根据当前 Task 和可调度角色范围，选择合适的通用或领域增强角色；
- 为 Todo Exploration 启动 Explorer，并通过 `exploration.assign` 登记实际执行引用；
- 对齐 Running Exploration 与实际 Explorer，处理完成、失联、失败或取消；
- 区分用户输入中的任务控制指令与认知信息，将针对整体 Subject 的信息写为全局用户断言认知，将针对特定 Exploration 的信息写为探索用户断言认知；认知必须保留输入原有的事实陈述、约束、建议或待验证性质，不得将断言当作已验证发现；
- 将已经持久化的 Exploration 用户提示转交仍在运行的对应 Explorer；
- 在关闭 Task 前停止或隔离全部相关 Explorer，再通过 `task.close` 提交最终状态，并向用户同步已登记的结果和依据。

Coordinator 不应亲自执行 Exploration，也不得通过 `exploration.add` 或 `exploration.retry` 自行创建 Exploration，不得以未写入 Workspace 协议状态的信息作为 Task 完成依据。Coordinator 只能依据 Decider 的 `complete` 建议以 `succeeded` 或 `unachievable` 关闭 Task，或依据用户、外部系统的明确终止请求以 `cancelled` 关闭 Task；不得自行登记 Task 的 `failed`。Coordinator 调用 `exploration.finish` 时只能使用 `cancelled`。

Explorer 的单次失败不必然导致 Task 失败。Coordinator 必须停止或隔离异常执行单元；探索尚未结束时登记 `cancelled`，已经由 Explorer 提交的结果则保持不变，再通过重新评估和按需调用 Decider 取得后续探索决策。

### 5.3 Decider

Decider 基于最新 Status View 评估当前 Task 的求解状态，并给出以下一种逻辑决策：

- `explore`：创建一个或多个新的 Todo Exploration；
- `wait`：当前没有必要创建新的 Exploration，等待正在进行的工作或明确的外部事件；
- `complete`：当前持久化的认知和探索记录已足以支持 Task 达成或不可达的结论。

Decider 必须满足以下约束：

- 只负责选择和创建 Exploration，不执行 Exploration；
- 创建 Exploration 可以通过 `exploration.add` 创建，也可以通过 `exploration.retry` 复制已存在的 Finished Exploration；创建的 Exploration 将随这两类操作写入 Task；创建的 Exploration 必须具有明确、相互可区分的目标；
- 创建的 Exploration 数量不得超过 Coordinator 提供的可用调度能力；
- `explore` 可以创建少于可用调度能力的 Exploration；这表示 Decider 认为在当前信息状态下没有更多值得立即执行的高价值方向，剩余调度能力不要求被填满；
- 存在尚未完成的 Exploration 时仍可以基于最新状态再次决策，不得将一组 Exploration 视为必须整体完成的固定轮次；
- `wait` 只能用于存在正在进行的工作或明确外部状态变化来源的情况；
- `complete` 只能建议 `succeeded` 或 `unachievable`，且结论与依据必须能够由当前 Workspace 的认知和探索记录支持。

Decider 不得直接关闭 Task。Coordinator 必须在重新核对当前协议状态并完成执行单元收尾后决定是否接受完成建议。

### 5.4 Explorer

Explorer 每次只执行一个指定的 Exploration，并以该 Exploration 的 Trace View 作为共享协议上下文。

Explorer 必须：

- 只以 `current_exploration` 描述的目标和范围开展工作；
- 读取 Trace View 中的相关图、全局 `global_hints` 和各 Exploration 的 `hints`，按来源理解断言认知与派生认知；
- 将可靠、可复用的发现及时通过 `cognition.add` 写为 `origin=exploration` 的派生认知；
- 在执行结束时通过 `exploration.finish` 报告 `succeeded`、`inconclusive`、`failed` 或 `cancelled`，并提供非空的执行过程与结果说明；`failed` 表示自身执行异常，`cancelled` 表示响应外部中止，不得以取消代替对执行失败的如实报告；
- 将支撑 Task Purpose 的发现按第 3.4 节准确标记为 `task_purpose_relevant`。

Explorer 不得创建新的 Exploration、决定 Task outcome 或关闭 Task。`outcome_reason` 只描述本次 Exploration 的执行结果，不能替代应持久化的派生 Cognition。

## 6. 事件驱动调度与恢复

Norns 的调度发生在 Runtime 中，由 Coordinator 响应协议状态变化和外部执行事件进行。调度过程不是持久化协议实体；除显式协议动作产生的实体及关系变化外，不形成额外的共享协议状态。

### 6.1 启动检查与调度原则

#### 启动检查与任务选择

Runtime 首次启动时，Coordinator 必须只读检查当前目录的 Workspace 与 Task 状态，不进行初始化、重置、任务创建或其他协议状态变更，并等待用户的下一步指示。实际 Runtime 独占由第 3.2 节规定的外部控制面或承载平台保证，不以 `runtime_occupied` 推断是否存在其他运行中的 Runtime。

根据检查结果和用户指令，按以下情形推进：

- 不存在 Workspace：用户提供明确的任务意图及首个 Task 所需的 Subject 信息后，初始化空 Workspace 并创建 Task，进入 Norns Loop。
- 存在合法 Workspace，但不存在 Task：用户提供明确的任务意图及 Subject 信息后，在已有 Workspace 中创建 Task，进入 Norns Loop；不重复初始化或重置 Workspace。
- 存在 Task，但无 Open Task：确认继续当前 Subject 还是重置为新 Subject。继续当前 Subject 时，根据用户选择创建新 Task 或通过 `task.reopen` 重新打开已有 Task；重置时，由协议实现按其 Workspace 生命周期机制处理，随后等待用户提供新 Subject 信息和任务意图，再创建 Task。Task 打开后进入 Norns Loop。
- 存在 Open Task：确认是否继续该 Task。继续时不创建或重新打开 Task，在完成第 6.4 节的执行状态对齐后进入 Norns Loop；终止时先停止或隔离实际 Explorer，再以 `cancelled` 关闭 Task，随后按不存在 Open Task 的情形处理。

用户指令已经明确相应选择时，无须重复询问该选择。版本不兼容、结构未知或状态不可信的 Workspace 不属于“不存在 Workspace”或“空 Workspace”；必须报告可区分的失败，不得通过初始化或重置自动覆盖。

#### 调度原则

- Norns 不划分固定的 Decide/Explore 轮次，也不设置等待一组 Exploration 全部完成的同步屏障。
- 一次 Decider 调用创建的多个 Exploration 不构成协议批次，每个 Exploration 独立进入 Todo、Running 和 Finished 状态。
- 调用下一次 Decider 不以前次创建的全部 Exploration 都已 Finished 为前提；其他 Explorer 仍在运行时，Coordinator 可以基于最新状态再次调用 Decider。
- Decider 返回的 Exploration 少于可用调度能力时，Coordinator 不得仅因仍有空闲能力就在相同信息状态下再次调用 Decider；再次调用前必须出现新的 Cognition、Exploration 完成状态或与当前 Task 相关的外部输入。
- 每次决策与调度都必须以当前协议状态和实际执行状态为依据，不得依赖已失效的 View、缓存计数或先前事件顺序。
- `max_concurrency` 是 Explorer 的额定并发数。Coordinator 应该以其约束主动调度，但超过该值只触发第 3.3 节定义的软警告，不得自动取消合法的 Running Exploration。
- Open Task 不得仅因暂时没有新事件、没有 Todo Exploration 或 Decider 返回 `wait` 而自动关闭。
- 已有 Todo Exploration 应优先占用可用 Explorer 槽位；调度后仍有空闲槽位、没有活动 Decider，且是首次决策或满足再次决策条件时，Coordinator 应该调用 Decider，并明确提供本次可新增的 Exploration 数量。
- 等待事件时单次阻塞不得超过一分钟；等待超时只要求检查最新状态，不构成新的决策依据，不得因此在相同信息状态下重复调用 Decider。

### 6.2 调度事件与重新评估

以下情况必须使 Coordinator 重新评估启动选择或已获准推进的 Task 的调度；事件本身不替代第 6.1 节要求的用户指示：

- Runtime 启动、恢复或重新取得当前 Task 的控制权；
- Task 被打开或重新打开；
- Cognition 被创建，尤其是 Explorer 发现或 Exploration 用户提示；
- Exploration 被创建、分配、完成，或其实际 Explorer 的状态发生变化；
- Decider 返回 `explore`、`wait` 或 `complete`；
- 用户或外部系统提供与当前 Task 有关的新输入或终止请求；
- 协议实现发现协议状态与实际执行状态不一致。

协议实现可以合并重复事件，也可以采用通知、轮询或其他机制发现事件，但不得因为事件丢失或乱序而跳过对最新状态的校验。一次重新评估不要求产生新的协议状态；没有可执行工作时，Coordinator 应进入事件驱动等待。

### 6.3 调度与执行过程

每次重新评估时，Coordinator 必须先对齐 Running Exploration 与实际 Explorer，再根据 `max_concurrency`、Todo 和 Running Exploration 以及实际执行状态计算可用调度能力。

Coordinator 按以下语义推进 Task，各步骤不构成必须完整执行一遍的固定循环：

1. 在可用调度能力内，为已有 Todo Exploration 启动 Explorer。
2. 实际 Explorer 的 `explorer_id` 和 `output_ref` 可用后，立即通过 `exploration.assign` 将对应 Exploration 登记为 Running；如果 assign 失败，必须停止或隔离该 Explorer。
3. 调度已有 Todo Exploration 后仍有可用调度能力且没有活动 Decider，并且是首次决策或满足第 6.1 节的再次调用条件时，应该使用最新 Status View 调用 Decider，并告知本次可新增的 Exploration 数量；计算时必须计入已启动但尚未成功 assign 的执行实例，不得为同一空闲槽位重复安排工作。Decider 返回 `explore` 时，新增或 retry 的 Exploration 进入 Todo，并参与后续调度。
4. Explorer 产生可靠发现时，必须先将其写为派生 Cognition；执行结束时，再通过 `exploration.finish` 提交本次执行结果。
5. Decider 返回 `wait` 时，Coordinator 不创建无依据的 Exploration，也不关闭 Task，而是等待其声明的进行中工作或外部状态变化。
6. Decider 返回 `complete` 时，Coordinator 必须重新核对当前协议状态和结论依据。接受建议后，停止或隔离全部实际 Explorer，再通过 `task.close` 以 `succeeded` 或 `unachievable` 关闭 Task，不得将 `complete` 解释为 `failed` 或 `cancelled`；该动作会取消仍为 Todo 或 Running 的 Exploration。关闭成功后向用户同步已登记的任务结果和结束理由。

用户或外部系统明确终止 Task 时，Coordinator 必须处理该控制指令，在完成相同的执行单元收尾后以 `cancelled` 关闭 Task，无须 Decider 的 `complete` 建议；不得仅将终止指令记录为 Cognition 而继续探索。Runtime、工具或协议实现发生不可恢复且使调度无法可靠继续的异常时，由外部控制面或承载平台完成执行单元隔离和状态收尾，并以 `failed` 关闭 Task；Coordinator 不自行登记该 outcome。

同一 Exploration 同一时间最多由一个实际 Explorer 执行。Explorer 失联、超时或已经结束但未登记协议结果时，Coordinator 必须核对最新协议状态和实际执行状态。Explorer 已经提交 finish 时，直接保留其结果，不重复结束；尚未提交时，必须先确认其已经停止或不能继续写入，再将对应 Running Exploration 结束为 `cancelled`，并在理由中记录具体异常，不代替 Explorer 推定执行成功或登记 `failed`。该状态变化构成重新评估事件；满足第 6.1 节条件时，应该调用 Decider。是否继续该方向以及是否通过 `exploration.retry` 创建新的 Todo Exploration，由 Decider 基于最新协议状态决定。任何角色都不得原地重新 assign 已结束的 Exploration。

用户提供任务提示或要求时，Coordinator 必须提取其中应共享的认知信息，通过 `cognition.add` 写为用户来源的断言认知。面向整体 Subject 的信息保存为全局用户认知；面向特定 Exploration 的信息必须原子完成认知持久化和目标 Exploration 的 `hints` ID 追加。目标 Exploration 仍在实际执行时，再尝试向对应 Explorer 投递已经保存的认知。认知创建和投递结果都构成重新评估事件；投递失败不得删除或回滚已经创建的 Cognition 或其 hints 关联，也不得将其视为尚未生效的协议输入。

### 6.4 Runtime 对齐与恢复

协议中的 Runtime 占用和 Running Exploration 表示逻辑状态，不单独证明原 Runtime 或实际 Explorer 仍然存活。Runtime 启动或恢复时，外部控制面或承载平台必须先保证不存在仍能并发修改该 Workspace 的旧 Runtime；Coordinator 无须通过占用布尔值识别旧 Runtime。用户确认继续 Task 后，Coordinator 必须在恢复探索调度前将协议记录与实际执行单元逐项对齐。

对齐必须遵循以下规则：

- Todo Exploration 没有已登记的实际执行实例；发现与其对应但尚未成功 assign 的 Explorer 时，必须停止或隔离该实例，再重新调度。
- Running Exploration 存在唯一、可验证且可继续控制的实际 Explorer 时，可以恢复观察与调度。
- Running Exploration 不存在可恢复的实际 Explorer 时，Coordinator 必须在确认原实例不能继续写入后，将尚未结束的 Exploration 以 `cancelled` 结束，再重新评估调度并按需调用 Decider；是否另行 retry 由 Decider 决定。核对时已被 Explorer 结束的记录保留原结果。
- 同一 Exploration 对应多个实际 Explorer 时，必须停止或隔离多余实例；无法可靠确定唯一实例时，必须全部停止或隔离，并由 Coordinator 将尚未结束的原 Exploration 以 `cancelled` 结束。
- Closed Task 或 Finished Exploration 对应的实际执行实例不得继续运行，必须停止或隔离。

外部控制面或承载平台能够安全建立唯一控制权，且完成上述对齐时，可以按用户指示继续当前 Open Task。无法可靠恢复 Runtime 或保证遗留执行单元不再写入时，必须由外部控制面或承载平台在完成隔离后以 `failed` 关闭 Task；用户或外部系统明确终止时可按取消流程以 `cancelled` 关闭。不得以恢复失败为由让 Coordinator 自行登记 Task 的 `failed`。

继续一个已经关闭的相同 Purpose 时，可以通过 `task.reopen` 恢复原 Task；需要重新执行 Finished Exploration 时，必须由 Decider 显式 retry，reopen 本身不得恢复或重新执行任何 Exploration。
