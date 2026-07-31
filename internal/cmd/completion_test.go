package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCompletionScripts(t *testing.T) {
	clearCommandEnv(t)
	tests := map[string]string{
		"bash":       "__start_j4a",
		"zsh":        "#compdef j4a",
		"fish":       "complete -c j4a",
		"powershell": "Register-ArgumentCompleter",
	}
	for shell, marker := range tests {
		t.Run(shell, func(t *testing.T) {
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := Execute([]string{"completion", shell}, strings.NewReader(""), stdout, stderr)
			if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), marker) {
				t.Fatalf("code=%d stdout=%q stderr=%q; want marker %q", code, stdout.String(), stderr.String(), marker)
			}
		})
	}
}

func TestCompletionValues(t *testing.T) {
	clearCommandEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`
[default]
host = "https://jira.example"

[profiles.zeta]
host = "https://zeta.example"

[profiles.alpha]
host = "https://alpha.example"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"output", []string{"__complete", "--output", ""}, []string{"text\tHuman-readable output", "json\tVersioned machine-readable output", ":4"}},
		{"auth type", []string{"__complete", "--auth-type", ""}, []string{"basic\tUsername and password authentication", "pat\tPersonal Access Token authentication", ":4"}},
		{"profiles", []string{"__complete", "--config", path, "--profile", ""}, []string{"alpha", "zeta", ":4"}},
		{"input format", []string{"__complete", "issues", "create", "--input-format", ""}, []string{"jira\tJira wiki markup", "markdown\tConvert Markdown to Jira markup", ":4"}},
		{"assignee", []string{"__complete", "issues", "list", "--assignee", ""}, []string{"me\tUse Jira currentUser()", ":4"}},
		{"issue list fixed filters", []string{"__complete", "issues", "list", "--sprint", ""}, []string{"active\tOpen sprints", "open\tOpen sprints", "closed\tClosed sprints", "future\tFuture sprints", ":4"}},
		{"issue list resolution", []string{"__complete", "issues", "list", "--resolution", ""}, []string{"unresolved\tIssues without a resolution", ":4"}},
		{"issue list reporter", []string{"__complete", "issues", "list", "--reporter", ""}, []string{"me\tUse Jira currentUser()", ":4"}},
		{"issue create sprint", []string{"__complete", "issues", "create", "--sprint", ""}, []string{"active\tOpen sprint", ":4"}},
		{"issue assign", []string{"__complete", "issues", "assign", "OPS-1", "--assignee", ""}, []string{"me\tAssign to the current Jira user", "none\tClear the assignee", ":4"}},
		{"issue move", []string{"__complete", "issues", "move", "OPS-1", "--sprint", ""}, []string{"active\tOpen sprint", ":4"}},
		{"issue update clear named fields", []string{"__complete", "issues", "update", "OPS-1", "--component", ""}, []string{"none\tClear all components", ":4"}},
		{"issue update clear fix versions", []string{"__complete", "issues", "update", "OPS-1", "--fix-version", ""}, []string{"none\tClear all fix versions", ":4"}},
		{"bulk assign", []string{"__complete", "issues", "bulk-assign", "--assignee", ""}, []string{"me\tAssign to the current Jira user", "none\tClear the assignee", ":4"}},
		{"file or stdin", []string{"__complete", "issues", "comment", "OPS-1", "--body-file", ""}, []string{"-\tRead from stdin", ":0"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := Execute(test.args, strings.NewReader(""), stdout, stderr)
			if code != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			for _, value := range test.want {
				if !strings.Contains(stdout.String(), value) {
					t.Fatalf("stdout=%q stderr=%q; missing %q", stdout.String(), stderr.String(), value)
				}
			}
		})
	}
}

func TestProfileCompletionDoesNotUseNetworkOrKeyring(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config.toml")
	body := fmt.Sprintf("[default]\nhost = %q\nuse_keyring = true\n\n[profiles.bot]\nhost = %q\nuse_keyring = true\n", server.URL, server.URL)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	a := &app{
		stdin: strings.NewReader(""), stdout: stdout, stderr: stderr,
		secretStore: failingSecretStore{},
	}
	code := a.execute([]string{"__complete", "--config", path, "--profile", ""})
	if code != 0 || calls.Load() != 0 || !strings.Contains(stdout.String(), "bot") {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if err := os.WriteFile(path, []byte("invalid = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	code = a.execute([]string{"__complete", "--config", path, "--profile", ""})
	if code != 0 || calls.Load() != 0 || !strings.Contains(stdout.String(), ":4") {
		t.Fatalf("invalid config code=%d calls=%d stdout=%q stderr=%q", code, calls.Load(), stdout.String(), stderr.String())
	}
}

type failingSecretStore struct{}

func (failingSecretStore) Get(string, string) (string, error) {
	return "", fmt.Errorf("keyring accessed")
}
func (failingSecretStore) Set(string, string, string) error { return fmt.Errorf("keyring accessed") }
func (failingSecretStore) Delete(string, string) error      { return fmt.Errorf("keyring accessed") }
