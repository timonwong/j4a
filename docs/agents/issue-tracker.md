# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues in `timonwong/jiro`. Use the `gh` CLI for all operations.

The `origin` remote points to GitHub, so `gh` can infer the repository in this
clone. The explicit `--repo timonwong/jiro` forms below remain suitable for
commands run outside the clone.

## Conventions

- **Create an issue**: `gh issue create --repo timonwong/jiro --title "..." --body "..."`. Use a heredoc for multi-line bodies.
- **Read an issue**: `gh issue view <number> --repo timonwong/jiro --comments`, filtering comments with `jq` and also fetching labels when needed.
- **List issues**: `gh issue list --repo timonwong/jiro --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'` with appropriate `--label` and `--state` filters.
- **Comment on an issue**: `gh issue comment <number> --repo timonwong/jiro --body "..."`
- **Apply / remove labels**: `gh issue edit <number> --repo timonwong/jiro --add-label "..."` / `--remove-label "..."`
- **Close**: `gh issue close <number> --repo timonwong/jiro --comment "..."`

## Pull requests as a triage surface

**PRs as a request surface: no.** _(Set to `yes` if this repo treats external PRs as feature requests; `/triage` reads this flag.)_

When set to `yes`, PRs run through the same labels and states as issues, using the `gh pr` equivalents:

- **Read a PR**: `gh pr view <number> --repo timonwong/jiro --comments` and `gh pr diff <number> --repo timonwong/jiro`.
- **List external PRs for triage**: `gh pr list --repo timonwong/jiro --state open --json number,title,body,labels,author,authorAssociation,comments`, then keep only `authorAssociation` values of `CONTRIBUTOR`, `FIRST_TIME_CONTRIBUTOR`, or `NONE`.
- **Comment / label / close**: use `gh pr comment`, `gh pr edit --add-label`/`--remove-label`, and `gh pr close`.

GitHub shares one number space across issues and PRs, so a bare `#42` may be either. Resolve it with `gh pr view 42 --repo timonwong/jiro` and fall back to `gh issue view 42 --repo timonwong/jiro`.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill says "fetch the relevant ticket"

Run `gh issue view <number> --repo timonwong/jiro --comments`.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single issue with **child** issues as tickets.

- **Map**: a single issue labelled `wayfinder:map`, holding the Notes / Decisions-so-far / Fog body.
- **Child ticket**: an issue linked to the map as a GitHub sub-issue. Where sub-issues aren't enabled, add the child to a task list in the map body and put `Part of #<map>` at the top of the child body. Labels use `wayfinder:<type>` (`research`, `prototype`, `grilling`, or `task`). Once claimed, assign the ticket to the driving developer.
- **Blocking**: use GitHub's native issue dependencies. Add an edge with `gh api --method POST repos/timonwong/jiro/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`, where `<blocker-db-id>` is the blocker's numeric database ID. Where dependencies aren't available, use a `Blocked by: #<n>, #<n>` line at the top of the child body.
- **Frontier query**: list the map's open children, drop tickets with an open blocker or an assignee, and select the first remaining ticket in map order.
- **Claim**: `gh issue edit <n> --repo timonwong/jiro --add-assignee @me`.
- **Resolve**: comment with the answer, close the child ticket, and append a context pointer to the map's Decisions-so-far.
