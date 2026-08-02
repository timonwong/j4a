// Package output renders jiro command results.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/timonwong/jiro/internal/apperr"
)

// Format selects the result representation.
type Format string

const (
	FormatText           Format = "text"
	FormatJSON           Format = "json"
	defaultTerminalWidth        = 80

	// SchemaVersion identifies the stable, public JSON envelope version.
	SchemaVersion = "1"
)

// ParseFormat validates a command-line output value. The empty value is text.
func ParseFormat(value string) (Format, error) {
	if value == "" {
		return FormatText, nil
	}
	format := Format(strings.ToLower(value))
	if format != FormatText && format != FormatJSON {
		return "", apperr.New(apperr.KindInvalidInput, "output must be text or json")
	}
	return format, nil
}

// Renderer writes command results. Quiet suppresses successful text only;
// structured output and errors remain available to scripts.
type Renderer struct {
	Stdout   io.Writer
	Stderr   io.Writer
	Format   Format
	Quiet    bool
	terminal Terminal
	warnings []Warning
}

// Terminal describes the output terminal capabilities used for text tables.
type Terminal struct {
	IsTTY bool
	Width int
}

// Warning describes a non-fatal condition encountered while producing a
// successful result.
type Warning struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// New creates a renderer. Nil streams are discarded, which is useful in tests.
func New(stdout, stderr io.Writer, format Format, quiet bool) Renderer {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if format == "" {
		format = FormatText
	}
	return Renderer{Stdout: stdout, Stderr: stderr, Format: format, Quiet: quiet}
}

// WithWarnings returns a copy of r that includes warnings with its successful
// output. Warnings are emitted in JSON and text modes.
func (r Renderer) WithWarnings(warnings ...Warning) Renderer {
	r.warnings = append([]Warning(nil), warnings...)
	return r
}

// WithTerminal returns a copy of r configured for terminal-aware tables.
func (r Renderer) WithTerminal(terminal Terminal) Renderer {
	r.terminal = terminal
	return r
}

// IsTTY reports the terminal decision already applied to this renderer.
func (r Renderer) IsTTY() bool {
	return r.terminal.IsTTY
}

type successEnvelope struct {
	SchemaVersion string    `json:"schemaVersion"`
	Data          any       `json:"data"`
	Warnings      []Warning `json:"warnings,omitempty"`
}

type errorEnvelope struct {
	SchemaVersion string    `json:"schemaVersion"`
	Error         errorBody `json:"error"`
}

type errorBody struct {
	Kind       apperr.Kind `json:"kind"`
	Message    string      `json:"message"`
	StatusCode int         `json:"statusCode,omitempty"`
}

// Success renders a normalized response. Text callers should pass a string or
// Table; JSON always uses the stable versioned envelope.
func (r Renderer) Success(data any) error {
	switch r.Format {
	case FormatJSON:
		return writeJSON(r.Stdout, successEnvelope{SchemaVersion: SchemaVersion, Data: data, Warnings: r.warnings})
	case FormatText, "":
		for _, warning := range r.warnings {
			if _, err := fmt.Fprintf(r.Stderr, "warning: %s\n", warning.Message); err != nil {
				return err
			}
		}
		if r.Quiet {
			return nil
		}
		switch value := data.(type) {
		case Table:
			return r.Table(value)
		case *Table:
			if value == nil {
				return nil
			}
			return r.Table(*value)
		case string:
			_, err := fmt.Fprintln(r.Stdout, value)
			return err
		case []byte:
			_, err := r.Stdout.Write(value)
			return err
		default:
			_, err := fmt.Fprintln(r.Stdout, value)
			return err
		}
	default:
		return apperr.New(apperr.KindInvalidInput, "output must be text or json")
	}
}

// Error writes a stable error object to stderr in JSON mode and human-readable
// text otherwise. The returned error is only an I/O failure, not the input.
func (r Renderer) Error(err error) error {
	if err == nil {
		return nil
	}
	app := apperr.As(err)
	if r.Format == FormatJSON {
		return writeJSON(r.Stderr, errorEnvelope{SchemaVersion: SchemaVersion, Error: errorBody{Kind: app.Kind, Message: app.Error(), StatusCode: app.StatusCode}})
	}
	_, writeErr := fmt.Fprintln(r.Stderr, app.Error())
	return writeErr
}

// Column describes one explicitly fixed or flexible text-table column.
type Column struct {
	header string
	fixed  bool
}

// Fixed creates a column whose values are never truncated.
func Fixed(header string) Column {
	return Column{header: header, fixed: true}
}

// Flexible creates a column whose values may be truncated to the viewport.
func Flexible(header string) Column {
	return Column{header: header}
}

// Table describes column-oriented human-readable output.
type Table struct {
	Columns []Column
	Rows    [][]string
}

// Table writes a terminal-aware table or non-terminal TSV to stdout.
func (r Renderer) Table(table Table) error {
	if len(table.Columns) == 0 {
		return fmt.Errorf("table requires at least one column")
	}
	for i, column := range table.Columns {
		if strings.TrimSpace(sanitizeCell(column.header)) == "" {
			return fmt.Errorf("table column %d header must not be blank", i)
		}
	}
	for i, row := range table.Rows {
		if len(row) != len(table.Columns) {
			return fmt.Errorf("table row %d has %d cells, want %d", i, len(row), len(table.Columns))
		}
	}
	if r.Quiet {
		return nil
	}
	headers := make([]string, len(table.Columns))
	for i, column := range table.Columns {
		headers[i] = sanitizeCell(column.header)
	}
	width := r.terminal.Width
	if r.terminal.IsTTY && width <= 0 {
		width = defaultTerminalWidth
	}
	tracked := &trackingWriter{writer: r.Stdout}
	printer := tableprinter.New(tracked, r.terminal.IsTTY, width)
	addField := func(column Column, cell string) {
		if column.fixed {
			printer.AddField(cell, tableprinter.WithTruncate(nil))
		} else {
			printer.AddField(cell)
		}
	}
	if r.terminal.IsTTY {
		for i, header := range headers {
			addField(table.Columns[i], header)
		}
		printer.EndRow()
	} else {
		printer.AddHeader(headers)
	}
	for _, row := range table.Rows {
		for i, cell := range row {
			cell = sanitizeCell(cell)
			addField(table.Columns[i], cell)
		}
		printer.EndRow()
	}
	if err := printer.Render(); err != nil {
		return err
	}
	return tracked.err
}

type trackingWriter struct {
	writer io.Writer
	err    error
}

func (w *trackingWriter) Write(value []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	written, err := w.writer.Write(value)
	if err != nil {
		w.err = err
	}
	return written, err
}

func sanitizeCell(value string) string {
	value = strings.ReplaceAll(ansi.Strip(value), "\r\n", "\n")
	var result strings.Builder
	result.Grow(len(value))
	for _, char := range value {
		switch char {
		case '\r', '\n', '\t':
			result.WriteByte(' ')
		default:
			if !unicode.IsControl(char) && !unicode.Is(unicode.Cf, char) {
				result.WriteRune(char)
			}
		}
	}
	return result.String()
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
