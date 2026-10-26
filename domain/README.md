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

`NORNS_CUSTOM_DOMAIN_DIR` 指向一个领域 Agent 目录。该目录必须包含严格 JSON 格式的
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

`math/` 是通用数学任务示例。如果 `agent.yaml` 位于项目根目录，可以这样启用：

```yaml
NORNS_CUSTOM_DOMAIN_DIR: "./domain/math"
```

如果使用项目默认的 `bin/agent.yaml`，则路径为：

```yaml
NORNS_CUSTOM_DOMAIN_DIR: "../domain/math"
```

启用后，nornsd 会动态生成 `decider-for-math` 和 `explorer-for-math`。

### Redteam

`redteam/norns_domain_agents/` 保存红队任务使用的 Decider 和 Explorer 领域知识。
使用项目默认的 `bin/agent.yaml` 时配置为：

```yaml
NORNS_CUSTOM_DOMAIN_DIR: "../domain/redteam/norns_domain_agents"
NORNS_CUSTOM_SKILLS_DIR: "../domain/redteam/skills"
```

容器镜像中的对应路径为：

```yaml
NORNS_CUSTOM_DOMAIN_DIR: "/norns/domain/redteam/norns_domain_agents"
NORNS_CUSTOM_SKILLS_DIR: "/norns/domain/redteam/skills"
```

## Claude Code Skill

`NORNS_CUSTOM_SKILLS_DIR` 指向包含一个或多个 Skill 子目录的目录。每个 Skill 子目录
至少包含一个 `SKILL.md`。这些 Skill 会合并进 nornsd 为当前 Runtime 生成的临时
Norns Plugin。

不启用 Norns 模式的普通 Claude Code 可以通过 `CUSTOM_SKILLS_DIR` 加载同一套 Skill；
它不会加载 `NORNS_CUSTOM_DOMAIN_DIR` 中的 Decider 和 Explorer 定义。

所有相对路径均以声明它们的 `agent.yaml` 所在目录为基准解析；部署到其他位置时也
可以直接使用绝对路径。
