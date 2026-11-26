# Websearch

仓库提供独立的 Go 工具 `norns-websearch`，用于补充不具备 WebSearch 能力的智能体平台。它不访问 Norns Workspace，也不依赖 Python 或 DDGS 运行时。

~~~sh
norns websearch --query "SQLite WAL shared reader behavior"
norns websearch --query "Norns protocol" --max-results 5
norns websearch --query "Norns protocol" --proxy socks5h://127.0.0.1:1080
~~~

参数与 MVP 保持一致：

- `--query`：必填搜索词；
- `-m`、`--max-results`：最大结果数，默认 `10`；
- `--proxy`：显式 HTTP、SOCKS5 或 SOCKS5H 代理地址，必须包含端口。

未指定代理时使用百度后端；显式指定代理时使用 Google 后端。两个后端分别维护请求参数和 HTML 解析器，公共层负责文本与 URL 规范化、按链接去重、简单相关性排序及结果截断。

成功时直接输出 JSON 数组，每项包含稳定字段 `title`、`href` 和 `body`。参数错误和搜索错误分别使用 `invalid_input`、`search_error`，写入 stderr；扩展输出不套核心 CLI 的 `ok/data/warnings/message` 信封。

## 构建

在仓库根目录执行：

~~~sh
./release.sh norns --with-ext websearch
~~~

仅编译本扩展用于本地开发：

~~~sh
cd norns
go build -o ../bin/norns-websearch ./internal/custom_extensions/websearch/cmd/norns-websearch
../bin/norns-websearch --help
~~~

构建产物命名为 `norns-websearch`（Windows 为 `norns-websearch.exe`），仅放在 `norns` 实际二进制所在目录；不会从 `NORNS_PLUGIN_PATH` 或系统 `PATH` 发现扩展。使用 `norns extension list` 查看发现的路径。

本插件的名称、独立版本、描述和使用前提见 [metadata.json](metadata.json)。发布入口会将本次选中插件的元数据合并为二进制同目录的 `norns.exts.meta.json`，供 `norns extension list` 展示；手动 `go build` 不生成该文件。

本插件的 `release_files` 为 `[]`，无需随发布附带配置文件。
