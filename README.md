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

`auth logout` cannot unset `JIRO_PASSWORD` or `JIRO_TOKEN` inherited from the
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

[profiles.bot]
host = "https://jira.example.com"
auth_type = "pat"
use_keyring = true
read_only = true
```

Select a profile with `--profile bot` or `JIRO_PROFILE=bot`. Configuration
precedence is CLI flags, environment variables, the selected profile, then the
default profile.

Supported environment variables include:

- `JIRO_HOST`, `JIRO_USERNAME`, `JIRO_PROFILE`
- `JIRO_AUTH_TYPE` (`basic` or `pat`)
- `JIRO_PASSWORD` for Basic Auth and `JIRO_TOKEN` for PAT
- `JIRO_API_VERSION`, `JIRO_READ_ONLY`, `JIRO_USE_KEYRING`
- `JIRO_CONFIG_FILE`

Secret precedence is environment, OS keyring, then TOML. When
`use_keyring = true`, a missing keyring entry is an error and jiro does not
silently fall back to a plaintext TOML secret.

For non-interactive login, provide connection fields with flags or environment
variables and provide the secret through `JIRO_PASSWORD` / `JIRO_TOKEN` or
stdin. Secrets are never accepted as command-line arguments.

```sh
printf '%s' "$JIRA_PAT" | jiro auth login \
  --profile bot \
  --host https://jira.example.com \
  --auth-type pat
```

## Commands

Resource commands use plural canonical names and singular aliases:

```text
jiro issues|issue
jiro projects|project
jiro fields|field
```

Core examples:

```sh
jiro myself
jiro issues list --project OPS --status "In Progress"
jiro issues list --resolution unresolved --reporter me --label agent --component API
jiro issues list --sprint active --created -7d --updated 'startOfWeek()'
jiro issues show OPS-42
jiro search 'project = OPS AND assignee = currentUser() ORDER BY updated DESC'

jiro issues create --project OPS --type Bug --summary "Broken deployment" \
  --parent OPS-10 --component API --fix-version 4.5 --sprint active
jiro issues update OPS-42 --priority High --component API --fix-version 4.5
jiro issues update OPS-42 --component none  # clear all components
jiro issues comment OPS-42 --body "Deployed to staging."
jiro issues transition OPS-42 --to "In Review"
jiro issues assign OPS-42 --assignee me
jiro issues move OPS-42 --sprint active

jiro issues link OPS-42 --to OPS-99 --type Blocks
jiro issues links OPS-42
jiro issues unlink 10001

jiro projects list
jiro fields list --custom
jiro cache fields refresh
```

Issue list filters are combined with `AND`; repeated `--label`, `--component`,
and `--fix-version` values use JQL `IN`. `--resolution unresolved` means no
resolution; `--assignee me` and `--reporter me` use Jira's `currentUser()`.
For `issues list`, Sprint accepts `active`/`open`, `closed`, `future`, an ID,
or a name. An
absolute `--created`/`--updated` date (`YYYY-MM-DD` or `YYYY/MM/DD`) selects
that whole day; relative values such as `-7d` and allowlisted Jira date
functions such as `startOfWeek()` select values on or after that operand.

`issues create --sprint` creates the issue first and then moves it; a failed
move never deletes the newly created issue. `issues update --component` and
`--fix-version` replace the full field, while a single `none` clears it.
Write-time Sprint specs accept a numeric ID, `active`, or a case-insensitive
name substring and use the first match in Jira board/page order.

Use JQL-only selection for bulk operations. Exactly one of `--dry-run` and
`--yes` is required; `--dry-run` preflights every matching issue, while
`--yes` performs serial writes without prompting. Bulk commands page through
all matches by default.

```sh
jiro issues bulk-transition --jql 'project = OPS AND status = Open' --to "In Progress" --dry-run
jiro issues bulk-assign --jql 'project = OPS' --assignee me --yes
```

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

Completion covers commands, aliases, flags, enum values, local input paths,
and named Profiles from the selected config file. It does not contact Jira or
read credentials from the OS keyring.

## Structured output

Text is always the default. These forms are equivalent:

```sh
jiro issues list -o json
jiro issues list -ojson
jiro issues list --output json
jiro issues list --output=json
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
are written to stderr. `--raw` bypasses jiro's schema and emits the Jira REST
response unchanged. For a partial result (exit code `7`), jiro first writes the
complete normalized result to stdout, then writes a structured
`partial_failure` error to stderr. This includes a successful issue creation
whose requested Sprint move failed and bulk runs with failed or unattempted
items.

`--raw` is unavailable for `issues create --sprint`, `issues bulk-transition`,
and `issues bulk-assign`; these composite commands must retain normalized
partial-result behavior. Single-request issue actions retain their raw Jira
REST response behavior.

`jiro schema` describes automation-facing Jira commands, flags, mutation
status, output shapes, and exit codes as machine-readable JSON. Shell
completion commands emit shell code and are outside that contract.

## Custom fields

Pass custom fields with repeatable `--field key=value` flags:

```sh
jiro issues create \
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
`jiro/fields/hosts/<url-slug+hash>--<principal-slug+hash>.json`. For example,
macOS uses `~/Library/Caches/jiro/fields/hosts/`. Refresh the current Principal's
snapshot explicitly with:

```sh
jiro cache fields refresh
```

An expired snapshot is refreshed before use. If Jira cannot refresh it, jiro
continues with the stale mapping, including for issue mutations, and emits a
warning. `jiro fields list --custom` uses the same snapshot; the complete
`fields list` and all `--raw` field requests remain live.

## Jira markup and Markdown

Description and comment input is Jira markup by default. Convert Markdown only
when explicitly requested:

```sh
jiro issues create \
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
