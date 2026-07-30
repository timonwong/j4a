package output

import (
	"bytes"
	"errors"
	"testing"

	"github.com/timonwong/j4a/internal/apperr"
)

func TestSuccessFormats(t *testing.T) {
	tests := []struct {
		name, format string
		quiet        bool
		data         any
		want         string
	}{
		{"json has stable envelope", "json", false, map[string]string{"key": "value"}, "{\"schemaVersion\":\"1\",\"data\":{\"key\":\"value\"}}\n"},
		{"raw does not wrap response", "raw", false, []byte("{\"jira\":true}"), "{\"jira\":true}"},
		{"text table is deterministic", "text", false, Table{Headers: []string{"KEY", "SUMMARY"}, Rows: [][]string{{"A-1", "short"}, {"LONG-22", "x"}}}, "KEY      SUMMARY\n-------  -------\nA-1      short  \nLONG-22  x      \n"},
		{"quiet suppresses text", "text", true, "done", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			format, err := ParseFormat(test.format)
			if err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if err := New(&stdout, &stderr, format, test.quiet).Success(test.data); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected stderr: %q", stderr.String())
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
			err := New(&stdout, &stderr, format, false).Error(apperr.New(apperr.KindAuth, "denied"))
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
