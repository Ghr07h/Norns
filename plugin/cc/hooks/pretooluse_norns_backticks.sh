#!/bin/sh
# Rewrite paired backticks in quoted Norns arguments, without running the input.
case "$(uname -s 2>/dev/null)" in
    Linux|Darwin) ;;
    *) exit 0 ;;
esac

# Byte-based JSON handling keeps UTF-8 and raw unrelated input fields intact.
LC_ALL=C
export LC_ALL
exec awk '
{ payload = payload $0 "\n" }

function hex_number(text,    i, digit, value) {
    value = 0
    for (i = 1; i <= length(text); i++) {
        digit = index("0123456789abcdef", tolower(substr(text, i, 1))) - 1
        if (digit < 0) return -1
        value = value * 16 + digit
    }
    return value
}

function utf8(code) {
    if (code < 128) return sprintf("%c", code)
    if (code < 2048) return sprintf("%c%c", 192 + int(code / 64), 128 + code % 64)
    if (code < 65536) return sprintf("%c%c%c", 224 + int(code / 4096), 128 + int(code / 64) % 64, 128 + code % 64)
    return sprintf("%c%c%c%c", 240 + int(code / 262144), 128 + int(code / 4096) % 64, 128 + int(code / 64) % 64, 128 + code % 64)
}

function json_string(text, start,    i, c, esc, value, code, low) {
    json_ok = 0
    if (substr(text, start, 1) != "\"") return ""
    value = ""
    for (i = start + 1; i <= length(text); i++) {
        c = substr(text, i, 1)
        if (c == "\"") {
            json_end = i + 1
            json_ok = 1
            return value
        }
        if (c != "\\") {
            if (c ~ /[[:cntrl:]]/) return ""
            value = value c
            continue
        }
        esc = substr(text, ++i, 1)
        if (esc == "n") value = value "\n"
        else if (esc == "r") value = value "\r"
        else if (esc == "t") value = value "\t"
        else if (esc == "b") value = value sprintf("%c", 8)
        else if (esc == "f") value = value sprintf("%c", 12)
        else if (esc == "\"" || esc == "\\" || esc == "/") value = value esc
        else if (esc == "u") {
            if (i + 4 > length(text)) return ""
            code = hex_number(substr(text, i + 1, 4))
            if (code < 0) return ""
            i += 4
            if (code >= 55296 && code <= 56319) {
                if (substr(text, i + 1, 2) != "\\u") return ""
                low = hex_number(substr(text, i + 3, 4))
                if (length(substr(text, i + 3, 4)) != 4 || low < 56320 || low > 57343) return ""
                code = 65536 + (code - 55296) * 1024 + low - 56320
                i += 6
            } else if (code >= 56320 && code <= 57343) return ""
            value = value utf8(code)
        } else return ""
    }
    return ""
}

function skip_space(text, pos) {
    while (substr(text, pos, 1) ~ /[[:space:]]/) pos++
    return pos
}

function json_value_end(text, pos,    c, depth, i, ignored) {
    c = substr(text, pos, 1)
    if (c == "\"") {
        ignored = json_string(text, pos)
        return json_ok ? json_end : 0
    }
    if (c == "{" || c == "[") {
        depth = 0
        for (i = pos; i <= length(text); i++) {
            c = substr(text, i, 1)
            if (c == "\"") {
                ignored = json_string(text, i)
                if (!json_ok) return 0
                i = json_end - 1
            } else if (c == "{" || c == "[") depth++
            else if (c == "}" || c == "]") {
                if (--depth == 0) return i + 1
            }
        }
        return 0
    }
    for (i = pos; i <= length(text); i++) {
        c = substr(text, i, 1)
        if (c ~ /[[:space:]]/ || c == "," || c == "}" || c == "]") break
    }
    return i > pos ? i : 0
}

# Locate a direct member, retaining its original JSON slice for updatedInput.
function member(text, object, key,    pos, name, end) {
    if (substr(text, object, 1) != "{") return 0
    pos = object + 1
    while (pos <= length(text)) {
        pos = skip_space(text, pos)
        name = json_string(text, pos)
        if (!json_ok) return 0
        pos = skip_space(text, json_end)
        if (substr(text, pos, 1) != ":") return 0
        pos = skip_space(text, pos + 1)
        end = json_value_end(text, pos)
        if (!end) return 0
        if (name == key) {
            member_end = end
            return pos
        }
        pos = skip_space(text, end)
        if (substr(text, pos, 1) != ",") return 0
        pos++
    }
    return 0
}

function json_escape(text,    i, c, out, n, controls) {
    for (n = 0; n < 32; n++) controls[sprintf("%c", n)] = n
    out = ""
    for (i = 1; i <= length(text); i++) {
        c = substr(text, i, 1)
        if (c == "\\") out = out "\\\\"
        else if (c == "\"") out = out "\\\""
        else if (c in controls) out = out sprintf("\\u%04x", controls[c])
        else out = out c
    }
    return out
}

function replace_pairs(text,    first, second, out) {
    out = ""
    while ((first = index(text, "`"))) {
        second = index(substr(text, first + 1), "`")
        if (!second) break
        second += first
        out = out substr(text, 1, first - 1) "**" substr(text, first + 1, second - first - 1) "**"
        text = substr(text, second + 1)
    }
    return out text
}

# Quotes within a paired backtick expression belong to that expression; they
# must not prematurely close its enclosing double-quoted argument.
function quote_end(line, start,    quote, i, c, end) {
    quote = substr(line, start, 1)
    for (i = start + 1; i <= length(line); i++) {
        c = substr(line, i, 1)
        if (quote == "\"" && c == "\\" && i < length(line)) i++
        else if (quote == "\"" && substr(line, i, 2) == "$(") i = substitution_end(line, i)
        else if (quote == "\"" && c == "`") {
            end = index(substr(line, i + 1), "`")
            if (end) i += end
        } else if (c == quote) return i
    }
    return length(line) + 1
}

function substitution_end(line, start,    i, c, depth) {
    depth = 1
    for (i = start + 2; i <= length(line); i++) {
        c = substr(line, i, 1)
        if (c == "\\") i++
        else if (c == "\047" || c == "\"") i = quote_end(line, i)
        else if (c == "(") depth++
        else if (c == ")" && --depth == 0) return i
    }
    return length(line) + 1
}

# word_literal preserves backslashes. Only source backtick positions in
# complete, same-line quoted strings can change in word_output.
function read_word(line, start, rewrite,    i, c, end, content) {
    word_literal = word_output = ""
    word_complete = 1
    for (i = start; i <= length(line); ) {
        c = substr(line, i, 1)
        if (c ~ /[[:space:];&|()<>]/) break
        if (c == "\\" && i < length(line)) {
            word_literal = word_literal substr(line, i, 2)
            word_output = word_output substr(line, i, 2)
            i += 2
        } else if (c == "\047" || c == "\"") {
            end = quote_end(line, i)
            if (end > length(line)) {
                word_complete = 0
                word_literal = word_literal substr(line, i)
                word_output = word_output substr(line, i)
                i = length(line) + 1
                break
            }
            content = substr(line, i + 1, end - i - 1)
            word_literal = word_literal content
            word_output = word_output c (rewrite ? replace_pairs(content) : content) c
            i = end + 1
        } else if (c == "`" && (end = index(substr(line, i + 1), "`"))) {
            end += i
            word_literal = word_literal substr(line, i, end - i + 1)
            word_output = word_output substr(line, i, end - i + 1)
            i = end + 1
        } else {
            word_literal = word_literal c
            word_output = word_output c
            i++
        }
    }
    word_end = i
}

function scan_substitution_docs(content,    i, c) {
    for (i = 1; i < length(content); i++) {
        c = substr(content, i, 1)
        if (c == "\\") i++
        else if (substr(content, i, 2) == "$(") {
            scan_here_docs(substr(content, i + 2))
            return
        }
    }
}

function heredoc_delimiter(raw,    i, c, quote, next_char, value) {
    quote = value = ""
    for (i = 1; i <= length(raw); i++) {
        c = substr(raw, i, 1)
        next_char = substr(raw, i + 1, 1)
        if (quote == "\047") {
            if (c == quote) quote = ""
            else value = value c
        } else if (quote == "\"") {
            if (c == quote) quote = ""
            else if (c == "\\" && next_char ~ /[$`"\\]/) value = value substr(raw, ++i, 1)
            else value = value c
        } else if (c == "\047" || c == "\"") quote = c
        else if (c == "\\" && i < length(raw)) value = value substr(raw, ++i, 1)
        else value = value c
    }
    return value
}

function scan_here_docs(line,    i, c, end, content, tabs) {
    for (i = 1; i <= length(line); ) {
        c = substr(line, i, 1)
        if (c == "#" && (i == 1 || substr(line, i - 1, 1) ~ /[[:space:];&|()]/)) return
        if (c == "\\") i += 2
        else if (c == "\047" || c == "\"") {
            end = quote_end(line, i)
            if (c == "\"") {
                content = substr(line, i + 1, end - i - 1)
                scan_substitution_docs(content)
            }
            i = end + 1
        } else if (substr(line, i, 3) == "<<<") i += 3
        else if (substr(line, i, 2) == "<<") {
            i += 2
            tabs = substr(line, i, 1) == "-"
            if (tabs) i++
            while (substr(line, i, 1) ~ /[ \t]/) i++
            read_word(line, i, 0)
            if (word_end == i || !word_complete) return
            doc_delimiter[++doc_count] = heredoc_delimiter(substr(line, i, word_end - i))
            doc_tabs[doc_count] = tabs
            i = word_end
        } else i++
    }
}

function reset_command() {
    command_start = 1
    is_norns = 0
    prefix = ""
    prefix_argument = 0
}

function recognize_word(word, raw,    name) {
    if (!command_start) return
    if (prefix_argument) {
        prefix_argument = 0
        return
    }
    if (prefix != "" && word == "--") {
        prefix = ""
        return
    }
    if (prefix == "command" && (word == "-v" || word == "-V")) {
        command_start = 0
        return
    }
    if ((prefix == "env" && (word == "-u" || word == "--unset" || word == "-C" || word == "--chdir")) || (prefix == "exec" && word == "-a")) {
        prefix_argument = 1
        return
    }
    if (prefix != "" && word ~ /^-/) return
    if (word ~ /^[A-Za-z_][A-Za-z_0-9]*=/) return
    if (prefix == "" && raw == word && word ~ /^(if|then|elif|else|do|while|until|!)$/) return
    if (word == "env" || word == "command" || word == "exec") {
        prefix = word
        return
    }
    command_start = 0
    name = word
    gsub(/\\/, "/", name)
    sub(/^.*\//, "", name)
    is_norns = name == "norns" || name == "norns.exe"
}

function rewrite_line(line,    i, c, start, out) {
    out = ""
    for (i = 1; i <= length(line); ) {
        c = substr(line, i, 1)
        if (c ~ /[[:space:]]/) {
            out = out c
            i++
        } else if (c == "#") {
            out = out substr(line, i)
            reset_command()
            break
        } else if (c ~ /[;&|()]/) {
            reset_command()
            out = out c
            i++
        } else if (substr(line, i, 3) == "<<<") {
            out = out "<<<"
            i += 3
        } else if (substr(line, i, 2) == "<<") {
            start = i
            i += 2
            if (substr(line, i, 1) == "-") i++
            while (substr(line, i, 1) ~ /[ \t]/) i++
            read_word(line, i, 0)
            i = word_end
            out = out substr(line, start, i - start)
        } else if (c == "<" || c == ">") {
            out = out c
            i++
        } else {
            start = i
            read_word(line, i, is_norns)
            out = out word_output
            i = word_end
            recognize_word(word_literal, substr(line, start, i - start))
        }
    }
    return out
}

function rewrite_command(command,    lines, count, i, line, candidate, out, end, j, slashes) {
    count = split(command, lines, "\n")
    doc_count = 0
    doc_head = 1
    reset_command()
    out = ""
    for (i = 1; i <= count; i++) {
        line = lines[i]
        if (doc_head <= doc_count) {
            candidate = line
            sub(/\r$/, "", candidate)
            if (doc_tabs[doc_head]) sub(/^\t*/, "", candidate)
            if (candidate == doc_delimiter[doc_head]) doc_head++
        } else {
            scan_here_docs(line)
            line = rewrite_line(line)
            end = lines[i]
            sub(/\r$/, "", end)
            slashes = 0
            for (j = length(end); j > 0 && substr(end, j, 1) == "\\"; j--) slashes++
            if (slashes % 2 == 0) reset_command()
        }
        out = out (i > 1 ? "\n" : "") line
    }
    return out
}

END {
    root = skip_space(payload, 1)
    root_end = json_value_end(payload, root)
    if (!root_end || skip_space(payload, root_end) <= length(payload)) exit 0
    pos = member(payload, root, "tool_name")
    if (!pos) exit 0
    tool = json_string(payload, pos)
    if (!json_ok || tool != "Bash") exit 0
    input_start = member(payload, root, "tool_input")
    input_end = member_end
    if (!input_start) exit 0
    command_start_json = member(payload, input_start, "command")
    command_end_json = member_end
    if (!command_start_json) exit 0
    command = json_string(payload, command_start_json)
    if (!json_ok) exit 0
    updated = rewrite_command(command)
    if (updated == command) exit 0
    input = substr(payload, input_start, command_start_json - input_start) "\"" json_escape(updated) "\"" substr(payload, command_end_json, input_end - command_end_json)
    printf "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"updatedInput\":%s}}\n", input
}
'
