package runners

import (
	"testing"
	"time"
)

func TestParsePlaywrightOutput_Success(t *testing.T) {
	jsonStr := `{
		"suites": [
			{
				"title": "login.spec.ts",
				"file": "tests/login.spec.ts",
				"specs": [
					{
						"title": "should login successfully",
						"ok": true,
						"tests": [
							{
								"status": "passed",
								"duration": 1234
							}
						]
					},
					{
						"title": "should show error on invalid credentials",
						"ok": true,
						"tests": [
							{
								"status": "passed",
								"duration": 567
							}
						]
					}
				],
				"suites": []
			}
		],
		"stats": {
			"total": 2,
			"passed": 2,
			"failed": 0,
			"skipped": 0,
			"duration": 1801
		}
	}`

	result, err := ParsePlaywrightOutput(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected Passed to be true")
	}
	if result.Total != 2 {
		t.Errorf("Total: got %d, want 2", result.Total)
	}
	if result.PassedN != 2 {
		t.Errorf("PassedN: got %d, want 2", result.PassedN)
	}
	if result.FailedN != 0 {
		t.Errorf("FailedN: got %d, want 0", result.FailedN)
	}
	if result.Duration != 1801*time.Millisecond {
		t.Errorf("Duration: got %v, want 1801ms", result.Duration)
	}
	if len(result.Suites) != 1 {
		t.Fatalf("Suites: got %d, want 1", len(result.Suites))
	}
	if result.Suites[0].Name != "login.spec.ts" {
		t.Errorf("Suite name: got %q, want %q", result.Suites[0].Name, "login.spec.ts")
	}
	if len(result.Suites[0].Specs) != 2 {
		t.Errorf("Specs: got %d, want 2", len(result.Suites[0].Specs))
	}
}

func TestParsePlaywrightOutput_Failures(t *testing.T) {
	jsonStr := `{
		"suites": [
			{
				"title": "checkout.spec.ts",
				"file": "tests/checkout.spec.ts",
				"specs": [
					{
						"title": "should complete checkout",
						"ok": false,
						"tests": [
							{
								"status": "failed",
								"duration": 5000,
								"error": {
									"message": "Timeout waiting for element",
									"stack": "at checkout.spec.ts:15"
								}
							}
						]
					},
					{
						"title": "should display cart",
						"ok": true,
						"tests": [
							{
								"status": "passed",
								"duration": 300
							}
						]
					}
				],
				"suites": []
			}
		],
		"stats": {
			"total": 2,
			"passed": 1,
			"failed": 1,
			"skipped": 0,
			"duration": 5300
		}
	}`

	result, err := ParsePlaywrightOutput(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Passed {
		t.Error("expected Passed to be false")
	}
	if result.FailedN != 1 {
		t.Errorf("FailedN: got %d, want 1", result.FailedN)
	}
	if result.PassedN != 1 {
		t.Errorf("PassedN: got %d, want 1", result.PassedN)
	}

	// Check that error message is captured
	failedSpec := result.Suites[0].Specs[0]
	if failedSpec.Passed {
		t.Error("expected first spec to be failed")
	}
	if failedSpec.Error != "Timeout waiting for element" {
		t.Errorf("Error: got %q, want %q", failedSpec.Error, "Timeout waiting for element")
	}
}

func TestParsePlaywrightOutput_Empty(t *testing.T) {
	result, err := ParsePlaywrightOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected Passed to be true for empty output")
	}
	if result.Total != 0 {
		t.Errorf("Total: got %d, want 0", result.Total)
	}
}

func TestParsePlaywrightOutput_InvalidJSON(t *testing.T) {
	_, err := ParsePlaywrightOutput("not valid json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParsePlaywrightOutput_NestedSuites(t *testing.T) {
	jsonStr := `{
		"suites": [
			{
				"title": "root",
				"file": "tests/root.spec.ts",
				"specs": [],
				"suites": [
					{
						"title": "describe block",
						"file": "",
						"specs": [
							{
								"title": "nested test",
								"ok": true,
								"tests": [
									{
										"status": "passed",
										"duration": 100
									}
								]
							}
						],
						"suites": []
					}
				]
			}
		],
		"stats": {
			"total": 1,
			"passed": 1,
			"failed": 0,
			"skipped": 0,
			"duration": 100
		}
	}`

	result, err := ParsePlaywrightOutput(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Suites) != 1 {
		t.Fatalf("Suites: got %d, want 1 (nested suite)", len(result.Suites))
	}
	if result.Suites[0].Name != "describe block" {
		t.Errorf("Suite name: got %q, want %q", result.Suites[0].Name, "describe block")
	}
}

func TestParsePlaywrightOutput_Skipped(t *testing.T) {
	jsonStr := `{
		"suites": [
			{
				"title": "feature.spec.ts",
				"file": "tests/feature.spec.ts",
				"specs": [
					{
						"title": "skipped test",
						"ok": true,
						"tests": [
							{
								"status": "skipped",
								"duration": 0
							}
						]
					}
				],
				"suites": []
			}
		],
		"stats": {
			"total": 1,
			"passed": 0,
			"failed": 0,
			"skipped": 1,
			"duration": 50
		}
	}`

	result, err := ParsePlaywrightOutput(jsonStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Passed {
		t.Error("expected Passed to be true (skipped tests don't fail)")
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped: got %d, want 1", result.Skipped)
	}
}

func TestPlaywrightResult_ToTestResult(t *testing.T) {
	pr := &PlaywrightResult{
		Passed:   true,
		Duration: 5 * time.Second,
		Total:    10,
		PassedN:  10,
	}

	tr := pr.ToTestResult("e2e-suite")

	if tr.Name != "e2e-suite" {
		t.Errorf("Name: got %q, want %q", tr.Name, "e2e-suite")
	}
	if tr.Type != "playwright" {
		t.Errorf("Type: got %q, want %q", tr.Type, "playwright")
	}
	if !tr.Passed {
		t.Error("expected Passed to be true")
	}
	if tr.Duration != 5*time.Second {
		t.Errorf("Duration: got %v, want 5s", tr.Duration)
	}
}
