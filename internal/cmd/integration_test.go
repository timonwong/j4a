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

	"github.com/timonwong/jiro/internal/fieldcache"
)

func TestVersion(t *testing.T) {
	if version != "dev" {
		t.Fatalf("default version = %q, want dev", version)
	}

	originalVersion := version
	version = "1.2.3-test"
	t.Cleanup(func() { version = originalVersion })

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--version"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stdout.String() != "jiro version 1.2.3-test\n" || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

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
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "-ojson", "auth", "status"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		Data          struct {
			Profile  string `json:"profile"`
			Instance string `json:"instance"`
			User     struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1" || envelope.Data.Profile != "default" || envelope.Data.Instance != server.URL || envelope.Data.User.Username != "alice" {
		t.Fatalf("envelope = %+v", envelope)
	}
}

func TestTextTableUsesOutputTerminalMode(t *testing.T) {
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
	configPath := writeCLIConfig(t, server.URL, false)
	const wantTSV = "OPS-1\tFix login\tOpen\tAlice\n"
	const wantTTY = "KEY    SUMMARY    STATUS  ASSIGNEE\nOPS-1  Fix login  Open    Alice\n"
	tests := []struct {
		name  string
		set   bool
		value string
		want  string
	}{
		{name: "unset", want: wantTSV},
		{name: "empty", set: true, value: "", want: wantTSV},
		{name: "zero", set: true, value: "0", want: wantTSV},
		{name: "false", set: true, value: "false", want: wantTSV},
		{name: "no", set: true, value: "no", want: wantTSV},
		{name: "off", set: true, value: "off", want: wantTSV},
		{name: "one", set: true, value: "1", want: wantTTY},
		{name: "true", set: true, value: "true", want: wantTTY},
		{name: "yes", set: true, value: "yes", want: wantTTY},
		{name: "on", set: true, value: "on", want: wantTTY},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.set {
				t.Setenv("JIRO_FORCE_TTY", test.value)
			}
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := Execute([]string{"--config", configPath, "issue", "list", "--project", "OPS"}, strings.NewReader(""), stdout, stderr)
			if code != 0 || stderr.Len() != 0 || stdout.String() != test.want {
				t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, stdout.String(), stderr.String(), test.want)
			}
		})
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
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "-ojson", "auth", "status"}, strings.NewReader(""), stdout, stderr)
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

func TestCustomFieldAliasAndJFMCreate(t *testing.T) {
	clearCommandEnv(t)
	var fieldCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/rest/api/2/myself":
			_, _ = io.WriteString(writer, `{"accountId":"account-alice","name":"alice","displayName":"Alice","active":true}`)
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
	a := &app{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr, fieldStore: fieldcache.New(t.TempDir(), nil)}
	code := a.execute([]string{
		"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issue", "add",
		"--project", "OPS", "--type", "Story", "--summary", "Structured output",
		"--description", "# Header", "--input-format=jfm", "--field", "story-points=5",
	})
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
		"--config", writeCLIConfig(t, server.URL, true), "-ojson", "issue", "comment", "add", "OPS-1", "--body", "blocked",
	}, strings.NewReader(""), stdout, stderr)
	if code != 2 || calls.Load() != 0 {
		t.Fatalf("read-only code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute([]string{
		"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issue", "comment", "add", "OPS-1",
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
	code := Execute([]string{"--config", path, "-ojson", "issue", "comment", "add", "OPS-1", "--body", "blocked"}, strings.NewReader(""), stdout, stderr)
	if code != 2 || !strings.Contains(stderr.String(), "read_only") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestSchemaCommandEnvelope(t *testing.T) {
	clearCommandEnv(t)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"schema"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("schema code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		SchemaVersion string          `json:"schemaVersion"`
		Data          json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(schemaDocument())
	if err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1" || !bytes.Equal(envelope.Data, expected) {
		t.Fatalf("schema envelope version=%q data=%s want=%s", envelope.SchemaVersion, envelope.Data, expected)
	}
}

func TestMissingRequiredFlagIsInputError(t *testing.T) {
	clearCommandEnv(t)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"-ojson", "issue", "add", "--project", "OPS"}, strings.NewReader(""), stdout, stderr)
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

func TestJSONShortcutIsNotRegistered(t *testing.T) {
	a := &app{stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard}
	if flag := a.rootCommand().PersistentFlags().Lookup("json"); flag != nil {
		t.Fatalf("json flag remains registered: %+v", flag)
	}
}

func TestInvalidOutputFailsBeforeNetwork(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "--output=raw", "auth", "status"}, strings.NewReader(""), stdout, stderr)
	if code != 2 || calls.Load() != 0 {
		t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
	}
}

func TestInvalidForceTTYOnlyAffectsTextBeforeNetwork(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/rest/api/2/myself" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		_, _ = io.WriteString(writer, `{"name":"alice","displayName":"Alice","active":true}`)
	}))
	defer server.Close()
	configPath := writeCLIConfig(t, server.URL, false)

	for _, value := range []string{"sometimes", "TRUE", " true", "on "} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("JIRO_FORCE_TTY", value)
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := Execute([]string{"--config", configPath, "auth", "status"}, strings.NewReader(""), stdout, stderr)
			if code != 2 || calls.Load() != 0 || stdout.Len() != 0 || stderr.String() != "JIRO_FORCE_TTY must be a boolean\n" {
				t.Fatalf("text code=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout.String(), stderr.String())
			}
		})
	}

	t.Setenv("JIRO_FORCE_TTY", "on ")
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"--config", configPath, "-ojson", "auth", "status"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || calls.Load() != 1 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"schemaVersion":"1"`) {
		t.Fatalf("JSON code=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout.String(), stderr.String())
	}
}

func TestInvalidForceTTYDoesNotAffectHelpVersionOrSchema(t *testing.T) {
	clearCommandEnv(t)
	t.Setenv("JIRO_FORCE_TTY", "sometimes")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "help", args: []string{"--help"}, want: "Usage:"},
		{name: "version", args: []string{"--version"}, want: "jiro version"},
		{name: "schema", args: []string{"schema"}, want: `"contractVersion":"3"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := Execute(test.args, strings.NewReader(""), stdout, stderr)
			if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
		})
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
		"JIRO_CONFIG_FILE", "JIRO_CONFIG", "JIRO_PROFILE", "JIRO_READ_ONLY", "JIRO_USE_KEYRING", "JIRO_FORCE_TTY",
		"JIRA_HOST", "JIRA_API_VERSION", "JIRA_TOKEN", "JIRA_USERNAME", "JIRA_PASSWORD", "JIRA_USER_AGENT",
		"JIRO_HOST", "JIRO_API_VERSION", "JIRO_TOKEN", "JIRO_USERNAME", "JIRO_PASSWORD", "JIRO_AUTH_TYPE",
	} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}
