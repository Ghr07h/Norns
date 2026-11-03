# Well

Well 是 nornsd HTTP API 的本地桌面客户端，用于管理多个 nornsd Server、浏览 Subject 与 Runtime，并查看 Event、Runtime artifact 和 Norns View。

Well 只通过 nornsd API 工作，不直接访问 Subject 文件系统、SQLite、raw log 或 Claude transcript。

## 技术栈

- Tauri 2 与 Rust `reqwest` 网络层
- React、TypeScript、Vite 与 Hash Router
- TanStack Query、Dexie、i18next 和 Lucide
- nornsd HTTP API（固定 `/api/` 前缀，按 API schema 检查兼容性）

前端不直接发出 HTTP 请求。所有远端操作通过 Rust 中明确列出的 typed Tauri command 完成，Bearer Token 不进入 URL 或前端日志。

## 使用

启动后直接进入 Servers 页面：

1. 添加 Server 名称、基础 URL 和 Bearer Token；URL 只填写 origin，不包含 `/api/` 或其他路径。
2. 连接成功后，Well 会绑定 nornsd 返回的稳定 `server_id`。
3. 进入 Server workspace 浏览 Subject 和 Queue，或创建 Subject 并提交首条 instruction。
4. 在 Subject 详情中查看实体概览、Runtime（含历史记录）、事件记录和可选 Norns View。

同一 URL 后的 `server_id` 改变时，Well 会阻止继续操作，直到用户确认清理该连接的本地缓存。不同 Server 即使产生相同 Subject UUID，也通过本地 Server ID 隔离。

## 本地开发

需要 Node.js、npm、Rust 1.89 或更高版本，以及当前平台的 Tauri 2 系统依赖。

```sh
npm install
npm run dev
```

浏览器开发模式需要注入 `window.__WELL_BRIDGE__` mock；连接真实 nornsd 时使用桌面开发模式：

```sh
npm run tauri dev
```

## 验证

```sh
npm run typecheck
npm test
npm run build
npm run prepare:versions
cargo fmt --check --manifest-path src-tauri/Cargo.toml
cargo clippy --manifest-path src-tauri/Cargo.toml -- -D warnings
cargo test --manifest-path src-tauri/Cargo.toml
```

## 界面设计

Well 与 Norns Visual 使用一致的冷灰蓝浅色体系：页面底色 `#f3f5f8`、白色内容面板、蓝色交互强调 `#3563c5`，运行状态使用独立的蓝、绿、琥珀、红语义色。

- `src/styles/tokens.css` 维护配色、阴影、圆角和动效参数；基础控件与页面布局分别位于 `global.css`、`workspace.css`。
- 正文以 15px 为基准，说明和标签使用 12–13px，ID 等技术信息使用 11–12px 等宽字体。
- 卡片使用轻阴影，悬停和弹窗使用短过渡；系统启用减少动态效果时关闭动画和过渡。
- 长详情、事件和弹窗正文在各自区域滚动；Subject 虚拟列表的 CSS 行高必须与 `ROW_HEIGHT` 保持一致。
- 桌面最小窗口为 880×620；修改界面时同时检查中英文、最小窗口、表单、运行时历史和 Norns 视图。
- Visual 继续在隔离的 iframe 中渲染，不通过更改 sandbox 或注入父页面样式来统一主题。

浏览器预览可运行 `VITE_WELL_DEMO=1 npm run dev`，使用演示数据，不连接真实服务。

## 构建

推荐从项目根目录使用统一入口，单独构建 Well 不需要 Go：

```sh
./release.sh well
```

产物位于 `release/<os>-<arch>/well/`，包含桌面可执行文件和 `bundle/` 安装包。只构建宿主平台；构建前先在 `well/` 执行 `npm ci` 并准备 Rust 与 Tauri 系统依赖。`./release.sh all` 同时构建两个 Go 产品，默认不带参数的 `./release.sh` 不构建 Well。

也可以在 `well/` 使用自己的发布入口，独立完成构建、产物整理和校验和生成：

```sh
npm run release
npm run release -- --output ../custom-release
```

构建实现与测试位于本目录 `scripts/`，不依赖两个 Go 项目的构建代码，默认同样输出到 `../release/`。原有桌面构建命令也保持可用，但不额外整理发布产物：

```sh
npm run tauri build
```

### 版本来源

根目录 `../versions.json` 是唯一人工维护的版本来源：`well.version` 是产品版本，`well.support_api_schema` 是客户端支持的 API schema。前端和 Rust 编译时均读取该文件。

`npm run dev`、`npm run build` 和 `npm run tauri ...` 会先自动运行 `prepare:versions`，把产品版本同步到 Cargo 必须保留的 `Cargo.toml` 与 `Cargo.lock` 元数据。它们是派生副本，不应手工修改；Tauri 未单独声明版本，运行时与安装包版本继承 Cargo 产品版本。私有 npm 包不声明产品版本。

后续调整版本时只修改根文件，再使用上述入口启动或构建。直接执行 Cargo 命令前先运行 `npm run prepare:versions`；Rust 构建会检查产品版本是否已同步，避免静默构建出旧版本。

连接时，Well 直接请求带末尾斜杠的 `/api/` 获取服务信息，并比较返回的整数 `api_schema` 与 `well.support_api_schema`。不一致时显示不兼容错误，不继续请求健康状态或进入工作区；不比较 Well 与 nornsd 的产品版本，也不根据 schema 改变请求路径。

## 本地数据与安全

Server registry 和偏好保存在 WebView `localStorage`，Event cache 保存在 IndexedDB。每个 Subject 跨重启保留最近 5,000 条 Event。

选择“记住 Token”会把 Bearer Token 保存到 WebView 本地存储，其保护强度取决于当前操作系统账户和应用数据目录；未选择时 Token 只保留在当前应用会话内存。Well 不提供用于加密该存储的主密码。

移除 Server、确认 Server identity 变化或清除缓存只删除 Well 本地数据，不会删除 nornsd 上的 Subject、Runtime 或 Event。服务端返回的 Workspace 与 artifact 路径只是远端定位信息，Well 不假定本机可以直接访问。

Norns Visual View 在 sandboxed iframe 中显示，不获得 Well storage、Tauri bridge、导航或外部网络权限。

生成 Norns View 后可以通过系统保存对话框导出。Well 保存 Visual HTML 或 Status/Trace JSON，并在服务端源文件名前增加 `well-`；文件只写入用户明确选择的位置。

## 文档

- [Well 设计](../documents/design/well.md)
- [nornsd 设计与 HTTP API](../documents/design/nornsd.md)
