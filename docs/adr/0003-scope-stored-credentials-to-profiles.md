# Scope stored credentials to profiles

jiro stores each credential under its Profile rather than sharing one keyring entry by Jira instance, username, and authentication type. This makes `auth login --profile` and `auth logout --profile` isolated and predictable even when multiple Profiles address the same Jira identity; because jiro is still pre-v1, the old identity-scoped keyring keys are not migrated or used as a fallback, and each Profile must log in again after this change.
