//go:build cgo

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProxiedServerInfo(t *testing.T) {
	requireSharedProxiedServer(t)
	t.Parallel()
	bd := buildEmbeddedBD(t)
	p := newSharedProxiedProject(t, bd, "inf")
	seedProxiedListData(t, bd, p)

	infoJSON := func(t *testing.T, args ...string) map[string]interface{} {
		t.Helper()
		out, err := bdProxiedRun(t, bd, p.dir, append([]string{"info", "--json"}, args...)...)
		if err != nil {
			t.Fatalf("info %v: %v\n%s", args, err, out)
		}
		var got map[string]interface{}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("unmarshal info: %v\n%s", err, out)
		}
		return got
	}

	infoText := func(t *testing.T, args ...string) string {
		t.Helper()
		out, err := bdProxiedRun(t, bd, p.dir, append([]string{"info"}, args...)...)
		if err != nil {
			t.Fatalf("info %v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	// ===== Default output (JSON: mode, count, config) =====

	t.Run("mode_and_count", func(t *testing.T) {
		got := infoJSON(t)
		if got["mode"] != "proxied-server" {
			t.Errorf("mode = %v, want proxied-server", got["mode"])
		}
		count, ok := got["issue_count"].(float64)
		if !ok {
			t.Fatalf("issue_count missing/not a number: %v", got["issue_count"])
		}
		want := len(bdProxiedListJSON(t, bd, p, "--all"))
		if int(count) != want {
			t.Errorf("issue_count = %d, want %d (list --all)", int(count), want)
		}
		if _, ok := got["config"].(map[string]interface{}); !ok {
			t.Errorf("expected config map in info output, got %v", got["config"])
		}
	})

	t.Run("info_count_excludes_wisps_and_matches_status", func(t *testing.T) {
		bdProxiedCreate(t, bd, p.dir, "Info durable count probe", "--type", "task")
		bdProxiedCreate(t, bd, p.dir, "Info ephemeral count probe", "--ephemeral")

		info := infoJSON(t)
		infoCount, ok := info["issue_count"].(float64)
		if !ok {
			t.Fatalf("info issue_count missing/not numeric: %v", info["issue_count"])
		}
		out, err := bdProxiedRun(t, bd, p.dir, "status", "--json")
		if err != nil {
			t.Fatalf("status --json: %v\n%s", err, out)
		}
		var status map[string]interface{}
		if err := json.Unmarshal(out, &status); err != nil {
			t.Fatalf("unmarshal status: %v\n%s", err, out)
		}
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

	// ===== Default output (human-readable) =====

	t.Run("info_default_text", func(t *testing.T) {
		out := infoText(t)
		if !strings.Contains(out, "Issue Count") {
			t.Errorf("expected 'Issue Count' in info output: %s", out)
		}
		if !strings.Contains(out, "Mode: proxied-server") {
			t.Errorf("expected proxied-server mode line in info output: %s", out)
		}
	})

	// ===== Non-zero issue count with seeded data =====

	t.Run("info_with_issues", func(t *testing.T) {
		got := infoJSON(t)
		count, _ := got["issue_count"].(float64)
		if int(count) == 0 {
			t.Errorf("expected non-zero issue count with seeded data: %v", got["issue_count"])
		}
		out := infoText(t)
		if strings.Contains(out, "Issue Count: 0") {
			t.Errorf("expected non-zero issue count in text output: %s", out)
		}
	})

	// ===== --schema (JSON) =====

	t.Run("schema", func(t *testing.T) {
		got := infoJSON(t, "--schema")
		schema, ok := got["schema"].(map[string]interface{})
		if !ok {
			t.Fatalf("schema block missing: %v", got["schema"])
		}
		tables, ok := schema["tables"].([]interface{})
		if !ok {
			t.Fatalf("schema tables missing: %v", schema["tables"])
		}
		foundIssues := false
		for _, tbl := range tables {
			if tbl == "issues" {
				foundIssues = true
			}
		}
		if !foundIssues {
			t.Errorf("expected 'issues' table in schema tables, got %v", tables)
		}
		if v, _ := schema["schema_version"].(string); v == "" || v == "unknown" {
			t.Errorf("schema_version = %q, want a resolved bd_version", v)
		}
		samples, ok := schema["sample_issue_ids"].([]interface{})
		if !ok || len(samples) == 0 {
			t.Errorf("expected sample_issue_ids, got %v", schema["sample_issue_ids"])
		}
		if pfx, _ := schema["detected_prefix"].(string); pfx == "" {
			t.Errorf("expected a detected_prefix, got empty")
		}
	})

	// ===== --schema (human-readable) =====

	t.Run("schema_text", func(t *testing.T) {
		out := infoText(t, "--schema")
		if !strings.Contains(out, "issues") {
			t.Errorf("expected 'issues' table in schema output: %s", out)
		}
		if !strings.Contains(out, "Schema") {
			t.Errorf("expected 'Schema' heading in schema output: %s", out)
		}
	})

	// ===== --whats-new (static, version-independent of storage mode) =====

	t.Run("whats_new_text", func(t *testing.T) {
		out := infoText(t, "--whats-new")
		if len(strings.TrimSpace(out)) == 0 {
			t.Error("expected non-empty --whats-new output")
		}
		if !strings.Contains(out, "v0.") {
			t.Errorf("expected a version string in whats-new output: %s", out[:min(200, len(out))])
		}
	})

	t.Run("whats_new_json", func(t *testing.T) {
		got := infoJSON(t, "--whats-new")
		if _, ok := got["current_version"]; !ok {
			t.Errorf("expected 'current_version' in whats-new JSON: %v", got)
		}
		if _, ok := got["recent_changes"]; !ok {
			t.Errorf("expected 'recent_changes' in whats-new JSON: %v", got)
		}
	})

	// ===== --thanks (static) =====

	t.Run("thanks", func(t *testing.T) {
		out := infoText(t, "--thanks")
		if len(strings.TrimSpace(out)) == 0 {
			t.Error("expected non-empty --thanks output")
		}
	})
}
