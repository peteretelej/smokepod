package smokepod

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/peteretelej/smokepod/internal/testfile"
)

// ErrVerifyFailed reports that verification did not pass (failing sections
// or manifest drift). The returned Result still carries the per-section
// details, so callers that want structured output can use it.
var ErrVerifyFailed = errors.New("verification failed")

// UsageError marks flow failures caused by how the flow was invoked
// (missing manifest, empty discovery, missing target): configuration
// problems rather than failing verifications. Error text is the
// user-facing message; the CLI prints it verbatim with a config-style
// exit code.
type UsageError struct {
	Err error
}

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// VerifyOptions configures one verify run.
type VerifyOptions struct {
	Target      string
	TargetArgs  []string
	Mode        string
	TestsDir    string
	FixturesDir string
	// TestFiles pins the exact test files to verify (probe uses this to
	// verify one file while the manifest still covers the whole TestsDir).
	// When nil, discovery runs on TestsDir.
	TestFiles   []string
	RunSections []string
	FailFast    bool
	AllowEmpty  bool
	// Repeat runs every command up to k times; all runs must match.
	// Values below 1 are treated as 1.
	Repeat int
	// LegacyFraming speaks the pre-JSON-RPC process-target protocol
	// (verify --legacy-framing migration window).
	LegacyFraming bool
	// ReportWriter receives the progress output (dots, failure diffs,
	// summaries). Defaults to os.Stderr; pass io.Discard for silent runs.
	ReportWriter io.Writer
}

// Verify runs the commands from the .test files against the target and
// compares the output with the recorded fixtures. Baselines are protected:
// the recorded manifest must exist and match the on-disk trees, and drift
// fails the run. The caller owns cancellation (context), timeout, and signal
// handling. Progress is reported on ReportWriter; the structured outcome is
// the returned Result.
func Verify(ctx context.Context, opts VerifyOptions) (*Result, error) {
	testFiles := opts.TestFiles
	if testFiles == nil {
		var err error
		testFiles, err = FindTestFiles(opts.TestsDir)
		if err != nil {
			return nil, &UsageError{fmt.Errorf("Error finding test files: %v", err)}
		}
		if len(testFiles) == 0 {
			if !opts.AllowEmpty {
				return nil, &UsageError{fmt.Errorf("no .test files found in %s; use --allow-empty to allow", opts.TestsDir)}
			}
			return nil, nil
		}
	}

	// Baseline protection: load the recorded manifest and compare it with the
	// current state on disk before running anything.
	recorded, err := LoadManifest(opts.FixturesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, &UsageError{fmt.Errorf("no %s in %s; baselines are protected - run `smokepod record` once with the same --tests/--fixtures to create one", ManifestFileName, opts.FixturesDir)}
		}
		return nil, fmt.Errorf("Error reading manifest: %w", err)
	}
	onDisk, err := ScanManifest(opts.TestsDir, opts.FixturesDir)
	if err != nil {
		return nil, fmt.Errorf("Error scanning baseline: %w", err)
	}
	drift := DiffManifest(recorded, onDisk)

	// Create shared ProcessTarget when the CLI provides both --mode process
	// and --target
	var sharedProcTarget *ProcessTarget
	if opts.Mode == "process" && opts.Target != "" {
		if _, err := exec.LookPath(opts.Target); err != nil {
			return nil, &UsageError{fmt.Errorf("target %q not found in PATH", opts.Target)}
		}
		pt, err := newProcessTargetForFraming(ctx, opts.LegacyFraming, opts.Target, opts.TargetArgs...)
		if err != nil {
			return nil, fmt.Errorf("Error creating shared process target: %w", err)
		}
		sharedProcTarget = pt
		defer func() { _ = sharedProcTarget.Close() }()
	}

	closeTarget := func(t Target) {
		if t != nil && t != sharedProcTarget {
			_ = t.Close()
		}
	}

	reportOut := opts.ReportWriter
	if reportOut == nil {
		reportOut = os.Stderr
	}
	reporter := NewVerifyReporter(reportOut)

	// Structural failures (stale/missing sections, command count mismatch)
	// take precedence over manifest drift messaging: they already pinpoint
	// the remediation for structure changes.
	structuralFailure := false

	sectionsPassed := 0
	sectionsFailed := 0
	sectionsXFail := 0
	sectionsXPass := 0
	sectionsTotal := 0
	var allSectionResults []SectionResult
	var changeEntries []ChangeEntry

	repeat := opts.Repeat
	if repeat < 1 {
		repeat = 1
	}

	for _, testFile := range testFiles {
		fixturePath := FixturePathFromTest(testFile, opts.TestsDir, opts.FixturesDir)

		fixture, err := ReadFixture(fixturePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading fixture %s: %v\n", fixturePath, err)
			sectionsFailed++
			sectionsTotal++
			if opts.FailFast {
				break
			}
			continue
		}

		tf, err := testfile.Parse(testFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", testFile, err)
			sectionsFailed++
			sectionsTotal++
			if opts.FailFast {
				break
			}
			continue
		}

		for _, w := range tf.Warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s: %s\n", testFile, w)
		}

		resolvedTarget, resolvedArgs, resolvedMode, err := ResolveTarget(testFile, tf.Metadata, opts.Target, opts.TargetArgs, opts.Mode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			sectionsFailed++
			sectionsTotal++
			if opts.FailFast {
				break
			}
			continue
		}

		if _, err := exec.LookPath(resolvedTarget); err != nil {
			fmt.Fprintf(os.Stderr, "%s: target %q not found in PATH\n", testFile, resolvedTarget)
			sectionsFailed++
			sectionsTotal++
			if opts.FailFast {
				break
			}
			continue
		}

		var targetExec Target
		if resolvedMode == "process" {
			if sharedProcTarget != nil {
				targetExec = sharedProcTarget
			} else {
				procTarget, err := newProcessTargetForFraming(ctx, opts.LegacyFraming, resolvedTarget, resolvedArgs...)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating process target for %s: %v\n", testFile, err)
					sectionsFailed++
					sectionsTotal++
					if opts.FailFast {
						break
					}
					continue
				}
				targetExec = procTarget
			}
		} else {
			targetExec = NewLocalTarget(resolvedTarget, resolvedArgs, nil, resolvedMode)
		}

		// Get sections to verify from .test file
		// When --run is specified, only get sections that exist in the .test file
		// (missing ones are handled separately below)
		var sections []*testfile.Section
		if len(opts.RunSections) > 0 {
			for _, name := range opts.RunSections {
				if s := tf.GetSection(name); s != nil {
					sections = append(sections, s)
				}
			}
		} else {
			sections, err = tf.GetSections(nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting sections from %s: %v\n", testFile, err)
				sectionsFailed++
				sectionsTotal++
				closeTarget(targetExec)
				if opts.FailFast {
					break
				}
				continue
			}
		}

		// Build set of .test section names for quick lookup
		testSectionSet := make(map[string]bool, len(tf.Sections))
		for name := range tf.Sections {
			testSectionSet[name] = true
		}

		// Build the --run filter set (nil when --run is not specified)
		var runFilterSet map[string]bool
		if len(opts.RunSections) > 0 {
			runFilterSet = make(map[string]bool, len(opts.RunSections))
			for _, name := range opts.RunSections {
				runFilterSet[name] = true
			}
		}

		// Check for stale fixture sections
		staleFailed := false
		for fixtureSectionName := range fixture.Sections {
			if testSectionSet[fixtureSectionName] {
				// Section exists in .test file: not stale
				continue
			}
			// Section is in fixture but not in .test file
			if runFilterSet != nil && !runFilterSet[fixtureSectionName] {
				// --run is specified and this section is outside the filter: ignore
				continue
			}
			// Stale: either --run is not specified (full scope) or section is in --run filter
			reporter.ReportFailure(fixtureSectionName, fmt.Sprintf(
				"stale fixture section: exists in fixture but not in .test file (fixture: %s)", fixturePath,
			))
			reporter.ReportSection(fixtureSectionName, "fail")
			sectionsFailed++
			sectionsTotal++
			allSectionResults = append(allSectionResults, SectionResult{Name: fixtureSectionName, Status: "fail"})
			staleFailed = true
			structuralFailure = true
			if opts.FailFast {
				break
			}
		}
		if staleFailed && opts.FailFast {
			closeTarget(targetExec)
			break
		}

		// Check for --run sections that don't exist in either .test or fixture
		if runFilterSet != nil {
			runMissingFailed := false
			for _, name := range opts.RunSections {
				if testSectionSet[name] {
					continue // exists in .test, will be verified below
				}
				if _, inFixture := fixture.Sections[name]; inFixture {
					continue // already reported as stale above
				}
				// Section doesn't exist anywhere
				reporter.ReportFailure(name, "section not found in .test file or fixture")
				reporter.ReportSection(name, "fail")
				sectionsFailed++
				sectionsTotal++
				allSectionResults = append(allSectionResults, SectionResult{Name: name, Status: "fail"})
				runMissingFailed = true
				if opts.FailFast {
					break
				}
			}
			if runMissingFailed && opts.FailFast {
				closeTarget(targetExec)
				break
			}
		}

		for _, section := range sections {
			sectionPassed := true

			fixtureCommands, hasFixture := fixture.Sections[section.Name]
			if !hasFixture {
				reporter.ReportFailure(section.Name, fmt.Sprintf(
					"missing fixture section: exists in .test but not in fixture (fixture: %s)", fixturePath,
				))
				reporter.ReportSection(section.Name, "fail")
				sectionsFailed++
				sectionsTotal++
				structuralFailure = true
				allSectionResults = append(allSectionResults, SectionResult{Name: section.Name, Status: "fail"})
				if opts.FailFast {
					break
				}
				continue
			}

			// Command-count mismatch detection (bidirectional)
			testCmdCount := len(section.Commands)
			fixtureCmdCount := len(fixtureCommands)
			commandCount := testCmdCount
			countMismatch := false
			if testCmdCount != fixtureCmdCount {
				commandCount = min(testCmdCount, fixtureCmdCount)
				countMismatch = true
			}

			changeAdded := false
			for i := 0; i < commandCount; i++ {
				tcmd := section.Commands[i]
				failedRuns := 0
				var mismatchDiff string
				var entry ChangeEntry
				for run := 1; run <= repeat; run++ {
					result, err := targetExec.Exec(ctx, tcmd.Cmd)
					if err != nil {
						fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
						sectionPassed = false
						failedRuns++
						continue
					}

					expected := fixtureCommands[i]

					testExpected := section.Commands[i].Expected
					stdoutResult := CompareOutputWithExpectations(
						expected.Stdout, result.Stdout, testExpected, false, tf.Normalize)
					stderrResult := CompareOutputWithExpectations(
						expected.Stderr, result.Stderr, testExpected, true, tf.Normalize)
					exitMatched := CompareExitCode(expected.ExitCode, result.ExitCode)

					if !stdoutResult.Matched || !stderrResult.Matched || !exitMatched {
						sectionPassed = false
						failedRuns++

						// Keep the first divergent run's diff and pair so the
						// report stays compact when runs disagree.
						if mismatchDiff == "" {
							var diffParts []string
							if !stdoutResult.Matched {
								diffParts = append(diffParts, "stdout:\n"+stdoutResult.Diff)
							}
							if !stderrResult.Matched {
								diffParts = append(diffParts, "stderr:\n"+stderrResult.Diff)
							}
							if !exitMatched {
								diffParts = append(diffParts, fmt.Sprintf("Exit code: expected %d, got %d", expected.ExitCode, result.ExitCode))
							}
							mismatchDiff = strings.Join(diffParts, "\n")

							entry = ChangeEntry{
								File:     testFile,
								Section:  section.Name,
								Line:     stdoutResult.FirstLine,
								Expected: stdoutResult.FirstExpected,
								Actual:   stdoutResult.FirstActual,
							}
							if stdoutResult.Matched {
								entry.Line = stderrResult.FirstLine
								entry.Expected = stderrResult.FirstExpected
								entry.Actual = stderrResult.FirstActual
								if stderrResult.Matched {
									entry.Line = 0
									entry.Expected = fmt.Sprintf("exit %d", expected.ExitCode)
									entry.Actual = fmt.Sprintf("exit %d", result.ExitCode)
								}
							}
						}
					}
				}

				if failedRuns > 0 && mismatchDiff != "" && !section.XFail {
					name := fmt.Sprintf("%s / %s", section.Name, tcmd.Cmd)
					if repeat > 1 {
						name += fmt.Sprintf(" (%d/%d runs failed)", failedRuns, repeat)
					}
					reporter.ReportFailure(name, mismatchDiff)
					if !changeAdded {
						changeAdded = true
						changeEntries = append(changeEntries, entry)
					}
				}
			}

			if countMismatch {
				reporter.ReportFailure(section.Name, fmt.Sprintf(
					"command count mismatch: .test has %d commands, fixture has %d (fixture: %s)",
					testCmdCount, fixtureCmdCount, fixturePath,
				))
				sectionPassed = false
				structuralFailure = true
			}

			// Determine section status
			sectionsTotal++
			var status string
			switch {
			case section.XFail && !sectionPassed:
				status = "xfail"
				sectionsXFail++
			case section.XFail && sectionPassed:
				status = "xpass"
				sectionsXPass++
				reporter.ReportXPass(section.Name, section.XFailReason, testFile, section.Line)
			case !section.XFail && sectionPassed:
				status = "pass"
				sectionsPassed++
			default:
				status = "fail"
				sectionsFailed++
			}

			reporter.ReportSection(section.Name, status)
			allSectionResults = append(allSectionResults, SectionResult{
				Name:        section.Name,
				Status:      status,
				XFailReason: section.XFailReason,
			})

			if (status == "fail" || status == "xpass") && opts.FailFast {
				break
			}
		}

		closeTarget(targetExec)

		if (sectionsFailed > 0 || sectionsXPass > 0) && opts.FailFast {
			break
		}
	}

	reporter.ReportChangeSummary(changeEntries)
	reporter.ReportSummary(sectionsPassed, sectionsFailed, sectionsXFail, sectionsXPass, sectionsTotal)

	result := &Result{
		Name:      "verify",
		Timestamp: time.Now(),
		Passed:    sectionsFailed == 0 && sectionsXPass == 0,
		Summary: Summary{
			Total:  sectionsTotal,
			Passed: sectionsPassed,
			Failed: sectionsFailed,
			XFail:  sectionsXFail,
			XPass:  sectionsXPass,
		},
		Tests: []TestResult{{
			Name:     "verify",
			Type:     "cli",
			Passed:   sectionsFailed == 0 && sectionsXPass == 0,
			Sections: allSectionResults,
		}},
		Changes: changeEntries,
		Drift:   drift,
	}

	if len(drift) > 0 && !structuralFailure {
		reporter.ReportManifestDrift(drift)
		return result, ErrVerifyFailed
	}

	if sectionsFailed > 0 || sectionsXPass > 0 {
		return result, ErrVerifyFailed
	}
	return result, nil
}

// newProcessTargetForFraming starts a process target speaking the current
// JSON-RPC framing, or the legacy one during the migration window.
func newProcessTargetForFraming(ctx context.Context, legacy bool, path string, args ...string) (*ProcessTarget, error) {
	if legacy {
		return NewProcessTargetLegacy(ctx, path, args...)
	}
	return NewProcessTarget(ctx, path, args...)
}
