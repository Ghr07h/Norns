package i18n

import (
	"os"
	"strings"
)

var chineseMessages = map[string]string{
	"Workspace initialized":     "Workspace 已初始化",
	"Workspace already exists":  "Workspace 已存在",
	"Workspace reset":           "Workspace 已重置",
	"Task opened":               "Task 已打开",
	"Task closed":               "Task 已关闭",
	"Task is already open":      "Task 已经打开",
	"Task reopened":             "Task 已重新打开",
	"Cognition added":           "Cognition 已添加",
	"Exploration added":         "Exploration 已添加",
	"Exploration retry created": "Exploration 重试已创建",
	"Exploration assigned":      "Exploration 已分配",
	"Exploration finished":      "Exploration 已完成",
	"todo Explorations":         "待执行 Explorations",
	"running Explorations":      "运行中 Explorations",
	"Visual View generated":     "Visual View 已生成",
	"View written to file":      "View 已写入文件",
	"operation applied, but running explorations exceed the configured concurrency": "操作已生效，但运行中的 Exploration 数量超过额定并发数",
	"view generated, but the browser could not be opened":                           "View 已生成，但无法打开浏览器",
	"internal error":               "内部错误",
	"Workspace is not initialized": "Workspace 尚未初始化",
	"Workspace is not initialized; run norns workspace init": "Workspace 尚未初始化；请运行 norns workspace init",
	"Workspace is busy":                                "Workspace 正忙",
	"SQLite writer is busy":                            "SQLite 写入正在占用",
	"Workspace already has an open Task":               "Workspace 已有打开的 Task",
	"another Task is open":                             "另一个 Task 正处于打开状态",
	"Workspace has no Task":                            "Workspace 中没有 Task",
	"Workspace has no open Task":                       "Workspace 中没有打开的 Task",
	"Task not found":                                   "未找到 Task",
	"Cognition not found":                              "未找到 Cognition",
	"Exploration not found":                            "未找到 Exploration",
	"concurrency must be a positive integer":           "concurrency 必须是正整数",
	"info is required for the first Task":              "首个 Task 必须提供 info",
	"invalid Task outcome":                             "Task outcome 无效",
	"invalid Exploration outcome":                      "Exploration outcome 无效",
	"origin must be user or exploration":               "origin 必须是 user 或 exploration",
	"exploration is required for origin=exploration":   "origin=exploration 时必须提供 exploration",
	"global Cognition cannot be task-purpose-relevant": "全局 Cognition 不能标记为 task-purpose-relevant",
	"Explorer Cognition requires a todo/running Exploration in the open Task": "Explorer Cognition 必须来自当前 open Task 的 todo/running Exploration",
	"retry source must be finished":                                           "retry 来源必须是 finished Exploration",
	"invalid Exploration finish transition":                                   "Exploration finish 状态转换无效",
	"Exploration state changed concurrently":                                  "Exploration 状态已被并发修改",
	"duplicate source Cognition ID":                                           "source Cognition ID 重复",
	"generated entity ID already exists":                                      "生成的实体 ID 已存在",
	"operation violates Workspace constraints":                                "操作违反 Workspace 约束",
	"Runtime is occupied without an open Task":                                "不存在 open Task，但 Runtime 仍处于占用状态",
	"open Task is missing Runtime occupancy":                                  "open Task 缺少 Runtime 占用标记，需要人工检查并处理 Workspace",
	"Runtime occupancy does not match the open Task; manual Workspace inspection and handling is required": "Runtime 占用状态与 open Task 不一致，需要人工检查并处理 Workspace",
	"close the open Task with outcome=cancelled before reset":                                              "reset 前请先以 outcome=cancelled 关闭 open Task",
	"reset state cannot be recovered deterministically":                                                    "无法确定性恢复 reset 状态",
	"Norns extension not found":                                                                            "未找到 Norns 扩展",
}

func Chinese() bool {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			value = strings.ToLower(strings.ReplaceAll(value, "-", "_"))
			return value == "zh" || strings.HasPrefix(value, "zh_")
		}
	}
	return false
}

func Message(message string) string {
	if !Chinese() {
		return message
	}
	if translated, ok := chineseMessages[message]; ok {
		return translated
	}
	prefixes := []struct{ english, chinese string }{
		{"invalid task ID: ", "无效的 task ID: "},
		{"invalid cognition ID: ", "无效的 cognition ID: "},
		{"invalid exploration ID: ", "无效的 exploration ID: "},
		{"Cognition not found: ", "未找到 Cognition: "},
		{"Norns extension not found: ", "未找到 Norns 扩展: "},
		{"unsupported protocol version ", "不支持的协议版本 "},
		{"unsupported schema version ", "不支持的 schema 版本 "},
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(message, prefix.english) {
			return prefix.chinese + strings.TrimPrefix(message, prefix.english)
		}
	}
	if strings.HasSuffix(message, " must not be empty") {
		return strings.TrimSuffix(message, " must not be empty") + " 不能为空"
	}
	return message
}

func Error(code, message string) string {
	localized := Message(message)
	if !Chinese() || localized != message {
		return localized
	}
	fallbacks := map[string]string{
		"INVALID_ARGUMENT":             "参数无效",
		"WORKSPACE_NOT_INITIALIZED":    "Workspace 尚未初始化",
		"UNSUPPORTED_PROTOCOL_VERSION": "协议版本不受支持",
		"UNSUPPORTED_SCHEMA_VERSION":   "schema 版本不受支持",
		"INVALID_DATABASE":             "Workspace 数据库无效",
		"NO_TASK":                      "Workspace 中没有 Task",
		"NO_OPEN_TASK":                 "Workspace 中没有打开的 Task",
		"OPEN_TASK_EXISTS":             "Workspace 已有打开的 Task",
		"TASK_NOT_FOUND":               "未找到 Task",
		"COGNITION_NOT_FOUND":          "未找到 Cognition",
		"EXPLORATION_NOT_FOUND":        "未找到 Exploration",
		"ID_CONFLICT":                  "实体 ID 冲突",
		"INVALID_REFERENCE":            "引用无效",
		"INVALID_STATE":                "状态转换无效",
		"GRAPH_CYCLE":                  "操作会在 Graph 中形成环",
		"WORKSPACE_STATE_UNTRUSTED":    "Workspace 状态不可信，需要人工检查并处理",
		"WORKSPACE_BUSY":               "Workspace 正忙",
		"RESET_OPEN_TASK":              "存在打开的 Task，无法 reset",
		"RESET_FAILED":                 "Workspace reset 失败",
		"RESET_RECOVERY_FAILED":        "Workspace reset 恢复失败",
		"DATABASE_ERROR":               "数据库操作失败",
		"IO_ERROR":                     "文件系统操作失败",
		"EXTENSION_NOT_FOUND":          "未找到 Norns 扩展",
		"INTERNAL_ERROR":               "内部错误",
	}
	if fallback, ok := fallbacks[code]; ok {
		return fallback + ": " + message
	}
	return message
}
