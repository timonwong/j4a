# jiro

`jiro` is a scriptable Jira CLI written in Go. It targets Jira Data Center and
Server first, keeps human-readable text as the default, and provides an
explicit, versioned JSON contract for automation and AI agents.

The command-line shape is inspired primarily by
[`rvben/jira-cli`](https://github.com/rvben/jira-cli), while the Go client and
Jira markup work draw on ideas from
[`ankitpokhrel/jira-cli`](https://github.com/ankitpokhrel/jira-cli). jiro does
not include a TUI.

## Status

jiro is under initial development. The v1 compatibility target is Jira Data
Center and Server REST API v2. Jira Cloud REST API v3 and ADF are not yet part
of the compatibility contract.

## Build

```sh
make build
./bin/jiro --help
```

## Install from a release

[GitHub Releases](https://github.com/timonwong/jiro/releases) provides standalone
binaries for Linux, macOS, and Windows on amd64 and arm64. Download the asset
matching your operating system and architecture. On Linux and macOS, make it
executable and optionally rename it:

```sh
chmod +x jiro_v0.1.0_darwin_arm64
mv jiro_v0.1.0_darwin_arm64 jiro
./jiro --version
```

Each release also includes a SHA-256 checksum file. Verify the matching entry
before installing the binary. For example, on macOS:

```sh
grep 'jiro_v0.1.0_darwin_arm64$' jiro_v0.1.0_checksums.txt | shasum -a 256 -c -
```

On Linux, use `sha256sum -c -` in place of `shasum -a 256 -c -`.

## Authentication and configuration

The default config path is `$XDG_CONFIG_HOME/jiro/config.toml`, falling back to
`~/.config/jiro/config.toml`.

Run `jiro auth login` for the default Profile or select a named Profile with
`--profile`. Login prompts for connection fields and a fresh credential,
verifies it with Jira, and only then updates the Profile. A new Profile must
explicitly select `basic` or `pat`; jiro does not guess an authentication type.

```sh
jiro auth login
jiro auth login --profile bot
```

Credentials are stored in the OS keyring by default. Each Profile owns an
independent keyring credential. To store the credential in the TOML instead,
explicitly disable the keyring; jiro then requires the file to have mode `0600`
on Unix-like systems.

Any manually maintained TOML containing `password` or `token` is subject to the
same `0600` requirement.

```sh
jiro auth login --use-keyring=false
```

Remove a Profile's persisted credential without deleting its non-secret
configuration:

```sh
jiro auth logout
jiro auth logout --profile bot
```

`auth logout` cannot unset `JIRA_PASSWORD` or `JIRA_TOKEN` inherited from the
shell and will report when one of those environment credentials remains active.

Verify the effective Profile and Credential against Jira with
`/rest/api/2/myself`:

```sh
jiro auth status
jiro auth status --profile bot
```

`auth status` exits successfully only when Jira accepts the effective
Credential. Its normalized output includes the Profile, Jira Instance,
authentication type, and authenticated user without exposing the secret or its
storage source.

The config file can also be maintained manually:

```toml
[default]
host = "https://jira.example.com"
username = "timon"
auth_type = "basic"
api_version = 2
use_keyring = true
user_agent = "jiro-automation/1"

[profiles.bot]
host = "https://jira.example.com"
auth_type = "pat"
use_keyring = true
read_only = true
user_agent = "jiro-bot/1"
```

Select a Profile with `--profile bot` or `JIRO_PROFILE=bot`. `--config` and
`--profile` are the only global config-selection flags; connection and
authentication overrides use environment variables.

Supported environment variables include:

- `JIRA_HOST`, `JIRA_API_VERSION`, `JIRA_USER_AGENT`
- `JIRA_TOKEN` for PAT, or the atomic `JIRA_USERNAME` and `JIRA_PASSWORD` pair
- `JIRO_PROFILE`, `JIRO_CONFIG_FILE`, `JIRO_CONFIG`
- `JIRO_READ_ONLY`, `JIRO_USE_KEYRING`
- `JIRO_FORCE_TTY` for forcing terminal-style tables through a pipe

A non-empty `JIRA_TOKEN` selects PAT and ignores the Basic variables. Without a
token, setting either `JIRA_USERNAME` or `JIRA_PASSWORD` requires both to be
non-empty and selects Basic Auth. If no environment Credential is present, jiro
uses the selected Profile's complete Credential; environment and persisted
Credential halves are never combined. `JIRA_HOST` is independent and can
override the Jira Instance while the selected Profile still supplies its
Credential. The legacy `JIRO_HOST`, `JIRO_API_VERSION`, `JIRO_TOKEN`,
`JIRO_USERNAME`, `JIRO_PASSWORD`, and `JIRO_AUTH_TYPE` variables are ignored.

`JIRA_USER_AGENT` overrides the selected Profile's `user_agent`, which inherits
from `[default]` when omitted. The built-in fallback is `jiro/<version>` or
`jiro/dev` for development builds. When `use_keyring = true`, a missing keyring
entry is an error and jiro does not silently fall back to a plaintext TOML
secret.

For non-interactive login, provide a complete environment Credential, or use an
explicit stdin mode. `--password-stdin` requires `JIRA_USERNAME`;
`--token-stdin` selects PAT and ignores the username. The two modes are mutually
exclusive and conflict with non-empty `JIRA_TOKEN` or `JIRA_PASSWORD`. Each mode
reads to EOF, removes exactly one trailing LF and an immediately preceding CR,
and rejects only an empty result. Secrets are never accepted as command-line
arguments.

```sh
printf '%s' "$JIRA_PAT" | JIRA_HOST=https://jira.example.com \
  jiro --profile bot auth login --token-stdin

printf '%s' "$JIRA_PASSWORD" | JIRA_HOST=https://jira.example.com \
  JIRA_USERNAME=timon jiro auth login --password-stdin
```

## Commands

Resource namespaces use singular canonical names without aliases:

```text
jiro issue
jiro project
jiro field
```

Core examples:

```sh
jiro issue list --project OPS --status "In Progress"
jiro issue list --resolution unresolved --reporter me --label agent --component API
jiro issue list --sprint active --created -7d --updated 'startOfWeek()'
jiro issue show OPS-42
jiro search 'project = OPS AND assignee = currentUser() ORDER BY updated DESC'

jiro issue add --project OPS --type Bug --summary "Broken deployment" \
  --parent OPS-10 --component API --fix-version 4.5 --sprint active
jiro issue update OPS-42 --priority High --component API --fix-version 4.5
jiro issue update OPS-42 --component none --sprint active
jiro issue comment add OPS-42 --body "Deployed to staging."
jiro issue move OPS-42 --to "In Review"
jiro issue assign OPS-42 --assignee me

jiro issue link add OPS-42 --to OPS-99 --type Blocks
jiro issue link list OPS-42
jiro issue link delete 10001
jiro issue link types

jiro project list
jiro field list --custom
jiro cache refresh
```

Issue list filters are combined with `AND`; repeated `--label`, `--component`,
and `--fix-version` values use JQL `IN`. `--resolution unresolved` means no
resolution; `--assignee me` and `--reporter me` use Jira's `currentUser()`.
For `issue list`, Sprint accepts `active`/`open`, `closed`, `future`, an ID,
or a name. An
absolute `--created`/`--updated` date (`YYYY-MM-DD` or `YYYY/MM/DD`) selects
that whole day; relative values such as `-7d` and allowlisted Jira date
functions such as `startOfWeek()` select values on or after that operand.

`issue add --sprint` creates the issue first and then moves it; a failed move
never deletes the newly created issue. `issue update --sprint` resolves the
Sprint before writing, updates ordinary fields first, and then moves the Issue;
a failed Sprint move after an ordinary field update is reported as a partial
failure. `issue update --component` and
`--fix-version` replace the full field, while a single `none` clears it.
Write-time Sprint specs accept a numeric ID, `active`, or a case-insensitive
name substring and use the first match in Jira board/page order.

Use JQL-only selection for bulk operations. Exactly one of `--dry-run` and
`--yes` is required; `--dry-run` preflights every matching issue, while
`--yes` performs serial writes without prompting. Bulk commands page through
all matches by default.

```sh
jiro issue bulk move --jql 'project = OPS AND status = Open' --to "In Progress" --dry-run
jiro issue bulk assign --jql 'project = OPS' --assignee me --yes
```

## Raw Jira API requests

`jiro api` sends one authenticated HTTP request to a complete relative endpoint
of the effective Jira Instance. It reuses the selected Profile, Credential,
authentication type, connection overrides, and any configured context path such
as `/jira`; it does not call `/rest/api/2/myself` first or automatically add a
`/rest/api/2/` prefix.

```sh
jiro api rest/api/2/myself
jiro api 'rest/api/2/search?jql=project%20%3D%20OPS'
jiro api rest/api/2/issue -F 'fields={"project":{"key":"OPS"},"summary":"Example","issuetype":{"name":"Task"}}'
jiro api rest/api/2/issue/OPS-42 --method PATCH --input update.json
jiro api rest/api/2/issue/OPS-42/attachments \
  --form file=@artifact.zip \
  -H 'X-Atlassian-Token: no-check'
```

The endpoint may start with `/` but must remain relative to the Jira Instance.
Absolute and scheme-relative URLs are rejected. Other URL syntax, including dot
segments, fragments, backslashes, controls, and encoding depth, is not
prevalidated; URL construction and Go's HTTP stack report any resulting error.
Supplying fields or a body changes the implicit method from GET to POST. An
explicit method is trimmed, uppercased, and passed to Go without an allowlist.
A `read_only` Profile permits only GET, HEAD, and OPTIONS, without an override.

Request body modes are:

- `--input FILE|-` streams bytes unchanged. With `--input`, fields are appended
  to the query string.
- `-f, --string-field key=value` always creates a JSON string.
- `-F, --field key=value` decodes a complete JSON value and otherwise uses a
  string; `@file` and `@-` read the value from a file or stdin. Fields form a
  top-level JSON object and use command-line last-wins order for duplicate body
  keys. GET, HEAD, and OPTIONS place fields in the query string instead;
  `--input` and `--form` may still provide bodies for those explicit methods.
- `--form key=value` creates multipart form data. `@file` and `@-` create file
  parts, while `@@text` sends the text `@text`. Form mode is mutually exclusive
  with the other body modes.

Use repeatable `-H, --header 'Name: value'` for request headers. Authorization,
Proxy-Authorization, Host, Content-Length, and User-Agent are always managed by
jiro; Content-Type is additionally managed in form mode. Other names and values
are passed to Go's HTTP stack without prevalidation. Repeated values for one
name retain command-line order, an empty value deletes all earlier values, and
later values may add the header again. Defaults are `Accept: application/json`
and `Content-Type: application/json` when a non-form body exists.

Go's standard compression behavior applies. When the Transport adds gzip, it
transparently decompresses the response. Supplying `Accept-Encoding` explicitly
leaves the response bytes to the caller without an additional decompression
layer.

Responses are raw Jira-owned bytes: any 2xx body is streamed unchanged to
stdout, while a non-2xx body is streamed unchanged to stderr and returns the
normal status-based exit code. `--include` prepends the protocol, status, and
sorted response headers visible after Go's normal response processing;
`Set-Cookie` values are not redacted. Global `--quiet` suppresses only a
successful body; global `--output` is unsupported for `api`. There is no jq or
template formatting, pagination, cache, application-level retry, verbose trace,
or output-file flag.

The default timeout is 30 seconds and `--timeout 0` disables it. Redirects are
never followed; every 3xx response is returned as a raw non-2xx API error. API
Requests use HTTP/1.1. `--insecure` disables TLS certificate and hostname
verification for this invocation only. This is unsafe and should be reserved
for explicitly trusted Jira Instances with a known certificate problem; it is
a silent no-op for HTTP Instances.

Transport access to arbitrary plugin or version paths does not add Jira Cloud
REST API v3 support. Jira Data Center and Server REST API v2 remain jiro's
canonical compatibility target.

## Shell completion

`jiro` uses Cobra's native completion support for Bash, Zsh, Fish, and
PowerShell. Generate and load the script for the current session with one of:

```sh
# Bash
source <(jiro completion bash)

# Zsh (requires compinit)
source <(jiro completion zsh)

# Fish
jiro completion fish | source

# PowerShell
jiro completion powershell | Out-String | Invoke-Expression
```

For normal use, generate the script once into the shell's completion directory
instead of running `jiro` during every shell startup. For example:

```sh
mkdir -p ~/.zsh/completions
jiro completion zsh > ~/.zsh/completions/_jiro
# Add ~/.zsh/completions to fpath before running compinit in ~/.zshrc.

mkdir -p ~/.config/fish/completions
jiro completion fish > ~/.config/fish/completions/jiro.fish
```

Completion covers commands, flags, enum values, local input paths,
and named Profiles from the selected config file. It does not contact Jira or
read credentials from the OS keyring.

## Structured output

Text is always the default. These forms are equivalent:

```sh
jiro issue list -o json
jiro issue list -ojson
jiro issue list --output json
jiro issue list --output=json
```

For commands with tabular text output, jiro adapts to stdout:

- A terminal receives a column-aligned table sized to the available width.
  Dedicated Issue Key, ID, Alias, and other fixed columns remain complete;
  descriptive columns and mixed detail-table `VALUE` columns may be truncated
  with `...`.
- A pipe or file receives headerless TSV with untruncated single-line values.
  An empty result writes zero bytes.

Table cells are rendered as safe single-line text. JSON retains the original
values and remains the automation contract. To keep terminal-style output in a
pipeline, set `JIRO_FORCE_TTY` to `1`, `true`, `yes`, or `on`:

```sh
JIRO_FORCE_TTY=1 jiro issue list --project OPS | less
```

The exact values `0`, `false`, `no`, and `off` leave normal terminal detection
in place. Any other value is a configuration error for text output. When
forced, jiro reads the controlling terminal width and falls back to 80 columns
if it cannot be determined.

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
are written to stderr. Typed commands expose only text and normalized JSON;
`jiro api` is the explicit raw HTTP exception documented above. For a partial result (exit
code `7`), jiro first writes the complete normalized result to stdout, then
writes a structured
`partial_failure` error to stderr. This includes a successful issue creation
whose requested Sprint move failed and bulk runs with failed or unattempted
items.

`jiro schema` describes automation-facing Jira commands, flags, mutation
status, output shapes, and exit codes as machine-readable JSON. Shell
completion commands emit shell code and are outside that contract.

## Custom fields

Pass custom fields with repeatable `--field key=value` flags:

```sh
jiro issue add \
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

The disposable JSON snapshots live under `github.com/adrg/xdg`'s
`xdg.CacheHome` at
`jiro/fields/hosts/<url-slug+hash>--<principal-slug+hash>.json`. This honors the
platform XDG cache location (including `XDG_CACHE_HOME` where applicable);
macOS defaults to `~/Library/Caches/jiro/fields/hosts/`. There is no jiro-specific
cache environment variable. Refresh the current Principal's snapshot explicitly
with:

```sh
jiro cache refresh
```

An expired snapshot is refreshed before use. If Jira cannot refresh it, jiro
continues with the stale mapping, including for issue mutations, and emits a
warning. `jiro field list --custom` uses the same snapshot; the complete
`field list` remains live.

## Jira markup and Markdown

Description and comment input is Jira markup by default. Convert Markdown only
when explicitly requested:

```sh
jiro issue add \
  --project OPS \
  --type Task \
  --summary "Document rollout" \
  --description-file issue.md \
  --input-format=markdown
```

The accepted Markdown Input dialect and exact conversion behavior are documented
in [`docs/markdown-input.md`](docs/markdown-input.md).

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
