# Provide a stable agent output contract

jiro exposes normalized, versioned JSON only when explicitly selected with `--output=json` or `-ojson`; human-readable text remains the default. Successful data is written to stdout, structured failures to stderr with stable exit codes, and `--raw` is reserved for unmodified Jira responses so agents never have to mistake Jira's version-dependent wire format for jiro's public schema. During initial development the schema remains version `1` while its success envelope may gain breaking fields such as structured warnings; text warnings go to stderr without changing the command's successful exit code.

The `--raw` portion of this decision is superseded by [ADR 0007](0007-use-singular-command-names-and-normalized-output-only.md).
