# Norns

## 概述

Norns 是一套面向特定模式的开放任务求解的多智能体协作套件，致力于通过任务状态图与领域能力实现更高效、更智能的任务求解效果。

## 特性与模块

Norns 套件特性：

- 任务状态图：以耦合任务求解过程的任务状态图表征任务探索过程与结果，提取更纯净的上下文以供决策使用。
- 领域能力：通过 Domain Extend Agents 实现领域能力注入，在不影响 Norns 协议的前提下提升领域任务上限与过程约束能力。
- 轨迹可读性：借助任务状态图与 Session Transcripts 转储机制，实现更易读的轨迹记录，进而支持更便捷的调试与训练数据沉淀需求。
- 多运行时节点调度：以 Norns Server 为物理机服务基础设施，提供运行时节点的并发调度能力。

Norns 套件由以下模块组成：

- Norns CLI：由特定角色智能体调用，遵循 Norns 协议实现对长程任务的独立认知与探索记录
- Nornsd Agent：包装 Code Agent 实现 Norns 角色规范与领域能力注入，启动交互式终端
- Nornsd Server：基于 Agent Runtime 实现并发调度批量命题的求解能力，可以耦合 Norns 模式，也可以仅用于原生 Claude Code 的调度管理
- Well：用于连接 Norns Server 提供的桌面客户端

## 使用文档

当前版本实现基于 Claude Code 2.1.199 版本的 Norns Agent

[快速开始](QUICKSTART.md#安装和使用) | [当前版本边界](QUICKSTART.md#当前版本边界) | [文档索引](QUICKSTART.md#文档)

## 项目结构

```text
documents/       协议规范与设计文档
norns/           Norns Protocol CLI
plugin/cc/       Claude Code 的 Norns Plugin 模板
nornsd/          Agent Runtime 的交互式终端与批量任务管理服务
well/            Nornsd Server 桌面客户端
domain/          可选领域知识包与模板示例
container/       通用 Nornsd Server、RedTeam 领域的 Nornsd Server 与自包含 Benchmark 及其 CC 对照组的容器构建文件
```

## 快速开始

安装依赖、获取二进制、配置 Agent、启动 nornsd Server 和运行 Docker 镜像的完整步骤见
[QUICKSTART.md](QUICKSTART.md#安装和使用)。

## 致谢

感谢以下开源项目，它们为 Norns 的设计与实现提供了重要参考和启发：

- [Cairn](https://github.com/oritera/Cairn)
- [aiscan](https://github.com/chainreactors/aiscan)
- [BreachWeave](https://github.com/m-sec-org/BreachWeave)
- [LuaN1aoAgent](https://github.com/SanMuzZzZz/LuaN1aoAgent)
- [PentestGPT](https://github.com/GreyDGL/PentestGPT)
- [PentestGPT v2](https://github.com/sickwell/PentestGPT-v2)
- [T3MP3ST](https://github.com/elder-plinius/T3MP3ST)

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
