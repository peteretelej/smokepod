// Package normalize provides built-in output normalization filters that
// tolerate volatile content (timestamps, UUIDs, paths, whitespace) before
// fixture comparison. The filter set is closed: users pick from these
// filters via the `# normalize:` directive; there are no user-defined
// regex filters.
package normalize

import (
	"regexp"
	"strings"
)

// Filters lists the built-in filter names in application order.
var Filters = []string{"timestamps", "uuids", "paths", "whitespace"}

// Valid reports whether name is a built-in filter.
func Valid(name string) bool {
	for _, f := range Filters {
		if f == name {
			return true
		}
	}
	return false
}

var (
	timestampRe = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[Tt ]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]\d{2}:?\d{2})?` + // ISO 8601
		`|\d{4}-\d{2}-\d{2}` + // date only
		`|\d{2}:\d{2}:\d{2}`) // time only
	uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	pathRe = regexp.MustCompile(`(?:[\w.\-]+)?(?:/[\w.\-]+)+/?`)
)

// Collapse returns s with every whitespace run replaced by a single space
// and leading/trailing whitespace removed.
func Collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Line applies the named filters to a single output line. Unknown filter
// names are ignored; the parser rejects them before comparison runs.
func Line(line string, filters []string) string {
	for _, f := range filters {
		switch f {
		case "timestamps":
			line = timestampRe.ReplaceAllString(line, "<timestamp>")
		case "uuids":
			line = uuidRe.ReplaceAllString(line, "<uuid>")
		case "paths":
			line = pathRe.ReplaceAllString(line, "<path>")
		case "whitespace":
			line = Collapse(line)
		}
	}
	return line
}

// Text applies the named filters to every line of s.
func Text(s string, filters []string) string {
	if len(filters) == 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = Line(line, filters)
	}
	return strings.Join(lines, "\n")
}
