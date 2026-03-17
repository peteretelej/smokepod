// Package testfile provides parsing for .test files.
package testfile

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// TestFile represents a parsed .test file.
type TestFile struct {
	Sections map[string]*Section
	Order    []string // preserve section order
}

// Section is a named group of commands.
type Section struct {
	Name     string
	Commands []Command
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
	sectionPattern  = regexp.MustCompile(`^##\s+(\S+)`)
	commandPattern  = regexp.MustCompile(`^\$\s+(.+)`)
	exitCodePattern = regexp.MustCompile(`^\[exit:(\d+)\]$`)
	suffixPattern   = regexp.MustCompile(`\s+\((re|stderr)(?:,(re|stderr))*\)$`)
)

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
		if strings.HasPrefix(strings.TrimSpace(line), "#") && !strings.HasPrefix(strings.TrimSpace(line), "##") {
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

			currentSection = &Section{Name: sectionName}
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

			// Parse suffix flags: (re), (stderr), (stderr,re), (re,stderr)
			isRegex := false
			isStderr := false
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
					}
				}
			}

			currentCommand.Expected = append(currentCommand.Expected, Expect{
				Line:     lineNum,
				Text:     text,
				IsRegex:  isRegex,
				IsStderr: isStderr,
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
