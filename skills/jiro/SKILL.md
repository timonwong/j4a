---
name: jiro
description: Manage Jira Data Center and Server with the jiro CLI. Use when the user wants to authenticate to Jira; inspect, search, create, update, assign, comment on, link, transition, or bulk-manage issues; inspect projects, fields, or custom-field metadata; or automate Jira through jiro's stable JSON contract.
---

# Jiro

Use `jiro` as the primary interface for Jira Data Center and Server. Work through the same control loop for every mutation: **inspect -> preflight -> mutate -> read back**. Finish only when the requested Jira state is visible in a read after the write.

## Establish the contract

Run these local checks before relying on remembered syntax:

```bash
jiro --version
jiro schema --output=json
jiro COMMAND --help
```

Treat `jiro schema --output=json` as the source of truth for command names, flags, mutation status, output shapes, and exit codes. Use the singular resource namespaces `issue`, `project`, and `field`. jiro targets Jira Data Center and Server REST API v2; verify compatibility before acting on Jira Cloud or ADF data.

Select a Profile explicitly when the task names one:

```bash
jiro --profile bot auth status --output=json
```

Run `jiro auth status --output=json` before Jira work when authentication or the selected Jira Instance is uncertain. Its success proves the effective Profile and Credential are accepted without exposing the secret.

Authenticate interactively for the default or a named Profile:

```bash
jiro auth login
jiro --profile bot auth login
```

For non-interactive login, use the atomic `JIRA_*` Credential contract or an explicit stdin mode. A non-empty `JIRA_TOKEN` selects PAT; otherwise `JIRA_USERNAME` and `JIRA_PASSWORD` must both be non-empty. `--password-stdin` requires `JIRA_USERNAME`, while `--token-stdin` selects PAT. Keep credentials out of command-line arguments:

```bash
printf '%s' "$JIRA_PAT" | JIRA_HOST=https://jira.example.com jiro --profile bot auth login --token-stdin --output=json
```

The OS keyring is the default credential store and each Profile owns an independent credential. Use `auth login --use-keyring=false` only when plaintext TOML storage is explicitly intended; jiro enforces mode `0600` on Unix-like systems. Remove only the selected Profile's persisted credential with `jiro --profile bot auth logout --output=json`, then inspect `environmentCredentialActive` because logout cannot remove credentials inherited from the shell.

## Inspect

Read the current state and the metadata that constrains the requested write. Use normalized JSON for automation and precise verification.

```bash
jiro issue show OPS-42 --output=json
jiro issue comment list OPS-42 --output=json
jiro issue list-transitions OPS-42 --output=json
jiro issue link list OPS-42 --output=json
jiro issue link types --output=json
jiro project show OPS --output=json
jiro field list --custom --output=json
```

For searches, use the structured filters when they express the request and JQL when they do not:

```bash
jiro issue list --project OPS --status "In Progress" --assignee me --output=json
jiro issue list --resolution unresolved --label agent --component API --all --output=json
jiro search 'project = OPS AND updated >= -7d ORDER BY updated DESC' --all --output=json
```

Confirm every target Issue Key, Jira Instance, current status, current assignee, relevant field value, transition, Link Type, and existing comment or link needed to judge the final state. An empty list is valid unless the user's request requires a match.

## Preflight

Resolve Jira-owned names and workflow choices before writing:

- Match a transition by the exact ID, unique name, or unique destination status returned by `issue list-transitions`.
- Resolve a Link Type with `issue link types`; preserve the outward direction from `FROM` to `--to`.
- Use `customfield_N` directly when its ID is known. Use a human alias only after `field list --custom` or `cache refresh` proves it is unique for the current Principal.
- Determine whether description or comment input is Jira Markup or Markdown Input. The default is Jira Markup; request conversion explicitly with `--input-format=markdown`.
- Use a file or `-` for stdin for long text. Keep inline and file forms mutually exclusive.
- Resolve write-time Sprint input as a numeric ID, `active`, or a case-insensitive name substring. Check that the first match in Jira board/page order is the intended Sprint.
- Treat `--component` and `--fix-version` on updates as full replacements. Use the single value `none` only when the requested final state is an empty field.

For bulk changes, preflight the complete JQL selection without changing Jira:

```bash
jiro issue bulk move --jql 'project = OPS AND status = Open' --to "In Progress" --dry-run --output=json
jiro issue bulk assign --jql 'project = OPS' --assignee me --dry-run --output=json
```

Review every returned item. Proceed only when the selection, targets, `ready` count, and any failures match the user's intended scope.

## Mutate

Use only the operation and fields the user requested.

### Create or update an issue

```bash
jiro issue add \
  --project OPS \
  --type Bug \
  --summary "Broken deployment" \
  --description-file issue.md \
  --input-format=markdown \
  --component API \
  --fix-version 4.5 \
  --field story-points=5 \
  --output=json

jiro issue update OPS-42 \
  --priority High \
  --component API \
  --fix-version 4.5 \
  --output=json
```

`--field key=value` decodes the value as JSON first and otherwise uses a string. Quote object and array values so the shell passes valid JSON. When `issue add --sprint` creates the issue but cannot move it, preserve the returned Issue Key and report the partial failure; the created issue remains in Jira. An ordinary `issue update` followed by a failed Sprint move can likewise leave the ordinary fields updated.

### Comment, transition, assign, or link

```bash
jiro issue comment add OPS-42 --body-file comment.md --input-format=markdown --output=json
jiro issue move OPS-42 --to "Start Review" --field story-points=5 --output=json
jiro issue assign OPS-42 --assignee me --output=json
jiro issue assign OPS-42 --assignee none --output=json
jiro issue link add OPS-42 --to OPS-99 --type Blocks --output=json
jiro issue link delete 10001 --output=json
```

Use the transition identifier proven during preflight, even when another transition has a similar label. Delete an Issue Link by its Jira Link ID, not by either Issue Key.

### Execute a preflighted bulk change

After the user has authorized the actual broad write, repeat the same JQL and target with `--yes`:

```bash
jiro issue bulk move --jql 'project = OPS AND status = Open' --to "In Progress" --yes --output=json
jiro issue bulk assign --jql 'project = OPS' --assignee me --yes --output=json
```

Keep the dry-run result and execution result distinct. Bulk writes run serially and may return `failed` or `not_attempted` outcomes.

## Read back

Read every consequential result through jiro after the write:

- Use `issue show` for creation, field updates, transitions, assignments, and Sprint membership fields returned by the instance.
- Use `issue comment list` for added comments.
- Use `issue link list` for added or removed links.
- Retain the Issue Keys from a bulk dry-run and read back every targeted issue after execution; use a list/search read with the needed `--fields` only when it proves the same complete key set and final values.
- Read normalized standard fields such as status and assignee from `.data.status` and `.data.assignee`. Request custom fields with `issue show --fields` and read them from `.data.fields`.

A zero exit code is not the completion criterion; the requested state in the readback is. Treat a silently absent field or an unintended destination status as incomplete work.

## Interpret output and failures

Text is the default. Use `--output=json` for agent or script consumption. Successful normalized data goes to stdout; structured errors go to stderr. Capture the streams separately.

The stable exit codes are:

- `0`: success
- `1`: unexpected error
- `2`: invalid input or configuration
- `3`: authentication failed
- `4`: resource not found
- `5`: Jira API error
- `6`: rate limited
- `7`: partial failure

Warnings describe a degraded success and do not change exit code `0`. On exit code `7`, jiro writes the complete normalized result to stdout before the structured error to stderr. Preserve and report both, including every succeeded, failed, and unattempted item. Typed commands expose no general raw-output mode; normalized JSON is their automation contract. `jiro api` is the sole raw HTTP exception.

Pause after permission errors, missing or ambiguous metadata, rate limiting, or uncertain output. Determine whether Jira applied the write before retrying.

## Refresh custom-field metadata

Human aliases use a disposable snapshot scoped to the normalized Jira Instance and authenticated Principal. Refresh it when aliases are stale, missing, ambiguous, or workflow-sensitive:

```bash
jiro cache refresh --output=json
jiro field list --custom --output=json
```

An expired snapshot may be used with a `stale_field_cache` warning when refresh fails. Treat that warning as a reason to disclose the risk and verify the written field directly. Direct `customfield_N` IDs bypass alias metadata.

## Use a bounded REST fallback

Use `jiro api` only after `jiro schema --output=json` and the relevant typed-command help prove that jiro lacks the requested operation. State that the fallback returns Jira's raw, version-dependent response rather than jiro's normalized contract.

Before sending a REST request:

1. Identify the installed Jira product and version, then consult the matching authoritative [Jira Data Center REST API documentation](https://developer.atlassian.com/server/jira/platform/about-the-jira-server-rest-apis/). Use the Jira Software API documentation for software-specific resources.
2. Reuse the selected Profile, or use the atomic environment contract: non-empty `JIRA_TOKEN`, otherwise both `JIRA_USERNAME` and `JIRA_PASSWORD`. `JIRA_HOST` may independently override the Jira Instance.
3. Use `jiro api` so credentials are read at execution time and Authorization remains managed. Keep xtrace and verbose header logging disabled, and keep Authorization values out of command arguments and output.
4. Use the configured HTTPS Jira base URL, including any context path, with normal certificate verification. Send PATs as Bearer credentials and Basic credentials according to Atlassian's [PAT](https://developer.atlassian.com/server/jira/platform/personal-access-token/) and [Basic Auth](https://developer.atlassian.com/server/jira/platform/basic-authentication/) guidance.
5. Query endpoint and field metadata before a write, send the smallest payload that satisfies the request, and read the changed resource back through REST afterward.

Treat Jira's response status and body as evidence, not as a stable jiro envelope. Stop and report the verified boundary when the endpoint, authentication path, required metadata, or final state cannot be established. Keep credentials in their provided environment source; leave jiro's TOML and OS keyring untouched. Keep the workflow jiro-first and REST-second rather than switching to another Jira CLI or browser UI.
