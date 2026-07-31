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
)

type textMutationCommand struct {
	name        string
	method      string
	path        string
	args        []string
	inlineFlag  string
	fileFlag    string
	payloadPath []string
	statusCode  int
	response    string
}

type capturedMutationRequest struct {
	method  string
	path    string
	payload map[string]any
	err     error
}

func TestMarkdownCapableMutationsShareTheTextInputContract(t *testing.T) {
	formats := []struct {
		name   string
		input  string
		format string
		want   string
	}{
		{
			name:   "Markdown Input is converted",
			input:  "# Release\n\n- **done**\n- [docs](https://example.com)",
			format: "markdown",
			want:   "h1. Release\n\n* *done*\n* [docs|https://example.com]",
		},
		{
			name:  "Jira Markup is the byte-preserving default",
			input: "{panel:title=Keep}\r\n* raw _Jira_ markup *\n{panel}\n",
			want:  "{panel:title=Keep}\r\n* raw _Jira_ markup *\n{panel}\n",
		},
	}
	sources := []string{"inline", "file", "stdin"}
	commands := []textMutationCommand{
		{
			name:        "issue create",
			method:      http.MethodPost,
			path:        "/rest/api/2/issue",
			args:        []string{"issue", "add", "--project", "OPS", "--type", "Story", "--summary", "Markup contract"},
			inlineFlag:  "--description",
			fileFlag:    "--description-file",
			payloadPath: []string{"fields", "description"},
			statusCode:  http.StatusOK,
			response:    `{"id":"1","key":"OPS-1"}`,
		},
		{
			name:        "issue update",
			method:      http.MethodPut,
			path:        "/rest/api/2/issue/OPS-1",
			args:        []string{"issue", "update", "OPS-1"},
			inlineFlag:  "--description",
			fileFlag:    "--description-file",
			payloadPath: []string{"fields", "description"},
			statusCode:  http.StatusNoContent,
		},
		{
			name:        "issue comment",
			method:      http.MethodPost,
			path:        "/rest/api/2/issue/OPS-1/comment",
			args:        []string{"issue", "comment", "add", "OPS-1"},
			inlineFlag:  "--body",
			fileFlag:    "--body-file",
			payloadPath: []string{"body"},
			statusCode:  http.StatusOK,
			response:    `{"id":"7","body":"accepted"}`,
		},
	}

	for _, format := range formats {
		for _, command := range commands {
			for _, source := range sources {
				t.Run(format.name+"/"+command.name+"/"+source, func(t *testing.T) {
					got := executeTextMutation(t, command, source, format.input, format.format)
					if got != format.want {
						t.Fatalf("request text = %q, want %q", got, format.want)
					}
				})
			}
		}
	}
}

func executeTextMutation(t *testing.T, command textMutationCommand, source, input, format string) string {
	t.Helper()
	clearCommandEnv(t)

	var calls atomic.Int32
	captured := make(chan capturedMutationRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var payload map[string]any
		err := json.NewDecoder(request.Body).Decode(&payload)
		captured <- capturedMutationRequest{method: request.Method, path: request.URL.Path, payload: payload, err: err}
		writer.WriteHeader(command.statusCode)
		if command.response != "" {
			_, _ = io.WriteString(writer, command.response)
		}
	}))
	defer server.Close()

	args := append([]string{"--config", writeCLIConfig(t, server.URL, false), "-ojson"}, command.args...)
	stdin := strings.NewReader("")
	switch source {
	case "inline":
		args = append(args, command.inlineFlag, input)
	case "file":
		path := filepath.Join(t.TempDir(), "input.txt")
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, command.fileFlag, path)
	case "stdin":
		stdin = strings.NewReader(input)
		args = append(args, command.fileFlag, "-")
	default:
		t.Fatalf("unknown source %q", source)
	}
	if format != "" {
		args = append(args, "--input-format="+format)
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute(args, stdin, stdout, stderr)
	if code != 0 || stderr.Len() != 0 || calls.Load() != 1 {
		t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
	}
	request := <-captured
	if request.err != nil {
		t.Fatal(request.err)
	}
	if request.method != command.method || request.path != command.path {
		t.Fatalf("request = %s %s, want %s %s", request.method, request.path, command.method, command.path)
	}
	value, ok := stringAtJSONPath(request.payload, command.payloadPath...)
	if !ok {
		t.Fatalf("payload path %v is not a string: %#v", command.payloadPath, request.payload)
	}
	return value
}

func stringAtJSONPath(value map[string]any, path ...string) (string, bool) {
	var current any = value
	for _, element := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = object[element]
		if !ok {
			return "", false
		}
	}
	result, ok := current.(string)
	return result, ok
}
