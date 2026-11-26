# Norns 快速开始

本文介绍如何安装并启动 Norns。项目定位和模块结构见 [README.md](README.md)，
各模块的完整参考资料见本文的[文档列表](#文档列表)章节。

## 安装和使用

### 1. 获取 Norns 二进制

`norns`、`nornsd` 和同目录的 `norns-*` 插件必须来自同一次构建或同一个 Release。
可以选择从源码构建，也可以直接下载预编译产物。

使用 Go 1.25 或更高版本从源码构建：

```sh
./release.sh
```

脚本默认只编译当前平台的 `norns`、`nornsd`，将结果写入
`release/<os>-<arch>/`。命令扩展默认不编译，例如使用 `./release.sh --with-ext websearch` 加入搜索扩展；`--with-ext` 可重复指定。需要全部六个 Go 目标时增加 `--all-platforms`。
选择当前系统对应的目录，把其中 Go 可执行文件复制到 `bin/`：

```sh
# 按当前系统修改为 darwin-arm64、darwin-x64、linux-arm64 或 linux-x64。
platform=darwin-arm64

mkdir -p ./bin
cp "./release/$platform/"norns* ./bin/
chmod +x ./bin/*
```

不安装 Go 时，可以从 [GitHub Releases](https://github.com/Ghr07h/norns/releases)
下载当前系统和架构对应的预编译产物，解压后同样将该平台目录中的全部文件放入
`./bin/`。不要混用不同版本或不同构建中的 `norns`、`nornsd` 与插件。

项目不会自动修改用户环境。启动前需要手动将 `bin/` 加入 PATH；以下设置只对当前
shell 生效，需要长期使用时请自行写入对应的 shell 配置：

```sh
export PATH="$(pwd)/bin:$PATH"

norns version --json
nornsd version --json
```

Windows 用户将 `release/windows-x64/` 或 `release/windows-arm64/` 中的 `.exe`
文件复制到 `bin\`，再手动将该目录加入用户 PATH。

### 2. 安装 Claude Code

Norns 当前要求 Claude Code 2.1.199，并需要可用的模型 API 地址、密钥和模型名称。

建议在当前用户的 `~/cc` 目录本地安装要求的 Claude Code 版本：

```sh
mkdir -p ~/cc
cd ~/cc
npm install @anthropic-ai/claude-code@2.1.199 --registry=https://registry.npmmirror.com
```

全新安装且从未运行过的 Claude Code 初次启动时会尝试联网进入 onboarding，可以
写入完成标记跳过该步骤。如果此前已有 Claude Code 运行记录，则不需要也不应执行
以下命令，因为它会覆盖已有的 `~/.claude.json`：

```sh
cat > ~/.claude.json <<'EOF'
{
  "hasCompletedOnboarding": true
}
EOF
```

本地安装后，Claude Code 可执行文件位于 `~/cc/node_modules/.bin/claude`。由于
`AGENT_EXEC` 不经过 shell，配置时需要将 `~` 展开为当前用户主目录的绝对路径：

```yaml
AGENT_EXEC:
  - "/absolute/path/to/home/cc/node_modules/.bin/claude"
```

### 3. 配置 Agent

回到 Norns 项目根目录。首次使用时复制配置模板；如果 `bin/nornsd.yaml` 已存在，不要
覆盖其中的访问密钥和自定义配置：

```sh
test -e ./bin/nornsd.yaml || cp ./nornsd/configs/nornsd/nornsd.example.yaml ./bin/nornsd.yaml
```

编辑 `bin/nornsd.yaml`，设置工作根目录、模型连接信息，并确认 `AGENT_EXEC` 指向上一步
安装的 Claude Code。配置不再包含 `VERSION`；下面列出常用字段：

```yaml
SERVICE_NORNSD_ROOT_DIR: "/absolute/path/to/nornsd-root"
SERVICE_HOST: "127.0.0.1"
SERVICE_PORT: 28081
SERVICE_TOKEN: "replace-with-a-long-random-token"
SERVICE_CONCURRENCY: 2

AGENT_LLM_URL: "https://your-api.example.com"
AGENT_LLM_KEY: "replace-with-secret"
AGENT_LLM_MODEL: "model-name"
AGENT_LLM_FAST_MODEL: ""

AGENT_NORNS_ENABLE: true
AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS: 3
AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS: 1200
AGENT_NORNS_ALLOW_HUMAN_INTERVENTION: false

AGENT_NORNS_EXTENDS_AGENTS_DIR: ""
AGENT_NORNS_CUSTOM_SKILLS_DIR: ""
AGENT_NORNS_CUSTOM_AGENTS_DIR: ""

AGENT_CUSTOM_AGENTS_DIR: ""
AGENT_CUSTOM_SKILLS_DIR: ""
AGENT_CUSTOM_PLUGIN_DIRS: []

AGENT_EXEC:
  - "/absolute/path/to/home/cc/node_modules/.bin/claude"
```

配置中的非空路径必须使用绝对路径，不会以配置目录或当前目录为基准解析。
`AGENT_EXEC` 省略或设为 `[]` 时从 PATH 查找 `claude`，`[""]` 不是省略值。
`AGENT_NORNS_*` 领域资源仅在 Norns 模式可用；`AGENT_CUSTOM_*` 仅在普通模式可用，
两类非空资源不能混用。`nornsd.yaml` 包含密钥，
不要提交到版本控制，也不要随 Runtime 产物公开分发。

发布构建会在 nornsd 二进制旁提供 `nornsd.example.yaml`，也可以从同一 Release 对应的
源码取得模板。已有配置不会自动覆盖或升级，升级时应对照新模板迁移字段。

### 4. 启动一个 Norns Agent

使用刚刚编辑的配置启动 Direct Agent：

```sh
nornsd agent --config ./bin/nornsd.yaml
```

nornsd 在 `SERVICE_NORNSD_ROOT_DIR` 下创建 Subject 工作目录，并在 `.nornsd/` 中保存
Runtime 配置、插件与日志，然后启动 Claude Code。工作根目录由配置决定，与当前目录无关。
它会固定传入 `--dangerously-skip-permissions`，并为 Claude Code 子进程设置
`IS_SANDBOX=1` 以兼容 root/sudo 环境，因此只应在可信且权限受限的工作目录中运行。

### 5. 批量服务与 Well

在项目根目录启动两个并发槽的 nornsd Server：

```sh
nornsd server \
  --configs ./bin/nornsd.yaml
```

监听地址、端口、Token、工作根目录和并发数均由 `SERVICE_*` 配置控制。
Direct Agent 和 Server 共用 Root 锁，同一个 Root 不能同时运行两者。
多槽位可共用一份配置，也可重复传入 `--configs ./slot1.yaml --configs ./slot2.yaml`；
多份配置数量须等于首份的 `SERVICE_CONCURRENCY`，`SERVICE_*` 仅以首份为准。
Runtime 创建时可通过可选 `config` JSON 对象覆盖 `AGENT_*`，不能覆盖 `SERVICE_*`。
Server 的 API 基础路径为 `/api/`，所有请求都需要 Bearer Token。生产环境中
不要直接将未加密的 nornsd HTTP 端口暴露到不可信网络，应使用 TLS 反向代理并限制访问来源。

Well 是该 API 的桌面客户端。打开 Well 后添加 Server：

- URL：`http://127.0.0.1:28081`，不要包含 `/api/`
- Token：与 `SERVICE_TOKEN` 相同

Well 可以创建 Subject、提交 Runtime、查看队列和事件，以及显示 Norns Status、Trace 与 Visual View、查询 Cog/Exp 详情和下载归档。Status/Trace 默认以 Markdown 预览，也可切换 YAML/JSON；HTTP API 本身默认 YAML。构建和本地开发方式见 [well/README.md](well/README.md)。

### 6. Well 手动构建

Windows 端安装相关依赖工具链：

```powershell
winget install --id OpenJS.NodeJS.LTS -e
winget install --id Rustlang.Rustup -e
winget install --id Microsoft.VisualStudio.2022.BuildTools -e --override "--wait --passive --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"
```

进入 `well` 目录并执行构建：

```sh
cd ./well
npm ci --registry=https://registry.npmmirror.com
npm run release
```

### 7. 领域知识

`nornsd.yaml` 可以通过 `AGENT_NORNS_EXTENDS_AGENTS_DIR` 插入领域知识。项目提供数学示例：

```yaml
AGENT_NORNS_ENABLE: true
AGENT_NORNS_EXTENDS_AGENTS_DIR: "/absolute/path/to/norns/domain/math"
```

将示例路径替换为本机目录的绝对路径。

nornsd 会根据 `domain/math/meta.json` 动态生成数学领域的 Decider 和 Explorer，
而不会改变协议状态机或通用角色边界。格式说明见 [domain/README.md](domain/README.md)。

### 8. 单独使用 norns

`norns` 也可以作为纯本地协议 CLI 使用，不需要启动 nornsd：

```sh
mkdir -p ./example-workspace
cd ./example-workspace

norns workspace init
norns task open \
  --purpose "确认一个开放任务是否可完成" \
  --concurrency 3 \
  --info "任务背景与当前已知信息"

norns view status
norns view visual --lang zh --open
```

完整命令、状态机和输出格式见 [norns/README.md](norns/README.md)。

### 9. Docker

当前镜像只封装预编译文件，不在 Docker 内编译 Go 源码。先生成六个平台目标：

```sh
./release.sh --all-platforms
```

`release/` 将包含 `darwin/linux/windows × x64/arm64` 的 `norns`、`nornsd` 和
通过 `--with-ext <名称>` 显式选中的 `norns-*` 扩展（不指定时只有两个主程序）。Docker 构建直接读取与当前主机架构一致的 Linux Release
目录，不读取本地 `bin/`。通用镜像默认使用 `python:3.13.15-slim`，固定安装 Node.js
24.22.1，并命名为 `nornsd-server:latest`：

```sh
container/nornsd/build.sh
```

构建脚本也支持 Apple Container，以及 `--image`、`--tag`、`--proxy`、`--use-mirror`
和 `--use-ecs-mirror`。Claude Code 2.1.199 通过 npm 非全局安装到
`/norns/engine/cc/`，项目 `domain/` 会复制到镜像的 `/norns/domain/`。

启动镜像时，入口读取 `/norns/config/nornsd.example.yaml`，通过白名单环境变量生成 `/norns/config/nornsd.yaml`：

```sh
docker run --detach --rm \
  --name nornsd \
  --publish 28081:28081 \
  --volume nornsd-data:/workspace \
  --env AGENT_LLM_URL='https://example.test/anthropic' \
  --env AGENT_LLM_KEY='replace-me' \
  --env AGENT_LLM_MODEL='model-name' \
  --env SERVICE_CONCURRENCY=1 \
  --env AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS=3 \
  --env AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS=1200 \
  --env AGENT_NORNS_ALLOW_HUMAN_INTERVENTION=false \
  nornsd-server:latest
```

镜像以 root 运行，数据目录固定为 `/workspace`。`AGENT_LLM_URL`、`AGENT_LLM_KEY` 和 `AGENT_LLM_MODEL` 必须显式传入且非空。`AGENT_NORNS_ENABLE` 默认是 `true`，可通过同名环境变量关闭；`SERVICE_CONCURRENCY` 默认是 `1`，可设置为其他正整数； `AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS` 默认是 `3`。入口仅接受当前名称的配置
环境变量。

入口脚本会将最终使用的 Server Token 持久化到数据卷；`SERVICE_TOKEN` 未设置或为空时，优先读取已有 Token，没有时生成随机 Token：

```sh
docker exec nornsd cat /workspace/.nornsd-token
```

随后可在 Well 中使用 `http://127.0.0.1:28081` 和该 Token 添加 Server。查看日志
或停止服务：

```sh
docker logs --follow nornsd
docker stop nornsd
```

镜像从指定的 `release/linux-<arch>/` 复制可执行文件到 `/norns/bin/`，并将公开的 `nornsd/configs/nornsd/nornsd.example.yaml` 安装为 `/norns/config/nornsd.example.yaml`。各镜像的 Dockerfile 使用独立构建上下文白名单，
本地 `bin/nornsd.yaml` 不会进入构建上下文。
完整构建参数、配置白名单和领域目录说明见 [container/README.md](container/README.md)。

同一目录还提供 `redteam/` 领域服务镜像和 `redteam_for_benchmark/` 自包含评测镜像，两者工具链一致。评测镜像固定使用 3 个 Runtime槽位，首轮默认限时 30 分钟，后续每轮默认 40 分钟；所有剩余题目都在运行且数量不超过 3 时取消单题限时。少于 3 道题时，优先为并行实例最少的题目补位，同数时按原题目顺序；每次补位创建全新 Subject，同题共享靶场，任一分支通关后停止同题其他分支，全部收尾后关闭靶场。开靶、关靶均最多尝试 5 次，重试间隔为 5、10、30、30 秒。
评测所需 SaaS 地址、Token、模型环境变量及完整运行命令见上述 Container 文档。

已结束 Benchmakr 容器复用

ECS 已结束容器复用，执行前需修改镜像为实际可用名称 `<norns-redteam-benchmark>:latest`：

```sh
docker run -d \
  --name norns-history \
  --volumes-from norns-redteam-benchmark \
  -p 28081:28081 \
  --entrypoint /norns/bin/nornsd \
  <norns-redteam-benchmark>:latest \
  server --configs /workspace/.nornsd/configs/nornsd.yaml
```

复用后需要处理取消的 Subject，因此需要几十秒或数分钟的启动时间，可以查看 docker log：

```sh
docker logs --tail 100 norns-history
```

## 版本与构建

根目录 `versions.json` 是唯一人工维护的协议、产品及兼容性版本来源。根目录不定义 Go 模块，`norns/` 和 `nornsd/` 保持独立模块。

| 清单字段 | 用途 |
| --- | --- |
| `protocol` | 协议规范版本，使用正整数。 |
| `norns.version`、`nornsd.version`、`well.version` | 各产品的三段式字符串版本，分别维护。 |
| `norns.support_protocol`、`nornsd.support_protocol` | 各自支持的协议版本；启用 Norns 模式时必须一致。 |
| `norns.database_schema`、`nornsd.database_schema` | 各自的数据库 schema。 |
| `nornsd.api_schema`、`well.support_api_schema` | 服务 API schema 与 Well 支持的 schema，配套发布时保持一致。 |
| `nornsd.agent_config_schema` | 配置结构的整数版本，由二进制报告；配置文件不包含 `VERSION`。 |
| `nornsd.plugins.cc` | CC Plugin 的整数版本，生成 Plugin 元信息时转换为 `<整数>.0.0`。 |

协议和 schema 使用正整数，不参与产品版本或 API 路径拼接。协议文档与 Plugin 源模板不再单独声明这些版本数值；外部工具依赖的版本不属于此清单。

统一构建入口为根目录 `release.sh`：

```sh
./release.sh                   # 当前平台的 norns 和 nornsd，默认不编译命令扩展
./release.sh norns             # 仅 norns 主程序
./release.sh norns --with-ext websearch  # norns 与 websearch 扩展
./release.sh nornsd            # 仅 nornsd
./release.sh well              # 仅 Well 桌面程序与安装包
./release.sh all               # 当前平台的全部三个产品
./release.sh --all-platforms   # 六个平台的 Go 产物，不构建 Well
./release.sh --gitrelease      # 固定生成 GitHub Release 附件，需在 macOS ARM64 上运行
./release.sh --gitrelease --with-ext websearch --with-ext deepseeksearch  # 在 Norns ZIP 中加入选定扩展
```

根入口只负责调度、汇总产物和生成校验和；各产品的构建实现与测试放在各自的 `scripts/` 中。Go 产品需要 Go 1.25 或更高版本，不依赖前端或 Rust 工具链。Well 使用自己的 Node.js 构建脚本，不需要 Go，但需要 npm、Rust 及宿主平台的 Tauri 系统依赖，并先在 `well/` 执行 `npm ci`。根入口生成校验和需要 `sha256sum` 或 `shasum`。Well 在对应宿主平台打包，`--all-platforms` 仅支持默认 Go 产品或单个 Go 产品。

也可以在各产品目录内独立构建，不调用兄弟项目的构建脚本，默认仍输出到根目录 `release/`：

```sh
(cd norns && go run ./scripts/release.go)
(cd nornsd && go run ./scripts/release.go)
(cd well && npm run release)
```

两个 Go 入口均支持 `--all-platforms`；三个入口均支持 `--output <directory>` 指定产物目录，并只读取根版本清单中本产品及所需公共协议字段。norns 入口额外支持可重复的 `--with-ext <名称>`；根入口在默认构建、`norns` 或 `all` 选择中接受此参数，仅向 norns 转发，单独构建 nornsd 或 Well 时拒绝使用。各目录自带构建逻辑，但仍共享仓库的 `versions.json`；nornsd 还需共享协议文档和 Plugin 源文件。

产物写入 `release/<os>-<arch>/`；Go 可执行文件放在该目录顶层，Well 可执行文件与 `bundle/` 安装包放在其 `well/` 子目录。单独构建只替换所选产品在所选平台的产物，保留其他产品和平台，并更新覆盖现有全部产物的 `release/SHA256SUMS`。所有所选产品构建成功后才发布到该目录。

构建 nornsd 时，会将 `nornsd/configs/nornsd/nornsd.example.yaml` 一起发布到各目标平台目录，与 nornsd 可执行文件同级并纳入校验和；只更新示例，不覆盖已有的 `nornsd.yaml`。nornsd 的独立构建入口也保持相同行为。

根入口构建 Well 时，在发布产物及校验和均成功写入后，删除 `well/src-tauri/target/release/bundle/` 原始打包目录；设置 `CARGO_TARGET_DIR` 时清理对应目录。保留编译缓存、可执行文件和 Debug 产物，失败时不执行此清理；仅构建 Go 产品时不清理 Well。

`--gitrelease` 使用固定产品与平台选择，忽略其他产品和平台选项；支持可重复的 `--with-ext <名称>`，仅为 Norns 构建和打包所选扩展。不传时不包含扩展。需要 Go、Node/npm、Rust、Tauri 系统依赖及 `zip`。在 macOS ARM64 上运行，输出 `release/gitrelease/`，共七个附件（`<version>` 分别取各产品版本）：

- `norns-v<version>-darwin-arm64.zip`、`norns-v<version>-windows-amd64.zip`、`norns-v<version>-linux-amd64.zip`。
- `nornsd-v<version>-darwin-arm64.zip`、`nornsd-v<version>-windows-amd64.zip`、`nornsd-v<version>-linux-amd64.zip`。
- `well-v<version>-darwin-arm64.dmg`，直接提供 DMG；暂不构建 Well Windows 安装包。

Norns ZIP 附带许可证与第三方声明；选择扩展时，还包含对应平台的 `norns-<名称>` 二进制（Windows 为 `.exe`）、`norns.exts.meta.json` 和扩展元数据中声明的附件，附件沿用 `norns.exts.<名称>.` 文件名前缀。Nornsd ZIP 附带许可证和 `nornsd.example.yaml`。仅打包本次生成的产品文件及所选扩展声明的附件，不收录用户配置或旧产物。全部构建、打包成功后整体替换 `gitrelease/`（移除旧附件），失败时保留此前附件。暂存目录位于 `release/` 内并在退出时清理；普通 `release/<os>-<arch>/` 产物保持原样，此模式不更新根 `release/SHA256SUMS`，`gitrelease/` 内也不额外放入校验和文件。

Go 发布构建沿用 `-trimpath -s -w`。Well 发布入口为 Rust 增加源码路径映射，将本机项目、用户、Cargo、Rustup 和临时目录路径替换为固定逻辑路径，发布前检查可执行文件是否仍含这些本机路径；已有 Rust 参数会保留，不修改全局环境配置。两个 Go 独立入口还支持重复的 `--target <os>/<arch>`，用于只构建选定平台，不能与 `--all-platforms` 合用。

修改清单后运行对应构建入口即可：Go 构建前会更新纳入 Git 的 `internal/version/versions_generated.go` 派生代码；nornsd 校验配置示例不含已删除的 `VERSION` 字段，并重新生成协议与 Plugin 内嵌资源，包含当前 `plugin/cc/` 下的 Skill 和 Agent 模板；Well 复用自身的 Cargo 版本同步流程。派生文件应随清单变更提交，不应手工修改生成的版本字段。普通 `go build` 使用已生成的版本和内嵌资源，不会自动刷新；两个模块的版本测试会检查是否与根清单一致。运行二进制不需要版本清单。`VCS_REF` 与 `BUILD_TIME` 仍可设置 Go 构建元数据，产品版本不由 `NORNS_VERSION` 覆盖。

API 路径固定使用 `/api/`，不随 schema 改变；`GET /api/` 的服务信息和 `nornsd version --json` 返回清单中的 `api_schema`。配置使用当前 `nornsd.yaml` 字段，不再包含 `VERSION`；数据库 schema 必须与清单声明一致，不会自动迁移旧配置或旧数据库。

已有 `nornsd.yaml` 是用户输入，不会被构建覆盖。升级配置 schema 时，应对照新模板检查字段并手动迁移。已安装产物的实际版本以 `norns version --json`、`nornsd version --json` 及服务信息为准，而不是当前源码目录的清单。

构建工具测试纳入各产品的测试入口，根脚本的调度测试随 norns 运行：

```sh
(cd norns && go test ./...)
(cd nornsd && go test ./...)
(cd well && npm test)
```

## 当前版本边界

| 项目 | 版本来源 |
| --- | --- |
| Norns protocol | `versions.json` 的 `protocol` |
| norns / nornsd / Well 产品版本 | `norns.version` / `nornsd.version` / `well.version` |
| norns / nornsd 支持的协议 | `norns.support_protocol` / `nornsd.support_protocol` |
| norns database schema | `norns.database_schema` |
| nornsd HTTP API schema | `nornsd.api_schema`，接口路径固定为 `/api/` |
| Well 支持的 API schema | `well.support_api_schema`，与 `nornsd.api_schema` 保持一致 |
| nornsd database schema | `nornsd.database_schema` |
| nornsd.yaml schema | `nornsd.agent_config_schema`，通过二进制的 `agent_config_version` 报告 |
| CC Plugin | `nornsd.plugins.cc`，构建时转换为 Plugin 元信息版本 |
| Claude Code | `2.1.199` |
| Go | `1.25` |

内部版本统一见根目录 [versions.json](versions.json)，本文不重复维护具体数值。
已安装二进制的实际版本通过 `norns version --json` 和 `nornsd version --json` 获取；
远端服务信息通过 `GET /api/` 获取。

`norns` 和 `nornsd` 必须来自同一次构建或同一个 Release。nornsd 启用 Norns
模式时会检查 `norns` 支持的协议版本，但不负责迁移不兼容的 Workspace。

## 文档列表

- [项目说明](README.md)
- [文档索引](documents/README.md)
- [Norns Protocol](documents/protocol/protocol.md)
- [norns CLI](norns/README.md)
- [nornsd](nornsd/README.md)
- [Well](well/README.md)
- [领域知识包](domain/README.md)

## 数据与安全

- 不要直接编辑 `.norns/norns.sqlite` 或 nornsd 的内部数据库; nornsd 负责 Runtime 生命周期；关闭 Norns Task 前应确保相关 Explorer 已停止写入。
- Root 下的 `.nornsd/` 集中保存生成配置、Agent transcript、日志和访问凭据；Subject 工作目录不保存这些运行数据。
- `nornsd.yaml` 中的 `AGENT_LLM_KEY`、Server Token 和 Well 保存的凭据都应按密钥处理。
