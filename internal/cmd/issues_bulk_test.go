package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/timonwong/j4a/internal/fieldcache"
)

func TestBulkTransitionDryRunPreflightsEveryJQLPage(t *testing.T) {
	clearCommandEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rest/api/2/search":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["jql"] != `project = OPS` {
				t.Fatalf("jql = %#v", payload["jql"])
			}
			if payload["startAt"] == nil {
				_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":50,"total":2,"issues":[{"id":"1","key":"OPS-1","fields":{"status":{"id":"1","name":"Open"}}}]}`)
			} else if payload["startAt"] == float64(1) {
				_, _ = io.WriteString(writer, `{"startAt":1,"maxResults":50,"total":2,"issues":[{"id":"2","key":"OPS-2","fields":{"status":{"id":"1","name":"Open"}}}]}`)
			} else {
				t.Fatalf("search payload = %#v", payload)
			}
		case "/rest/api/2/issue/OPS-1/transitions":
			_, _ = io.WriteString(writer, `{"transitions":[{"id":"31","name":"Finish","to":{"id":"3","name":"Done"}}]}`)
		case "/rest/api/2/issue/OPS-2/transitions":
			_, _ = io.WriteString(writer, `{"transitions":[{"id":"11","name":"Start","to":{"id":"2","name":"In Progress"}}]}`)
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", writeCLIConfig(t, server.URL, true), "-ojson", "issues", "bulk-transition",
		"--jql", "project = OPS", "--to", "Done", "--dry-run",
	}, strings.NewReader(""), stdout, stderr)
	if code != 7 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		Data struct {
			Operation    string `json:"operation"`
			DryRun       bool   `json:"dryRun"`
			Total        int    `json:"total"`
			Ready        int    `json:"ready"`
			Failed       int    `json:"failed"`
			NotAttempted int    `json:"notAttempted"`
			Items        []struct {
				IssueKey string `json:"issueKey"`
				Outcome  string `json:"outcome"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Operation != "transition" || !envelope.Data.DryRun || envelope.Data.Total != 2 || envelope.Data.Ready != 1 || envelope.Data.Failed != 1 || envelope.Data.NotAttempted != 0 {
		t.Fatalf("data = %+v", envelope.Data)
	}
	if len(envelope.Data.Items) != 2 || envelope.Data.Items[0].IssueKey != "OPS-1" || envelope.Data.Items[0].Outcome != "ready" || envelope.Data.Items[1].Outcome != "failed" {
		t.Fatalf("items = %+v", envelope.Data.Items)
	}
	if !strings.Contains(stderr.String(), `"kind":"partial_failure"`) {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestBulkTransitionStopsOnSystemicFailureAndMarksRemaining(t *testing.T) {
	clearCommandEnv(t)
	requests := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch request.URL.Path {
		case "/rest/api/2/search":
			_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":50,"total":3,"issues":[{"id":"1","key":"OPS-1","fields":{"status":{"name":"Open"}}},{"id":"2","key":"OPS-2","fields":{"status":{"name":"Open"}}},{"id":"3","key":"OPS-3","fields":{"status":{"name":"Open"}}}]}`)
		case "/rest/api/2/issue/OPS-1/transitions", "/rest/api/2/issue/OPS-2/transitions":
			if request.Method == http.MethodGet {
				_, _ = io.WriteString(writer, `{"transitions":[{"id":"31","name":"Finish","to":{"id":"3","name":"Done"}}]}`)
				return
			}
			if request.URL.Path == "/rest/api/2/issue/OPS-2/transitions" {
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(writer, `{"errorMessages":["database unavailable"]}`)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issues", "bulk-transition",
		"--jql", "project = OPS", "--to", "Done", "--yes",
	}, strings.NewReader(""), stdout, stderr)
	if code != 7 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		Data struct {
			Ready, Succeeded, Failed, NotAttempted int
			Items                                  []struct {
				IssueKey string `json:"issueKey"`
				Outcome  string `json:"outcome"`
				Error    string `json:"error"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Ready != 2 || envelope.Data.Succeeded != 1 || envelope.Data.Failed != 1 || envelope.Data.NotAttempted != 1 {
		t.Fatalf("data = %+v", envelope.Data)
	}
	if len(envelope.Data.Items) != 3 || envelope.Data.Items[2].IssueKey != "OPS-3" || envelope.Data.Items[2].Outcome != "not_attempted" || !strings.Contains(envelope.Data.Items[2].Error, "database unavailable") {
		t.Fatalf("items = %+v", envelope.Data.Items)
	}
	for _, request := range requests {
		if strings.Contains(request, "OPS-3/transitions") {
			t.Fatalf("OPS-3 should not have been attempted: %v", requests)
		}
	}
}

func TestBulkAssignResolvesMeOnceAndContinuesAfterIssueFailure(t *testing.T) {
	clearCommandEnv(t)
	myselfCalls := 0
	assignOrder := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rest/api/2/myself":
			myselfCalls++
			_, _ = io.WriteString(writer, `{"name":"alice","displayName":"Alice","active":true}`)
		case "/rest/api/2/search":
			_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":50,"total":2,"issues":[{"id":"1","key":"OPS-1","fields":{"assignee":{"name":"bob","displayName":"Bob"}}},{"id":"2","key":"OPS-2","fields":{"assignee":null}}]}`)
		case "/rest/api/2/issue/OPS-1/assignee", "/rest/api/2/issue/OPS-2/assignee":
			assignOrder = append(assignOrder, request.URL.Path)
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["name"] != "alice" {
				t.Fatalf("payload = %#v", payload)
			}
			if strings.Contains(request.URL.Path, "OPS-1") {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(writer, `{"errorMessages":["user cannot be assigned"]}`)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issues", "bulk-assign",
		"--jql", "project = OPS", "--assignee", "me", "--yes",
	}, strings.NewReader(""), stdout, stderr)
	if code != 7 || myselfCalls != 1 {
		t.Fatalf("code=%d myself=%d stdout=%s stderr=%s", code, myselfCalls, stdout.String(), stderr.String())
	}
	if len(assignOrder) != 2 || !strings.Contains(assignOrder[0], "OPS-1") || !strings.Contains(assignOrder[1], "OPS-2") {
		t.Fatalf("assign order = %v", assignOrder)
	}
	var envelope struct {
		Data struct {
			Total, Ready, Succeeded, Failed, NotAttempted int
			Items                                         []struct {
				Outcome string `json:"outcome"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Total != 2 || envelope.Data.Ready != 2 || envelope.Data.Succeeded != 1 || envelope.Data.Failed != 1 || envelope.Data.NotAttempted != 0 {
		t.Fatalf("data = %+v", envelope.Data)
	}
	if len(envelope.Data.Items) != 2 || envelope.Data.Items[0].Outcome != "failed" || envelope.Data.Items[1].Outcome != "succeeded" {
		t.Fatalf("items = %+v", envelope.Data.Items)
	}
}

func TestBulkAssignNoMatchesIsSuccessfulWithoutUserLookup(t *testing.T) {
	clearCommandEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/rest/api/2/search" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":50,"total":0,"issues":[]}`)
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", writeCLIConfig(t, server.URL, true), "-ojson", "issues", "bulk-assign",
		"--jql", "project = EMPTY", "--assignee", "me", "--dry-run",
	}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"total":0`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestBulkTransitionNoMatchesSkipsFieldAliasResolution(t *testing.T) {
	clearCommandEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/rest/api/2/search" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":50,"total":0,"issues":[]}`)
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", writeCLIConfig(t, server.URL, true), "-ojson", "issues", "bulk-transition",
		"--jql", "project = EMPTY", "--to", "Done", "--field", "unknown-alias=1", "--dry-run",
	}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"total":0`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestBulkCommandGuardsRunBeforeNetworkOrCredentialResolution(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	configPath := writeCLIConfig(t, server.URL, false)

	tests := [][]string{
		{"--config", configPath, "--raw", "issues", "bulk-assign", "--jql", "project = OPS", "--assignee", "alice", "--dry-run"},
		{"--config", configPath, "issues", "bulk-transition", "--jql", "project = OPS", "--to", "Done"},
		{"--config", configPath, "issues", "bulk-transition", "--jql", "project = OPS", "--to", "Done", "--dry-run", "--yes"},
		{"--config", configPath, "--raw", "issues", "create", "--project", "OPS", "--type", "Task", "--summary", "No raw composite", "--sprint", "42"},
	}
	for _, args := range tests {
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		if code := Execute(args, strings.NewReader(""), stdout, stderr); code != 2 {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s", args, code, stdout.String(), stderr.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("network calls = %d", calls.Load())
	}

	readOnlyPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(readOnlyPath, []byte("[default]\nhost = \"https://jira.invalid\"\nusername = \"alice\"\nread_only = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", readOnlyPath, "issues", "bulk-assign", "--jql", "project = OPS", "--assignee", "alice", "--yes",
	}, strings.NewReader(""), stdout, stderr)
	if code != 2 || !strings.Contains(stderr.String(), "read_only") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestBulkTransitionResolvesCommonFieldAliasesOnce(t *testing.T) {
	clearCommandEnv(t)
	var fieldCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rest/api/2/myself":
			_, _ = io.WriteString(writer, `{"accountId":"account-alice","name":"alice","displayName":"Alice","active":true}`)
		case "/rest/api/2/field":
			fieldCalls.Add(1)
			_, _ = io.WriteString(writer, `[{"id":"customfield_10006","name":"Story Points","custom":true,"schema":{"type":"number"}}]`)
		case "/rest/api/2/search":
			_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":50,"total":2,"issues":[{"id":"1","key":"OPS-1","fields":{"status":{"name":"Open"}}},{"id":"2","key":"OPS-2","fields":{"status":{"name":"Open"}}}]}`)
		case "/rest/api/2/issue/OPS-1/transitions", "/rest/api/2/issue/OPS-2/transitions":
			_, _ = io.WriteString(writer, `{"transitions":[{"id":"31","name":"Finish","to":{"id":"3","name":"Done"}}]}`)
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	a := &app{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr, fieldStore: fieldcache.New(t.TempDir(), nil)}
	code := a.execute([]string{
		"--config", writeCLIConfig(t, server.URL, true), "-ojson", "issues", "bulk-transition",
		"--jql", "project = OPS", "--to", "Done", "--field", "story-points=5", "--dry-run",
	})
	if code != 0 || stderr.Len() != 0 || fieldCalls.Load() != 1 {
		t.Fatalf("code=%d fieldCalls=%d stdout=%s stderr=%s", code, fieldCalls.Load(), stdout.String(), stderr.String())
	}
}
