package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIssueShowTextUsesManStyleSectionsWithoutANSIOnNonTTY(t *testing.T) {
	clearCommandEnv(t)
	code, stdout, stderr := runIssueShowPresentation(t, map[string]any{
		"summary":   "Ship release",
		"status":    map[string]any{"id": "1", "name": "Open"},
		"issuetype": map[string]any{"id": "2", "name": "Task"},
		"priority":  map[string]any{"id": "3", "name": "High"},
		"assignee":  map[string]any{"name": "alice", "displayName": "Alice Example"},
		"reporter":  map[string]any{"name": "bob", "displayName": "Bob Example"},
		"labels":    []string{"release", "urgent"},
		"parent":    map[string]any{"id": "1", "key": "OPS-1"},
		"components": []map[string]any{
			{"id": "1", "name": "CLI"},
			{"id": "2", "name": "API"},
		},
		"fixVersions": []map[string]any{
			{"id": "1", "name": "4.4"},
			{"id": "2", "name": "4.5"},
		},
		"description": "Ready to ship",
	})
	const want = "KEY\n" +
		"    OPS-42\n\n" +
		"SUMMARY\n" +
		"    Ship release\n\n" +
		"STATUS\n" +
		"    Open\n\n" +
		"TYPE\n" +
		"    Task\n\n" +
		"PRIORITY\n" +
		"    High\n\n" +
		"ASSIGNEE\n" +
		"    Alice Example\n\n" +
		"REPORTER\n" +
		"    Bob Example\n\n" +
		"LABELS\n" +
		"    release, urgent\n\n" +
		"PARENT\n" +
		"    OPS-1\n\n" +
		"COMPONENTS\n" +
		"    CLI, API\n\n" +
		"FIX VERSIONS\n" +
		"    4.4, 4.5\n\n" +
		"DESCRIPTION\n" +
		"    Ready to ship\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, stdout, stderr, want)
	}
	if strings.Contains(stdout, "\x1b[") {
		t.Fatalf("non-TTY output contains ANSI: %q", stdout)
	}
}

func TestIssueShowTextUsesANSIBoldHeadersWhenTTYIsForced(t *testing.T) {
	clearCommandEnv(t)
	t.Setenv("JIRO_FORCE_TTY", "on")
	code, stdout, stderr := runIssueShowPresentation(t, map[string]any{"summary": "Ship release"})
	const want = "\x1b[1mKEY\x1b[0m\n" +
		"    OPS-42\n\n" +
		"\x1b[1mSUMMARY\x1b[0m\n" +
		"    Ship release\n\n" +
		"\x1b[1mSTATUS\x1b[0m\n\n" +
		"\x1b[1mTYPE\x1b[0m\n\n" +
		"\x1b[1mPRIORITY\x1b[0m\n\n" +
		"\x1b[1mASSIGNEE\x1b[0m\n\n" +
		"\x1b[1mREPORTER\x1b[0m\n\n" +
		"\x1b[1mLABELS\x1b[0m\n\n" +
		"\x1b[1mPARENT\x1b[0m\n\n" +
		"\x1b[1mCOMPONENTS\x1b[0m\n\n" +
		"\x1b[1mFIX VERSIONS\x1b[0m\n\n" +
		"\x1b[1mDESCRIPTION\x1b[0m\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, stdout, stderr, want)
	}
}

func TestIssueShowTextPreservesPhysicalLinesWhileSanitizingValues(t *testing.T) {
	clearCommandEnv(t)
	code, stdout, stderr := runIssueShowPresentation(t, map[string]any{
		"summary":     "one\ttwo\r\nthree\rfour \x1b[31mred\x1b[0m\x00\u202e\u200b",
		"description": "first\r\n\r\nsecond\tthird",
	})
	const want = "KEY\n" +
		"    OPS-42\n\n" +
		"SUMMARY\n" +
		"    one two\n" +
		"    three\n" +
		"    four red\n\n" +
		"STATUS\n\n" +
		"TYPE\n\n" +
		"PRIORITY\n\n" +
		"ASSIGNEE\n\n" +
		"REPORTER\n\n" +
		"LABELS\n\n" +
		"PARENT\n\n" +
		"COMPONENTS\n\n" +
		"FIX VERSIONS\n\n" +
		"DESCRIPTION\n" +
		"    first\n\n" +
		"    second third\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, stdout, stderr, want)
	}
}

func TestIssueShowTextPreservesTrailingBlankPhysicalLines(t *testing.T) {
	clearCommandEnv(t)
	code, stdout, stderr := runIssueShowPresentation(t, map[string]any{
		"summary":     "first\r\n\r\n\r\n",
		"description": "final value",
	})
	const want = "KEY\n" +
		"    OPS-42\n\n" +
		"SUMMARY\n" +
		"    first\n\n\n\n" +
		"STATUS\n\n" +
		"TYPE\n\n" +
		"PRIORITY\n\n" +
		"ASSIGNEE\n\n" +
		"REPORTER\n\n" +
		"LABELS\n\n" +
		"PARENT\n\n" +
		"COMPONENTS\n\n" +
		"FIX VERSIONS\n\n" +
		"DESCRIPTION\n" +
		"    final value\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, stdout, stderr, want)
	}
}

func TestIssueShowTextIncludesEmptySections(t *testing.T) {
	clearCommandEnv(t)
	code, stdout, stderr := runIssueShowPresentation(t, map[string]any{})
	const want = "KEY\n" +
		"    OPS-42\n\n" +
		"SUMMARY\n\n" +
		"STATUS\n\n" +
		"TYPE\n\n" +
		"PRIORITY\n\n" +
		"ASSIGNEE\n\n" +
		"REPORTER\n\n" +
		"LABELS\n\n" +
		"PARENT\n\n" +
		"COMPONENTS\n\n" +
		"FIX VERSIONS\n\n" +
		"DESCRIPTION\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, stdout, stderr, want)
	}
}

func TestIssueShowJSONRemainsNormalizedIssue(t *testing.T) {
	clearCommandEnv(t)
	code, stdout, stderr := runIssueShowPresentation(t, map[string]any{
		"summary":     "Ship release",
		"description": "Ready to ship",
	}, "-ojson")
	const want = "{\"schemaVersion\":\"1\",\"data\":{\"id\":\"42\",\"key\":\"OPS-42\",\"summary\":\"Ship release\",\"description\":\"Ready to ship\",\"fields\":{\"description\":\"Ready to ship\",\"summary\":\"Ship release\"}}}\n"
	if code != 0 || stderr != "" || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q want=%q", code, stdout, stderr, want)
	}
}

func runIssueShowPresentation(t *testing.T, fields map[string]any, extraArgs ...string) (int, string, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/rest/api/2/issue/OPS-42" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if err := json.NewEncoder(writer).Encode(map[string]any{
			"id":     "42",
			"key":    "OPS-42",
			"fields": fields,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	args := []string{"--config", writeCLIConfig(t, server.URL, false)}
	args = append(args, extraArgs...)
	args = append(args, "issue", "show", "OPS-42")
	code := Execute(args, strings.NewReader(""), stdout, stderr)
	return code, stdout.String(), stderr.String()
}
