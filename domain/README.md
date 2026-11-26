# Domain

`domain/` 保存 Norns 的领域知识和 Claude Code Skill。当前包含：

```text
domain/
├── math/                           # 数学领域 Agent 示例
└── redteam/
    ├── norns_domain_agents/        # Norns 红队领域 Agent
    └── skills/                     # 红队 Claude Code Skill
```

## Norns 领域 Agent

`AGENT_NORNS_EXTENDS_AGENTS_DIR` 指向一个领域 Agent 目录，且仅在
`AGENT_NORNS_ENABLE: true` 时可用。该目录必须包含严格 JSON 格式的
`meta.json`，以及其中引用的 Decider 和 Explorer 知识文件：

```json
{
  "domain_name": "example",
  "domain_decider_description": "领域决策器说明。",
  "domain_decider_extra_info_file": "decider.md",
  "domain_explorer_description": "领域探索器说明。",
  "domain_explorer_extra_info_file": "explorer.md"
}
```

- `domain_name` 只能包含小写字母、数字、下划线和连字符，最长 64 个字符。
- 两个 description 均不能为空。
- 两个知识文件路径必须是领域目录内的相对普通文件，不能指向目录外部。

领域知识只扩展 Agent 的专业判断，不改变 Norns 协议、通用角色边界或 Workspace
状态。

### Math

`math/` 是通用数学任务示例。在 `nornsd.yaml` 中使用本机目录的绝对路径启用：

```yaml
AGENT_NORNS_ENABLE: true
AGENT_NORNS_EXTENDS_AGENTS_DIR: "/absolute/path/to/norns/domain/math"
```

启用后，nornsd 会动态生成 `decider-for-math` 和 `explorer-for-math`。

### Redteam

`redteam/norns_domain_agents/` 保存红队任务使用的 Decider 和 Explorer 领域知识。
在 `nornsd.yaml` 中配置为：

```yaml
AGENT_NORNS_ENABLE: true
AGENT_NORNS_EXTENDS_AGENTS_DIR: "/absolute/path/to/norns/domain/redteam/norns_domain_agents"
AGENT_NORNS_CUSTOM_SKILLS_DIR: "/absolute/path/to/norns/domain/redteam/skills"
```

容器镜像中的对应路径为：

```yaml
AGENT_NORNS_EXTENDS_AGENTS_DIR: "/norns/domain/redteam/norns_domain_agents"
AGENT_NORNS_CUSTOM_SKILLS_DIR: "/norns/domain/redteam/skills"
```

## Claude Code Skill

`AGENT_NORNS_CUSTOM_SKILLS_DIR` 指向包含一个或多个 Skill 子目录的目录。每个 Skill 子目录
至少包含一个 `SKILL.md`。这些 Skill 会合并进 nornsd 为当前 Runtime 生成的临时
Norns Plugin。

不启用 Norns 模式时可以通过 `AGENT_CUSTOM_SKILLS_DIR` 加载同一套 Skill；
此时应清空 `AGENT_NORNS_EXTENDS_AGENTS_DIR` 和其他 `AGENT_NORNS_CUSTOM_*` 资源字段。

所有非空配置路径必须为绝对路径。完整自定义角色可使用 `AGENT_NORNS_CUSTOM_AGENTS_DIR`，
自定义 Main Agent 和复用历史 Runtime 插件分别使用 `AGENT_MAINAGENT_FILE`、
`AGENT_REUSE_RUNTIME_DIR`；格式和互斥规则见 [nornsd/README.md](../nornsd/README.md#资源与-plugin)。
