# Use Goldmark for JFM and a dedicated Jira parser

JFM-to-Jira conversion continues to parse CommonMark through Goldmark plus jiro-owned JFM extensions, while Jira-to-JFM conversion uses a purpose-built Jira Markup parser. Both parser outputs adapt into a thin normalized semantic model with source spans before target serialization, so canonicalization and warning rules are shared without forcing Jira notation into Goldmark's CommonMark source-segment model. The existing Markdown Input `ToJira` implementation remains independent and unchanged.
