// Package output renders stable j4a command results.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/timonwong/j4a/internal/apperr"
)

// Format selects the result representation.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatRaw  Format = "raw"

	// SchemaVersion identifies the stable, public JSON envelope version.
	SchemaVersion = "1"
)

// ParseFormat validates a command-line output value. The empty value is text.
func ParseFormat(value string) (Format, error) {
	if value == "" {
		return FormatText, nil
	}
	format := Format(strings.ToLower(value))
	if format != FormatText && format != FormatJSON && format != FormatRaw {
		return "", apperr.New(apperr.KindInvalidInput, "output must be text, json, or raw")
	}
	return format, nil
}

// Renderer writes command results. Quiet suppresses successful text only;
// structured output and errors remain available to scripts.
type Renderer struct {
	Stdout io.Writer
	Stderr io.Writer
	Format Format
	Quiet  bool
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

type successEnvelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Data          any    `json:"data"`
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
		return writeJSON(r.Stdout, successEnvelope{SchemaVersion: SchemaVersion, Data: data})
	case FormatRaw:
		return r.Raw(data)
	case FormatText, "":
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
		return apperr.New(apperr.KindInvalidInput, "output must be text, json, or raw")
	}
}

// Raw writes an unmodified Jira response. It is only valid for raw output.
func (r Renderer) Raw(data any) error {
	if r.Format != FormatRaw {
		return apperr.New(apperr.KindInvalidInput, "raw output requires format raw")
	}
	switch value := data.(type) {
	case []byte:
		_, err := r.Stdout.Write(value)
		return err
	case json.RawMessage:
		_, err := r.Stdout.Write(value)
		return err
	case string:
		_, err := io.WriteString(r.Stdout, value)
		return err
	default:
		return apperr.New(apperr.KindInvalidInput, "raw output requires bytes, string, or json.RawMessage")
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

// Table is a deliberately simple deterministic text table.
type Table struct {
	Headers []string
	Rows    [][]string
}

// Table writes a deterministic padded table to stdout.
func (r Renderer) Table(table Table) error {
	if r.Quiet {
		return nil
	}
	columns := len(table.Headers)
	for _, row := range table.Rows {
		if len(row) > columns {
			columns = len(row)
		}
	}
	if columns == 0 {
		return nil
	}
	widths := make([]int, columns)
	consider := func(row []string) {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	consider(table.Headers)
	for _, row := range table.Rows {
		consider(row)
	}
	var buffer bytes.Buffer
	writeRow := func(row []string) {
		for i := 0; i < columns; i++ {
			if i > 0 {
				buffer.WriteString("  ")
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			buffer.WriteString(cell)
			if padding := widths[i] - len(cell); padding > 0 {
				buffer.WriteString(strings.Repeat(" ", padding))
			}
		}
		buffer.WriteByte('\n')
	}
	if len(table.Headers) > 0 {
		writeRow(table.Headers)
		separator := make([]string, columns)
		for i := range separator {
			separator[i] = strings.Repeat("-", widths[i])
		}
		writeRow(separator)
	}
	for _, row := range table.Rows {
		writeRow(row)
	}
	_, err := r.Stdout.Write(buffer.Bytes())
	return err
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
