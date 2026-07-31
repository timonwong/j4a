// Package markup translates explicit Markdown Input into Jira Markup.
package markup

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
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
	markdownParser := goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough),
		goldmark.WithParserOptions(parser.WithBlockParsers(
			util.Prioritized(jiraListParser{BlockParser: parser.NewListParser()}, 299),
		)),
	)
	document := markdownParser.Parser().Parse(text.NewReader(source))
	result, err := jiraMarkupRenderer{source: source}.renderDocument(document)
	if err != nil {
		return "", err
	}
	return result, nil
}

type jiraMarkupRenderer struct {
	source []byte
}

// jiraListParser preserves Goldmark's list behavior except that Jira's
// Markdown Input dialect lets any non-empty ordered list interrupt a paragraph.
type jiraListParser struct {
	parser.BlockParser
}

func (p jiraListParser) Open(parent ast.Node, reader text.Reader, context parser.Context) (ast.Node, parser.State) {
	if _, isList := parent.(*ast.List); isList {
		return nil, parser.NoChildren
	}
	last := context.LastOpenedBlock().Node
	if !ast.IsParagraph(last) || last.Parent() != parent {
		return nil, parser.NoChildren
	}
	line, _ := reader.PeekLine()
	marker, start, ok := orderedListInterrupt(line)
	if !ok || start == 1 {
		return nil, parser.NoChildren
	}
	list := ast.NewList(marker)
	list.Start = start
	return list, parser.HasChildren
}

func orderedListInterrupt(line []byte) (byte, int, bool) {
	index := 0
	for index < len(line) && line[index] == ' ' {
		index++
	}
	if index > 3 {
		return 0, 0, false
	}
	startIndex := index
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if digitCount := index - startIndex; digitCount == 0 || digitCount > 9 || index >= len(line) {
		return 0, 0, false
	}
	marker := line[index]
	if marker != '.' && marker != ')' {
		return 0, 0, false
	}
	start, err := strconv.Atoi(string(line[startIndex:index]))
	if err != nil {
		return 0, 0, false
	}
	index++
	if index >= len(line) || util.IsBlank(line[index:]) {
		return 0, 0, false
	}
	indentWidth, _ := util.IndentWidth(line[index:], index)
	if indentWidth == 0 {
		return 0, 0, false
	}
	return marker, start, true
}

const (
	listItemBlockReason   = "list items may contain only text and nested lists"
	blockquoteBlockReason = "blockquotes may contain only paragraphs"
)

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
		return r.renderList(typed, "")
	case *ast.Blockquote:
		return r.renderBlockquote(typed)
	case *ast.ThematicBreak:
		return "----", nil
	case *ast.HTMLBlock:
		return escapeUserTextForJiraMarkup(r.htmlBlockSource(typed), true), nil
	default:
		return "", r.unsupported(node)
	}
}

func (r jiraMarkupRenderer) renderBlockquote(blockquote *ast.Blockquote) (string, error) {
	paragraphs := make([]string, 0, blockquote.ChildCount())
	for child := blockquote.FirstChild(); child != nil; child = child.NextSibling() {
		paragraph, ok := child.(*ast.Paragraph)
		if !ok {
			return "", r.conversionError(child, blockquoteBlockReason)
		}
		content, err := r.renderInlines(paragraph, true)
		if err != nil {
			return "", err
		}
		paragraphs = append(paragraphs, content)
	}
	return "{quote}\n" + strings.Join(paragraphs, "\n\n") + "\n{quote}", nil
}

func (r jiraMarkupRenderer) renderList(list *ast.List, parentMarkers string) (string, error) {
	if !list.IsTight {
		return "", r.conversionError(list, "loose list items with multiple blocks are not supported")
	}
	marker := parentMarkers + "*"
	if list.IsOrdered() {
		marker = parentMarkers + "#"
	}
	lines := make([]string, 0, list.ChildCount())
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*ast.ListItem)
		if !ok {
			return "", r.unsupported(list)
		}
		var content string
		var nestedStart ast.Node
		switch firstBlock := item.FirstChild().(type) {
		case *ast.TextBlock:
			var err error
			content, err = r.renderInlines(firstBlock, false)
			if err != nil {
				return "", err
			}
			nestedStart = firstBlock.NextSibling()
		case *ast.List:
			nestedStart = firstBlock
		case nil:
		default:
			return "", r.conversionError(firstBlock, listItemBlockReason)
		}
		itemMarkup := marker
		if content != "" {
			itemMarkup += " " + content
		}
		lines = append(lines, itemMarkup)
		for block := nestedStart; block != nil; block = block.NextSibling() {
			nestedList, ok := block.(*ast.List)
			if !ok {
				return "", r.conversionError(block, listItemBlockReason)
			}
			nestedMarkup, err := r.renderList(nestedList, marker)
			if err != nil {
				return "", err
			}
			lines = append(lines, nestedMarkup)
		}
	}
	return strings.Join(lines, "\n"), nil
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
	return r.conversionError(node, "unsupported Markdown Input syntax")
}

func (r jiraMarkupRenderer) conversionError(node ast.Node, reason string) *ConversionError {
	line, column := sourcePosition(r.source, node)
	return &ConversionError{
		Line:     line,
		Column:   column,
		NodeType: node.Kind().String(),
		Reason:   reason,
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
