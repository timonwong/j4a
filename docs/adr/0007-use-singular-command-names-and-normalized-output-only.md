# Use singular command names and normalized output only

jiro uses singular resource namespaces without compatibility aliases: `issue`, `project`, and `field`. Related Issue actions are grouped under `comment`, `link`, and `bulk`; issue creation is `issue add`, workflow transitions use `issue move`, and Sprint assignment is expressed through `issue add --sprint` or `issue update --sprint`. The CLI exposes only human-readable text and normalized schema-versioned JSON, so `--raw` and raw Jira wire responses are removed from the public contract. These changes replace the earlier plural-command and raw-output decisions in place while `contractVersion` remains `3` and the JSON envelope `schemaVersion` remains `1`.

The normalized-output-only decision is superseded for `jiro api` only by [ADR 0009](0009-add-an-authenticated-raw-jira-api-command.md). Singular grammar and typed command output remain unchanged.
