# Markdown Input Conversion

This document defines the conversion contract from Markdown Input to Jira Markup.

## Input dialect

- Goldmark CommonMark core with the table and strikethrough extensions.
- A list may interrupt a preceding paragraph without a blank line.
- Bare URL and email linkification is disabled; explicit Markdown links and angle-bracket links remain links.
- Task lists have no special semantics. `[x]` and `[ ]` are ordinary text inside list items.
- Markdown source is never preprocessed or repaired.
- Markdown Input cannot embed raw Jira Markup.
- Raw HTML is rendered as escaped literal text.

## Structural contract

- Textual content, structural ownership, and block order must be preserved.
- Conversion fails rather than silently reparenting, reordering, or dropping content.
- Unsupported presentation metadata may be normalized or ignored: ordered lists start at one, table alignment and link or image titles are discarded, and unrecognized fence metadata is not emitted.
- Lists support paragraph interruption, loose or tight items with nested lists, and mixed marker chains.
- A list item may contain one text paragraph followed by nested lists. Additional paragraphs and block nodes such as fenced code or blockquotes fail conversion.
- Blockquotes always use `{quote}` and may contain one or more paragraphs with inline formatting.
- Nested blockquotes and blockquotes containing lists, tables, fenced code, or other non-paragraph blocks fail conversion.
- Table cells may contain text, emphasis, strong text, strikethrough, inline code, and explicit links.
- Images, hard breaks, raw HTML, and block-level content inside table cells fail conversion.
- Table alignment metadata is ignored, and literal pipes are escaped for the table-cell context.

## Basic mappings

- ATX and setext headings render as `h1.` through `h6.`.
- Strong text renders as `*...*`.
- Emphasis renders as `_..._`.
- Strikethrough renders as `-...-`.
- Thematic breaks render as `----` regardless of their Markdown marker.
- Explicit labeled links render as `[label|destination]`.
- Inline formatting nests in AST order.
- The renderer does not add Jira-only underline, color, panel, theme, or line-number styling.
- Table header cells use `||`, and table body cells use `|`.

## Code

- Indented CommonMark code blocks render as untyped Jira code blocks.
- Fenced languages `javascript`, `js`, `jsx`, and `mjs` map to Jira `javascript`.
- Fenced languages `bash`, `sh`, `shell`, and `zsh` map to Jira `bash`.
- Language matching is case-insensitive.
- Empty or unknown languages use an untyped code block.
- Unrecognized info-string content is never copied into Jira macro parameters.
- Code blocks with more than 20 logical content lines emit `collapse=true`; shorter blocks omit the parameter.
- A parser-provided terminal newline does not count as an additional content line.
- Known-language headers use `{code:language=<language>}` and add `|collapse=true` only for long blocks.
- Untyped blocks use `{code}` and long untyped blocks use `{code:collapse=true}`.
- No theme, line-number, border, or explicit `collapse=false` parameters are emitted.
- Inline code uses `{{...}}`, escapes Jira-sensitive delimiters, and may insert U+200B after a delimiter-sensitive or trailing backslash to protect the closing notation.

## Links and images

- Link and image destinations preserve absolute, relative, protocol-relative, and custom-scheme values accepted by the parser.
- `javascript:`, `vbscript:`, and `data:` destinations are rejected, as are control characters that cannot be safely serialized.
- Images place only the destination in the Jira image target.
- Non-empty image alternative text maps to an `alt` attribute; Markdown image titles are ignored.

## Escaping and serialization

- Every user-authored text node is escaped for its Jira output context; only renderer-generated Jira control syntax remains unescaped.
- `[x]` and `[ ]` list text serializes as `\[x\]` and `\[ \]`.
- Soft breaks serialize as one space.
- CommonMark hard breaks serialize as Jira's `\\` forced-line-break notation followed by a source newline.
- Blank lines separate paragraphs.
- Top-level blocks are separated by exactly one blank line (`\n\n`).
- List items and table rows are separated by one newline; paragraphs inside a quote are separated by one blank line.
- Canonical output adds no leading or terminal whitespace.
- Separators between blocks and list items are explicit and deterministic.
- Whitespace inside code blocks is preserved.
- Exact-output tests compare the complete output without trimming.

## Errors and mutation boundary

- Conversion returns no partial output and an error for unknown input formats or unrepresentable structures.
- The markup package error records the source line, column, node type, and reason.
- Command handling maps conversion failures to the existing `invalid_input` category and exit code 2.
- The source position is included in the human-readable message; the stable JSON error envelope remains unchanged.
- Conversion completes before any Jira mutation is attempted.

## Verification

- Port Markdown-to-Jira conversion scenarios from `jadujoel/markdown-to-jira`, but define expected output from this contract.
- Do not port its HTML preview, debug, renderer-internal, malformed-input correction, bare-autolink, or task-list-specific tests.
- Complex fixtures use complete golden output rather than partial string assertions.
- Add source-position, no-network-on-error, and race-safety coverage.
- This contract does not require live Jira rendered-output E2E tests.
