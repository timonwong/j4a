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
		"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issue", "add",
		"--project", "OPS", "--type", "Story", "--summary", "Structured output",
		"--description", "# Header", "--input-format=markdown", "--field", "story-points=5",
	})
	if code != 0 || stderr.Len() != 0 || fieldCalls.Load() != 1 {
		t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, fieldCalls.Load(), stdout.String(), stderr.String())
	}
}

func TestMarkdownInputConversionFailureStopsMutationBeforeJiraRequest(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	configPath := writeCLIConfig(t, server.URL, false)
	unsupportedTableCell := "| H |\n| --- |\n| ![alt](image.png) |"
	errorMessage := "convert Markdown Input: Markdown Input conversion failed at line 3, column 3 (Image): images are not supported in table cells"
	outputs := []struct {
		name string
		arg  string
		want string
	}{
		{
			name: "stable JSON envelope",
			arg:  "-ojson",
			want: `{"schemaVersion":"1","error":{"kind":"invalid_input","message":"` + errorMessage + `"}}` + "\n",
		},
		{
			name: "readable text error",
			want: errorMessage + "\n",
		},
	}

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "issue create",
			args: []string{
				"issue", "add", "--project", "OPS", "--type", "Story", "--summary", "Unsupported input",
				"--description", unsupportedTableCell, "--input-format=markdown", "--field", "story-points=5",
			},
		},
		{
			name: "issue update",
			args: []string{
				"issue", "update", "OPS-1", "--description", unsupportedTableCell, "--input-format=markdown",
				"--field", "story-points=5",
			},
		},
		{
			name: "issue comment",
			args: []string{"issue", "comment", "add", "OPS-1", "--body", unsupportedTableCell, "--input-format=markdown"},
		},
	}
	for _, test := range tests {
		for _, output := range outputs {
			t.Run(test.name+"/"+output.name, func(t *testing.T) {
				calls.Store(0)
				stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
				args := []string{"--config", configPath}
				if output.arg != "" {
					args = append(args, output.arg)
				}
				args = append(args, test.args...)
				code := Execute(args, strings.NewReader(""), stdout, stderr)
				if code != 2 || stdout.Len() != 0 || calls.Load() != 0 {
					t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
				}
				if stderr.String() != output.want {
					t.Fatalf("stderr = %q, want %q", stderr.String(), output.want)
				}
			})
		}
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

func TestSchema(t *testing.T) {
	clearCommandEnv(t)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{"schema"}, strings.NewReader(""), stdout, stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("schema code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		Data          struct {
			ContractVersion string `json:"contractVersion"`
			Output          struct {
				SchemaVersion  string `json:"schemaVersion"`
				PartialFailure string `json:"partialFailure"`
			} `json:"output"`
			ExitCodes map[string]string `json:"exitCodes"`
			Types     map[string]any    `json:"types"`
			Commands  []struct {
				Name     string         `json:"name"`
				Aliases  []string       `json:"aliases"`
				Auth     bool           `json:"auth"`
				Mutating bool           `json:"mutating"`
				JSONData map[string]any `json:"jsonData"`
				Flags    []struct {
					Name       string `json:"name"`
					Type       string `json:"type"`
					Repeatable bool   `json:"repeatable"`
					Required   bool   `json:"required"`
					Default    any    `json:"default"`
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
	if strings.Contains(stdout.String(), `"raw"`) {
		t.Fatalf("removed raw contract remains in schema: %s", stdout.String())
	}
	if envelope.Data.Output.SchemaVersion != "1" ||
		!strings.Contains(envelope.Data.Output.PartialFailure, "stdout") ||
		!strings.Contains(envelope.Data.Output.PartialFailure, "stderr") ||
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
		repeatable   bool
		required     bool
		kind         string
		defaultValue any
	})
	for _, command := range envelope.Data.Commands {
		if len(command.Aliases) != 0 {
			t.Fatalf("%s still exposes aliases %v", command.Name, command.Aliases)
		}
		commands[command.Name] = struct {
			auth, mutating bool
		}{command.Auth, command.Mutating}
		flags[command.Name] = make(map[string]struct {
			repeatable   bool
			required     bool
			kind         string
			defaultValue any
		}, len(command.Flags))
		for _, flag := range command.Flags {
			flags[command.Name][flag.Name] = struct {
				repeatable   bool
				required     bool
				kind         string
				defaultValue any
			}{
				repeatable:   flag.Repeatable,
				required:     flag.Required,
				kind:         flag.Type,
				defaultValue: flag.Default,
			}
		}
		if strings.HasPrefix(command.Name, "config") {
			t.Fatalf("removed config command remains in schema: %q", command.Name)
		}
		if command.Name == "auth status" {
			for _, field := range []string{"profile", "instance", "authType", "user"} {
				if command.JSONData[field] == nil {
					t.Fatalf("auth status JSON schema is missing %q: %#v", field, command.JSONData)
				}
			}
			for _, field := range []string{"authenticated", "credentialStore"} {
				if command.JSONData[field] != nil {
					t.Fatalf("auth status JSON schema unexpectedly includes %q: %#v", field, command.JSONData)
				}
			}
		}
		if strings.HasPrefix(command.Name, "issue bulk ") {
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
	for _, name := range []string{"auth login", "auth logout"} {
		metadata, ok := commands[name]
		if !ok || metadata.auth || !metadata.mutating {
			t.Fatalf("%s schema = %+v, present=%t", name, metadata, ok)
		}
	}
	if metadata, ok := commands["auth status"]; !ok || !metadata.auth || metadata.mutating {
		t.Fatalf("auth status schema = %+v, present=%t", metadata, ok)
	}
	for _, name := range []string{"login", "logout", "myself"} {
		if _, ok := commands[name]; ok {
			t.Fatalf("removed top-level command remains in schema: %q", name)
		}
	}
	if metadata, ok := commands["cache refresh"]; !ok || !metadata.auth || !metadata.mutating {
		t.Fatalf("cache refresh schema = %+v, present=%t", metadata, ok)
	}
	for _, command := range []struct {
		name     string
		mutating bool
		flags    []string
	}{
		{"issue list", false, []string{"resolution", "reporter", "label", "component", "fix-version", "sprint", "parent", "created", "updated"}},
		{"issue add", true, []string{"parent", "component", "fix-version", "sprint"}},
		{"issue update", true, []string{"component", "fix-version", "sprint"}},
		{"issue move", true, []string{"to"}},
		{"issue assign", true, []string{"assignee"}},
		{"issue link list", false, nil},
		{"issue link add", true, []string{"to", "type"}},
		{"issue link delete", true, nil},
		{"issue link types", false, nil},
		{"issue bulk move", true, []string{"jql", "to", "field", "dry-run", "yes"}},
		{"issue bulk assign", true, []string{"jql", "assignee", "dry-run", "yes"}},
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
	for _, command := range []string{"issue list", "issue add", "issue update"} {
		for _, name := range []string{"label", "component", "fix-version"} {
			if flag, ok := flags[command][name]; ok && !flag.repeatable {
				t.Fatalf("%s --%s is not repeatable", command, name)
			}
		}
	}
	for _, command := range []string{"issue add", "issue update", "issue comment add"} {
		inputFormat, ok := flags[command]["input-format"]
		if !ok || inputFormat.kind != "enum:jira|markdown" || inputFormat.defaultValue != "jira" {
			t.Fatalf("%s --input-format schema = %+v, present=%t", command, inputFormat, ok)
		}
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
