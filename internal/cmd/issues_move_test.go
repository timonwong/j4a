package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIssueMoveSendsAtomicCommentAndResolution(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/rest/api/2/issue/OPS-1/transitions" {
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		if request.Method == http.MethodGet {
			_, _ = io.WriteString(writer, `{"transitions":[{"id":"31","name":"Finish","to":{"id":"3","name":"Done"}}]}`)
			return
		}
		var payload struct {
			Transition map[string]string `json:"transition"`
			Fields     map[string]any    `json:"fields"`
			Update     map[string][]struct {
				Add map[string]string `json:"add"`
			} `json:"update"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		resolution, ok := payload.Fields["resolution"].(map[string]any)
		if payload.Transition["id"] != "31" || !ok || resolution["name"] != "Fixed" {
			t.Fatalf("payload = %#v", payload)
		}
		comments := payload.Update["comment"]
		if len(comments) != 1 || comments[0].Add["body"] != "h1. Deployed" {
			t.Fatalf("comment update = %#v", comments)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	code := Execute([]string{
		"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issue", "move", "OPS-1",
		"--to", "Done", "--comment", "# Deployed", "--input-format", "jfm", "--resolution", "Fixed",
	}, strings.NewReader(""), stdout, stderr)
	if code != 0 || calls.Load() != 2 || stderr.Len() != 0 {
		t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls.Load(), stdout.String(), stderr.String())
	}
	var envelope struct {
		Data struct {
			Key          string `json:"key"`
			Transitioned bool   `json:"transitioned"`
			Commented    bool   `json:"commented"`
			Resolution   string `json:"resolution"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Key != "OPS-1" || !envelope.Data.Transitioned || !envelope.Data.Commented || envelope.Data.Resolution != "Fixed" {
		t.Fatalf("data = %+v", envelope.Data)
	}
}

func TestIssueMoveResolutionForms(t *testing.T) {
	tests := []struct {
		name       string
		resolution string
		want       any
	}{
		{name: "name", resolution: "Fixed", want: map[string]any{"name": "Fixed"}},
		{name: "numeric ID", resolution: "10104", want: map[string]any{"id": "10104"}},
		{name: "none clears", resolution: "NoNe", want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearCommandEnv(t)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					_, _ = io.WriteString(writer, `{"transitions":[{"id":"31","name":"Finish","to":{"id":"3","name":"Done"}}]}`)
					return
				}
				var payload struct {
					Fields map[string]any `json:"fields"`
					Update any            `json:"update"`
				}
				if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if !equalJSONValue(payload.Fields["resolution"], test.want) || payload.Update != nil {
					t.Fatalf("payload = %#v, want resolution %#v", payload, test.want)
				}
				writer.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
			code := Execute([]string{
				"--config", writeCLIConfig(t, server.URL, false), "-ojson", "issue", "move", "OPS-1",
				"--to", "Done", "--resolution", test.resolution,
			}, strings.NewReader(""), stdout, stderr)
			if code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"commented":false`) {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			wantOutput := test.resolution
			if strings.EqualFold(wantOutput, "none") {
				wantOutput = "none"
			}
			if !strings.Contains(stdout.String(), `"resolution":"`+wantOutput+`"`) {
				t.Fatalf("stdout = %s", stdout.String())
			}
		})
	}
}

func TestIssueMoveRejectsInvalidCommentFlagsBeforeNetwork(t *testing.T) {
	clearCommandEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	configPath := writeCLIConfig(t, server.URL, false)

	for _, args := range [][]string{
		{"issue", "move", "OPS-1", "--to", "Done", "--comment", "  \t\n"},
		{"issue", "move", "OPS-1", "--to", "Done", "--input-format", "jfm"},
		{"issue", "move", "OPS-1", "--to", "Done", "--comment", "text", "--input-format", "html"},
		{"issue", "move", "OPS-1", "--to", "Done", "--resolution", "  "},
	} {
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		fullArgs := append([]string{"--config", configPath, "-ojson"}, args...)
		if code := Execute(fullArgs, strings.NewReader(""), stdout, stderr); code != 2 || stdout.Len() != 0 {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s", args, code, stdout.String(), stderr.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("network calls = %d", calls.Load())
	}
}

func TestIssueMutationFieldHelpIsExplicitlyCustomOnly(t *testing.T) {
	for _, args := range [][]string{
		{"issue", "add", "--help"},
		{"issue", "update", "--help"},
		{"issue", "move", "--help"},
		{"issue", "bulk", "move", "--help"},
	} {
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		if code := Execute(args, strings.NewReader(""), stdout, stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("args=%v code=%d stdout=%s stderr=%s", args, code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), "custom field as alias=value or customfield_N=value; repeatable") {
			t.Fatalf("args=%v help does not describe the custom-field boundary:\n%s", args, stdout.String())
		}
	}
}

func equalJSONValue(got, want any) bool {
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	return bytes.Equal(gotJSON, wantJSON)
}
