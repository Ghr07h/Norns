# TSecBench Flag 提交扩展

将题目的一个 flag 提交到 TSecBench，返回平台判定。命令先查询题目列表核验靶场状态，仅在目标题目的 `container_status` 为 `available` 时提交。不调用启动、提示或关闭接口，不需要初始化 Norns Workspace。

## 构建与安装

在项目根目录执行：

```sh
./release.sh norns --with-ext tsec
# 如需同时发布其他扩展，继续追加 --with-ext <名称>。
```

将同一平台目录下的 `norns`、`norns-tsec`（Windows 为 `.exe`）和 `norns.exts.meta.json` 一起部署。扩展只能从 Norns 实际二进制的同目录发现，可通过 `norns extension list` 查看。

## 配置与调用

从 TSecBench 平台取得当前跑分任务的配置，设置环境变量：

```sh
export BENCHMARK_BASE_URL='https://platform.example.test'
export BENCHMARK_TOKEN='replace-with-task-token'

norns tsec --unique-code 'web_sql_injection_01' --flag 'flag{example_flag_value}'
norns tsec --help
```

`BENCHMARK_BASE_URL` 为服务基础地址，不包含 `/openapi/v1/challenges/submit`；允许反向代理路径前缀，禁止用户名密码、查询参数和 URL fragment。Token 仅通过 `BENCHMARK_TOKEN` 请求头发送，不通过 URL 或 CLI 参数传入。

`--unique-code` 和 `--flag` 均必填；flag 按原始内容发送，不去除首尾空白，长度允许 1～4096 个 Unicode 字符。一道题有多个 flag 时分别调用。当前运行环境已连接目标内网，插件不进行 VPN 操作。

## 请求与输出

首先请求题目列表，按 `unique_code` 精确匹配目标题目：

```http
GET /openapi/v1/challenges
BENCHMARK_TOKEN: <当前任务 Token>
```

仅 `container_status=available` 时继续调用提交接口；`pending`、`stop_pending`、`stopped`、未知或缺失状态均返回 `container_not_ready`，不发送 flag。题目不存在返回 `challenge_not_found`；查询失败或响应无效时返回相应错误，不继续提交。靶场状态可能在核验后发生变化，因此仍需处理提交接口返回的错误。

```http
POST /openapi/v1/challenges/submit
BENCHMARK_TOKEN: <当前任务 Token>
Content-Type: application/json

{"unique_code":"web_sql_injection_01","flag":"flag{example_flag_value}"}
```

HTTP 200 时，stdout 输出平台原始 JSON 字段，退出码为 `0`：

```json
{"correct":true,"awarded":50,"cumulative_score":80,"correct_flag_count":2,"total_flag_count":3,"matched_flag_index":1}
```

错误 flag 返回 `correct:false`，仍为退出码 `0`，表示平台完成了判定；需读取 `correct` 判断答案是否正确。错误 flag 的 `matched_flag_index` 为 `null`。

提交接口返回 HTTP 409、业务码为 `duplicate` 时，表示该 flag 此前已正确验收，按幂等成功处理：stdout 输出以下 JSON，stderr 为空，退出码为 `0`，因此不会中断使用 `&&` 串联的后续命令：

```json
{"correct":true,"duplicate":true}
```

此处 `correct=true` 表示此前已验收，不代表本次重新得分。平台的 duplicate 响应未提供分数和作答进度，因此不补写 `awarded`、`cumulative_score`、`correct_flag_count`、`total_flag_count` 或 `matched_flag_index`；整题是否通关仍需查询题目列表。

其他 API、配置、网络或输出错误向 stderr 输出 JSON，退出码为 `1`；本地命令参数错误退出码为 `2`。例如靶场未开启或尚未就绪时：

```json
{"error":{"code":"container_not_ready","message":"Challenge container is not available; flag was not submitted. Wait until the container is available before retrying","detail":{"container_status":"stopped"}}}
```

除上述提交接口 HTTP 409 duplicate 的成功转换外，保留平台业务错误码，如 `task_not_found`、`challenge_not_found`、`invalid_state`、`resource_unavailable` 和 `internal_error`。题目列表查询返回 duplicate 或提交接口以其他 HTTP 状态返回 duplicate 时，仍按 API 错误处理。FastAPI 的 HTTP 422 校验错误使用 `validation_error`，`detail` 保留数组结构；无有效业务错误 JSON 时使用 `http_error`。服务器返回的 `Retry-After` 在存在时输出为 `retry_after`。错误信息中匹配到的 Token 和 flag 会替换为 `[REDACTED]`；不输出原始网络错误或非 JSON 响应正文。

每次调用先查询一次题目列表，核验通过后最多发送一次提交，两步共用 60 秒超时，不跟随 HTTP 重定向、不自动重试。状态核验阶段的超时或网络中断表示未提交 flag；提交阶段的超时或网络中断则可能已经被平台处理，调用方可通过题目列表对账后决定是否重试，再次提交已正确登记的 flag 时，将平台的 HTTP 409 duplicate 转换为上述幂等成功结果。插件不会自动关闭靶场或读取会扣分的提示。
