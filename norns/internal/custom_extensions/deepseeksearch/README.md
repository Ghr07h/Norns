# DeepSeek Search

`norns-deepseeksearch` 调用 [DeepSeek 网页搜索与抓取 API](https://api-docs.deepseek.com/zh-cn/guides/search/)，不访问 Norns Workspace。源码在 `internal/custom_extensions/deepseeksearch/`，构建时显式选择：

~~~sh
# 从仓库根目录执行，可同时选择其他扩展。
./release.sh norns --with-ext deepseeksearch --with-ext websearch
~~~

通过 `NORNS_EXTENSION_DEEPSEEKSEARCH_CONFIG` 指定配置文件地址；支持 YAML 或 JSON，仅从该文件读取配置，不从其他 API Key 环境变量回退。示例见 [config.example.yaml](config.example.yaml)：

~~~yaml
api_key: "YOUR_DEEPSEEK_API_KEY"
base_url: "https://api.deepseek.com"
timeout_seconds: 30
~~~

`api_key` 必填；`base_url` 和 `timeout_seconds` 可省略，默认值如上，超时 `0` 也使用默认值。`base_url` 为 API 基础地址，也接受以 `/v1` 结尾的地址；不能包含用户名、密码、查询串或 fragment。未知配置字段会报错。

~~~sh
export NORNS_EXTENSION_DEEPSEEKSEARCH_CONFIG=/absolute/path/deepseeksearch.yaml
norns deepseeksearch --query 'DeepSeek site:github.com lang:zh after:2025' --count 5
norns deepseeksearch search --query 'DeepSeek V4' --need-content --ori-question 'DeepSeek 最新模型有什么改进？'
norns deepseeksearch fetch --url 'https://example.com/article'
~~~

默认执行 `search`，可显式指定 `search` 或 `fetch`：

- 搜索：`--query` 必填，1～128 个 Unicode 字符且不能全为空白，搜索操作符原样透传；`--count`（别名 `-m`、`--max-results`）为 1～50，默认 `10`；`--need-content` 默认关闭；`--ori-question` 可选，提供时为 1～128 个 Unicode 字符；`--localization` 可选，传入本地化优先使用的 IP 地址。
- 抓取：`--url` 必填，最多 8192 个 Unicode 字符，必须为含 host、无用户名密码的 HTTP/HTTPS URL。

成功输出 API 原生结构的 JSON：搜索为 `{"results":[...]}`，保留 `rerank_score`、`title`、`url`、`snippet`、`summary`、`content`、`site_name`、`site_icon`、`date`、`authority`；无匹配结果时 `results` 为 `[]`，仍然成功。抓取为 `{"title":"...","content":"Markdown 正文"}`。不套核心 CLI 的输出信封，也不对搜索结果重新排序。

失败时 stdout 为空，stderr 输出 `{"error":{...}}`，包含 `code`、`message`；API 错误额外包含 `status`，并在可用时保留 `type`、`api_code` 和 `retry_after`。不自动重试；遇到 408、429、5xx 时可等待后重试，402 需充值，其他 4xx 应先修正请求。退出码为 `0` 成功、`2` 参数错误、`1` 配置/网络/API/输出错误。`--help` 不需要配置文件或凭据。

## 本地开发构建

在仓库根目录进入 Norns 模块后执行：

~~~sh
cd norns
go build -o ../bin/norns-deepseeksearch ./internal/custom_extensions/deepseeksearch/cmd/norns-deepseeksearch
../bin/norns-deepseeksearch --help
~~~

构建产物命名为 `norns-deepseeksearch`（Windows 为 `norns-deepseeksearch.exe`），仅放在 `norns` 实际二进制所在目录；不会从 `NORNS_PLUGIN_PATH` 或系统 `PATH` 发现扩展。使用 `norns extension list` 查看发现的路径。

本插件的名称、独立版本、描述和使用前提见 [metadata.json](metadata.json)。发布入口会将本次选中插件的元数据合并为二进制同目录的 `norns.exts.meta.json`，供 `norns extension list` 展示；手动 `go build` 不生成该文件。

`release_files` 使用空 `target_file` 保留原始文件名，发布时自动加上 `norns.exts.deepseeksearch.` 前缀，将 [config.example.yaml](config.example.yaml) 发布为二进制同目录的 `norns.exts.deepseeksearch.config.example.yaml`。复制该模板并填写自己的 Key 后，将 `NORNS_EXTENSION_DEEPSEEKSEARCH_CONFIG` 指向实际配置文件。Docker 构建会将 Release 中的模板复制到 `/norns/bin/norns.exts.deepseeksearch.config.example.yaml`；真实配置仍在运行时提供。
