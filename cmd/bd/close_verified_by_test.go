package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const closeVerifiedByHelperEnv = "BD_CLOSE_VERIFIED_BY_TEST_ARGS"

// TestCloseVerifiedByCLI exercises the real command entry point in subprocesses so
// fatal CLI validation paths can be asserted without mutating a non-test database.
func TestCloseVerifiedByCLI(t *testing.T) {
	if encodedArgs := os.Getenv(closeVerifiedByHelperEnv); encodedArgs != "" {
		var args []string
		if err := json.Unmarshal([]byte(encodedArgs), &args); err != nil {
			t.Fatalf("decode helper args: %v", err)
		}
		os.Args = append([]string{"bd"}, args...)
		main()
		return
	}

	dir := t.TempDir()
	runCloseVerifiedByCLI(t, dir, true, "init", "--prefix", "test", "--quiet")

	createOut := runCloseVerifiedByCLI(t, dir, true, "create", "Verified closure", "--json")
	id := closeVerifiedByIssueID(t, createOut)
	proof := "go test ./cmd/bd/... -run Close -count=1 -> PASS"
	runCloseVerifiedByCLI(t, dir, true, "close", id, "--verified-by", proof)

	showText := runCloseVerifiedByCLI(t, dir, true, "show", id)
	if !strings.Contains(showText, "verified-by: "+proof) {
		t.Fatalf("show text does not contain verification proof:\n%s", showText)
	}

	showJSON := runCloseVerifiedByCLI(t, dir, true, "show", id, "--json", "--include-comments")
	issue := closeVerifiedByShownIssue(t, showJSON)
	if issue["status"] != "closed" {
		t.Fatalf("status = %v, want closed", issue["status"])
	}
	comments, ok := issue["comments"].([]interface{})
	if !ok || len(comments) != 1 {
		t.Fatalf("comments = %#v, want one structured verification comment", issue["comments"])
	}
	comment, ok := comments[0].(map[string]interface{})
	if !ok || comment["text"] != "verified-by: "+proof {
		t.Fatalf("verification comment = %#v", comments[0])
	}

	createOut = runCloseVerifiedByCLI(t, dir, true, "create", "Unverified closure", "--json")
	unverifiedID := closeVerifiedByIssueID(t, createOut)
	runCloseVerifiedByCLI(t, dir, true, "close", unverifiedID)
	issue = closeVerifiedByShownIssue(t, runCloseVerifiedByCLI(t, dir, true, "show", unverifiedID, "--json"))
	if issue["status"] != "closed" {
		t.Fatalf("close without --verified-by status = %v, want closed", issue["status"])
	}
	if comments, ok := issue["comments"].([]interface{}); ok && len(comments) != 0 {
		t.Fatalf("close without --verified-by added comments: %#v", comments)
	}

	createOut = runCloseVerifiedByCLI(t, dir, true, "create", "Empty verification", "--json")
	emptyID := closeVerifiedByIssueID(t, createOut)
	emptyOut := runCloseVerifiedByCLI(t, dir, false, "close", emptyID, "--verified-by", "")
	if !strings.Contains(emptyOut, "--verified-by cannot be empty") {
		t.Fatalf("empty --verified-by error is unclear:\n%s", emptyOut)
	}
	issue = closeVerifiedByShownIssue(t, runCloseVerifiedByCLI(t, dir, true, "show", emptyID, "--json"))
	if issue["status"] != "open" {
		t.Fatalf("empty --verified-by changed status to %v, want open", issue["status"])
	}
}

func runCloseVerifiedByCLI(t *testing.T, dir string, wantSuccess bool, args ...string) string {
	t.Helper()

	encodedArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encode helper args: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCloseVerifiedByCLI$")
	cmd.Dir = dir
	beadsDir := filepath.Join(dir, ".beads")
	dbPath := filepath.Join(beadsDir, "beads.db")
	cmd.Env = append(os.Environ(),
		closeVerifiedByHelperEnv+"="+string(encodedArgs),
		"BEADS_NO_DAEMON=1",
		"BEADS_TEST_MODE=1",
		"BEADS_DIR="+beadsDir,
		"BEADS_DB="+dbPath,
		"BD_DB="+dbPath,
		"BD_ACTOR=test-user",
		"BEADS_ACTOR=test-user",
	)
	out, err := cmd.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("bd %v failed: %v\n%s", args, err, out)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("bd %v succeeded, want failure\n%s", args, out)
	}
	return string(out)
}

func closeVerifiedByIssueID(t *testing.T, output string) string {
	t.Helper()

	start := strings.Index(output, "{")
	if start < 0 {
		t.Fatalf("create output does not contain JSON: %s", output)
	}
	var issue map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(output[start:])).Decode(&issue); err != nil {
		t.Fatalf("parse create output: %v\n%s", err, output)
	}
	id, _ := issue["id"].(string)
	if id == "" {
		t.Fatalf("create output has no issue id: %s", output)
	}
	return id
}

func closeVerifiedByShownIssue(t *testing.T, output string) map[string]interface{} {
	t.Helper()

	start := strings.Index(output, "[")
	if start < 0 {
		t.Fatalf("show output does not contain JSON: %s", output)
	}
	var issues []map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(output[start:])).Decode(&issues); err != nil {
		t.Fatalf("parse show output: %v\n%s", err, output)
	}
	if len(issues) != 1 {
		t.Fatalf("show returned %d issues, want one", len(issues))
	}
	return issues[0]
}
