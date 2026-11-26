# Norns

## 概述

Norns 是一套面向开放任务求解的多智能体套件，致力于通过任务状态的自描述与可选的垂域能力增强实现上限更高、效率更好的任务求解效果。

Norns 套件由以下模块组成：

- Norns CLI：交由特定角色智能体调用，遵循 Norns 协议实现对长程任务的独立认知与探索记录。
- Nornsd Agent：通过 Plugin 机制包装 Code Agent 实现 Norns 角色规范与领域能力注入，启动交互式终端。
- Nornsd Server：对多个 Agent Runtime 进行并发调度以进行批量命题求解。可以耦合 Norns 模式，也可以仅用于原生 Code Agent 的并发调度管理。
- Well：用于连接 Norns Server 服务的桌面客户端。

## 特性与协议

Norns 模式特性：

- 任务状态图：以耦合任务求解过程的任务状态图表征任务探索过程与结果，提取更纯净的上下文以供决策使用。
- 领域能力：通过 Domain Extend Agents/Skills 实现领域能力注入，在不影响 Norns 协议的前提下提升领域任务上限并约束求解过程。
- 轨迹可读性：借助任务状态图与 Session Transcripts 转储机制，实现更易读的轨迹记录，进而支持更便捷的调试与训练数据沉淀、提取需求。
- 多运行时节点调度：以 Norns Server 为物理机服务基础设施，提供运行时节点的并发调度能力和更高维度的任务协作能力。

Norns 协议原语：

- Subject（目标实体）： 任务域（Task Domain）中待处理的抽象概念或具象化实例。
- Task（任务意图）： 施加于目标实体的操作规约与预期达成的终态目标。
- Cognition（认知信息）： 关于目标实体的断言认知（Task 初始信息、用户输入）和派生认知（探索发现）。
- Exploration（启发式探索）： 旨在扩充实体信息边界的主动式数据采集与知识发现机制。

## 安装和使用

当前版本 Norns Agent 实现基于的 Code Agent 范围仅包含 Claude Code v2.1.199。

安装依赖、获取二进制、配置 Agent、启动 nornsd Server 和运行 Docker 镜像的完整步骤见
[QUICKSTART.md](QUICKSTART.md#安装和使用)。

## 快照示例

基于 Norns 的自描述特性，可以手动在任务目录执行 `norns view visual` 命令以获取当前时刻实体状态的快照。示例如下：

<p align="center">
  <a href="./resources/readme/norns-1-visual.jpg"><img src="./resources/readme/norns-1-visual.jpg" alt="Norns 可视化快照" width="48%"></a>
  <a href="./resources/readme/norns-2-graph.jpg"><img src="./resources/readme/norns-2-graph.jpg" alt="Norns 知识图谱" width="48%"></a>
</p>
<p align="center">
  <a href="./resources/readme/norns-3-node.jpg"><img src="./resources/readme/norns-3-node.jpg" alt="Norns 节点详情" width="48%"></a>
  <a href="./resources/readme/norns-4-exp.jpg"><img src="./resources/readme/norns-4-exp.jpg" alt="Norns 探索记录" width="48%"></a>
</p>

在 Norns Server 模式下，则可以通过 Well 客户端连接并获取更方便的实体信息。示例如下：

<p align="center">
  <a href="./resources/readme/well-1-visual.jpg"><img src="./resources/readme/well-1-visual.jpg" alt="Well 实体状态视图" width="48%"></a>
  <a href="./resources/readme/well-2-event.jpg"><img src="./resources/readme/well-2-event.jpg" alt="Well 事件记录" width="48%"></a>
</p>

## 领域评测

### RedTeam

截至 f17ae8e 提交，以 DeepSeek-4.1-flash 驱动下的网络安全领域的 Tsec Benchmark 领域评测结果如下（不调用 hint 接口）：

<p align="center">
  <a href="./resources/benchmark/tsec_d41.jpg"><img src="./resources/benchmark/tsec_d41.jpg" alt="Tsec Benchmark 领域评测结果" width="720"></a>
</p>

## 项目结构

```text
documents/       协议规范与设计文档
norns/           Norns Protocol CLI
plugin/cc/       Claude Code 的 Norns Plugin 模板
nornsd/          Agent Runtime 的交互式终端与批量任务管理服务
well/            Nornsd Server 桌面客户端
domain/          可选领域知识包与模板示例
container/       通用 Nornsd Server、RedTeam 领域服务与自包含 Benchmark 的容器构建文件
```

## 致谢

感谢以下开源项目，它们为 Norns 的设计与实现提供了重要参考和启发：

- [Cairn](https://github.com/oritera/Cairn)
- [aiscan](https://github.com/chainreactors/aiscan)
- [BreachWeave](https://github.com/m-sec-org/BreachWeave)
- [LuaN1aoAgent](https://github.com/SanMuzZzZz/LuaN1aoAgent)
- [PentestGPT](https://github.com/GreyDGL/PentestGPT)
- [PentestGPT v2](https://github.com/sickwell/PentestGPT-v2)
- [T3MP3ST](https://github.com/elder-plinius/T3MP3ST)
- [sapper-v2](https://github.com/N1nEmAn/sapper-v2)
- [heimdall-agent](https://github.com/QiantangCredit/heimdall-agent)
- [StrikeAgent_AtkBrain-Flash](https://github.com/Yean-Sec/StrikeAgent_AtkBrain-Flash)

## 免责声明

本项目仅用于安全研究、技术验证和已获得明确授权的测试。使用者只能在本人拥有或已
取得合法授权的系统、网络和数据范围内使用本项目，不得将其用于未授权访问、攻击、
破坏、数据获取或其他违法违规活动。使用者应自行了解并遵守所在地区的法律法规及
目标系统的授权边界，并对使用行为及其后果承担全部责任。

Norns 和 nornsd 可以驱动第三方 Code Agent 自动执行命令、访问网络、调用工具及修改
工作区文件；部分运行模式会跳过交互式权限确认。模型输出和自动化操作具有不确定性，
可能产生错误、资源消耗、数据损坏或意外网络行为。请在隔离且权限受限的环境中运行，
事先备份重要数据，并对高风险操作和输出结果进行人工复核。

本项目按“现状”提供，不对适用性、可靠性、准确性、安全性或无侵权作任何明示或默示
保证。项目作者和贡献者不对因使用或无法使用本项目造成的直接或间接损失承担责任。
项目调用的模型服务、第三方工具和依赖仍受其各自的许可证、服务条款和隐私政策约束。
