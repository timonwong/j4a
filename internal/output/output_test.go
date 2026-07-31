package output

import (
	"bytes"
	"errors"
	"testing"

	"github.com/timonwong/jiro/internal/apperr"
)

func TestSuccessFormats(t *testing.T) {
	tests := []struct {
		name, format string
		quiet        bool
		data         any
		warnings     []Warning
		want         string
		wantStderr   string
	}{
		{"json has stable envelope", "json", false, map[string]string{"key": "value"}, nil, "{\"schemaVersion\":\"1\",\"data\":{\"key\":\"value\"}}\n", ""},
		{"json includes warnings", "json", false, map[string]string{"key": "value"}, []Warning{{Code: "partial", Message: "some results were omitted", Details: map[string]any{"limit": 100}}}, "{\"schemaVersion\":\"1\",\"data\":{\"key\":\"value\"},\"warnings\":[{\"code\":\"partial\",\"message\":\"some results were omitted\",\"details\":{\"limit\":100}}]}\n", ""},
		{"raw does not wrap response", "raw", false, []byte("{\"jira\":true}"), []Warning{{Code: "ignored", Message: "ignored"}}, "{\"jira\":true}", ""},
		{"text table is deterministic", "text", false, Table{Headers: []string{"KEY", "SUMMARY"}, Rows: [][]string{{"A-1", "short"}, {"LONG-22", "x"}}}, nil, "KEY      SUMMARY\n-------  -------\nA-1      short  \nLONG-22  x      \n", ""},
		{"text writes warnings to stderr", "text", false, "done", []Warning{{Code: "partial", Message: "some results were omitted"}}, "done\n", "warning: some results were omitted\n"},
		{"quiet suppresses text but not warnings", "text", true, "done", []Warning{{Code: "partial", Message: "some results were omitted"}}, "", "warning: some results were omitted\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			format, err := ParseFormat(test.format)
			if err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if err := New(&stdout, &stderr, format, test.quiet).WithWarnings(test.warnings...).Success(test.data); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.want)
			}
			if stderr.String() != test.wantStderr {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.wantStderr)
			}
		})
	}
}

func TestErrorsAndValidation(t *testing.T) {
	tests := []struct{ name, format, want string }{
		{"json errors go to stderr", "json", "{\"schemaVersion\":\"1\",\"error\":{\"kind\":\"auth\",\"message\":\"denied\"}}\n"},
		{"text errors go to stderr", "text", "denied\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			format, _ := ParseFormat(test.format)
			var stdout, stderr bytes.Buffer
			err := New(&stdout, &stderr, format, false).WithWarnings(Warning{Code: "partial", Message: "ignored"}).Error(apperr.New(apperr.KindAuth, "denied"))
			if err != nil {
				t.Fatal(err)
			}
			if stdout.Len() != 0 || stderr.String() != test.want {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
	if _, err := ParseFormat("yaml"); apperr.As(err).Kind != apperr.KindInvalidInput {
		t.Fatalf("ParseFormat error = %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := New(&stdout, &stderr, FormatText, false).Raw([]byte("x")); !errors.Is(err, apperr.As(err)) || apperr.As(err).Kind != apperr.KindInvalidInput {
		t.Fatalf("Raw error = %v", err)
	}
}
