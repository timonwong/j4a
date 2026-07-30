# Provide a stable agent output contract

j4a exposes normalized, versioned JSON only when explicitly selected with `--output=json` or `-ojson`; human-readable text remains the default. Successful data is written to stdout, structured failures to stderr with stable exit codes, and `--raw` is reserved for unmodified Jira responses so agents never have to mistake Jira's version-dependent wire format for j4a's public schema.
