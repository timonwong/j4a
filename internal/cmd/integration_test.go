package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestJSONOutputAndBasicAuth(t *testing.T) {
	clearCommandEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "alice" || password != "secret" {
			t.Fatalf("BasicAuth = %q/%q/%t", username, password, ok)
		}
		if request.URL.Path != "/rest/api/2/myself" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"name":"alice","displayName":"Alice","emailAddress":"alice@example.com","active":true}`)
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "-ojson", "myself"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		Data          struct {
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1" || envelope.Data.Username != "alice" {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestTextDefaultAndSingularAlias(t *testing.T) {
	clearCommandEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/rest/api/2/search" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(fmt.Sprint(payload["jql"]), `project = "OPS"`) {
			t.Fatalf("jql = %v", payload["jql"])
		}
		_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":50,"total":1,"issues":[{"id":"1","key":"OPS-1","fields":{"summary":"Fix login","status":{"id":"1","name":"Open"},"assignee":{"name":"alice","displayName":"Alice"}}}]}`)
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "issue", "list", "--project", "OPS"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "OPS-1") || strings.Contains(stdout.String(), "schemaVersion") {
		t.Fatalf("unexpected text output: %s", stdout.String())
	}
}

func TestJSONErrorUsesStderrAndExitCode(t *testing.T) {
	clearCommandEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{"errorMessages":["bad credentials"]}`)
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "--json", "myself"}, strings.NewReader(""), stdout, stderr)
	if code != 3 || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Kind != "auth" {
		t.Fatalf("error = %+v", envelope.Error)
	}
}

func TestCustomFieldAliasAndMarkdownCreate(t *testing.T) {
	clearCommandEnv(t)
	var fieldCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rest/api/2/field":
			fieldCalls.Add(1)
			_, _ = io.WriteString(writer, `[{"id":"customfield_10006","name":"Story Points","custom":true,"schema":{"type":"number"}}]`)
		case "/rest/api/2/issue":
			var payload struct {
				Fields map[string]any `json:"fields"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if value := payload.Fields["customfield_10006"]; value != float64(5) {
				t.Fatalf("story points = %#v", value)
			}
			if description := fmt.Sprint(payload.Fields["description"]); !strings.Contains(description, "h1. Header") {
				t.Fatalf("description = %q", description)
			}
			_, _ = io.WriteString(writer, `{"id":"1","key":"OPS-1"}`)
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", writeCLIConfig(t, server.URL, false), "--json", "issues", "create",
		"--project", "OPS", "--type", "Story", "--summary", "Structured output",
		"--description", "# Header", "--input-format=markdown", "--field", "story-points=5",
	}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 || fieldCalls.Load() != 1 {
		t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, fieldCalls.Load(), stdout.String(), stderr.String())
	}
}

func TestReadOnlyBlocksMutationAndInputConflict(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", writeCLIConfig(t, server.URL, true), "--json", "issues", "comment", "OPS-1", "--body", "blocked",
	}, strings.NewReader(""), stdout, stderr)
	if code != 2 || calls.Load() != 0 {
		t.Fatalf("read-only code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute([]string{
		"--config", writeCLIConfig(t, server.URL, false), "--json", "issues", "comment", "OPS-1",
		"--body", "inline", "--body-file", "-",
	}, strings.NewReader("file"), stdout, stderr)
	if code != 2 || calls.Load() != 0 {
		t.Fatalf("conflict code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
	}
}

func TestReadOnlyBlocksBeforeCredentialResolution(t *testing.T) {
	clearCommandEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[default]\nhost = \"https://jira.invalid\"\nusername = \"alice\"\nread_only = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--config", path, "--json", "issues", "comment", "OPS-1", "--body", "blocked"}, strings.NewReader(""), stdout, stderr)
	if code != 2 || !strings.Contains(stderr.String(), "read_only") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestRawAndSchema(t *testing.T) {
	clearCommandEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"raw":true}`)
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "--raw", "myself"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stdout.String() != `{"raw":true}` || stderr.Len() != 0 {
		t.Fatalf("raw code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute([]string{"schema"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("schema code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		Data          struct {
			ContractVersion string `json:"contractVersion"`
			Commands        []struct {
				Name     string `json:"name"`
				Auth     bool   `json:"auth"`
				Mutating bool   `json:"mutating"`
			} `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1" || envelope.Data.ContractVersion != "2" || len(envelope.Data.Commands) == 0 {
		t.Fatalf("schema = %+v", envelope)
	}
	commands := make(map[string]struct {
		auth, mutating bool
	})
	for _, command := range envelope.Data.Commands {
		commands[command.Name] = struct {
			auth, mutating bool
		}{command.Auth, command.Mutating}
		if strings.HasPrefix(command.Name, "config") {
			t.Fatalf("removed config command remains in schema: %q", command.Name)
		}
	}
	for _, name := range []string{"login", "logout"} {
		metadata, ok := commands[name]
		if !ok || metadata.auth || !metadata.mutating {
			t.Fatalf("%s schema = %+v, present=%t", name, metadata, ok)
		}
	}
}

func TestMissingRequiredFlagIsInputError(t *testing.T) {
	clearCommandEnv(t)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--json", "issues", "create", "--project", "OPS"}, strings.NewReader(""), stdout, stderr)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		Error struct {
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Kind != "invalid_input" {
		t.Fatalf("error = %+v", envelope.Error)
	}
}

func TestInvalidOutputFailsBeforeNetwork(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "--output=raw", "myself"}, strings.NewReader(""), stdout, stderr)
	if code != 2 || calls.Load() != 0 {
		t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
	}
}

func writeCLIConfig(t *testing.T, host string, readOnly bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	body := fmt.Sprintf("[default]\nhost = %q\nusername = \"alice\"\npassword = \"secret\"\nread_only = %t\n", host, readOnly)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func clearCommandEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"J4A_CONFIG_FILE", "J4A_CONFIG", "J4A_PROFILE", "J4A_HOST", "J4A_USERNAME", "J4A_AUTH_TYPE",
		"J4A_API_VERSION", "J4A_READ_ONLY", "J4A_USE_KEYRING", "J4A_PASSWORD", "J4A_TOKEN",
	} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}
