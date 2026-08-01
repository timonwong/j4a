# Jiro Flavored Markdown

This document defines the bidirectional conversion contract between Jira Markup and Jiro Flavored Markdown (JFM).

## Product boundary

- The first version is a pair of pure string conversion capabilities between Jira Markup and JFM.
- It does not define Jira network access, Issue or comment retrieval, command output, or a complete Issue export document.

## Output dialect

- JFM is based on CommonMark with table and strikethrough syntax.
- It is distinct from Markdown Input.
- Supported constructs round-trip to canonical Jira Markup, but neither direction preserves the original source bytes or non-canonical spelling.
- Inserted, superscript, subscript, and colored text use controlled inline HTML.
- Jira citation notation is not supported in the first version.
- Task-list markers have no checkbox semantics and remain ordinary list text.
- Bare URLs have no autolink semantics; only explicit Markdown links and angle-bracket autolinks are links.
- Reference-style links and images are accepted only as `FromJFM` input syntax. `ToJFM` always emits canonical inline syntax or a jiro directive and never emits reference definitions.
- JFM does not repair malformed Markdown or implicitly interpret raw Jira Markup islands.
- A JFM document has no required front matter, magic header, or embedded format-version directive; it remains directly readable as Markdown.
- JFM capability evolves with jiro releases. New syntax should be additive where possible, and an older parser handles unknown directives through the established literal fallback and warning contract.

## Canonical document serialization

- Top-level blocks are separated by exactly one blank line.
- The complete converted document has no leading or trailing whitespace.
- Structural output uses LF line endings and does not append a terminal newline.
- A source newline inside an ordinary paragraph without Jira `\\` forced-break notation normalizes to one space.
- Literal code and noformat bodies preserve their authored whitespace, including their internal LF or CRLF line endings, and do not use structural line-ending or paragraph normalization.
- Panel, quote, list, table, and other non-literal bodies use canonical LF line endings.

## Escaping

- Conversion parses source escaping in its original Jira or JFM context, produces semantic literal content, and then serializes that content for the target context.
- A Jira backslash decodes an escaped Jira delimiter; doubled backslashes decode to one literal backslash.
- A backslash before a non-delimiter remains ordinary text without a warning, including Windows-style paths such as `C:\temp`.
- Re-encoding prevents decoded Jira notation from becoming unintended Markdown formatting and prevents decoded JFM text from becoming unintended Jira notation.
- A warning is reserved for an escape that truncates or prevents closure of a recognized construct.
- `FromJFM` decodes valid CommonMark named and numeric character references to their semantic Unicode characters before applying Jira-context escaping.
- `ToJFM` serializes semantic Unicode characters directly and does not introduce character references; Markdown-sensitive characters are escaped only as required by their output context.
- An invalid character reference is ordinary visible text and does not produce a warning.
- Input strings may contain invalid UTF-8 bytes. Each invalid byte is replaced with U+FFFD, produces its own warning, and counts as one source column for location reporting.
- Invalid UTF-8 is a source-content problem rather than a fatal error; both conversion directions always return valid UTF-8 text when they complete successfully.

## Directive grammar

- Inline directives use `:name[content]{attributes}`.
- Container directives use at least three opening colons, a name and optional attributes, a body, and a closing fence with the same number of colons.
- Input permits spaces or tabs between attributes and around `=`, but an attribute list never spans a physical line.
- An inline directive permits no whitespace between its name, content brackets, and attribute braces.
- A container opening fence permits only whitespace after its optional attribute list.
- Canonical serialization uses one ASCII space between attributes, no whitespace around `=`, and no trailing whitespace.
- A directive name matches `[A-Za-z][A-Za-z0-9-]*`; an attribute name matches `[A-Za-z][A-Za-z0-9_-]*`.
- JFM does not support `.class` or `#id` attribute shorthand.
- A Jira parameter name that cannot be represented by the attribute-name grammar makes the complete Jira construct remain visible and produces a warning rather than being renamed.
- An invalid directive or attribute identifier in JFM makes the complete directive use escaped Jira literal fallback and produces a warning.
- Directive names are matched case-insensitively and serialize as canonical lowercase names.
- Known attribute names are matched case-insensitively and serialize with their directive-defined canonical spelling, including Jira camelCase names.
- Attributes that differ only by case still count as duplicates and produce warnings.
- An outer container fence is longer than any colon fence that appears as a nested container in its body.
- Canonical serialization uses the shortest safe fence length.
- Every directive defines a fixed canonical order for its known attributes; source attribute order is not preserved for known attributes.
- Unknown attributes remain after known attributes and retain their relative source order. Each unknown attribute produces a warning.
- Duplicate attributes remain in source order and produce warnings rather than silently using first-wins or last-wins behavior.
- Every directive applies the same value-typing rules. Known boolean values are matched case-insensitively and serialize as lowercase unquoted `true` or `false`.
- Every non-boolean value is a double-quoted string, including empty and numeric-looking values such as `firstline="10"` or `borderWidth="1"`.
- An invalid value for a known boolean attribute remains visible as a double-quoted string and produces a warning.
- Attribute values use JSON-style escapes for double quote, backslash, newline, carriage return, and tab: `\"`, `\\`, `\n`, `\r`, and `\t`.
- Attribute values never span physical source lines.
- Bare attributes are reserved for explicitly defined presence-only flags such as image `thumbnail`; they are distinct from boolean-valued attributes.
- Unknown escape sequences remain visible and produce warnings.
- Inline directive content never spans a physical source line. `]` and backslash are escaped as `\]` and `\\`.
- Each inline directive defines its content model: `:link[...]` parses supported inline JFM, while `:image[...]` contains plain alternative text only.
- Content that violates its directive's content model remains visible and produces a warning.
- `:::panel`, `:::code`, and `:image` can serialize unknown or duplicate attributes into Jira's parameter-bearing syntax. Those attributes retain the required order and produce warnings without making the complete directive literal.
- `:link` and `:::table` have no Jira target location for extra attributes. An unknown or duplicate attribute makes the complete directive escaped Jira literal text and produces a warning.
- A directive missing a required attribute, including `:link` without `target` or `:image` without `src`, is malformed; the complete directive becomes escaped Jira literal text and produces a warning.

```markdown
::::panel{title="Data"}
:::table
|A|1|
|B|2|
:::
::::
```

## Unsupported notation

- Unsupported source notation does not fail the complete conversion in either direction.
- The unsupported source notation remains present in JFM, and the conversion reports a warning.
- Unsupported notation is never silently dropped.
- Unsupported macro delimiters remain in their original Jira form while recognized content in the macro body continues through best-effort conversion.
- Warnings identify that the macro was not recognized and that its body was converted without knowledge of the macro's render mode.
- The fallback does not wrap unsupported content in a separate raw directive.
- Warnings are returned as structured data alongside JFM and are never embedded in the Markdown text.
- Every warning records its source line, source column, construct, and reason.
- `ConversionWarning.Construct` is an open, machine-stable vocabulary of lowercase kebab-case identifiers rather than free-form prose. The package provides exported constants for identifiers it emits, such as `link`, `image`, `code-block`, `reference-definition`, `directive`, and `jira-macro`.
- New construct identifiers may be added without changing the field's `string` type. Callers must not treat the current identifier set as closed.
- `ConversionWarning.Reason` is human-readable explanatory text and is not a machine-stable identifier.
- `FromJFM` preserves unknown directives, custom HTML elements, malformed supported directives, and unsupported Markdown constructs as escaped literal Jira text with warnings while continuing the rest of the conversion.
- Warnings remain in parser discovery order, which follows source occurrence order.
- Multiple warnings at the same position retain discovery order; warnings are not sorted, merged, or deduplicated.
- Every unsupported or malformed occurrence produces its own warning.
- Warning line and column values are one-based.
- Columns count Unicode code points rather than UTF-8 bytes.
- LF, CRLF, and a bare CR each advance the line number once.
- A construct-level warning points to the first source character of the construct.
- An attribute-level warning points to the first character of the attribute name.
- An escape warning points to the backslash that triggers the problem.
- A reference-definition warning points to its opening `[`, and a macro warning points to its opening `{`.
- Warnings expose only this start location; the first-version API does not include an end position.

```go
type JFMResult struct {
	Markdown string
	Warnings []ConversionWarning
}

type JiraMarkupResult struct {
	Markup   string
	Warnings []ConversionWarning
}

type ConversionWarning struct {
	Line      int
	Column    int
	Construct string
	Reason    string
}
```

## Library API

```go
func ToJFM(ctx context.Context, jiraMarkup string) (JFMResult, error)
func FromJFM(ctx context.Context, jfm string) (JiraMarkupResult, error)
```

- The first-version Go API remains in `internal/markup`; it is available to jiro commands and repository tests but is not a public Go module compatibility promise.
- The JFM text-format contract is stable independently of the Go package's internal visibility.
- The existing Markdown Input `ToJira` API remains unchanged.
- Both JFM conversion directions use best-effort conversion and return structured warnings separately from their text result.
- Both new conversion APIs honor context cancellation during parsing and serialization.
- Given the same input and a context that is not cancelled, each conversion returns byte-for-byte identical text and warnings in the same order.
- `ToJFM` and `FromJFM` are safe for concurrent calls and do not depend on mutable package-global state.
- Conversion does not read Jira profiles, locale settings, network state, or external configuration. Context cancellation is the only external state that may affect completion.

## Internal architecture

- `FromJFM` continues to use Goldmark for CommonMark parsing, with jiro-owned extensions for JFM directives and controlled syntax.
- `ToJFM` uses a purpose-built Jira Markup parser because Jira macros, marker-chain lists, escaping, and fallback boundaries are not CommonMark grammar.
- Both parsers adapt into a thin normalized semantic model that carries source spans. Target serializers consume that model to share construct, canonicalization, attribute, and warning rules.
- Goldmark's AST is not forced to serve as the Jira parser's interchange model; fabricating CommonMark source segments for Jira notation would make locations, escaping, and unsupported-notation preservation fragile.
- The existing Markdown Input `ToJira` parser and renderer remain unchanged.

## Errors

- Unknown, unsupported, or malformed source notation is preserved and reported through structured warnings rather than a fatal error in both directions.
- The converter does not impose a fixed input-size limit.
- The contract does not currently define a nesting-depth limit.
- Fatal errors are limited to context cancellation, output failures, and internal invariant failures.
- Cancellation and deadline failures preserve `errors.Is(err, context.Canceled)` and `errors.Is(err, context.DeadlineExceeded)` respectively.
- Internal invariant and other conversion-engine failures wrap the exported sentinel `ErrConversion` so callers can use `errors.Is` without depending on error text.
- Source-content problems never wrap `ErrConversion` and never become fatal errors; they remain result text plus structured warnings.
- A fatal error returns the zero value of the direction-specific result type and never exposes partial converted text or warnings.

## Initial supported scope

- Every canonical Jira construct emitted by the existing Markdown Input converter.
- ATX headings H1 through H6.
- Bold, italic, combined bold and italic, and strikethrough text.
- Inserted, superscript, subscript, and colored text.
- Ordered, unordered, and nested lists.
- Named and unnamed links.
- Images.
- Inline monospaced text.
- Typed and untyped preformatted blocks.
- Forced line breaks and thematic breaks.
- Single-paragraph blockquotes.
- Tables.
- Jira panels.

Every canonical Jira Markup value produced by the existing Markdown Input converter must convert to JFM without warnings and return to semantically equivalent canonical Jira Markup.

## Headings

- Jira line-start headings `h1.` through `h6.` render as Markdown ATX headings `#` through `######`.
- `FromJFM` accepts CommonMark setext H1 and H2 headings in addition to ATX input.
- `FromJFM` accepts CommonMark ATX headings with up to three leading spaces and an optional closing hash sequence without warnings.
- Canonical JFM always serializes headings in ATX form.
- `ToJFM` starts the ATX marker at the beginning of the line and never emits closing hashes.
- A non-empty Markdown heading uses exactly one space after its marker.
- An empty Jira heading such as `h3.` renders as `###`.
- JFM serializes headings back as canonical `hN. Title` or `hN.` for an empty heading.
- `h0.`, `h7.`, and malformed heading-like text remain visible and produce a warning.

## Canonical inline formatting

- Jira `*bold*` renders as Markdown `**bold**`.
- Jira `_italic_` renders as Markdown `*italic*`.
- Jira `-strikethrough-` renders as Markdown `~~strikethrough~~`.
- A span that is both bold and italic renders as Markdown `***bold and italic***`.
- `FromJFM` accepts CommonMark emphasis delimited by either `*` or `_`, and strong emphasis delimited by either `**` or `__`, without warnings.
- `ToJFM` always serializes italic as `*...*`, bold as `**...**`, and a single span carrying both marks as `***...***`.
- Distinct nested spans preserve their source nesting rather than being merged solely because their delimiters touch.
- Jira text-effect delimiters must form complete spans; ordinary hyphenated text such as `release-note` remains literal text.

## Lists

- `FromJFM` accepts CommonMark unordered-list markers `-`, `*`, and `+` without warnings; `ToJFM` always serializes them as canonical `-` markers.
- Ordered list items use `1.` markers because Jira does not retain authored ordinal values.
- `FromJFM` accepts CommonMark ordered-list markers ending in either `.` or `)`.
- An ordered list starting at `1` converts without a warning. Any other start value still converts structurally but produces a warning at the list marker because Jira cannot retain the authored start number.
- `ToJFM` always serializes ordered items with `1.` regardless of their position.
- Each nested level uses four spaces of indentation.
- Mixed Jira marker chains preserve the ordered or unordered type at every level.
- A reversible list item contains one inline paragraph followed by optional nested lists.
- Additional paragraphs, fenced code, blockquotes, tables, panels, and other block children inside an item are not structurally converted; their source remains visible and produces a warning.
- A list item whose nesting level has no parent is not flattened and does not cause synthetic empty parents. Its Jira source line remains visible and produces a warning.
- A top-level change between unordered and ordered list types starts a new Markdown block separated by one blank line.

```text
* parent
*# ordered child
*#* unordered grandchild
```

renders as:

```markdown
- parent
    1. ordered child
        - unordered grandchild
```

## Links

- An unnamed absolute URL uses a Markdown URI autolink: `[https://example.com]` renders as `<https://example.com>`.
- An unnamed `mailto:` destination uses a Markdown email autolink: `[mailto:user@example.com]` renders as `<user@example.com>`.
- An unnamed document anchor renders with its target as its label: `[#section]` renders as `[#section](#section)`.
- A named self-contained destination uses an ordinary Markdown link: `[Docs|https://example.com]` renders as `[Docs](https://example.com)` and `[Jump|#section]` renders as `[Jump](#section)`.
- Jira links have no separate title semantic. A Markdown link with an optional title is preserved as escaped Jira literal text and produces a warning; the converter does not silently discard the title or present the link as fully supported.
- `FromJFM` accepts resolved full, collapsed, and shortcut reference-style links and converts them with the same Jira destination rules as inline links.
- `ToJFM` always serializes those links inline or as `:link`; it never creates Markdown link reference definitions.
- A valid reference definition used by at least one link or image is consumed during `FromJFM`; the definition line itself produces neither Jira output nor a warning.
- An unused valid reference definition, or a duplicate definition shadowed by an earlier definition with the same normalized label, is preserved as escaped Jira literal text and produces a warning rather than being silently dropped.
- An unresolved reference is ordinary visible CommonMark text and converts as text without a warning.
- Jira-context-dependent link targets use the inline directive syntax `:link[content]{target="..."}`.
- The `content` is the displayed label. The required `target` attribute contains the complete Jira target, including prefixes such as `^` for attachments and `~` for users.
- The link directive's canonical attribute order is `target`.
- `target` is required and may occur only once; violations use the malformed-directive fallback.
- The target is always present and double-quoted, even when it is identical to the displayed content.
- Jira link directives are supported reversible constructs and do not produce warnings.
- Destinations using `javascript:`, `vbscript:`, or `data:` never render as ordinary Markdown links. They use `:link`, remain reversible, and produce warnings in both directions.
- Dangerous-scheme matching is case-insensitive and occurs after decoding source escaping and ignoring obfuscating leading whitespace or control characters.

```markdown
:link[PROJ-123]{target="PROJ-123"}
:link[Issue]{target="PROJ-123"}
:link[attachment.zip]{target="^attachment.zip"}
:link[username]{target="~username"}
```

## Images

- A Jira image containing only a destination and optional `alt` attribute renders as a standard Markdown image.
- `!https://example.com/a.png|alt=architecture!` renders as `![architecture](https://example.com/a.png)`.
- `!images/a.png!` renders as `![](images/a.png)`.
- Jira escaping is decoded and then serialized for the Markdown destination and alternative-text contexts independently.
- A Markdown image title is supported by `FromJFM` and maps to the Jira image `title` attribute.
- Because Jira image titles are additional attributes, `ToJFM` serializes an image with a title as `:image` rather than as standard Markdown image syntax.
- The existing Markdown Input converter's title-discarding behavior remains unchanged and is not part of JFM conversion.
- `FromJFM` accepts resolved reference-style images and converts them with the same Jira destination and alternative-text rules as inline images.
- `ToJFM` always serializes those images inline or as `:image`; it never creates Markdown image reference definitions.
- A Jira image with any additional supported attribute renders as an `:image[alt]{attributes}` inline directive.
- The image directive requires a double-quoted `src` attribute. Its content stores the alternative text.
- `src` is required and may occur only once; a missing or duplicate `src` uses the malformed-directive fallback.
- Supported Jira image attributes are `thumbnail`, `align`, `border`, `bordercolor`, `hspace`, `vspace`, `width`, `height`, and `title`; their names remain unchanged.
- The image directive's canonical attribute order is `src`, `thumbnail`, `align`, `border`, `bordercolor`, `hspace`, `vspace`, `width`, `height`, `title`.
- The presence-only `thumbnail` flag is emitted without a value.
- A supported image directive is reversible and does not produce a warning.
- Image destinations using `javascript:`, `vbscript:`, or `data:` always use `:image` rather than standard Markdown image syntax and produce warnings in both directions.

```markdown
:image[Fish]{src="fish.gif" thumbnail align="right" width="320" title="Preview"}
```

## Controlled inline HTML

- Jira `+inserted+` renders as `<ins>inserted</ins>`.
- Jira `^superscript^` renders as `<sup>superscript</sup>`.
- Jira `~subscript~` renders as `<sub>subscript</sub>`.
- Jira `{color:red}red text!{color}` renders as `<font color="red">red text!</font>`.
- A non-empty color value is preserved without a whitelist and HTML-attribute escaped before serialization.
- An empty or malformed color macro remains in its original Jira form and produces a warning.
- Supported Jira formatting inside these spans continues through JFM conversion.
- Nested Jira constructs retain their source nesting order. The converter does not reorder controlled HTML and Markdown delimiters.
- Controlled tag and attribute names are accepted case-insensitively, but canonical JFM output always uses lowercase tag names and lowercase `color`.
- `FromJFM` accepts the controlled `font` color value in valid unquoted, single-quoted, or double-quoted HTML attribute form; `ToJFM` always emits a double-quoted value.
- `<ins>`, `<sup>`, and `<sub>` accept no attributes. `<font>` accepts exactly one `color` attribute.
- Self-closing, mismatched, or unclosed controlled tags, attributes on `<ins>`, `<sup>`, or `<sub>`, and extra or duplicate `<font>` attributes make the complete HTML span escaped Jira literal text and produce a warning rather than being repaired or partially converted.

## Tables

- A Jira table with exactly one leading header row and body rows renders as a canonical GFM pipe table.
- A GFM table cell may contain only text, bold, italic, strikethrough, inline code, and standard Markdown links.
- If any cell contains an image, hard break, controlled HTML, directive, block content, or other Jira-only notation, the complete table uses the reversible `:::table` form instead.
- Table fallback applies to the whole table; GFM cells and raw Jira cells are never mixed in one table.
- Every cell delimiter has one surrounding space in JFM.
- Literal pipes and backslashes use Markdown table-cell-context escaping.
- Significant Jira cell leading or trailing whitespace triggers whole-table `:::table` fallback because GFM trims cell-edge whitespace.
- Code-span internal whitespace is never trimmed by table serialization.
- The GFM separator row uses `---` for every column.
- A GFM table without alignment metadata converts to Jira without a warning.
- If a GFM separator row contains left, right, or center alignment markers, `FromJFM` still converts the table content but produces one warning at the separator row because Jira cannot retain the alignment semantics.
- `ToJFM` does not infer alignment and always emits `---` for every separator cell.
- Jira table shapes that GFM cannot represent use the supported `:::table` container directive.
- A `:::table` body contains canonical Jira table rows and is not interpreted as Markdown.
- The table directive defines no attributes.
- Any attribute on `:::table` uses the malformed-directive fallback because Jira table rows have no parameter syntax.
- The table directive is reversible and does not produce a warning.

```jira
||Name||Value||
|Alpha|1|
|Beta|2|
```

renders as:

```markdown
| Name | Value |
| --- | --- |
| Alpha | 1 |
| Beta | 2 |
```

A headerless Jira table uses the directive form:

```markdown
:::table
|Alpha|1|
|Beta|2|
:::
```

## Blockquotes

- Jira `bq. one paragraph` and the `{quote}...{quote}` macro both render as Markdown blockquotes.
- A quote macro may contain multiple paragraphs.
- `FromJFM` accepts CommonMark lazy-continuation blockquotes and other valid forms that omit repeated markers without warnings.
- `ToJFM` never emits a lazy form: every non-empty quoted line begins with `> `, and a paragraph separator inside the quote is a line containing only `>`.
- Quote paragraphs may contain supported inline formatting, links, and controlled inline HTML.
- Lists, code blocks, tables, panels, nested quotes, and other non-paragraph children inside a quote are not structurally converted in the first version; their Jira syntax remains visible and produces a warning.
- JFM converts both Jira source forms back to the canonical `{quote}` macro form.

## Breaks

- Jira's `\\` forced-line-break notation renders as a Markdown backslash hard break.
- `FromJFM` accepts both a backslash hard break and the CommonMark form using two or more trailing spaces, serializing either as canonical Jira `\\` followed by a source newline.
- `ToJFM` always emits the visible backslash hard-break form and never emits trailing-space hard breaks.
- Jira `----` thematic breaks render as Markdown `---`.
- `FromJFM` accepts every thematic-break spelling recognized by the configured CommonMark parser, including valid `---`, `***`, `___`, and spaced forms, and serializes them as canonical Jira `----`.
- `ToJFM` always emits canonical `---`.

## Code and preformatted text

- Jira `{{monospaced}}` renders as a Markdown code span.
- Jira `{noformat}...{noformat}` renders as a fenced code block without a language.
- A Jira `{code}` macro with only an optional language renders as a fenced code block with the corresponding language.
- A Jira `{code}` macro with parameters beyond its language renders as a supported `:::code{attributes}` container directive.
- `FromJFM` accepts backtick and tilde code fences of any valid CommonMark length of at least three.
- `FromJFM` accepts a CommonMark four-space indented code block as an untyped preformatted block and converts it to Jira `{noformat}` without a warning.
- `ToJFM` never emits indented code blocks; it always uses fenced form. An indented code block inside a list item remains subject to the unsupported complex-list-item rule.
- A standard fenced code block supports an empty info string or exactly one language token. An info string containing additional tokens or metadata makes the complete block unsupported: it remains visible as escaped Jira literal text and produces a warning; parameterized code must use `:::code`.
- Code directive attribute names remain the Jira names. `collapse` and `linenumbers` are boolean-valued; the other code attributes are strings under the common directive typing rules.
- The recognized code directive attributes use this canonical order: `language`, `title`, `theme`, `linenumbers`, `firstline`, `collapse`, `borderStyle`, `borderColor`, `borderWidth`, `bgColor`, `titleBGColor`, `titleColor`.
- Unknown code directive attributes remain after the recognized attributes in their original relative order and produce a warning.
- Code languages are matched case-insensitively for the existing aliases: `js`, `jsx`, and `mjs` normalize to `javascript`; `sh`, `shell`, and `zsh` normalize to `bash`.
- Other single-token language values pass through unchanged without warnings.
- A language value that cannot be represented safely in a fence info string uses the quoted `language` attribute of `:::code`.
- `FromJFM` never adds `collapse=true` based on code-block length. Only an explicit `collapse=true` code directive attribute enables Jira collapse behavior.
- The existing Markdown Input converter's line-count-based collapse behavior remains unchanged and is not part of JFM conversion.
- The body of `{noformat}` and `{code}` is literal text. Jira formatting notation inside it is never interpreted.
- Inline code uses a backtick delimiter one character longer than the longest backtick run in its body, with a minimum length of one.
- `ToJFM` always uses a backtick fence one character longer than the longest backtick run in its body, with a minimum length of three.
- Inline code adds only the padding spaces required by CommonMark code-span rules; literal body content is otherwise unchanged.
- `ToJFM` removes U+200B only when it matches jiro's canonical inline-code delimiter-safety patterns: after a trailing backslash or between a backslash and an escaped Jira delimiter.
- U+200B in any other inline-code position remains literal content.
- `FromJFM` reuses the existing Jira inline-code escaping rules and inserts U+200B only where required to protect closing or escaped delimiters.

```markdown
:::code{language="javascript" title="Example" collapse=true}
const x = 1
:::
```

## Panels

- Jira `{panel}` macros render as a jiro-owned `:::panel{...}` container directive.
- The directive preserves the panel body and Jira attributes instead of degrading the panel into a blockquote or table.
- A panel body recursively supports every JFM block and inline construct, including headings, lists, code, tables, blockquotes, images, links, controlled HTML, and nested panels.
- Nested container directives use the variable-length colon-fence rules.
- Attribute names remain the original Jira names: `title`, `borderStyle`, `borderColor`, `borderWidth`, `bgColor`, `titleBGColor`, and `titleColor`.
- The panel directive's canonical attribute order is `title`, `borderStyle`, `borderColor`, `borderWidth`, `bgColor`, `titleBGColor`, `titleColor`.
- The directive does not use the unrelated ADF `panelType` model.

```markdown
:::panel{title="Warning" borderStyle="dashed" borderColor="#DE350B" bgColor="#FFEBE6"}
Panel content with **Markdown**.
:::
```

## Verification

- Exact-output tests compare the complete JFM result and the complete structured warning set.
- New exact-output cases use the repository's `txtar` golden convention in both directions, including an explicit `warnings.json` section when the expected list is empty.
- Warning-free supported Jira satisfies `Jira -> JFM -> Jira == canonical Jira`.
- Warning-free supported JFM satisfies `JFM -> Jira -> JFM == canonical JFM`.
- Input-only spellings such as reference-style links, alternate list markers, tilde code fences, and other accepted CommonMark variants need not retain their source spelling; they must retain supported semantics and normalize to canonical JFM.
- Inputs that produce warnings are not covered by a complete round-trip guarantee; they remain covered by the construct-specific preservation and best-effort rules.
- Fuzz tests exercise arbitrary byte strings in both directions and require no panic, no unbounded hang, and valid UTF-8 on successful completion.
- Race and concurrency tests verify deterministic concurrent calls, and cancellation tests verify the context error contract.
- The first-version verification scope contains no live Jira or network end-to-end test because both APIs are pure string conversions.

## Delivery boundary

- The first version is delivered and accepted as one complete bidirectional feature, even if its implementation is split into internal stages or commits.
- A single completed direction is not separately integrated or described as supported.
- Acceptance requires both internal APIs, structured warnings, supported JFM directives, `txtar` exact-output fixtures, round-trip properties, fuzz coverage, concurrency and race coverage, and cancellation tests.
- The existing Markdown Input `ToJira` behavior remains unchanged throughout delivery.
