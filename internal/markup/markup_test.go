package markup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

type markdownConversionCase struct {
	name  string
	input string
	want  string
}

type markdownGoldenFixture struct {
	Input string `json:"input"`
	Want  string `json:"want"`
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

func assertMarkdownConversionError(t *testing.T, input string, want ConversionError) {
	t.Helper()
	got, err := ToJira(input, Markdown)
	if got != "" {
		t.Fatalf("ToJira() output = %q, want no partial output", got)
	}
	var conversionErr *ConversionError
	if !errors.As(err, &conversionErr) {
		t.Fatalf("ToJira() error = %T %v, want *ConversionError", err, err)
	}
	if *conversionErr != want {
		t.Fatalf("ConversionError = %+v, want %+v", conversionErr, want)
	}
}

func TestToJiraMatchesComplexStructureGoldenFixtures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"complex_mixed_list", "formatted_blockquote"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contents, err := os.ReadFile("testdata/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var fixture markdownGoldenFixture
			if err := json.Unmarshal(contents, &fixture); err != nil {
				t.Fatal(err)
			}
			got, err := ToJira(fixture.Input, Markdown)
			if err != nil {
				t.Fatal(err)
			}
			if got != fixture.Want {
				t.Fatalf("ToJira() = %q, want %q", got, fixture.Want)
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

func TestToJiraRendersCodeBlocks(t *testing.T) {
	t.Parallel()
	tests := []markdownConversionCase{
		{
			name:  "indented code block",
			input: "    first\n      second",
			want:  "{code}\nfirst\n  second\n{code}",
		},
		{
			name:  "javascript aliases are case insensitive",
			input: "```JsX\nconst answer = 42\n```",
			want:  "{code:language=javascript}\nconst answer = 42\n{code}",
		},
		{
			name:  "shell aliases are case insensitive",
			input: "```SHELL\necho ready\n```",
			want:  "{code:language=bash}\necho ready\n{code}",
		},
		{
			name:  "unknown language and metadata are ignored",
			input: "```go title=secret\nfmt.Println(\"safe\")\n```",
			want:  "{code}\nfmt.Println(\"safe\")\n{code}",
		},
		{
			name:  "recognized language ignores trailing metadata",
			input: "```javascript title=secret\nconsole.log(\"safe\")\n```",
			want:  "{code:language=javascript}\nconsole.log(\"safe\")\n{code}",
		},
		{
			name:  "blank code block",
			input: "```\n```",
			want:  "{code}\n{code}",
		},
		{
			name:  "authored blank line is preserved",
			input: "```\n\n```",
			want:  "{code}\n\n{code}",
		},
		{
			name:  "authored whitespace is preserved",
			input: "```text\n  first  \n\tsecond\n\n```",
			want:  "{code}\n  first  \n\tsecond\n\n{code}",
		},
	}
	assertMarkdownConversions(t, tests)

	for _, test := range []struct {
		name     string
		language string
		want     string
	}{
		{name: "javascript", language: "javascript", want: "javascript"},
		{name: "js", language: "JS", want: "javascript"},
		{name: "jsx", language: "jsx", want: "javascript"},
		{name: "mjs", language: "MJS", want: "javascript"},
		{name: "bash", language: "bash", want: "bash"},
		{name: "sh", language: "SH", want: "bash"},
		{name: "shell", language: "shell", want: "bash"},
		{name: "zsh", language: "ZSH", want: "bash"},
	} {
		t.Run("language alias/"+test.name, func(t *testing.T) {
			t.Parallel()
			input := "```" + test.language + "\ncode\n```"
			want := "{code:language=" + test.want + "}\ncode\n{code}"
			got, err := ToJira(input, Markdown)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("ToJira() = %q, want %q", got, want)
			}
		})
	}
}

func TestToJiraCollapsesCodeBlocksOnlyAboveTheAuthoredLineThreshold(t *testing.T) {
	t.Parallel()
	for _, lineCount := range []int{20, 21} {
		t.Run(fmt.Sprintf("%d logical lines", lineCount), func(t *testing.T) {
			t.Parallel()
			lines := make([]string, lineCount)
			for index := range lines {
				lines[index] = fmt.Sprintf("line %d", index+1)
			}
			input := "```\n" + strings.Join(lines, "\n")
			wantHeader := "{code}"
			if lineCount > 20 {
				wantHeader = "{code:collapse=true}"
			}
			want := wantHeader + "\n" + strings.Join(lines, "\n") + "\n{code}"

			got, err := ToJira(input, Markdown)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("ToJira() = %q, want %q", got, want)
			}
			if strings.Contains(got, "collapse=false") || strings.Contains(got, "theme=") ||
				strings.Contains(got, "linenumbers=") || strings.Contains(got, "border") {
				t.Fatalf("ToJira() emitted forbidden code parameters: %q", got)
			}
		})
	}
}

func TestToJiraProtectsInlineCodeDelimiters(t *testing.T) {
	t.Parallel()
	tests := []markdownConversionCase{
		{
			name:  "curly braces",
			input: "`{key}={value}`",
			want:  "{{\\{key\\}=\\{value\\}}}",
		},
		{
			name:  "backslash is preserved",
			input: "`C:\\Users\\admin`",
			want:  `{{C:\Users\admin}}`,
		},
		{
			name:  "trailing backslash protects the closing delimiter",
			input: "`path\\`",
			want:  "{{path\\\u200b}}",
		},
		{
			name:  "backslash before braces gets delimiter protection",
			input: "`\\{test\\}`",
			want:  "{{\\\u200b\\{test\\\u200b\\}}}",
		},
		{
			name:  "backslash before escaped delimiters gets delimiter protection",
			input: "`\\* \\_ \\-`",
			want:  "{{\\\u200b\\* \\\u200b\\_ \\\u200b\\-}}",
		},
		{
			name:  "square brackets and pipes are escaped",
			input: "`[item|value]`",
			want:  "{{\\[item\\|value\\]}}",
		},
	}
	assertMarkdownConversions(t, tests)
}

func TestToJiraPreservesTightNestedListOwnershipAndOrder(t *testing.T) {
	t.Parallel()
	input := "- alpha\n  1. first\n     - leaf\n  2. second\n- omega"
	want := "* alpha\n*# first\n*#* leaf\n*# second\n* omega"

	got, err := ToJira(input, Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ToJira() = %q, want %q", got, want)
	}
}

func TestToJiraPreservesNestedListOwnedByEmptyParentItem(t *testing.T) {
	t.Parallel()
	input := "-\n  - child\n- sibling"
	want := "*\n** child\n* sibling"

	got, err := ToJira(input, Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ToJira() = %q, want %q", got, want)
	}
}

func TestToJiraNormalizesOrderedListStartValue(t *testing.T) {
	t.Parallel()
	got, err := ToJira("7. seven\n8. eight", Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if want := "# seven\n# eight"; got != want {
		t.Fatalf("ToJira() = %q, want %q", got, want)
	}
}

func TestToJiraOrderedListAboveOneInterruptsParagraphs(t *testing.T) {
	t.Parallel()
	tests := []markdownConversionCase{
		{
			name:  "top-level paragraph",
			input: "Intro\n3. three\n4. four",
			want:  "Intro\n\n# three\n# four",
		},
		{
			name:  "nested item text",
			input: "- parent\n  3. child\n  4. next",
			want:  "* parent\n*# child\n*# next",
		},
	}
	assertMarkdownConversions(t, tests)
}

func TestToJiraPreservesFormattedBlockquoteParagraphs(t *testing.T) {
	t.Parallel()
	input := "> **alpha**\n>\n> beta _two_"
	want := "{quote}\n*alpha*\n\nbeta _two_\n{quote}"

	got, err := ToJira(input, Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ToJira() = %q, want %q", got, want)
	}
}

func TestToJiraRejectsLooseMultiBlockListItems(t *testing.T) {
	t.Parallel()
	assertMarkdownConversionError(t, "- first paragraph\n\n  second paragraph", ConversionError{
		Line:     1,
		Column:   1,
		NodeType: "List",
		Reason:   "loose list items with multiple blocks are not supported",
	})
}

func TestToJiraRejectsFencedCodeInsideListItem(t *testing.T) {
	t.Parallel()
	assertMarkdownConversionError(t, "- item\n  ```go\n  code\n  ```", ConversionError{
		Line:     2,
		Column:   3,
		NodeType: "FencedCodeBlock",
		Reason:   "list items may contain only text and nested lists",
	})
}

func TestToJiraRejectsBlockquoteInsideListItem(t *testing.T) {
	t.Parallel()
	assertMarkdownConversionError(t, "- item\n  > quote", ConversionError{
		Line:     2,
		Column:   3,
		NodeType: "Blockquote",
		Reason:   "list items may contain only text and nested lists",
	})
}

func TestToJiraRejectsOtherBlockNodesInsideListItem(t *testing.T) {
	t.Parallel()
	assertMarkdownConversionError(t, "- item\n  # heading", ConversionError{
		Line:     2,
		Column:   3,
		NodeType: "Heading",
		Reason:   "list items may contain only text and nested lists",
	})
}

func TestToJiraRejectsNestedBlockquotes(t *testing.T) {
	t.Parallel()
	assertMarkdownConversionError(t, "> outer\n>\n> > nested", ConversionError{
		Line:     3,
		Column:   3,
		NodeType: "Blockquote",
		Reason:   "blockquotes may contain only paragraphs",
	})
}

func TestToJiraRejectsNonParagraphBlocksInsideBlockquotes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		line     int
		column   int
		nodeType string
	}{
		{name: "list", input: "> intro\n>\n> - item", line: 3, column: 3, nodeType: "List"},
		{name: "table", input: "> | A |\n> | - |\n> | B |", line: 1, column: 3, nodeType: "Table"},
		{name: "fenced code", input: "> ```go\n> code\n> ```", line: 1, column: 3, nodeType: "FencedCodeBlock"},
		{name: "indented code", input: ">     code", line: 1, column: 7, nodeType: "CodeBlock"},
		{name: "heading", input: "> # heading", line: 1, column: 3, nodeType: "Heading"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertMarkdownConversionError(t, test.input, ConversionError{
				Line:     test.line,
				Column:   test.column,
				NodeType: test.nodeType,
				Reason:   "blockquotes may contain only paragraphs",
			})
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

func TestToJiraRendersExplicitLinkWithFormattedLabel(t *testing.T) {
	t.Parallel()
	got, err := ToJira(`[**Docs**](https://example.com "ignored title")`, Markdown)
	if err != nil {
		t.Fatal(err)
	}
	if want := `[*Docs*|https://example.com]`; got != want {
		t.Fatalf("ToJira() = %q, want %q", got, want)
	}
}

func TestToJiraRendersOnlyIntentionalCommonMarkLinks(t *testing.T) {
	t.Parallel()
	assertMarkdownConversions(t, []markdownConversionCase{
		{name: "angle-bracket URL", input: `<https://example.com/docs>`, want: `[https://example.com/docs|https://example.com/docs]`},
		{name: "angle-bracket email", input: `<user@example.com>`, want: `[user@example.com|mailto:user@example.com]`},
		{name: "reference link", input: "[Docs][guide]\n\n[guide]: ../guide/start \"ignored title\"", want: `[Docs|../guide/start]`},
		{name: "bare URL and email remain text", input: `https://example.com user@example.com`, want: `https://example.com user@example.com`},
	})
}

func TestToJiraPreservesSafeLinkDestinations(t *testing.T) {
	t.Parallel()
	assertMarkdownConversions(t, []markdownConversionCase{
		{name: "absolute", input: `[docs](https://example.com/a?x=1#top)`, want: `[docs|https://example.com/a?x=1#top]`},
		{name: "root relative", input: `[docs](/guide/start)`, want: `[docs|/guide/start]`},
		{name: "path relative", input: `[docs](../guide/start)`, want: `[docs|../guide/start]`},
		{name: "protocol relative", input: `[cdn](//cdn.example.com/asset)`, want: `[cdn|//cdn.example.com/asset]`},
		{name: "custom scheme", input: `[issue](jira:PROJ-1)`, want: `[issue|jira:PROJ-1]`},
		{name: "CommonMark escapes and entities are decoded", input: `[custom](custom+v1:thing\(1\)?x=1&amp;y=2)`, want: `[custom|custom+v1:thing(1)?x=1&y=2]`},
	})
}

func TestToJiraRejectsDangerousLinkDestinationSchemesWithoutPartialOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		scheme   string
		nodeType string
	}{
		{name: "mixed-case javascript", input: "safe\n\n[bad](JaVaScRiPt:alert(1))", scheme: "javascript", nodeType: "Link"},
		{name: "entity-normalized vbscript", input: `[bad](vb&#x73;cript:msgbox)`, scheme: "vbscript", nodeType: "Link"},
		{name: "uppercase data", input: `[bad](DATA:text/plain,unsafe)`, scheme: "data", nodeType: "Link"},
		{name: "angle-bracket dangerous URL", input: `<JaVaScRiPt:alert>`, scheme: "javascript", nodeType: "AutoLink"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			line := 1
			if strings.HasPrefix(test.input, "safe") {
				line = 3
			}
			assertMarkdownConversionError(t, test.input, ConversionError{
				Line:     line,
				Column:   1,
				NodeType: test.nodeType,
				Reason:   fmt.Sprintf("dangerous destination scheme %q is not allowed", test.scheme),
			})
		})
	}
}

func TestToJiraRejectsLinkDestinationsContainingControlCharacters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		line     int
		nodeType string
		reason   string
	}{
		{name: "explicit link", input: "safe\n\n[bad](<https://example.com/a\x01b>)", line: 3, nodeType: "Link", reason: "destination contains control character U+0001"},
		{name: "angle-bracket link", input: "<https://example.com/a\x7fb>", line: 1, nodeType: "AutoLink", reason: "destination contains control character U+007F"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertMarkdownConversionError(t, test.input, ConversionError{
				Line:     test.line,
				Column:   1,
				NodeType: test.nodeType,
				Reason:   test.reason,
			})
		})
	}
}

func TestToJiraRendersImagesWithDestinationAndOptionalAltAttribute(t *testing.T) {
	t.Parallel()
	assertMarkdownConversions(t, []markdownConversionCase{
		{
			name:  "non-empty alternative text is separate from the target",
			input: `![architecture](https://example.com/diagram.png "ignored title")`,
			want:  `!https://example.com/diagram.png|alt=architecture!`,
		},
		{
			name:  "empty alternative text omits the attribute",
			input: `![](images/diagram.png "ignored title")`,
			want:  `!images/diagram.png!`,
		},
		{
			name:  "reference image with protocol-relative destination",
			input: "![architecture][diagram]\n\n[diagram]: //cdn.example.com/diagram.png \"ignored title\"",
			want:  `!//cdn.example.com/diagram.png|alt=architecture!`,
		},
		{
			name:  "custom-scheme image",
			input: `![chart](asset+v1:diagram-42)`,
			want:  `!asset+v1:diagram-42|alt=chart!`,
		},
	})
}

func TestToJiraRejectsUnsafeImageDestinationsWithoutPartialOutput(t *testing.T) {
	t.Parallel()
	t.Run("dangerous scheme", func(t *testing.T) {
		t.Parallel()
		assertMarkdownConversionError(t, "safe\n\n![bad](DaTa:text/plain,unsafe)", ConversionError{
			Line:     3,
			Column:   1,
			NodeType: "Image",
			Reason:   `dangerous destination scheme "data" is not allowed`,
		})
	})
	t.Run("control character", func(t *testing.T) {
		t.Parallel()
		assertMarkdownConversionError(t, "![bad](<images/a\x7fb.png>)", ConversionError{
			Line:     1,
			Column:   1,
			NodeType: "Image",
			Reason:   "destination contains control character U+007F",
		})
	})
}

func TestToJiraEscapesLinksAndImagesInTheirOwnOutputContexts(t *testing.T) {
	t.Parallel()
	assertMarkdownConversions(t, []markdownConversionCase{
		{
			name:  "link label and destination have separate delimiters",
			input: `[**label** | value!](<custom:one|two]!>)`,
			want:  `[*label* \| value\!|custom:one\|two\]!]`,
		},
		{
			name:  "image destination and alt attribute remain isolated",
			input: `![**architecture** | v1, x=y!](<images/a,b=c|d!e.png>)`,
			want:  `!images/a,b=c\|d\!e.png|alt=architecture \| v1\, x\=y\!!`,
		},
	})
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
		{name: "discards earlier blocks", input: "supported\n\n| A |\n| - |\n| B |", line: 3, column: 1, nodeType: "Table"},
		{name: "table extension is enabled", input: "| A |\n| - |\n| B |", line: 1, column: 1, nodeType: "Table"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertMarkdownConversionError(t, test.input, ConversionError{
				Line:     test.line,
				Column:   test.column,
				NodeType: test.nodeType,
				Reason:   "unsupported Markdown Input syntax",
			})
		})
	}
}
