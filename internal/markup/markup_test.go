package markup

import (
	"errors"
	"strings"
	"testing"
)

type markdownConversionCase struct {
	name  string
	input string
	want  string
}

func assertMarkdownConversions(t *testing.T, tests []markdownConversionCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToJira(test.input, Markdown)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("ToJira() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestToJiraPassThrough(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		format InputFormat
	}{
		{name: "Jira Markup", input: "* existing Jira markup *", format: JiraMarkup},
		{name: "empty Jira Markup", format: JiraMarkup},
		{name: "empty Markdown Input", format: Markdown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToJira(test.input, test.format)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.input {
				t.Fatalf("ToJira() = %q, want %q", got, test.input)
			}
		})
	}
}

func TestToJiraConvertsInitialMarkdownInputTracer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "paragraph", input: "Release notes", want: "Release notes"},
		{name: "empty heading", input: "#", want: "h1."},
		{name: "heading and paragraph", input: "# Release\n\nReady", want: "h1. Release\n\nReady"},
		{name: "inline code", input: "Run `jiro issues list` now.", want: "Run {{jiro issues list}} now."},
		{name: "unordered tight list", input: "- one\n- two", want: "* one\n* two"},
		{name: "ordered tight list", input: "1. one\n2. two", want: "# one\n# two"},
		{name: "list interrupts paragraph", input: "Release\n- one\n- two", want: "Release\n\n* one\n* two"},
		{name: "task marker remains text", input: "- [x] done", want: "* \\[x\\] done"},
		{name: "bare URL remains text", input: "Visit https://example.com", want: "Visit https://example.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToJira(test.input, Markdown)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("ToJira() = %q, want %q", got, test.want)
			}
			if got != "" && strings.TrimSpace(got) != got {
				t.Fatalf("ToJira() has leading or terminal whitespace: %q", got)
			}
		})
	}
}

func TestToJiraCanonicalizesCoreDocumentBlocks(t *testing.T) {
	t.Parallel()
	tests := []markdownConversionCase{
		{
			name:  "ATX headings preserve all levels",
			input: "# One\n\n## Two\n\n### Three\n\n#### Four\n\n##### Five\n\n###### Six",
			want:  "h1. One\n\nh2. Two\n\nh3. Three\n\nh4. Four\n\nh5. Five\n\nh6. Six",
		},
		{name: "setext level one heading", input: "Release\n=======", want: "h1. Release"},
		{name: "setext level two heading", input: "Release\n-------", want: "h2. Release"},
		{name: "closing ATX sequence is ignored", input: "## Release ##", want: "h2. Release"},
		{name: "asterisk thematic break", input: "***", want: "----"},
		{name: "underscore thematic break", input: "___", want: "----"},
		{name: "hyphen thematic break", input: "---", want: "----"},
		{
			name:  "top-level blocks use one blank line",
			input: "\n\n# Release\n\n\nFirst paragraph.\n\n---\n\nSecond paragraph.\n\n",
			want:  "h1. Release\n\nFirst paragraph.\n\n----\n\nSecond paragraph.",
		},
	}
	assertMarkdownConversions(t, tests)
}

func TestToJiraRendersInlineFormattingInASTNestingOrder(t *testing.T) {
	t.Parallel()
	tests := []markdownConversionCase{
		{name: "strong", input: "**bold**", want: "*bold*"},
		{name: "emphasis", input: "_italic_", want: "_italic_"},
		{name: "strikethrough", input: "~~deleted~~", want: "-deleted-"},
		{
			name:  "nested formatting",
			input: "**bold with _italic and ~~deleted~~_ text**",
			want:  "*bold with _italic and -deleted-_ text*",
		},
		{
			name:  "renderer formatting surrounds escaped user text",
			input: "**{panel}**",
			want:  `*\{panel\}*`,
		},
	}
	assertMarkdownConversions(t, tests)
}

func TestToJiraSerializesInlineBreaks(t *testing.T) {
	t.Parallel()
	tests := []markdownConversionCase{
		{name: "soft break", input: "line one\nline two", want: "line one line two"},
		{name: "two-space hard break", input: "line one  \nline two", want: "line one\\\\\nline two"},
		{name: "backslash hard break", input: "line one\\\nline two", want: "line one\\\\\nline two"},
		{
			name:  "breaks preserve inline nesting",
			input: "**first\nsecond  \nthird**",
			want:  "*first second\\\\\nthird*",
		},
		{
			name:  "hard break starts a new escaping context",
			input: "first  \nh2. user heading",
			want:  "first\\\\\nh2\\. user heading",
		},
	}
	assertMarkdownConversions(t, tests)
}

func TestToJiraEscapesUserTextForJiraMarkup(t *testing.T) {
	t.Parallel()
	tests := []markdownConversionCase{
		{
			name:  "Jira Markup notation in Markdown Input remains literal text",
			input: "Jira {panel} [label] !image! +under+ ^super^ \\~sub\\~ \\*literal\\* release-note",
			want:  "Jira \\{panel\\} \\[label\\] \\!image\\! \\+under\\+ \\^super\\^ \\~sub\\~ \\*literal\\* release\\-note",
		},
		{
			name:  "line-sensitive Jira Markup remains literal text",
			input: "h1. user heading\n\nbq. user quote\n\n??user citation??",
			want:  "h1\\. user heading\n\nbq\\. user quote\n\n\\?\\?user citation\\?\\?",
		},
		{name: "heading-like text within a line", input: "prefix h1. text", want: "prefix h1. text"},
		{
			name:  "task markers have no task-list semantics",
			input: "- [x] done\n- [ ] todo",
			want:  "* \\[x\\] done\n* \\[ \\] todo",
		},
		{name: "literal backslash", input: `path C:\temp`, want: `path C:\\temp`},
		{name: "CommonMark entity", input: "&ast;literal&ast;", want: `\*literal\*`},
	}
	assertMarkdownConversions(t, tests)
}

func TestToJiraRendersRawHTMLAsEscapedLiteralText(t *testing.T) {
	t.Parallel()
	tests := []markdownConversionCase{
		{
			name:  "inline HTML",
			input: "Use <span>{panel}</span> literally.",
			want:  `Use <span>\{panel\}</span> literally.`,
		},
		{
			name:  "HTML block",
			input: "<div>\n*not emphasis*\n{panel}\n</div>\n",
			want:  "<div>\n\\*not emphasis\\*\n\\{panel\\}\n</div>",
		},
		{
			name:  "HTML block edge whitespace",
			input: "   <div>\ncontent\n</div>   \n",
			want:  "<div>\ncontent\n</div>",
		},
		{
			name:  "HTML comment",
			input: "Before\n\n<!-- {panel} -->\n\nAfter",
			want:  "Before\n\n<\\!\\-\\- \\{panel\\} \\-\\->\n\nAfter",
		},
	}
	assertMarkdownConversions(t, tests)
}

func TestToJiraRejectsUnknownInputFormat(t *testing.T) {
	t.Parallel()
	got, err := ToJira("unchanged", InputFormat("html"))
	if err == nil {
		t.Fatal("ToJira() error = nil, want unknown format error")
	}
	if got != "" {
		t.Fatalf("ToJira() output = %q, want no output", got)
	}
}

func TestToJiraReportsUnsupportedMarkdownInputSyntaxWithoutPartialOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		line     int
		column   int
		nodeType string
	}{
		{name: "discards earlier blocks", input: "supported\n\n> not supported", line: 3, column: 1, nodeType: "Blockquote"},
		{name: "table extension is enabled", input: "| A |\n| - |\n| B |", line: 1, column: 1, nodeType: "Table"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToJira(test.input, Markdown)
			if got != "" {
				t.Fatalf("ToJira() output = %q, want no partial output", got)
			}
			var conversionErr *ConversionError
			if !errors.As(err, &conversionErr) {
				t.Fatalf("ToJira() error = %T %v, want *ConversionError", err, err)
			}
			if conversionErr.Line != test.line || conversionErr.Column != test.column ||
				conversionErr.NodeType != test.nodeType || conversionErr.Reason != "unsupported Markdown Input syntax" {
				t.Fatalf("ConversionError = %+v", conversionErr)
			}
		})
	}
}
