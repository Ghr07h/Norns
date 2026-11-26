# Container Images

本目录维护三个基于 Python slim 的独立镜像。所有构建都以项目根目录为 Context，直接读取 `release/linux-<arch>/` 中的预编译二进制，并使用公开的 `nornsd/configs/nornsd/nornsd.example.yaml`。构建过程不会读取本地 `bin/`。

| 目录 | 默认镜像 | 用途 |
| --- | --- | --- |
| `nornsd/` | `nornsd-server:latest` | 通用 nornsd Server，仅包含基础运行环境 |
| `redteam/` | `nornsd-redteam-server:latest` | 内置 redteam Domain、Skill 和安全工具的 nornsd Server |
| `redteam_for_benchmark/` | `nornsd-redteam-benchmark:latest` | 内置 redteam nornsd 与 benchmark FIFO 调度器 |

每个镜像目录都提供专用 `Dockerfile.dockerignore` 限制构建 Context。真实 `nornsd.yaml`、Runtime 数据、日志和本地密钥不会作为镜像输入。

## 共同前置条件

先在项目根目录生成六个平台的 Release：

```sh
./release.sh --all-platforms
```

镜像只使用 `release/linux-x64` 或 `release/linux-arm64`。发布默认不编译命令扩展；需要搜索扩展时执行 `./release.sh --all-platforms --with-ext websearch`，多个扩展重复传入 `--with-ext <名称>`。`norns`、`nornsd` 和 Release 中已有的 `norns-*` 扩展必须来自同一次构建。

Benchmark 镜像必须包含 `tsec` 扩展，发布时执行 `./release.sh --all-platforms --with-ext tsec`；构建、打包和运行入口均会检查 `norns-tsec` 是否存在。扩展使用已有的 `BENCHMARK_BASE_URL`、`BENCHMARK_TOKEN`，无需额外配置。

协议、产品和 schema 版本由根目录 `versions.json` 维护，并在构建时写入二进制。
容器不另外维护或固定 norns/nornsd 产品版本、协议版本或 API schema，也不需要复制 `versions.json` 到镜像。更新清单后需重新生成 Linux Release，再构建镜像或运行 `package.sh`，不能沿用旧二进制。

`nornsd/configs/nornsd/nornsd.example.yaml` 使用当前 `SERVICE_*` / `AGENT_*` 配置。镜像验证会检查字段布局与二进制报告的配置 schema 元数据；不修改模板，也不迁移旧数据库。Node.js、Claude Code 等外部依赖的固定版本保持不变。

nornsd API 固定使用 `/api/`：服务信息请求 `GET /api/`（保留末尾斜杠），健康检查请求 `GET /api/health`，其余资源使用 `/api/subjects/...` 等路径。Benchmark 通过服务信息获取 `version`、整数 `api_schema` 等元数据，不将版本拼入 URL，也不固定比较产品版本。

Benchmark 的内部探活继续携带 Bearer Token；外部评测平台的 `/openapi/v1/challenges/*` 属于独立 SaaS 接口，保持不变。

Benchmark 在 Runtime 为 waiting 时，通过 nornsd 的 Subject 信息取得工作目录，在该目录执行 `norns task list --status open`，解析 JSON 信封的 `data.tasks`，判断 Task 是否仍打开，以决定是否追加继续指令。常规监控不再生成或传输 Status View 的整张图；未物化或尚未初始化的工作区视为暂时无法查询，不当作 Task 已关闭。命令错误或异常响应沿用尝试失败处理。首次、续跑及继续指令统一包含“任务信息”：寻找对应数量的 `xxx{xxx}` 格式 flag，发现后及时通知 Coordinator，由 Coordinator 使用 `norns tsec --unique-code <当前题目> --flag <原文>` 逐个提交，直到平台验收全部通过。控制面不再从事件、工具输出或认知中正则抽取并代交 flag，也不解析命令文本来推断是否通关。调度器通过题目列表轮询平台验收进度，并在 Runtime 停止后再次对账；超时、FIFO、同题并发、靶场启停及服务恢复机制继续保留。

收尾阶段创建同题新的并行 Subject 前，会在已登记的同题 Subject 工作目录执行 `norns view status --output-reference`，生成 YAML Status 文件。提示词的 `reference_context` 只追加这些文件的绝对路径，不内联 Status 正文；新分支可自行读取，优先尝试其他记录尚未覆盖的方向，并以当前任务提供的靶场信息为准。来源包括当前主分支、此前替换的主分支，以及本轮已创建的并行分支；未物化、未初始化或无法生成 Status 的 Subject 会跳过。并行分支的来源登记仅保存在当前评测进程内，跨进程仍只保留主分支的 Subject 映射。

提交命令先查询题目列表，仅在靶场 `available` 时提交；其他状态返回 `container_not_ready`，不发送 flag。退出码为 0 代表判定完成，仍须读取 `correct`；错误答案不能算通关，`duplicate` 按已正确登记处理。任务提示词禁止互联网检索与其他互联网资源，当前题目入口信息列出靶场目标。结果文件保留原字段结构：控制面不再持有提交回执，`flags_submitted`、`flags_correct` 保持空列表，`awarded` 为 0；验收数量和完成状态以平台为准。文档中的题目列表接口没有累计得分字段，`cumulative_score` 仅在平台实际提供该字段时同步，默认 0；每次提交的得分见 Coordinator 的工具输出。

容器回归测试统一维护在 `container/tests/`，运行 `python3 -m unittest discover -s container/tests -v`，需要 PyYAML，不依赖 Docker 或镜像内工具。用例覆盖三份验证器及 Benchmark 的配置与鉴权、入口启动和清理、Token 持久化、Event 游标分页、稳定 Runtime ID、FIFO 并发重试、轮次限时与收尾不限时、同题多开与共享靶场收尾、平台验收进度和服务恢复。

三个镜像使用相同的容器目录约定：

```text
/norns/bin/                         norns、nornsd、插件和 benchmark 脚本
/norns/domain/                      Domain 内容
/norns/config/nornsd.example.yaml    镜像内配置模板
/norns/config/nornsd.yaml            容器启动时生成的最终配置
/norns/engine/cc/                   Claude Code npm 安装目录
/workspace/                         Subject 工作目录、.nornsd/ 运行数据、日志和 Token
```

通用镜像复制整个项目 `domain/`；redteam 和 benchmark 镜像仅复制 `domain/redteam/`。三个目录各自保留完整 Dockerfile、入口和验证代码，不依赖 `container/` 下的公共运行文件。

三个 `build.sh` 支持相同参数，`--backend` 默认为 `docker`，`--use-mirror` 和 `--use-ecs-mirror` 默认关闭：

- `--backend apple|docker`
- `--image IMAGE`
- `--tag IMAGE`
- `--proxy host:port`
- `--use-mirror`
- `--use-ecs-mirror`

三个 Dockerfile 公开以下共同 Build ARG；两个 redteam 镜像另有相同的工具版本锚点及 `MAVEN_MIRROR_URL`：

```text
PYTHON_IMAGE
CLAUDE_CODE_VERSION
NODE_VERSION
NODE_DIST_URL
NORNS_RELEASE_DIR
HTTP_PROXY / HTTPS_PROXY / ALL_PROXY / NO_PROXY
http_proxy / https_proxy / all_proxy / no_proxy
PIP_INDEX_URL
PIP_TRUSTED_HOST
NPM_REGISTRY
APT_SOURCE_BLOCK
```

直接执行 `docker build` 时可用 `--build-arg` 覆盖；构建脚本根据镜像源和代理选项生成这些参数。

默认版本为 Python 3.13.15 slim、Node.js 24.22.1 和 Claude Code 2.1.199。

三个镜像目录都提供独立的 `package.sh`，用于生成可上传到远端构建机的最小项目树：

```sh
container/nornsd/package.sh
container/redteam/package.sh
container/redteam_for_benchmark/package.sh
```

归档分别写入对应目录下的 `nornsd.zip`、`redteam.zip`、`redteam_for_benchmark.zip`。每个压缩包包含两套 Linux Release、该镜像需要的
Domain、配置模板及自身构建文件；解压后从项目树根目录运行压缩包内的 `build.sh`。

## 运行时环境变量约定

以下分别列出三个镜像的必选、可选业务环境变量；默认值指未设置该变量时的行为。这些变量由入口或 benchmark 脚本显式读取，不等同于可以用环境变量覆盖配置文件中的任意字段。构建期 `ARG` 见前文；`PATH`、代理、各工具自身的环境变量沿用对应工具的规则。

三个镜像均固定将 `SERVICE_NORNSD_ROOT_DIR` 写为 `/workspace`，将 `AGENT_EXEC` 写为镜像内 Claude Code 的绝对路径。直接设置这两个环境变量不会覆盖入口生成的值；持久化 Root 请挂载 `/workspace`。入口生成 `/norns/config/nornsd.yaml`，通过 `nornsd server --configs` 启动服务。

服务与 Agent 配置仅接受下表中的 `SERVICE_*`、`AGENT_*` 名称。必选字符串必须非空；可选布尔值支持 `true/false`、`1/0`、`yes/no`、`on/off`。服务与 Agent 的整数配置须为正整数，端口还须符合服务端合法端口范围；评测参数的取值范围见各自说明，部分参数允许使用 `0`。

所有资源路径均指容器内已存在的绝对路径，可通过 Volume 挂载。

`AGENT_CUSTOM_PLUGIN_DIRS` 可传单个目录字符串或 YAML/JSON 字符串列表，例如 `--env AGENT_CUSTOM_PLUGIN_DIRS='["/plugins/a", "/plugins/b"]'`；空字符串或 `[]` 表示不添加插件。

两个 redteam 镜像的内置扩展和 Skill 也可通过对应变量传空字符串关闭。

`AGENT_NORNS_EXTENDS_AGENTS_DIR`、`AGENT_NORNS_CUSTOM_AGENTS_DIR` 和 `AGENT_NORNS_CUSTOM_SKILLS_DIR` 的非空值仅允许在 Norns 模式使用；`AGENT_CUSTOM_AGENTS_DIR`、`AGENT_CUSTOM_SKILLS_DIR` 和 `AGENT_CUSTOM_PLUGIN_DIRS`的非空值仅允许在普通模式使用。关闭 Norns 模式或设置复用目录时，入口不自动注入内置 Norns 资源。`AGENT_MAINAGENT_FILE` 在两种模式均可使用；除非了解修改 Main Agent 的影响，否则请保留默认值。

`AGENT_REUSE_RUNTIME_DIR` 与其他非空插件资源及 `AGENT_MAINAGENT_FILE` 互斥。nornsd 读取旧 Runtime 的 `launch.json`，将旧 `plugins/` 复制到新 Runtime，并沿用旧 Main Agent；此时探索并发数、超时问询周期和人类干预配置不再改写插件，LLM 配置仍使用新值。

`SERVICE_TOKEN` 未设置或为空时，优先复用 `/workspace/.nornsd-token`，也可读取旧 `/workspace/.nornsd-password`；没有已有 Token 时生成随机值。显式传入非空 Token 时会覆盖并持久化。

可用 `docker exec <container> cat /workspace/.nornsd-token` 查看当前 Token。

## 通用 nornsd

`nornsd/` 默认使用 `python:3.13.15-slim`，固定安装 Node.js 24.22.1 和 Claude Code，并以 root 启动 nornsd Server。镜像只内置公开配置模板，运行时通过白名单环境变量生成完整配置；默认启用 Norns 模式，Server Runtime 槽位数为 1，Norns 探索并发数为 3。

构建脚本会按当前主机架构选择 `release/linux-x64` 或 `release/linux-arm64`：

```sh
container/nornsd/build.sh
```

Apple Container 使用 `--backend apple`。可用 `--image` 或 `--tag` 覆盖基础镜像和输出镜像，并可使用 `--proxy`、`--use-mirror` 或 `--use-ecs-mirror` 控制构建网络。

启动示例：

```sh
docker run --detach --rm \
  --name nornsd \
  --publish 28081:28081 \
  --volume nornsd-data:/workspace \
  --env AGENT_LLM_URL='https://example.test/anthropic' \
  --env AGENT_LLM_KEY='replace-me' \
  --env AGENT_LLM_MODEL='model-name' \
  nornsd-server:latest
```

### 必选环境变量

| 变量 | 解释 |
| --- | --- |
| `AGENT_LLM_URL` | LLM API 基础地址。 |
| `AGENT_LLM_KEY` | LLM API 访问密钥。 |
| `AGENT_LLM_MODEL` | 主模型名称。 |

### 可选环境变量

| 变量 | 默认值 | 解释 |
| --- | --- | --- |
| `AGENT_LLM_FAST_MODEL` | 与主模型相同 | 快速模型名称；设置为空时也回退到主模型。 |
| `SERVICE_HOST` | `0.0.0.0` | nornsd 监听地址。 |
| `SERVICE_PORT` | `28081` | nornsd 监听端口。 |
| `SERVICE_TOKEN` | 复用或自动生成 | 本地 nornsd API 的 Bearer Token，持久化到 `/workspace/.nornsd-token`。 |
| `SERVICE_CONCURRENCY` | `1` | 服务同时运行的 Runtime 槽位数，须为正整数。 |
| `AGENT_NORNS_ENABLE` | `true` | 是否启用 Norns 模式；`false` 使用普通 Claude Code 模式。 |
| `AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS` | `3` | 单个 Norns Task 的额定探索并发数，与服务 Runtime 槽位数不同。 |
| `AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS` | `1200` | 运行中 Exploration 的超时问询周期，单位秒。 |
| `AGENT_NORNS_ALLOW_HUMAN_INTERVENTION` | `false` | 是否允许 Coordinator 向人类提问并等待实时回答。 |
| `AGENT_NORNS_EXTENDS_AGENTS_DIR` | 空字符串 | Norns 角色扩展目录；读取 `meta.json` 及其引用内容，结合通用角色生成领域角色。 |
| `AGENT_NORNS_CUSTOM_AGENTS_DIR` | 空字符串 | 完整自定义 Norns Agent Markdown 目录；原样复制，并将角色名称加入 Coordinator 白名单。 |
| `AGENT_NORNS_CUSTOM_SKILLS_DIR` | 空字符串 | 追加到 Norns Plugin 的 Skill 目录。 |
| `AGENT_CUSTOM_AGENTS_DIR` | 空字符串 | 普通模式自定义 Agent Markdown 目录。 |
| `AGENT_CUSTOM_SKILLS_DIR` | 空字符串 | 普通模式自定义 Skill 目录。 |
| `AGENT_CUSTOM_PLUGIN_DIRS` | `[]` | 普通模式额外 Plugin 目录列表，按列表顺序复制、加载。 |
| `AGENT_MAINAGENT_FILE` | 空字符串 | 自定义 Main Agent Markdown 文件；Norns 模式替换 Coordinator，普通模式复制到 custom Plugin，并以 frontmatter 的 `name` 指定启动 Agent。 |
| `AGENT_REUSE_RUNTIME_DIR` | 空字符串 | 历史 Runtime 运行数据目录；复用其插件和 Main Agent，路径须包含 `launch.json` 与 `plugins/`。 |

## Redteam nornsd

`redteam/` 在 Python slim 上安装 Node.js 24、Claude Code、渗透测试工具链，以及
`domain/redteam` 的 Agent 扩展和 Skill。工具安装、版本锚点、Maven 镜像和功能验证与 benchmark 镜像一致，包括 Java 8/11/21、Maven、JADX/APKTool、pwndbg、取证/OCR、数据库客户端及 Playwright 浏览器；服务镜像不包含评测调度器。默认启用 Norns 模式并监听 `0.0.0.0:28081`。

构建脚本会按当前主机架构选择 Linux Release：

```sh
container/redteam/build.sh
```

Apple Container 使用 `--backend apple`。可选构建参数：

- `--image IMAGE`：覆盖 Python 基础镜像；
- `--tag IMAGE`：覆盖输出镜像；
- `--proxy host:port`：为国外资源配置构建期代理；
- `--use-mirror`：使用阿里云公网 APT/PyPI 和 npmmirror；
- `--use-ecs-mirror`：使用阿里云 ECS 内网 APT/PyPI，并自动启用其他国内镜像。

启动示例：

```sh
docker run --detach \
  --name norns-redteam \
  --publish 28081:28081 \
  --volume norns-redteam-workspace:/workspace \
  --env AGENT_LLM_URL='https://example.test/anthropic' \
  --env AGENT_LLM_KEY='replace-me' \
  --env AGENT_LLM_MODEL='model-name' \
  nornsd-redteam-server:latest
```

### 必选环境变量

| 变量 | 解释 |
| --- | --- |
| `AGENT_LLM_URL` | LLM API 基础地址。 |
| `AGENT_LLM_KEY` | LLM API 访问密钥。 |
| `AGENT_LLM_MODEL` | 主模型名称。 |

### 可选环境变量

| 变量 | 默认值 | 解释 |
| --- | --- | --- |
| `AGENT_LLM_FAST_MODEL` | 与主模型相同 | 快速模型名称；设置为空时也回退到主模型。 |
| `SERVICE_HOST` | `0.0.0.0` | nornsd 监听地址。 |
| `SERVICE_PORT` | `28081` | nornsd 监听端口。 |
| `SERVICE_TOKEN` | 复用或自动生成 | 本地 nornsd API 的 Bearer Token，持久化到 `/workspace/.nornsd-token`。 |
| `SERVICE_CONCURRENCY` | `1` | 服务同时运行的 Runtime 槽位数，须为正整数。 |
| `AGENT_NORNS_ENABLE` | `true` | 是否启用 Norns 模式；`false` 使用普通 Claude Code 模式。 |
| `AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS` | `3` | 单个 Norns Task 的额定探索并发数，与服务 Runtime 槽位数不同。 |
| `AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS` | `1200` | 运行中 Exploration 的超时问询周期，单位秒。 |
| `AGENT_NORNS_ALLOW_HUMAN_INTERVENTION` | `false` | 是否允许 Coordinator 向人类提问并等待实时回答。 |
| `AGENT_NORNS_EXTENDS_AGENTS_DIR` | `/norns/domain/redteam/extend_agents` | Norns 角色扩展目录；读取 `meta.json` 及其引用内容，结合通用角色生成领域角色。 |
| `AGENT_NORNS_CUSTOM_AGENTS_DIR` | 空字符串 | 完整自定义 Norns Agent Markdown 目录；原样复制，并将角色名称加入 Coordinator 白名单。 |
| `AGENT_NORNS_CUSTOM_SKILLS_DIR` | `/norns/domain/redteam/custom_skills` | 追加到 Norns Plugin 的 Skill 目录。 |
| `AGENT_CUSTOM_AGENTS_DIR` | 空字符串 | 普通模式自定义 Agent Markdown 目录。 |
| `AGENT_CUSTOM_SKILLS_DIR` | 空字符串 | 普通模式自定义 Skill 目录。 |
| `AGENT_CUSTOM_PLUGIN_DIRS` | `[]` | 普通模式额外 Plugin 目录列表，按列表顺序复制、加载。 |
| `AGENT_MAINAGENT_FILE` | 空字符串 | 自定义 Main Agent Markdown 文件；Norns 模式替换 Coordinator，普通模式复制到 custom Plugin，并以 frontmatter 的 `name` 指定启动 Agent。 |
| `AGENT_REUSE_RUNTIME_DIR` | 空字符串 | 历史 Runtime 运行数据目录；复用其插件和 Main Agent，路径须包含 `launch.json` 与 `plugins/`。 |

默认 Root 是 `/workspace`；服务数据位于 `/workspace/.nornsd/`，Subject 工作目录以 `<subject_uuid>_<UTC>` 命名。Server Token 持久化到 `/workspace/.nornsd-token`。镜像以
root 运行以保留部分安全工具能力，但默认不增加额外 capability；仅在任务确有需要时
增加 `NET_RAW` 或 `NET_ADMIN`，不要默认使用 `--privileged`。

镜像验证：

```sh
docker run --rm --entrypoint python3 \
  nornsd-redteam-server:latest \
  /usr/local/bin/nornsd-verify
```

## Redteam benchmark

`redteam_for_benchmark/` 是自包含评测镜像：镜像内启动一个 nornsd，worker 持续消费同一个 FIFO 尝试队列。nornsd Runtime 槽位和 worker 数固定为 3，不提供并发环境变量。这不改变单个 Task 内的 `AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS`。
首轮单题限时 30 分钟，后续每轮固定 40 分钟。未完成题目回到队尾，不等待其他题目的首轮结束；第二轮及后续轮次使用 `resume_session` 延续最新 Runtime Session。

当所有剩余未完成题目均已运行、没有其他待调度题目（包括待重试题目），且数量不超过 3 时，进入不限单题时间的收尾阶段。最后三道题都已运行，或只剩两道题运行时，都会动态忽略单题限时，不停止或重启当前 Runtime；若还有题目排队则暂不进入收尾阶段。全局超时、停止信号、SaaS 任务结束和异常处理保持有效。未通关题目仍可重试，不再通过递增限时及其上限限制重试轮数；进入收尾阶段后，后续尝试继续保持不限单题时间。

收尾阶段剩余未通关题目少于 3 道时，空闲槽位用于同题多开：优先选择当前并行尝试数最少的题目，数量相同时按原题目顺序选择。例如 A、B 各有一个尝试时，先为 A 增加一个；仅剩 A 时，A 可占满 3 个槽位。每次补位都创建全新 Subject 和 Runtime，不续接上一个补位分支的会话；原主分支仍按既有规则复用 Subject 并恢复会话。

同题各分支共享一个 SaaS 靶场，任意分支通关后停止该题其余 Runtime，全部收尾后才关闭靶场；单个分支失败不会关闭其他分支仍在使用的靶场。全局并发始终不超过 3。

结果仍按不同题目统计通关数和得分，`attempts` 中记录各次尝试的 `subject_id`、`runtime_id` 和 `fresh_subject`，用于区分独立补位分支与主分支重试。

重试指令使用“对应结束原因的恢复指令前缀 + 公共上下文”：区分正常结束但未通关、内存超限、单题超时及外部取消，并填入题目编号、平台验收进度与当前靶场地址，提醒靶场重启会重置状态且可能变更地址。其他异常使用实际错误原因，不冒充超时或内存超限。平台到期、benchmark 全局停止或停止信号不会自动恢复。结果文件同时保留 `runtime_state`、`runtime_end_reason` 和 `runtime_error_code`，Runtime 错误与取消不再统一记录为 `ended`。

结果中的`timeout_disabled: true` 表示已取消单题时限；原有 `timeout_minutes` 保留各轮的名义预算，此时不再用于中止 Runtime。

nornsd 的连接故障或 HTTP 5xx 会暂停整个队列的新派发和开靶，当前失败尝试清理后进入有次数上限的服务恢复流程。默认最多恢复 3 次，重新派发前分别等待 5、10、20 秒并检查健康状态；每次健康等待最多 300 秒。服务恢复失败或超过次数上限时，保存进度并以非零状态退出，不把题目标为不可解。成功执行 Runtime 后重置连续恢复次数，单纯健康检查通过不重置。基础设施失败单独记入结果的 `service_failures`，不消耗解题轮次；最终失败原因写入 `infrastructure_error`。收尾阶段的不限时策略不绕过这些保护。

错误和服务恢复重试日志默认携带 API 返回的 SQLite 原因、`sqlite_code` 和请求 ID；Runtime 结束及普通重试日志携带状态、结束原因、错误码和错误消息，无需开启`BENCHMARK_DIAGNOSTICS`。这些原因直接输出到 stdout 和过程日志，便于在托管平台的有限日志窗口中定位故障；原因中的已知凭据会脱敏，换行合并为单行，过长内容截断。

构建：

```sh
container/redteam_for_benchmark/build.sh
```

构建脚本支持与 redteam Server 相同的 `--backend`、`--image`、`--tag`、`--proxy`、
`--use-mirror` 和 `--use-ecs-mirror` 参数。

需要上传到远端构建时，运行该目录的统一打包脚本：

```sh
container/redteam_for_benchmark/package.sh
```

压缩包写入 `container/redteam_for_benchmark/redteam_for_benchmark.zip`。解压后应从项目树
根目录执行同一构建命令。

运行时必须提供 benchmark 地址、Token 和单一模型配置：

```sh
docker run --rm \
  --name norns-redteam-benchmark \
  --volume norns-redteam-benchmark-workspace:/workspace \
  --env BENCHMARK_BASE_URL='https://benchmark.example.test' \
  --env BENCHMARK_TOKEN='replace-me' \
  --env AGENT_LLM_URL='https://llm.example.test/anthropic' \
  --env AGENT_LLM_KEY='replace-me' \
  --env AGENT_LLM_MODEL='model-name' \
  nornsd-redteam-benchmark:latest
```

### 必选环境变量

| 变量 | 解释 |
| --- | --- |
| `AGENT_LLM_URL` | LLM API 基础地址。 |
| `AGENT_LLM_KEY` | LLM API 访问密钥。 |
| `AGENT_LLM_MODEL` | 主模型名称；必须只有一个模型，不支持逗号分隔的多模型列表。 |
| `BENCHMARK_BASE_URL` | SaaS 评测服务基础地址，不含 `/openapi/v1/challenges` 路径。 |
| `BENCHMARK_TOKEN` | SaaS 评测服务 Token，与本地 nornsd 的 `SERVICE_TOKEN` 相互独立。 |

### 可选环境变量：服务与 Agent

| 变量 | 默认值 | 解释 |
| --- | --- | --- |
| `AGENT_LLM_FAST_MODEL` | 与主模型相同 | 快速模型名称；设置为空时也回退到主模型。 |
| `SERVICE_HOST` | `0.0.0.0` | nornsd 监听地址。内部探活和调度固定访问 `127.0.0.1`，监听地址须覆盖本机回环访问。 |
| `SERVICE_PORT` | `28081` | nornsd 监听端口。内部探活和调度随之使用该端口。 |
| `SERVICE_TOKEN` | 复用或自动生成 | 本地 nornsd API 的 Bearer Token，持久化到 `/workspace/.nornsd-token`。 |
| `AGENT_NORNS_ENABLE` | `true` | 是否启用 Norns 模式；`false` 使用普通 Claude Code 模式。 |
| `AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS` | `3` | 单个 Norns Task 的额定探索并发数，与服务 Runtime 槽位数不同。 |
| `AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS` | `1200` | 运行中 Exploration 的超时问询周期，单位秒。 |
| `AGENT_NORNS_ALLOW_HUMAN_INTERVENTION` | `false` | 是否允许 Coordinator 向人类提问并等待实时回答。 |
| `AGENT_NORNS_EXTENDS_AGENTS_DIR` | `/norns/domain/redteam/extend_agents` | Norns 角色扩展目录；读取 `meta.json` 及其引用内容，结合通用角色生成领域角色。 |
| `AGENT_NORNS_CUSTOM_AGENTS_DIR` | 空字符串 | 完整自定义 Norns Agent Markdown 目录；原样复制，并将角色名称加入 Coordinator 白名单。 |
| `AGENT_NORNS_CUSTOM_SKILLS_DIR` | `/norns/domain/redteam/custom_skills` | 追加到 Norns Plugin 的 Skill 目录。 |
| `AGENT_CUSTOM_AGENTS_DIR` | 空字符串 | 普通模式自定义 Agent Markdown 目录。 |
| `AGENT_CUSTOM_SKILLS_DIR` | 空字符串 | 普通模式自定义 Skill 目录。 |
| `AGENT_CUSTOM_PLUGIN_DIRS` | `[]` | 普通模式额外 Plugin 目录列表，按列表顺序复制、加载。 |
| `AGENT_MAINAGENT_FILE` | 空字符串 | 自定义 Main Agent Markdown 文件；Norns 模式替换 Coordinator，普通模式复制到 custom Plugin，并以 frontmatter 的 `name` 指定启动 Agent。 |
| `AGENT_REUSE_RUNTIME_DIR` | 空字符串 | 历史 Runtime 运行数据目录；复用其插件和 Main Agent，路径须包含 `launch.json` 与 `plugins/`。 |

`SERVICE_CONCURRENCY` 在本镜像中不生效；nornsd Runtime 槽位和评测 worker 数固定为 3。

每次实际开靶请求前（含开靶失败后的重试），以及新分支复用已开启的共享靶场时，都会重新查询平台题目列表并同步该题验收进度。若平台已记录通关，则直接标记完成，不再开启靶场或创建 Runtime；即使此前提交响应丢失，后续尝试也能据此收敛。平台状态查询失败时不会使用缓存状态开靶，沿用现有失败重试流程。

开靶、关靶均固定最多尝试 5 次（包含首次），四次重试前分别等待 5、10、30、30 秒。成功或收到明确终止条件时提前结束，第五次失败后不再等待。关靶返回 `closed: false` 也会重试；题目不存在或 duplicate 视为已经关闭，`invalid_state` 仍交由上层判断任务状态。

### 可选环境变量：评测调度与输出

| 变量 | 默认值 | 解释 |
| --- | --- | --- |
| `BENCHMARK_INITIAL_LIMIT_MINUTES` | `30` | 首轮单题限时，单位分钟，须大于 0。 |
| `BENCHMARK_RETRY_LIMIT_MINUTES` | `40` | 第二轮及后续每轮的固定单题限时，单位分钟，须大于 0。 |
| `BENCHMARK_OVERALL_TIMEOUT` | `0` | 整个评测的限时，单位秒；`0` 表示不限。 |
| `BENCHMARK_ONLY_CODES` | 空字符串 | 仅运行指定题目的 `unique_code`，多个值用逗号分隔；空值运行全部题目。 |
| `BENCHMARK_POLL_INTERVAL` | `4` | Runtime、Event 和平台验收进度轮询间隔，单位秒，须大于 0。 |
| `BENCHMARK_EVENT_LIMIT` | `5000` | 每次读取 Event 的上限，取值范围 `1..5000`。 |
| `BENCHMARK_MAX_CONTINUE` | `6` | 单次解题尝试内，Task 已关闭但尚未通关且 Runtime 仍处于 waiting 时，最多追加继续指令的次数；`0` 不追加。 |
| `BENCHMARK_CONTINUE_COOLDOWN` | `20` | 同一尝试内追加继续指令的最小间隔，单位秒。 |
| `BENCHMARK_CONTAINER_READY_TIMEOUT` | `120` | 开靶后等待靶场变为 available 的最长时间，单位秒；到期回退到 start 返回的地址。 |
| `BENCHMARK_CONTAINER_READY_INTERVAL` | `3` | 等待靶场就绪时查询平台的间隔，单位秒。 |
| `BENCHMARK_SERVICE_RECOVERY_ATTEMPTS` | `3` | 连续服务故障最多恢复次数，须大于等于 0；`0` 表示首次故障即停止。 |
| `BENCHMARK_SERVICE_RECOVERY_INTERVAL` | `5` | 服务恢复前的基础等待秒数，须大于 0；连续失败时指数增加。 |
| `BENCHMARK_HEALTH_WAIT` | `300` | 评测脚本启动及每次服务恢复时等待 nornsd 健康的上限，单位秒，须大于 0。 |
| `BENCHMARK_STOP_GRACE` | `90` | 评测脚本等待 Runtime 优雅停止的时间，单位秒。 |
| `ENTRYPOINT_HEALTH_WAIT` | `180` | 容器入口启动 nornsd 后的健康等待上限，单位秒；与评测脚本的 `BENCHMARK_HEALTH_WAIT` 分别计算。 |
| `ENTRYPOINT_STOP_WAIT` | `45` | 容器退出时等待 nornsd 响应 SIGTERM 的时间，单位秒；超时后升级为 SIGKILL。 |
| `BENCHMARK_RESULT_FILE` | `/workspace/benchmark-result.json` | 增量结果 JSON 文件；启动时将同路径的旧结果改名归档为 `.previous-<run_id>`。 |
| `BENCHMARK_SUBJECT_MAP_DIR` | `/workspace` | Subject 注册映射文件的目录；文件名为 `.benchmark-subjects-<port>.json`。 |
| `BENCHMARK_LOG_FILE` | `/workspace/benchmark-process.log` | 过程日志文件，追加写入；默认使用工作目录 `/workspace`，写入失败时仍输出到终端。 |
| `BENCHMARK_DIAGNOSTICS` | `0` | 设为 `1`、`true`、`yes` 或 `on` 输出额外诊断日志。 |
| `BENCHMARK_TEST_ENV` | `0` | 设为 `1`、`true`、`yes` 或 `on` 在提示词中追加临时测试环境说明；入口参数 `--test` 也会将其设为 `1`。 |

不再读取 `FLAG_PATTERN`；flag 由智能体判断并通过 `tsec` 原样提交，支持 API 允许的 1～4096 个 Unicode 字符。

### 由入口管理的变量

下列变量不作为正常镜像启动时的可选配置：

| 变量 | 正常入口中的处理 |
| --- | --- |
| `BENCHMARK_NORNSD_BASE_URLS` | 自动设置为本容器 nornsd 的回环地址及实际 `SERVICE_PORT`，覆盖外部值。 |
| `BENCHMARK_NORNSD_TOKEN` | 自动设置为入口持久化的 `SERVICE_TOKEN`，覆盖外部值。 |
| `BENCHMARK_RUN_ID` | 每次启动生成新 ID，覆盖外部值，用于校验结果属于本次运行。 |

本地复现异常时，可在原有容器启动参数中追加 `--env BENCHMARK_DIAGNOSTICS=1`。
Benchmark 会向 stdout 和过程日志输出带 `[DIAG]` 标记的诊断：Runtime 的错误详情、进程退出信息与会话恢复来源，错误 Runtime 对应的最近 stderr，脚本自身的开靶、关靶请求及响应，以及平台验收进度快照。每条诊断包含独立 UTC 时间；平台请求使用请求 ID 关联响应，新增诊断会脱敏已知凭据并截断过长内容，不改变原有日志内容和结果文件格式。stderr 只读取 API 已返回的本地 raw 日志路径的最后 64 KiB，并筛选当前 Runtime 的最后 5 条记录；远端路径不可读或日志尚未写完时可能没有 stderr，需要保留工作区原始日志。诊断开关本身不增加 API 请求、不改变状态同步逻辑；Coordinator 的 `norns tsec` 提交回执应查看 Agent 工具事件，不属于脚本自身的平台请求诊断。

过程日志写入 `/workspace/benchmark-process.log`，nornsd Token 持久化到 `/workspace/.nornsd-token`。镜像验证：

```sh
docker run --rm --entrypoint python3 \
  nornsd-redteam-benchmark:latest \
  /usr/local/bin/nornsd-verify
```

## 安全边界

- 三个镜像都只复制公开的配置模板，不复制本地 `bin/nornsd.yaml`；
- `AGENT_LLM_KEY`、benchmark Token 和 Server Token 只在运行时注入；
- 不要把包含真实凭据的 `.env`、Runtime 工作区或构建缓存发布为镜像层；
- nornsd HTTP API 使用明文 Bearer Token，跨主机部署时应位于可信网络或 TLS 反向代理后。
