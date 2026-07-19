//go:build cgo

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// bdInfo runs "bd info" with the given args and returns stdout.
func bdInfo(t *testing.T, bd, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"info"}, args...)
	cmd := exec.Command(bd, fullArgs...)
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	stdout, stderr, err := runCommandBuffers(t, cmd)
	if err != nil {
		t.Fatalf("bd info %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func TestEmbeddedInfo(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ti")

	// ===== Default Output =====

	t.Run("info_default", func(t *testing.T) {
		out := bdInfo(t, bd, dir)
		if !strings.Contains(out, ".beads") {
			t.Errorf("expected .beads path in info output: %s", out)
		}
		if !strings.Contains(out, "Issue Count") {
			t.Errorf("expected 'Issue Count' in info output: %s", out)
		}
	})

	t.Run("info_with_issues", func(t *testing.T) {
		bdCreate(t, bd, dir, "Info test issue 1", "--type", "task")
		bdCreate(t, bd, dir, "Info test issue 2", "--type", "task")

		out := bdInfo(t, bd, dir)
		// Should show non-zero issue count
		if strings.Contains(out, "Issue Count: 0") {
			t.Errorf("expected non-zero issue count after creating issues: %s", out)
		}
	})

	t.Run("info_count_excludes_wisps_and_matches_status", func(t *testing.T) {
		bdCreate(t, bd, dir, "Info durable count probe", "--type", "task")
		bdCreate(t, bd, dir, "Info ephemeral count probe", "--ephemeral")

		var info map[string]interface{}
		if err := json.Unmarshal([]byte(bdInfo(t, bd, dir, "--json")), &info); err != nil {
			t.Fatalf("parse info JSON: %v", err)
		}
		infoCount, ok := info["issue_count"].(float64)
		if !ok {
			t.Fatalf("info issue_count missing/not numeric: %v", info["issue_count"])
		}
		status := bdStatusJSON(t, bd, dir, "--no-activity")
		summary, ok := status["summary"].(map[string]interface{})
		if !ok {
			t.Fatalf("status summary missing: %v", status)
		}
		statusCount, ok := summary["total_issues"].(float64)
		if !ok {
			t.Fatalf("status total_issues missing/not numeric: %v", summary["total_issues"])
		}
		if infoCount != statusCount {
			t.Errorf("info issue_count = %v, want durable status total_issues %v", infoCount, statusCount)
		}
	})

	// ===== Schema Flag =====

	t.Run("info_schema", func(t *testing.T) {
		out := bdInfo(t, bd, dir, "--schema")
		if !strings.Contains(out, "issues") {
			t.Errorf("expected 'issues' table in schema output: %s", out)
		}
		if !strings.Contains(out, "Schema") {
			t.Errorf("expected 'Schema' heading in output: %s", out)
		}
	})

	// ===== Whats New =====

	t.Run("info_whats_new", func(t *testing.T) {
		out := bdInfo(t, bd, dir, "--whats-new")
		if len(strings.TrimSpace(out)) == 0 {
			t.Error("expected non-empty --whats-new output")
		}
		if !strings.Contains(out, "v0.") {
			t.Errorf("expected version string in whats-new output: %s", out[:min(200, len(out))])
		}
	})

	// Note: --json tests skipped — info's local --json flag shadows the
	// root persistent flag, causing it to not produce JSON output.
	// This is an existing bug, not related to embedded mode.
}

// TestEmbeddedInfoConcurrent exercises info operations concurrently.
func TestEmbeddedInfoConcurrent(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ix")

	for i := 0; i < 3; i++ {
		bdCreate(t, bd, dir, fmt.Sprintf("info-concurrent-%d", i), "--type", "task")
	}

	const numWorkers = 8

	type workerResult struct {
		worker int
		err    error
	}

	results := make([]workerResult, numWorkers)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(worker int) {
			defer wg.Done()
			r := workerResult{worker: worker}

			var args []string
			switch worker % 3 {
			case 0:
				args = []string{"info"}
			case 1:
				args = []string{"info", "--schema"}
			case 2:
				args = []string{"info", "--whats-new"}
			}
			cmd := exec.Command(bd, args...)
			cmd.Dir = dir
			cmd.Env = bdEnv(dir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				r.err = fmt.Errorf("info (worker %d): %v\n%s", worker, err, out)
				results[worker] = r
				return
			}

			if len(strings.TrimSpace(string(out))) == 0 {
				r.err = fmt.Errorf("info (worker %d): empty output", worker)
				results[worker] = r
				return
			}

			results[worker] = r
		}(w)
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil && !strings.Contains(r.err.Error(), "one writer at a time") {
			t.Errorf("worker %d failed: %v", r.worker, r.err)
		}
	}
}
