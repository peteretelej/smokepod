// Package testfile provides parsing for .test files.
package testfile

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/peteretelej/smokepod/internal/normalize"
)

// TestFile represents a parsed .test file.
type TestFile struct {
	Sections map[string]*Section
	Order    []string            // preserve section order
	Metadata map[string][]string // directives from pre-section comments
	Warnings []string            // parser warnings (non-fatal)
	// Normalize holds the built-in filter names from `# normalize:`
	// directives, applied to expected and actual output before comparison.
	Normalize []string
}

// Section is a named group of commands.
type Section struct {
	Name        string
	Commands    []Command
	XFail       bool
	XFailReason string
	Line        int // source line number of the ## header
}

// Command is a single test command with expectations.
type Command struct {
	Line     int      // source line number
	Cmd      string   // the command to run
	Expected []Expect // expected output lines
	ExitCode int      // expected exit code (default 0)
}

// Expect is a single expected output line.
type Expect struct {
	Line     int
	Text     string
	IsRegex  bool
	IsStderr bool
	IsNorm   bool // whitespace-insensitive, case-insensitive match
	IsJSON   bool // structural JSON match (key order independent, arrays ordered)
}

// parser state
type parserState int

const (
	stateIdle parserState = iota
	stateInSection
	stateInCommand
)

// ParseError represents a parsing error with line information.
type ParseError struct {
	Line    int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

var (
	sectionPattern  = regexp.MustCompile(`^##\s+(\S+)(?:\s+\((xfail)(?::\s*([^)]+))?\))?`)
	commandPattern  = regexp.MustCompile(`^\$\s+(.+)`)
	exitCodePattern = regexp.MustCompile(`^\[exit:(\d+)\]$`)
	suffixPattern   = regexp.MustCompile(`\s+\((re|stderr|norm|json)(?:,(re|stderr|norm|json))*\)$`)
	metadataPattern = regexp.MustCompile(`^#\s+(\w[\w-]*):\s+(.+)$`)
)

var knownDirectives = map[string]bool{
	"target": true, "target-arg": true, "mode": true, "xfail": true, "normalize": true,
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 1; j <= lb; j++ {
		d[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			d[i][j] = min(d[i-1][j]+1, min(d[i][j-1]+1, d[i-1][j-1]+cost))
		}
	}
	return d[la][lb]
}

func closestDirective(key string) string {
	best := ""
	bestDist := 3 // threshold: suggest only if distance <= 2
	for known := range knownDirectives {
		d := editDistance(key, known)
		if d < bestDist || (d == bestDist && known < best) {
			bestDist = d
			best = known
		}
	}
	return best
}

// parseNormalizeFilters validates a `# normalize:` value: a comma-separated
// list of built-in filter names. The set is closed, so an unknown name is a
// parse error (with a suggestion when one is close) rather than a silently
// ignored filter.
func parseNormalizeFilters(value string, lineNum int) ([]string, error) {
	var filters []string
	for _, part := range strings.Split(value, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			return nil, &ParseError{Line: lineNum, Message: "empty normalize filter in # normalize: directive"}
		}
		if !normalize.Valid(name) {
			msg := fmt.Sprintf("unknown normalize filter %q (built-in filters: %s)", name, strings.Join(normalize.Filters, ", "))
			if suggestion := closestFilter(name); suggestion != "" {
				msg += fmt.Sprintf(", did you mean %q?", suggestion)
			}
			return nil, &ParseError{Line: lineNum, Message: msg}
		}
		filters = append(filters, name)
	}
	return filters, nil
}

func closestFilter(name string) string {
	best := ""
	bestDist := 3 // threshold: suggest only if distance <= 2
	for _, known := range normalize.Filters {
		d := editDistance(name, known)
		if d < bestDist || (d == bestDist && known < best) {
			bestDist = d
			best = known
		}
	}
	return best
}

// Parse reads and parses a .test file.
func Parse(path string) (*TestFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening test file: %w", err)
	}
	defer func() { _ = f.Close() }()

	tf := &TestFile{
		Sections: make(map[string]*Section),
	}

	scanner := bufio.NewScanner(f)
	state := stateIdle
	var currentSection *Section
	var currentCommand *Command
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Handle comments
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "#") && !strings.HasPrefix(trimmedLine, "##") {
			if len(tf.Order) == 0 {
				// Before the first section header, attempt to parse as a metadata directive
				if matches := metadataPattern.FindStringSubmatch(trimmedLine); matches != nil {
					key := matches[1]
					value := strings.TrimSpace(matches[2])
					if tf.Metadata == nil {
						tf.Metadata = make(map[string][]string)
					}
					tf.Metadata[key] = append(tf.Metadata[key], value)

					if key == "normalize" {
						filters, err := parseNormalizeFilters(value, lineNum)
						if err != nil {
							return nil, err
						}
						tf.Normalize = append(tf.Normalize, filters...)
					}

					// Warn on unknown directives
					if !knownDirectives[key] {
						suggestion := closestDirective(key)
						if suggestion != "" {
							tf.Warnings = append(tf.Warnings, fmt.Sprintf(
								"line %d: unknown directive %q, did you mean %q?", lineNum, key, suggestion))
						} else {
							tf.Warnings = append(tf.Warnings, fmt.Sprintf(
								"line %d: unknown directive %q", lineNum, key))
						}
					}
				}
			} else {
				// After the first section header, warn if a known directive appears
				if matches := metadataPattern.FindStringSubmatch(trimmedLine); matches != nil {
					key := matches[1]
					if knownDirectives[key] {
						tf.Warnings = append(tf.Warnings, fmt.Sprintf(
							"line %d: directive '# %s: ...' will be ignored (directives must appear before the first section)",
							lineNum, key))
					}
				}
			}
			continue
		}

		// Check for section header
		if matches := sectionPattern.FindStringSubmatch(line); matches != nil {
			// Finalize previous command if any
			if currentCommand != nil && currentSection != nil {
				currentSection.Commands = append(currentSection.Commands, *currentCommand)
				currentCommand = nil
			}

			sectionName := matches[1]
			if _, exists := tf.Sections[sectionName]; exists {
				return nil, &ParseError{Line: lineNum, Message: fmt.Sprintf("duplicate section: %s", sectionName)}
			}

			currentSection = &Section{
				Name:        sectionName,
				XFail:       matches[2] == "xfail",
				XFailReason: matches[3],
				Line:        lineNum,
			}
			tf.Sections[sectionName] = currentSection
			tf.Order = append(tf.Order, sectionName)
			state = stateInSection
			continue
		}

		// Check for command
		if matches := commandPattern.FindStringSubmatch(line); matches != nil {
			if state == stateIdle {
				return nil, &ParseError{Line: lineNum, Message: "command before section header"}
			}

			// Multi-line command: if previous command has no expected output, concatenate
			if currentCommand != nil && state == stateInCommand && len(currentCommand.Expected) == 0 {
				currentCommand.Cmd += "\n" + matches[1]
				continue
			}

			// Finalize previous command if any
			if currentCommand != nil {
				currentSection.Commands = append(currentSection.Commands, *currentCommand)
			}

			currentCommand = &Command{
				Line:     lineNum,
				Cmd:      matches[1],
				ExitCode: 0, // default
			}
			state = stateInCommand
			continue
		}

		// Inside a command, collect expected output
		if state == stateInCommand {
			trimmed := strings.TrimSpace(line)

			// Empty line ends output collection (but stays in section)
			if trimmed == "" {
				if currentCommand != nil {
					currentSection.Commands = append(currentSection.Commands, *currentCommand)
					currentCommand = nil
				}
				state = stateInSection
				continue
			}

			// Check for exit code assertion
			if matches := exitCodePattern.FindStringSubmatch(trimmed); matches != nil {
				code, _ := strconv.Atoi(matches[1])
				currentCommand.ExitCode = code
				continue
			}

			// Parse suffix flags: (re), (stderr), (norm), (json), combinable
			isRegex := false
			isStderr := false
			isNorm := false
			isJSON := false
			text := line
			if loc := suffixPattern.FindStringIndex(line); loc != nil {
				suffix := line[loc[0]:]
				text = line[:loc[0]]
				// Extract flags from inside parentheses
				inner := suffix[strings.Index(suffix, "(")+1 : strings.LastIndex(suffix, ")")]
				for _, flag := range strings.Split(inner, ",") {
					switch strings.TrimSpace(flag) {
					case "re":
						isRegex = true
					case "stderr":
						isStderr = true
					case "norm":
						isNorm = true
					case "json":
						isJSON = true
					}
				}
			}

			currentCommand.Expected = append(currentCommand.Expected, Expect{
				Line:     lineNum,
				Text:     text,
				IsRegex:  isRegex,
				IsStderr: isStderr,
				IsNorm:   isNorm,
				IsJSON:   isJSON,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading test file: %w", err)
	}

	// Finalize last command
	if currentCommand != nil && currentSection != nil {
		currentSection.Commands = append(currentSection.Commands, *currentCommand)
	}

	return tf, nil
}

// GetSection returns a section by name, or nil if not found.
func (tf *TestFile) GetSection(name string) *Section {
	return tf.Sections[name]
}

// GetSections returns sections by names, or all sections if names is empty.
// Returns an error if any named section doesn't exist.
func (tf *TestFile) GetSections(names []string) ([]*Section, error) {
	if len(names) == 0 {
		// Return all sections in order
		sections := make([]*Section, 0, len(tf.Order))
		for _, name := range tf.Order {
			sections = append(sections, tf.Sections[name])
		}
		return sections, nil
	}

	sections := make([]*Section, 0, len(names))
	for _, name := range names {
		section := tf.Sections[name]
		if section == nil {
			return nil, fmt.Errorf("section not found: %s", name)
		}
		sections = append(sections, section)
	}
	return sections, nil
}
