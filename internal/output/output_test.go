package output

import (
	"bytes"
	"errors"
	"strings"
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
		{"text table is TSV when stdout is not a TTY", "text", false, Table{Columns: []Column{Fixed("KEY"), Flexible("SUMMARY")}, Rows: [][]string{{"A-1", "short\nvalue"}, {"LONG-22", "x"}}}, nil, "A-1\tshort value\nLONG-22\tx\n", ""},
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
	for _, value := range []string{"yaml", "raw"} {
		if _, err := ParseFormat(value); apperr.As(err).Kind != apperr.KindInvalidInput {
			t.Fatalf("ParseFormat(%q) error = %v", value, err)
		}
	}
}

func TestTableRendersSafeSingleLineTTYOutput(t *testing.T) {
	var stdout bytes.Buffer
	renderer := New(&stdout, nil, FormatText, false).WithTerminal(Terminal{IsTTY: true, Width: 80})
	table := Table{
		Columns: []Column{Fixed("KEY"), Flexible("SUMMARY")},
		Rows:    [][]string{{"OPS-1", "修复\t登录\r\n\x1b[31murgent\x1b[0m\x00"}},
	}

	if err := renderer.Table(table); err != nil {
		t.Fatal(err)
	}
	const want = "KEY    SUMMARY\nOPS-1  修复 登录 urgent\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestTableTruncatesOnlyFlexibleTTYColumns(t *testing.T) {
	var stdout bytes.Buffer
	renderer := New(&stdout, nil, FormatText, false).WithTerminal(Terminal{IsTTY: true, Width: 14})
	table := Table{
		Columns: []Column{Fixed("ID"), Flexible("SUMMARY")},
		Rows:    [][]string{{"9", "hello"}, {"10", "long description"}},
	}

	if err := renderer.Table(table); err != nil {
		t.Fatal(err)
	}
	const want = "ID  SUMMARY\n9   hello\n10  long de...\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestFixedTTYColumnsMayOverflowTheViewport(t *testing.T) {
	var stdout bytes.Buffer
	renderer := New(&stdout, nil, FormatText, false).WithTerminal(Terminal{IsTTY: true, Width: 8})
	table := Table{
		Columns: []Column{Fixed("ISSUE"), Flexible("SUMMARY")},
		Rows:    [][]string{{"OPS-123", "hidden"}},
	}

	if err := renderer.Table(table); err != nil {
		t.Fatal(err)
	}
	const want = "ISSUE    \nOPS-123  \n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestTTYTableFallsBackToEightyColumns(t *testing.T) {
	var stdout bytes.Buffer
	renderer := New(&stdout, nil, FormatText, false).WithTerminal(Terminal{IsTTY: true})
	table := Table{
		Columns: []Column{Fixed("ID"), Flexible("SUMMARY")},
		Rows: [][]string{{"10",
			"01234567890123456789012345678901234567890123456789012345678901234567890123456789"}},
	}

	if err := renderer.Table(table); err != nil {
		t.Fatal(err)
	}
	const want = "ID  SUMMARY\n10  0123456789012345678901234567890123456789012345678901234567890123456789012...\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestEmptyTableUsesNativeTerminalMode(t *testing.T) {
	table := Table{Columns: []Column{Fixed("KEY"), Flexible("SUMMARY")}}
	tests := []struct {
		name     string
		terminal Terminal
		want     string
	}{
		{name: "TTY keeps the header", terminal: Terminal{IsTTY: true, Width: 80}, want: "KEY  SUMMARY\n"},
		{name: "non-TTY emits no rows", terminal: Terminal{}, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := New(&stdout, nil, FormatText, false).WithTerminal(test.terminal).Table(table); err != nil {
				t.Fatal(err)
			}
			if stdout.String() != test.want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.want)
			}
		})
	}
}

func TestTableRejectsMissingColumns(t *testing.T) {
	err := New(nil, nil, FormatText, false).Table(Table{})
	if err == nil || !strings.Contains(err.Error(), "at least one column") {
		t.Fatalf("error = %v", err)
	}
}

func TestTableRejectsBlankColumnHeader(t *testing.T) {
	err := New(nil, nil, FormatText, false).Table(Table{Columns: []Column{Fixed(" \t")}})
	if err == nil || !strings.Contains(err.Error(), "column 0 header must not be blank") {
		t.Fatalf("error = %v", err)
	}
}

func TestTableRejectsNonRectangularRowsBeforeWriting(t *testing.T) {
	for _, row := range [][][]string{{{"OPS-1"}}, {{"OPS-1", "Open", "extra"}}} {
		var stdout bytes.Buffer
		err := New(&stdout, nil, FormatText, false).Table(Table{
			Columns: []Column{Fixed("KEY"), Flexible("STATUS")},
			Rows:    row,
		})
		if err == nil || !strings.Contains(err.Error(), "row 0 has") {
			t.Fatalf("row=%v error=%v", row, err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("row=%v stdout=%q", row, stdout.String())
		}
	}
}

func TestTableReturnsNonTTYWriteError(t *testing.T) {
	want := errors.New("write failed")
	err := New(errorWriter{err: want}, nil, FormatText, false).Table(Table{
		Columns: []Column{Fixed("KEY")},
		Rows:    [][]string{{"OPS-1"}},
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
