# j4a

`j4a` is a scriptable Jira CLI written in Go. It targets Jira Data Center and
Server first, keeps human-readable text as the default, and provides an
explicit, versioned JSON contract for automation and AI agents.

The command-line shape is inspired primarily by
[`rvben/jira-cli`](https://github.com/rvben/jira-cli), while the Go client and
Jira markup work draw on ideas from
[`ankitpokhrel/jira-cli`](https://github.com/ankitpokhrel/jira-cli). j4a does
not include a TUI.

## Status

j4a is under initial development. The v1 compatibility target is Jira Data
Center and Server REST API v2. Jira Cloud REST API v3 and ADF are not yet part
of the compatibility contract.

## Build

```sh
make build
./bin/j4a --help
```

## Login and configuration

The default config path is `$XDG_CONFIG_HOME/j4a/config.toml`, falling back to
`~/.config/j4a/config.toml`.

Run `j4a login` for the default Profile or select a named Profile with
`--profile`. Login prompts for connection fields and a fresh credential,
verifies it with Jira, and only then updates the Profile. A new Profile must
explicitly select `basic` or `pat`; j4a does not guess an authentication type.

```sh
j4a login
j4a login --profile bot
```

Credentials are stored in the OS keyring by default. Each Profile owns an
independent keyring credential. To store the credential in the TOML instead,
explicitly disable the keyring; j4a then requires the file to have mode `0600`
on Unix-like systems.

Any manually maintained TOML containing `password` or `token` is subject to the
same `0600` requirement.

```sh
j4a login --use-keyring=false
```

Remove a Profile's persisted credential without deleting its non-secret
configuration:

```sh
j4a logout
j4a logout --profile bot
```

`logout` cannot unset `J4A_PASSWORD` or `J4A_TOKEN` inherited from the shell and
will report when one of those environment credentials remains active.

The config file can also be maintained manually:

```toml
[default]
host = "https://jira.example.com"
username = "timon"
auth_type = "basic"
api_version = 2
use_keyring = true

[profiles.bot]
host = "https://jira.example.com"
auth_type = "pat"
use_keyring = true
read_only = true
```

Select a profile with `--profile bot` or `J4A_PROFILE=bot`. Configuration
precedence is CLI flags, environment variables, the selected profile, then the
default profile.

Supported environment variables include:

- `J4A_HOST`, `J4A_USERNAME`, `J4A_PROFILE`
- `J4A_AUTH_TYPE` (`basic` or `pat`)
- `J4A_PASSWORD` for Basic Auth and `J4A_TOKEN` for PAT
- `J4A_API_VERSION`, `J4A_READ_ONLY`, `J4A_USE_KEYRING`
- `J4A_CONFIG_FILE`

Secret precedence is environment, OS keyring, then TOML. When
`use_keyring = true`, a missing keyring entry is an error and j4a does not
silently fall back to a plaintext TOML secret.

For non-interactive login, provide connection fields with flags or environment
variables and provide the secret through `J4A_PASSWORD` / `J4A_TOKEN` or stdin.
Secrets are never accepted as command-line arguments.

```sh
printf '%s' "$JIRA_PAT" | j4a login \
  --profile bot \
  --host https://jira.example.com \
  --auth-type pat
```

## Commands

Resource commands use plural canonical names and singular aliases:

```text
j4a issues|issue
j4a projects|project
j4a fields|field
```

Core examples:

```sh
j4a myself
j4a issues list --project OPS --status "In Progress"
j4a issues show OPS-42
j4a search 'project = OPS AND assignee = currentUser() ORDER BY updated DESC'

j4a issues create --project OPS --type Bug --summary "Broken deployment"
j4a issues update OPS-42 --priority High --field story-points=5
j4a issues comment OPS-42 --body "Deployed to staging."
j4a issues transition OPS-42 --to "In Review"

j4a projects list
j4a fields list --custom
j4a cache fields refresh
```

## Structured output

Text is always the default. These forms are equivalent:

```sh
j4a issues list -o json
j4a issues list -ojson
j4a issues list --output json
j4a issues list --output=json
```

Normalized JSON uses a stable envelope:

```json
{"schemaVersion":"1","data":{"issues":[],"total":0,"startAt":0,"maxResults":50}}
```

Successful commands that had to accept a degraded condition add structured
warnings without changing the exit code:

```json
{"schemaVersion":"1","data":{"key":"OPS-42","updated":true},"warnings":[{"code":"stale_field_cache","message":"using stale custom field metadata","details":{"fetchedAt":"2026-07-29T10:00:00Z"}}]}
```

Successful data is written to stdout. In JSON mode, structured failures are
written to stderr and paired with stable exit codes. Human-readable warnings
are written to stderr. `--raw` bypasses j4a's schema and emits the Jira REST
response unchanged.

`j4a schema` describes commands, flags, mutation status, output shapes, and
exit codes as machine-readable JSON.

## Custom fields

Pass custom fields with repeatable `--field key=value` flags:

```sh
j4a issues create \
  --project OPS \
  --type Story \
  --summary "Agent-friendly output" \
  --field story-points=5 \
  --field customfield_10006='{"id":"123"}'
```

A direct ID such as `customfield_10006` is used as-is and has priority. It does
not call `/myself`, read the cache, or query Jira field metadata. A human alias
such as `story-points` is resolved through a 24-hour custom field metadata
snapshot scoped to the normalized Jira base URL and the Principal returned by
`/myself`; missing or ambiguous aliases remain errors. Values are decoded as
JSON first and fall back to strings.

The disposable JSON snapshots live under the platform user cache directory at
`j4a/fields/hosts/<url-slug+hash>--<principal-slug+hash>.json`. For example,
macOS uses `~/Library/Caches/j4a/fields/hosts/`. Refresh the current Principal's
snapshot explicitly with:

```sh
j4a cache fields refresh
```

An expired snapshot is refreshed before use. If Jira cannot refresh it, j4a
continues with the stale mapping, including for issue mutations, and emits a
warning. `j4a fields list --custom` uses the same snapshot; the complete
`fields list` and all `--raw` field requests remain live.

## Jira markup and Markdown

Description and comment input is Jira markup by default. Convert Markdown only
when explicitly requested:

```sh
j4a issues create \
  --project OPS \
  --type Task \
  --summary "Document rollout" \
  --description-file issue.md \
  --input-format=markdown
```

Long text accepts inline input, a file path, or `-` for stdin. Inline and file
forms are mutually exclusive.

## Development

```sh
make fmt
make check
```

The domain glossary is in [`CONTEXT.md`](CONTEXT.md), and durable design
decisions are recorded in [`docs/adr`](docs/adr).

## License

MIT
