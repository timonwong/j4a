package markup

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var kindJFMContainerDirective = ast.NewNodeKind("JFMContainerDirective")

type jfmContainerDirective struct {
	ast.BaseBlock
	Opening   text.Segment
	Closing   text.Segment
	Attrs     text.Segment
	Name      string
	Fence     int
	RawBody   bool
	Closed    bool
	Malformed string
}

func (node *jfmContainerDirective) Kind() ast.NodeKind { return kindJFMContainerDirective }
func (node *jfmContainerDirective) Pos() int           { return node.Opening.Start }
func (node *jfmContainerDirective) IsRaw() bool        { return node.RawBody }
func (node *jfmContainerDirective) Dump(source []byte, level int) {
	ast.DumpHelper(node, source, level, map[string]string{"Name": node.Name}, nil)
}

type jfmContainerDirectiveParser struct{}

func (jfmContainerDirectiveParser) Trigger() []byte { return []byte{':'} }

func (jfmContainerDirectiveParser) Open(_ ast.Node, reader text.Reader, context parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	position := context.BlockOffset()
	if position < 0 || position > 3 || position >= len(line) || line[position] != ':' {
		return nil, parser.NoChildren
	}
	fenceEnd := position
	for fenceEnd < len(line) && line[fenceEnd] == ':' {
		fenceEnd++
	}
	fenceLength := fenceEnd - position
	if fenceLength < 3 {
		return nil, parser.NoChildren
	}
	lineEnd := len(line)
	for lineEnd > fenceEnd && (line[lineEnd-1] == '\n' || line[lineEnd-1] == '\r') {
		lineEnd--
	}
	nameEnd := fenceEnd
	for nameEnd < lineEnd && line[nameEnd] != '{' && line[nameEnd] != ' ' && line[nameEnd] != '\t' {
		nameEnd++
	}
	name := string(line[fenceEnd:nameEnd])
	if name == "" {
		return nil, parser.NoChildren
	}
	node := &jfmContainerDirective{
		Opening:   text.NewSegment(segment.Start+position, segment.Start+lineEnd),
		Name:      name,
		Fence:     fenceLength,
		Malformed: validateDirectiveName(name),
	}
	node.SetLines(text.NewSegments())
	restStart := nameEnd
	for restStart < lineEnd && (line[restStart] == ' ' || line[restStart] == '\t') {
		restStart++
	}
	if restStart < lineEnd {
		if line[restStart] != '{' {
			node.Malformed = "container directive has trailing source after its name"
		} else {
			attributeEnd := findDirectiveClosure(line, restStart+1, '}')
			if attributeEnd < 0 || !util.IsBlank(line[attributeEnd+1:lineEnd]) {
				node.Malformed = "container directive has a malformed attribute list"
			} else {
				node.Attrs = text.NewSegment(segment.Start+restStart+1, segment.Start+attributeEnd)
			}
		}
	}
	lowerName := strings.ToLower(name)
	node.RawBody = lowerName != "panel"
	return node, parser.NoChildren
}

func (jfmContainerDirectiveParser) Continue(rawNode ast.Node, reader text.Reader, _ parser.Context) parser.State {
	node := rawNode.(*jfmContainerDirective)
	line, segment := reader.PeekLine()
	position := util.FirstNonSpacePosition(line)
	if position >= 0 && position <= 3 {
		lineEnd := len(line)
		for lineEnd > position && (line[lineEnd-1] == '\n' || line[lineEnd-1] == '\r') {
			lineEnd--
		}
		closing := strings.Repeat(":", node.Fence)
		if position+len(closing) <= lineEnd && string(line[position:position+len(closing)]) == closing && util.IsBlank(line[position+len(closing):lineEnd]) {
			node.Closing = text.NewSegment(segment.Start+position, segment.Start+lineEnd)
			node.Closed = true
			reader.AdvanceToEOL()
			return parser.Close
		}
	}
	if node.RawBody {
		node.Lines().Append(segment)
		reader.AdvanceToEOL()
		return parser.Continue | parser.NoChildren
	}
	return parser.Continue | parser.HasChildren
}

func (jfmContainerDirectiveParser) Close(rawNode ast.Node, _ text.Reader, _ parser.Context) {
	node := rawNode.(*jfmContainerDirective)
	if !node.Closed && node.Malformed == "" {
		node.Malformed = "container directive is missing its closing fence"
	}
}

func (jfmContainerDirectiveParser) CanInterruptParagraph() bool { return true }
func (jfmContainerDirectiveParser) CanAcceptIndentedLine() bool { return false }
