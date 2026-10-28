#!/bin/sh
# Block Bash grep patterns that trigger embedded ugrep's bounded-repeat OOM.
#
# Claude Code versions before 2.1.235 bundle ugrep 7.5.0 and shadow grep in
# Bash tool shells. Two bounded repeats can make regex compilation consume
# memory exponentially. See anthropics/claude-code#84960 and Genivia/ugrep#556.

case "$(uname -s 2>/dev/null)" in
    Linux|Darwin) ;;
    *) exit 0 ;;
esac

# Do not eval the inspected command. The awk program decodes the two JSON
# strings it needs, lexes shell words and operators, and emits a deny decision.
# Any malformed input or internal parsing error exits silently (fail open).
exec awk '
BEGIN {
    min_ranges = 2
    min_bound = 10
}

{
    payload = payload $0 "\n"
}

function json_string_value(text, key, start,    needle, rest, offset, pos, i, c, esc, out, hex) {
    json_ok = 0
    needle = "\"" key "\""
    rest = substr(text, start)
    offset = index(rest, needle)
    if (!offset) return ""
    pos = start + offset - 1 + length(needle)
    while (substr(text, pos, 1) ~ /[[:space:]]/) pos++
    if (substr(text, pos, 1) != ":") return ""
    pos++
    while (substr(text, pos, 1) ~ /[[:space:]]/) pos++
    if (substr(text, pos, 1) != "\"") return ""
    pos++
    out = ""
    esc = 0
    for (i = pos; i <= length(text); i++) {
        c = substr(text, i, 1)
        if (esc) {
            if (c == "n") out = out "\n"
            else if (c == "r") out = out "\r"
            else if (c == "t") out = out "\t"
            else if (c == "b") out = out sprintf("%c", 8)
            else if (c == "f") out = out sprintf("%c", 12)
            else if (c == "u") {
                hex = substr(text, i + 1, 4)
                if (hex !~ /^[0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f][0-9A-Fa-f]$/) return ""
                # Shell syntax is ASCII. Preserve valid JSON Unicode escapes;
                # they cannot create grep words, operators, or repeat bounds.
                out = out "\\u" hex
                i += 4
            } else if (c == "\"" || c == "\\" || c == "/") out = out c
            else return ""
            esc = 0
        } else if (c == "\\") {
            esc = 1
        } else if (c == "\"") {
            json_ok = 1
            json_end = i + 1
            return out
        } else {
            out = out c
        }
    }
    return ""
}

function reset_segment(    i) {
    for (i = 1; i <= word_count; i++) delete words[i]
    word_count = 0
}

function add_word(word) {
    if (word != "") words[++word_count] = word
}

function count_bounds(text,    i, j, c, lower, upper, have_upper, bound) {
    bound_count = 0
    max_bound = 0
    for (i = 1; i <= length(text); i++) {
        if (substr(text, i, 1) != "{") continue
        j = i + 1
        while (substr(text, j, 1) ~ /[[:space:]]/) j++
        lower = ""
        while ((c = substr(text, j, 1)) ~ /[0-9]/) {
            lower = lower c
            j++
        }
        if (lower == "") continue
        while (substr(text, j, 1) ~ /[[:space:]]/) j++
        if (substr(text, j, 1) != ",") continue
        j++
        while (substr(text, j, 1) ~ /[[:space:]]/) j++
        upper = ""
        while ((c = substr(text, j, 1)) ~ /[0-9]/) {
            upper = upper c
            j++
        }
        while (substr(text, j, 1) ~ /[[:space:]]/) j++
        if (substr(text, j, 1) != "}") continue
        have_upper = (upper != "")
        bound = have_upper ? upper + 0 : lower + 0
        bound_count++
        if (bound > max_bound) max_bound = bound
        i = j
    }
}

function check_segment(    i, j, args, previous) {
    for (i = 1; i <= word_count; i++) {
        if (words[i] != "grep" && words[i] != "ugrep") continue
        previous = i > 1 ? words[i - 1] : ""
        if (previous == "command" || previous == "builtin") continue
        args = ""
        for (j = i + 1; j <= word_count; j++) args = args " " words[j]
        count_bounds(args)
        if (bound_count >= min_ranges && max_bound >= min_bound) {
            offending = words[i] args
            if (length(offending) > 300) offending = substr(offending, 1, 300)
            return 1
        }
    }
    return 0
}

function raw_fallback(command) {
    count_bounds(command)
    if (bound_count < min_ranges || max_bound < min_bound) return 0
    return command ~ /(^|[[:space:];&|()])([efzu]?grep)([^[:alnum:]_]|$)/
}

function dangerous_command(command,    i, c, state, escaped, word) {
    reset_segment()
    state = 0
    escaped = 0
    word = ""
    for (i = 1; i <= length(command); i++) {
        c = substr(command, i, 1)
        if (state == 1) {
            if (c == "\047") state = 0
            else word = word c
            continue
        }
        if (state == 2) {
            if (escaped) {
                word = word c
                escaped = 0
            } else if (c == "\\") escaped = 1
            else if (c == "\"") state = 0
            else word = word c
            continue
        }
        if (escaped) {
            word = word c
            escaped = 0
        } else if (c == "\\") escaped = 1
        else if (c == "\047") state = 1
        else if (c == "\"") state = 2
        else if (c ~ /[[:space:]]/) {
            add_word(word)
            word = ""
            if (c == "\n") {
                if (check_segment()) return 1
                reset_segment()
            }
        } else if (c ~ /[;&|()<>]/) {
            add_word(word)
            word = ""
            if (check_segment()) return 1
            reset_segment()
        } else word = word c
    }
    if (state != 0 || escaped) return raw_fallback(command)
    add_word(word)
    return check_segment()
}

function json_escape(text,    i, c, out) {
    out = ""
    for (i = 1; i <= length(text); i++) {
        c = substr(text, i, 1)
        if (c == "\\") out = out "\\\\"
        else if (c == "\"") out = out "\\\""
        else if (c == "\n") out = out "\\n"
        else if (c == "\r") out = out "\\r"
        else if (c == "\t") out = out "\\t"
        else out = out c
    }
    return out
}

END {
    tool_name = json_string_value(payload, "tool_name", 1)
    if (!json_ok || tool_name != "Bash") exit 0
    tool_input_at = index(payload, "\"tool_input\"")
    if (!tool_input_at) exit 0
    command = json_string_value(payload, "command", tool_input_at)
    if (!json_ok || !dangerous_command(command)) exit 0

    reason = "BLOCKED: this grep would run Claude Code embedded ugrep because the Bash tool shadows grep. Its regex compiler has exponential memory growth on two bounded repeats. Found " bound_count " ranges (largest bound " max_bound ") in: " offending ". Use /usr/bin/grep for a named text file, the built-in Grep tool for tree-wide searches, or awk for context extraction."
    printf "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"%s\"}}\n", json_escape(reason)
}
' 
