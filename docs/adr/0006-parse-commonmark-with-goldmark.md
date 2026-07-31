# Parse CommonMark with Goldmark

Markdown Input is parsed with Goldmark's CommonMark core plus only the table and strikethrough extensions, then rendered by a jiro-owned Jira Markup renderer. jiro replaces both Blackfriday dependencies and does not preprocess or repair source text, so the public input contract is independent of legacy parser and renderer behavior.
