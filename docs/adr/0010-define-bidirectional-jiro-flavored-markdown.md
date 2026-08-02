---
status: superseded by ADR-0011
---

# Define bidirectional Jiro Flavored Markdown

jiro defines Jiro Flavored Markdown (JFM) as a complete bidirectional format between Markdown and Jira Markup. Its normative syntax and conversion semantics live only in `docs/jiro-flavored-markdown.md`; this ADR records the implementation boundary rather than duplicating the format specification.

JFM remains independent from Markdown Input and leaves the existing `ToJira` behavior unchanged. JFM accepts the Markdown Input language as a semantic subset, then adds reversible Jira-specific directives and richer preservation where Jira Markup can represent the information. JFM-to-Jira uses the repository's CommonMark pipeline with jiro-owned extensions, Jira-to-JFM uses a dedicated Jira Markup parser, and both directions share a normalized semantic model with source spans.

Recognized structures use best-effort semantic conversion: representable content continues through conversion and unrepresentable information is discarded with deterministic warnings. Literal fallback is reserved for constructs whose semantic boundaries cannot be trusted, including unknown or malformed directives and malformed controlled HTML. This distinction keeps formatting intact without interpreting unsafe input speculatively.
