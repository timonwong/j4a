# Testing

Repository-wide testdata conventions for implementation and review work.

## Golden testdata

- Write new golden testdata as `txtar` archives instead of ad hoc JSON fixtures or separate input/output files.
- Keep one independently understandable golden case in each archive.
- Parse archives with `golang.org/x/tools/txtar`.
- Do not mechanically migrate unrelated existing golden fixtures while implementing a feature; keep format-only migrations separately scoped.

For Jira Markup to Jiro Flavored Markdown fixtures, every archive contains all three sections, including an explicit empty warning list:

```text
-- input.jira --
h1. Example

-- want.md --
# Example

-- warnings.json --
[]
```

For Jiro Flavored Markdown to Jira Markup fixtures, use the corresponding section names and likewise include all three sections:

```text
-- input.md --
# Example

-- want.jira --
h1. Example

-- warnings.json --
[]
```

## JFM specification examples

Normative examples in `docs/jiro-flavored-markdown.md` are executable conformance cases. Each example has a stable `jfm-spec-example` marker, a direction, and exact `Input`, `Output`, and `Warnings` fences. Keep examples focused on format rules visible to users; parser, renderer, API, and test-harness details do not belong in the specification.

Use `txtar` golden fixtures for exhaustive edge cases. A specification example and a golden may cover the same rule when the example explains a core language guarantee and the golden protects additional boundary detail.
