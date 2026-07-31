// Package markup translates explicit Markdown Input into Jira Markup.
package markup

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// InputFormat selects whether a text value is already Jira Markup or needs
// Markdown Input conversion.
type InputFormat string

const (
	// JiraMarkup preserves the value exactly. This is the default input format.
	JiraMarkup InputFormat = "jira"
	// Markdown converts CommonMark-compatible Markdown Input to Jira Markup.
	Markdown InputFormat = "markdown"
)

// ConversionError describes Markdown Input that cannot be represented safely.
type ConversionError struct {
	Line     int
	Column   int
	NodeType string
	Reason   string
}

func (e *ConversionError) Error() string {
	return fmt.Sprintf("Markdown Input conversion failed at line %d, column %d (%s): %s", e.Line, e.Column, e.NodeType, e.Reason)
}

// ToJira returns the complete Jira Markup result or an error. Failed
// conversions never return partial output.
func ToJira(input string, format InputFormat) (string, error) {
	switch format {
	case JiraMarkup:
		return input, nil
	case Markdown:
		if input == "" {
			return "", nil
		}
	default:
		return "", fmt.Errorf("unknown input format %q", format)
	}

	source := []byte(input)
	parser := goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))
	document := parser.Parser().Parse(text.NewReader(source))
	result, err := jiraMarkupRenderer{source: source}.renderDocument(document)
	if err != nil {
		return "", err
	}
	return result, nil
}

type jiraMarkupRenderer struct {
	source []byte
}

func (r jiraMarkupRenderer) renderDocument(document ast.Node) (string, error) {
	blocks := make([]string, 0, document.ChildCount())
	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		block, err := r.renderBlock(node)
		if err != nil {
			return "", err
		}
		blocks = append(blocks, block)
	}
	return strings.Join(blocks, "\n\n"), nil
}

func (r jiraMarkupRenderer) renderBlock(node ast.Node) (string, error) {
	switch typed := node.(type) {
	case *ast.Paragraph:
		return r.renderInlines(typed, true)
	case *ast.Heading:
		content, err := r.renderInlines(typed, false)
		if err != nil {
			return "", err
		}
		heading := fmt.Sprintf("h%d.", typed.Level)
		if content != "" {
			heading += " " + content
		}
		return heading, nil
	case *ast.List:
		return r.renderSimpleList(typed)
	case *ast.ThematicBreak:
		return "----", nil
	case *ast.HTMLBlock:
		return escapeUserTextForJiraMarkup(r.htmlBlockSource(typed), true), nil
	default:
		return "", r.unsupported(node)
	}
}

func (r jiraMarkupRenderer) renderSimpleList(list *ast.List) (string, error) {
	if !list.IsTight {
		return "", r.unsupported(list)
	}
	marker := "*"
	if list.IsOrdered() {
		marker = "#"
	}
	items := make([]string, 0, list.ChildCount())
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*ast.ListItem)
		if !ok || item.ChildCount() != 1 {
			return "", r.unsupported(list)
		}
		textBlock, ok := item.FirstChild().(*ast.TextBlock)
		if !ok {
			return "", r.unsupported(list)
		}
		content, err := r.renderInlines(textBlock, false)
		if err != nil {
			return "", err
		}
		itemMarkup := marker
		if content != "" {
			itemMarkup += " " + content
		}
		items = append(items, itemMarkup)
	}
	return strings.Join(items, "\n"), nil
}

func (r jiraMarkupRenderer) renderInlines(parent ast.Node, atLineStart bool) (string, error) {
	var result strings.Builder
	for node := parent.FirstChild(); node != nil; node = node.NextSibling() {
		content, err := r.renderInline(node, atLineStart)
		if err != nil {
			return "", err
		}
		result.WriteString(content)
		if content != "" {
			atLineStart = strings.HasSuffix(content, "\n")
		}
	}
	return result.String(), nil
}

func (r jiraMarkupRenderer) renderInline(node ast.Node, atLineStart bool) (string, error) {
	switch typed := node.(type) {
	case *ast.Text:
		value := typed.Segment.Value(r.source)
		if !typed.IsRaw() {
			value = util.UnescapePunctuations(value)
			value = util.ResolveNumericReferences(value)
			value = util.ResolveEntityNames(value)
		}
		content := escapeUserTextForJiraMarkup(value, atLineStart)
		if typed.HardLineBreak() {
			return content + "\\\\\n", nil
		}
		if typed.SoftLineBreak() {
			return content + " ", nil
		}
		return content, nil
	case *ast.CodeSpan:
		content, err := r.renderCodeSpan(typed)
		if err != nil {
			return "", err
		}
		return "{{" + content + "}}", nil
	case *ast.Emphasis:
		delimiter := "_"
		if typed.Level == 2 {
			delimiter = "*"
		}
		return r.renderDelimitedInline(typed, delimiter)
	case *extensionast.Strikethrough:
		return r.renderDelimitedInline(typed, "-")
	case *ast.RawHTML:
		return escapeUserTextForJiraMarkup(typed.Segments.Value(r.source), atLineStart), nil
	default:
		return "", r.unsupported(node)
	}
}

func (r jiraMarkupRenderer) htmlBlockSource(block *ast.HTMLBlock) []byte {
	var result []byte
	for index := range block.Lines().Len() {
		segment := block.Lines().At(index)
		result = append(result, segment.Value(r.source)...)
	}
	if block.HasClosure() {
		result = append(result, block.ClosureLine.Value(r.source)...)
	}
	return bytes.TrimSpace(result)
}

func escapeUserTextForJiraMarkup(value []byte, atLineStart bool) string {
	var result strings.Builder
	remaining := string(value)
	for remaining != "" {
		if atLineStart {
			if prefixLength := jiraLineControlPrefixLength(remaining); prefixLength != 0 {
				result.WriteString(remaining[:prefixLength-1])
				result.WriteString(`\.`)
				remaining = remaining[prefixLength:]
				atLineStart = false
				continue
			}
		}
		character, size := utf8.DecodeRuneInString(remaining)
		if strings.ContainsRune(`\{}[]!*?_-+^~|#`, character) {
			result.WriteByte('\\')
		}
		result.WriteRune(character)
		remaining = remaining[size:]
		atLineStart = character == '\n'
	}
	return result.String()
}

func jiraLineControlPrefixLength(value string) int {
	if len(value) >= 3 && value[0] == 'h' && value[1] >= '1' && value[1] <= '6' && value[2] == '.' &&
		(len(value) == 3 || value[3] == ' ') {
		return 3
	}
	if strings.HasPrefix(value, "bq.") && (len(value) == 3 || value[3] == ' ') {
		return 3
	}
	return 0
}

func (r jiraMarkupRenderer) renderDelimitedInline(node ast.Node, delimiter string) (string, error) {
	content, err := r.renderInlines(node, false)
	if err != nil {
		return "", err
	}
	return delimiter + content + delimiter, nil
}

func (r jiraMarkupRenderer) renderCodeSpan(code *ast.CodeSpan) (string, error) {
	var result strings.Builder
	for child := code.FirstChild(); child != nil; child = child.NextSibling() {
		content, ok := child.(*ast.Text)
		if !ok {
			return "", r.unsupported(code)
		}
		value := content.Segment.Value(r.source)
		if bytes.HasSuffix(value, []byte("\n")) {
			result.Write(value[:len(value)-1])
			result.WriteByte(' ')
		} else {
			result.Write(value)
		}
	}
	return result.String(), nil
}

func (r jiraMarkupRenderer) unsupported(node ast.Node) *ConversionError {
	line, column := sourcePosition(r.source, node)
	return &ConversionError{
		Line:     line,
		Column:   column,
		NodeType: node.Kind().String(),
		Reason:   "unsupported Markdown Input syntax",
	}
}

func sourcePosition(source []byte, node ast.Node) (int, int) {
	position := node.Pos()
	if position < 0 {
		position = firstAuthoredPosition(node)
		if position >= 0 && node.Type() == ast.TypeBlock {
			position = lineStart(source, position)
		}
	}
	if position < 0 {
		position = 0
	}
	start := lineStart(source, position)
	return bytes.Count(source[:start], []byte("\n")) + 1, utf8.RuneCount(source[start:position]) + 1
}

func firstAuthoredPosition(node ast.Node) int {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if position := child.Pos(); position >= 0 {
			return position
		}
		if position := firstAuthoredPosition(child); position >= 0 {
			return position
		}
	}
	return -1
}

func lineStart(source []byte, position int) int {
	if position > len(source) {
		position = len(source)
	}
	if index := bytes.LastIndexByte(source[:position], '\n'); index >= 0 {
		return index + 1
	}
	return 0
}
