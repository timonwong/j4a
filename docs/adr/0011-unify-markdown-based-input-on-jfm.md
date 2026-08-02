---
status: accepted
---

# Unify Markdown-based input and conversion on JFM

jiro retires Markdown Input as an independent contract and uses Jiro Flavored Markdown (JFM) as its sole Markdown-based format. `jfm` is the canonical `--input-format` value and `markdown` is its permanent, warning-free alias; both use the bidirectional JFM engine, while `jira` continues to pass Jira Markup through unchanged. The legacy `ToJira` conversion path and its separate specification are removed so mutations and standalone conversion cannot drift semantically.

Offline conversion is exposed as `jiro jfm to-jira [FILE|-]` and `jiro jfm from-jira [FILE|-]`. These commands require no Jira configuration, Credential, or network access; default text output is the exact converted document, while JSON uses the existing envelope with `jiraMarkup` or `jfm` data and structured conversion warnings. Typed Issue and Comment commands continue to return their existing Jira Markup fields without JFM projection flags or sibling fields.
