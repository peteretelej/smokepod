package smokepod

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ScaffoldFileName is the .test file init writes inside the tests directory.
const ScaffoldFileName = "smoke.test"

// InitFixturesDir is the fixtures directory init records into, relative to
// the working directory.
const InitFixturesDir = "fixtures"

// ErrScaffoldExists is returned when init would overwrite an existing
// scaffolded .test file or baseline and InitOptions.Force is not set.
var ErrScaffoldExists = errors.New("init would overwrite existing files")

// ErrTargetNotFound is returned when InitOptions.Target is not an
// executable in PATH.
var ErrTargetNotFound = errors.New("target not found in PATH")

// InitOptions configures one init run. With an empty Target only a fully
// commented template is scaffolded; otherwise the target is probed and a
// fixture baseline is recorded.
type InitOptions struct {
	Target string
	Dir    string
	Probes []string
	Force  bool
}

// InitResult reports what init wrote and what to run next.
type InitResult struct {
	Files   []string // files init wrote, in write order
	Skipped []string // probes whose command failed (commented out)
	Next    []string // follow-up commands
}

// Init scaffolds a .test file and, with a target, records its fixture
// baseline. It only overwrites the files it generates: an existing scaffolded
// .test file or manifest is refused unless opts.Force is set. Unrelated
// files are never touched.
func Init(ctx context.Context, opts InitOptions) (*InitResult, error) {
	dir := opts.Dir
	if dir == "" {
		dir = "tests"
	}
	testPath := filepath.Join(dir, ScaffoldFileName)

	var existing []string
	if _, err := os.Stat(testPath); err == nil {
		existing = append(existing, testPath)
	}
	if opts.Target != "" {
		if _, err := os.Stat(ManifestPath(InitFixturesDir)); err == nil {
			existing = append(existing, ManifestPath(InitFixturesDir))
		}
	}
	if len(existing) > 0 && !opts.Force {
		return nil, fmt.Errorf("%w: %s (use --force to overwrite)", ErrScaffoldExists, strings.Join(existing, ", "))
	}

	if opts.Target == "" {
		if err := writeScaffoldFile(testPath, bareTemplate(dir)); err != nil {
			return nil, err
		}
		return &InitResult{
			Files: []string{testPath},
			Next: []string{
				fmt.Sprintf("smokepod init --target <binary> --dir %s", dir),
			},
		}, nil
	}

	if _, err := exec.LookPath(opts.Target); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrTargetNotFound, opts.Target)
	}

	sections, skipped, err := probeTarget(ctx, opts.Target, opts.Probes)
	if err != nil {
		return nil, err
	}

	if err := writeScaffoldFile(testPath, renderScaffold(sections)); err != nil {
		return nil, err
	}
	res := &InitResult{Files: []string{testPath}, Skipped: skipped}

	summary, err := Record(ctx, RecordOptions{
		Target:      opts.Target,
		TestsDir:    dir,
		FixturesDir: InitFixturesDir,
		Update:      opts.Force,
		Indent:      "  ",
	})
	if err != nil {
		return nil, fmt.Errorf("recording baseline: %w", err)
	}
	if summary.Failed > 0 && summary.Recorded == 0 {
		return nil, fmt.Errorf("recording baseline failed: %d section(s) errored, no fixture written", summary.Failed)
	}

	res.Files = append(res.Files,
		filepath.Join(InitFixturesDir, strings.TrimSuffix(ScaffoldFileName, ".test")+".fixture.json"),
		ManifestPath(InitFixturesDir),
	)
	res.Next = []string{
		fmt.Sprintf("smokepod verify --target %s --tests %s --fixtures %s", opts.Target, dir, InitFixturesDir),
		fmt.Sprintf("smokepod update --target %s --tests %s --fixtures %s", opts.Target, dir, InitFixturesDir),
	}
	return res, nil
}

// scaffoldSection is one section of the scaffolded .test file.
type scaffoldSection struct {
	name     string
	command  string // text after "$ "; the target is referenced as $SMOKEPOD_TARGET
	expected []string
	active   bool // false renders the section commented out
}

// probeTarget runs the template probes (--version, --help, plus each user
// probe) against the target and returns the scaffold sections in probe
// order, plus the names of skipped probes.
func probeTarget(ctx context.Context, target string, probes []string) ([]scaffoldSection, []string, error) {
	type probeSpec struct {
		name string
		args []string
	}
	used := map[string]bool{"version": true, "help": true}
	specs := []probeSpec{
		{name: "version", args: []string{"--version"}},
		{name: "help", args: []string{"--help"}},
	}
	for _, p := range probes {
		args := strings.Fields(p)
		if len(args) == 0 {
			return nil, nil, fmt.Errorf("invalid probe %q: empty command", p)
		}
		specs = append(specs, probeSpec{name: scaffoldSectionName(args[0], used), args: args})
	}

	var sections []scaffoldSection
	var skipped []string
	for _, s := range specs {
		out := probeRun(ctx, target, s.args)
		if out.execErr != nil || out.exit != 0 {
			skipped = append(skipped, s.name)
			sections = append(sections, failedProbeSection(s.name, s.args, out))
			continue
		}
		sections = append(sections, okProbeSection(s.name, s.args, out))
	}
	return sections, skipped, nil
}

type probeResult struct {
	stdout  string
	stderr  string
	exit    int
	execErr error
}

func probeRun(ctx context.Context, target string, args []string) probeResult {
	cmd := exec.CommandContext(ctx, target, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	res := probeResult{}
	err := cmd.Run()
	res.stdout = stdout.String()
	res.stderr = stderr.String()
	if err == nil {
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.exit = exitErr.ExitCode()
	} else {
		res.execErr = err
	}
	return res
}

func okProbeSection(name string, args []string, out probeResult) scaffoldSection {
	command := `"$SMOKEPOD_TARGET" ` + strings.Join(args, " ")
	var expected []string
	switch name {
	case "version":
		// The recorded line becomes a regex hint so version bumps do not
		// break the smoke test.
		if line := firstLine(out.stdout); line != "" {
			expected = append(expected, templatizeVersion(line)+" (re)")
		} else if line := firstLine(out.stderr); line != "" {
			expected = append(expected, line+" (stderr)")
		}
	default:
		if line := firstLine(out.stdout); line != "" {
			expected = append(expected, line)
		} else if line := firstLine(out.stderr); line != "" {
			expected = append(expected, line+" (stderr)")
		}
	}
	return scaffoldSection{name: name, command: command, expected: expected, active: true}
}

func failedProbeSection(name string, args []string, out probeResult) scaffoldSection {
	command := `"$SMOKEPOD_TARGET" ` + strings.Join(args, " ")
	var expected []string
	if line := firstLine(out.stderr); line != "" {
		expected = append(expected, line+" (stderr)")
	}
	if out.execErr != nil {
		expected = append(expected, "# probe error: "+out.execErr.Error())
	} else {
		expected = append(expected, fmt.Sprintf("[exit:%d]", out.exit))
	}
	return scaffoldSection{name: name, command: command, expected: expected, active: false}
}

func renderScaffold(sections []scaffoldSection) string {
	var b strings.Builder
	b.WriteString("# Smoke test scaffolded by smokepod init\n")
	b.WriteString("#\n")
	b.WriteString("# Commands run through $SMOKEPOD_TARGET, which smokepod sets to\n")
	b.WriteString("# the --target binary for both record and verify. Edit freely.\n")
	b.WriteString("# normalize: timestamps,paths\n")

	// Active sections first so commented-out ones never sit before the
	// first real section, where the parser would read their comments as
	// metadata directives.
	for _, s := range sections {
		if !s.active {
			continue
		}
		b.WriteString("\n")
		writeScaffoldSection(&b, s)
	}
	for _, s := range sections {
		if s.active {
			continue
		}
		b.WriteString("\n")
		writeScaffoldSection(&b, s)
	}
	return b.String()
}

func writeScaffoldSection(b *strings.Builder, s scaffoldSection) {
	prefix := ""
	if !s.active {
		prefix = "# "
	}
	fmt.Fprintf(b, "%s## %s\n", prefix, s.name)
	fmt.Fprintf(b, "%s$ %s\n", prefix, s.command)
	for _, line := range s.expected {
		fmt.Fprintf(b, "%s%s\n", prefix, line)
	}
}

// bareTemplate is the fully commented scaffold written when no target is
// given. Comment lines must avoid "<word>: <text>" shapes before the first
// section: the parser reads those as metadata directives.
func bareTemplate(dir string) string {
	return `# Smoke test scaffolded by smokepod init
#
# No --target was given, so every section below is commented out.
# Scaffold against a binary to get probed sections:
#   smokepod init --target ./your-binary --dir ` + dir + `
# Or finish this file by hand, then record and verify a baseline:
#   smokepod record --target ./your-binary --tests ` + dir + ` --fixtures ` + InitFixturesDir + `
#   smokepod verify --target ./your-binary --tests ` + dir + ` --fixtures ` + InitFixturesDir + `
#
# normalize: timestamps,paths

# ## version
# $ "$SMOKEPOD_TARGET" --version
# your-binary version 1.0.0 (re)

# ## help
# $ "$SMOKEPOD_TARGET" --help
# first line of help output
`
}

func writeScaffoldFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// scaffoldSectionName derives a section name from a probe command's first
// word, keeping names unique with numeric suffixes.
func scaffoldSectionName(word string, used map[string]bool) string {
	var b strings.Builder
	for _, r := range word {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "probe"
	}
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[name] = true
	return name
}

// templatizeVersion turns a version line into a regex hint that survives
// version bumps: digit runs become \d+, everything else is quoted literally.
func templatizeVersion(line string) string {
	var b strings.Builder
	inDigits := false
	for _, r := range line {
		if r >= '0' && r <= '9' {
			if !inDigits {
				b.WriteString(`\d+`)
				inDigits = true
			}
			continue
		}
		inDigits = false
		b.WriteString(regexp.QuoteMeta(string(r)))
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimRight(s, "\r")
}
