#!/bin/sh
# smokepod sandbox shim: speak smokepod's process-mode framing on stdio for
# environments where smokepod itself cannot run (containers, remote sandboxes).
#
# Request (one per line, written by `smokepod verify --mode process`):
#   {"jsonrpc":"2.0","id":N,"method":"exec","params":{"command":"..."}}
# Response (one per line):
#   {"jsonrpc":"2.0","id":N,"result":{"stdout":"...","stderr":"...","exit_code":N}}
#
# Needs only POSIX sh plus sed, awk, tail, od (all present in busybox), and
# delegates every command to `sh -c`. See docs/sandboxes.md for the protocol,
# its limitations, and the pinned static-binary alternative.
set -u

out="/tmp/.smokepod-shim.$$.out"
err="/tmp/.smokepod-shim.$$.err"
trap 'rm -f "$out" "$err"' EXIT

# json_escape FILE: print the JSON-escaped string body (no quotes) for the
# file's bytes. Escapes backslash, double quote, tab, CR, newlines, and other
# C0 control characters. NUL bytes are not representable (sh and awk drop
# them); the conformance battery only runs commands with tame output.
json_escape() {
    _trailing=""
    if [ -s "$1" ]; then
        case "$(tail -c 1 "$1" | od -An -c | tr -d ' ')" in
            '\n') _trailing='\n' ;;
        esac
    fi
    awk '
        BEGIN {
            ORS = ""
            CTL = ""
            for (_i = 1; _i < 32; _i++) CTL = CTL sprintf("%c", _i)
            CTL = CTL "\177"
        }
        function esc(line,   _i, _n, _c, _pos, _out) {
            _n = length(line)
            _out = ""
            for (_i = 1; _i <= _n; _i++) {
                _c = substr(line, _i, 1)
                if (_c == "\\") _out = _out "\\\\"
                else if (_c == "\"") _out = _out "\\\""
                else if (_c == "\t") _out = _out "\\t"
                else if (_c == "\r") _out = _out "\\r"
                else {
                    _pos = index(CTL, _c)
                    if (_pos > 0) _out = _out sprintf("\\u%04x", _pos - 1)
                    else _out = _out _c
                }
            }
            return _out
        }
        { if (NR > 1) print "\\n"; print esc($0) }
    ' "$1"
    printf '%s' "$_trailing"
}

# json_unescape: decode a JSON string body produced by Go's encoding/json
# (\\ \" \n \t \r \uXXXX) in one left-to-right pass, so `\\n` stays a literal
# backslash-n instead of collapsing to a newline. Non-ASCII \uXXXX codes are
# emitted as raw bytes; \u0000 cannot survive the shell.
json_unescape() {
    awk '
        function hexval(s,   _i, _c, _n, _d) {
            _n = 0
            for (_i = 1; _i <= length(s); _i++) {
                _c = tolower(substr(s, _i, 1))
                _d = index("0123456789abcdef", _c) - 1
                if (_d < 0) return -1
                _n = _n * 16 + _d
            }
            return _n
        }
        {
            line = $0
            out = ""
            while ((i = index(line, "\\")) > 0) {
                out = out substr(line, 1, i - 1)
                c = substr(line, i + 1, 1)
                if (c == "n")      { out = out "\n"; line = substr(line, i + 2) }
                else if (c == "t") { out = out "\t"; line = substr(line, i + 2) }
                else if (c == "r") { out = out "\r"; line = substr(line, i + 2) }
                else if (c == "\"") { out = out "\""; line = substr(line, i + 2) }
                else if (c == "\\") { out = out "\\"; line = substr(line, i + 2) }
                else if (c == "u") {
                    code = hexval(substr(line, i + 2, 4))
                    if (code < 0) { out = out substr(line, i, 6); line = substr(line, i + 6) }
                    else { out = out sprintf("%c", code); line = substr(line, i + 6) }
                }
                else { out = out "\\" c; line = substr(line, i + 2) }
            }
            print out line
        }
    '
}

while IFS= read -r req; do
    # smokepod writes compact JSON, so these literals only match real
    # framing fields, never escaped quotes inside the command string.
    case "$req" in
        *'"method":"exec","params":{"command":"'*) ;;
        *'"jsonrpc"'*)
            printf '{"jsonrpc":"2.0","id":null,"error":{"code":-32601,"message":"unsupported method"}}\n'
            continue
            ;;
        *)
            continue
            ;;
    esac

    id=$(printf '%s\n' "$req" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
    cmd=$(printf '%s\n' "$req" | sed -e 's/.*"params":{"command":"//' -e 's/"}}$//')
    if [ -z "$id" ]; then
        printf '{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"unparseable request"}}\n'
        continue
    fi

    sh -c "$(printf '%s' "$cmd" | json_unescape)" </dev/null >"$out" 2>"$err"
    code=$?

    printf '{"jsonrpc":"2.0","id":%s,"result":{"stdout":"%s","stderr":"%s","exit_code":%s}}\n' \
        "$id" "$(json_escape "$out")" "$(json_escape "$err")" "$code"
done
