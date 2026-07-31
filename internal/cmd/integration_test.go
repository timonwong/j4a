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
	code := Execute([]string{"--config", writeCLIConfig(t, server.URL, false), "-ojson", "myself"}, strings.NewReader(""), stdout, stderr)
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
		"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issues", "create",
		"--project", "OPS", "--type", "Story", "--summary", "Structured output",
		"--description", "# Header", "--input-format=markdown", "--field", "story-points=5",
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
		"--config", writeCLIConfig(t, server.URL, true), "-ojson", "issues", "comment", "OPS-1", "--body", "blocked",
	}, strings.NewReader(""), stdout, stderr)
	if code != 2 || calls.Load() != 0 {
		t.Fatalf("read-only code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute([]string{
		"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issues", "comment", "OPS-1",
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
	code := Execute([]string{"--config", path, "-ojson", "issues", "comment", "OPS-1", "--body", "blocked"}, strings.NewReader(""), stdout, stderr)
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
			Output          struct {
				SchemaVersion   string `json:"schemaVersion"`
				PartialFailure  string `json:"partialFailure"`
				RawRestrictions string `json:"rawRestrictions"`
			} `json:"output"`
			ExitCodes map[string]string `json:"exitCodes"`
			Types     map[string]any    `json:"types"`
			Commands  []struct {
				Name     string         `json:"name"`
				Auth     bool           `json:"auth"`
				Mutating bool           `json:"mutating"`
				JSONData map[string]any `json:"jsonData"`
				Flags    []struct {
					Name       string `json:"name"`
					Repeatable bool   `json:"repeatable"`
					Required   bool   `json:"required"`
				} `json:"flags"`
			} `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1" || envelope.Data.ContractVersion != "3" || len(envelope.Data.Commands) == 0 {
		t.Fatalf("schema = %+v", envelope)
	}
	if envelope.Data.Output.SchemaVersion != "1" ||
		!strings.Contains(envelope.Data.Output.PartialFailure, "stdout") ||
		!strings.Contains(envelope.Data.Output.PartialFailure, "stderr") ||
		!strings.Contains(envelope.Data.Output.RawRestrictions, "bulk-transition") ||
		envelope.Data.ExitCodes["7"] != "partial failure" {
		t.Fatalf("output contract = %+v exitCodes=%+v", envelope.Data.Output, envelope.Data.ExitCodes)
	}
	for _, name := range []string{"Version", "Sprint", "IssueLink", "IssueLinkType", "BatchResult", "BatchItem"} {
		if envelope.Data.Types[name] == nil {
			t.Fatalf("schema type %s is missing: %#v", name, envelope.Data.Types)
		}
	}
	commands := make(map[string]struct {
		auth, mutating bool
	})
	flags := make(map[string]map[string]struct {
		repeatable bool
		required   bool
	})
	for _, command := range envelope.Data.Commands {
		commands[command.Name] = struct {
			auth, mutating bool
		}{command.Auth, command.Mutating}
		flags[command.Name] = make(map[string]struct {
			repeatable bool
			required   bool
		}, len(command.Flags))
		for _, flag := range command.Flags {
			flags[command.Name][flag.Name] = struct {
				repeatable bool
				required   bool
			}{flag.Repeatable, flag.Required}
		}
		if strings.HasPrefix(command.Name, "config") {
			t.Fatalf("removed config command remains in schema: %q", command.Name)
		}
		if strings.HasPrefix(command.Name, "issues bulk-") {
			items, ok := command.JSONData["items"].([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("%s items schema = %#v", command.Name, command.JSONData["items"])
			}
			item, ok := items[0].(map[string]any)
			if !ok || item["current"] == nil || item["target"] == nil || item["outcome"] == nil {
				t.Fatalf("%s item schema = %#v", command.Name, items[0])
			}
		}
	}
	for _, name := range []string{"login", "logout"} {
		metadata, ok := commands[name]
		if !ok || metadata.auth || !metadata.mutating {
			t.Fatalf("%s schema = %+v, present=%t", name, metadata, ok)
		}
	}
	if metadata, ok := commands["cache fields refresh"]; !ok || !metadata.auth || !metadata.mutating {
		t.Fatalf("cache fields refresh schema = %+v, present=%t", metadata, ok)
	}
	for _, command := range []struct {
		name     string
		mutating bool
		flags    []string
	}{
		{"issues list", false, []string{"resolution", "reporter", "label", "component", "fix-version", "sprint", "parent", "created", "updated"}},
		{"issues create", true, []string{"parent", "component", "fix-version", "sprint"}},
		{"issues update", true, []string{"component", "fix-version"}},
		{"issues move", true, []string{"sprint"}},
		{"issues assign", true, []string{"assignee"}},
		{"issues links", false, nil},
		{"issues link", true, []string{"to", "type"}},
		{"issues unlink", true, nil},
		{"issues link-types", false, nil},
		{"issues bulk-transition", true, []string{"jql", "to", "field", "dry-run", "yes"}},
		{"issues bulk-assign", true, []string{"jql", "assignee", "dry-run", "yes"}},
	} {
		metadata, ok := commands[command.name]
		if !ok || !metadata.auth || metadata.mutating != command.mutating {
			t.Fatalf("%s schema = %+v, present=%t", command.name, metadata, ok)
		}
		for _, name := range command.flags {
			if _, ok := flags[command.name][name]; !ok {
				t.Fatalf("%s is missing --%s", command.name, name)
			}
		}
	}
	for _, command := range []string{"issues list", "issues create", "issues update"} {
		for _, name := range []string{"label", "component", "fix-version"} {
			if flag, ok := flags[command][name]; ok && !flag.repeatable {
				t.Fatalf("%s --%s is not repeatable", command, name)
			}
		}
	}
}

func TestMissingRequiredFlagIsInputError(t *testing.T) {
	clearCommandEnv(t)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"-ojson", "issues", "create", "--project", "OPS"}, strings.NewReader(""), stdout, stderr)
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
		"JIRO_CONFIG_FILE", "JIRO_CONFIG", "JIRO_PROFILE", "JIRO_HOST", "JIRO_USERNAME", "JIRO_AUTH_TYPE",
		"JIRO_API_VERSION", "JIRO_READ_ONLY", "JIRO_USE_KEYRING", "JIRO_PASSWORD", "JIRO_TOKEN",
	} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}
