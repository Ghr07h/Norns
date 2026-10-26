# Well

Well 是 nornsd HTTP API v1 的本地桌面客户端，用于管理多个 nornsd Server、浏览 Subject 与 Runtime，并查看 Event、Runtime artifact 和 Norns View。

Well 只通过 nornsd API 工作，不直接访问 Subject 文件系统、SQLite、raw log 或 Claude transcript。

## 技术栈

- Tauri 2 与 Rust `reqwest` 网络层
- React、TypeScript、Vite 与 Hash Router
- TanStack Query、Dexie、i18next 和 Lucide
- nornsd HTTP API v1

前端不直接发出 HTTP 请求。所有远端操作通过 Rust 中明确列出的 typed Tauri command 完成，Bearer Token 不进入 URL 或前端日志。

## 使用

启动后直接进入 Servers 页面：

1. 添加 Server 名称、基础 URL 和 Bearer Token；URL 不包含 `/api/v1`。
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
cargo fmt --check --manifest-path src-tauri/Cargo.toml
cargo clippy --manifest-path src-tauri/Cargo.toml -- -D warnings
cargo test --manifest-path src-tauri/Cargo.toml
```

## 构建

```sh
npm run tauri build
```

## 本地数据与安全

Server registry 和偏好保存在 WebView `localStorage`，Event cache 保存在 IndexedDB。每个 Subject 跨重启保留最近 5,000 条 Event。

选择“记住 Token”会把 Bearer Token 保存到 WebView 本地存储，其保护强度取决于当前操作系统账户和应用数据目录；未选择时 Token 只保留在当前应用会话内存。Well 不提供用于加密该存储的主密码。

移除 Server、确认 Server identity 变化或清除缓存只删除 Well 本地数据，不会删除 nornsd 上的 Subject、Runtime 或 Event。服务端返回的 Workspace 与 artifact 路径只是远端定位信息，Well 不假定本机可以直接访问。

Norns Visual View 在 sandboxed iframe 中显示，不获得 Well storage、Tauri bridge、导航或外部网络权限。

生成 Norns View 后可以通过系统保存对话框导出。Well 保存 Visual HTML 或 Status/Trace JSON，并在服务端源文件名前增加 `well-`；文件只写入用户明确选择的位置。

## 文档

- [Well 设计](../documents/design/well.md)
- [nornsd 设计与 HTTP API v1](../documents/design/nornsd.md)
