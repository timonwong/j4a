# Keep Board and Sprint discovery read-only

jiro exposes `board list` and `sprint list` as singular, top-level, read-only
discovery commands. It does not add Board or Sprint lifecycle mutations.
Sprint assignment remains an Issue mutation through `issue add --sprint` and
`issue update --sprint`, preserving ADR-0007's command boundary.

Both discovery commands fetch every Jira Agile API page and preserve Jira
order. `sprint list` defaults to active Sprints, supports locally validated
`active`, `closed`, `future`, and `all` states, and can fan out across every
Board or every Board matched by one selector. Positive numeric selectors match
only a Board ID; other non-empty selectors use a case-insensitive Board name
substring. Matching multiple Boards is intentional.

Each Sprint result represents one `(queried Board, Sprint)` relationship, so
duplicate Sprint IDs are retained. Normalized output distinguishes the queried
Board's `boardId` and `boardName` from Jira's `originBoardId`.

Cross-Board failures follow ADR-0005. jiro continues querying other Boards and,
when at least one Board request succeeds, writes the successful results plus
`failedBoards` to stdout, writes `partial_failure` to stderr, and exits `7`.
If no selected Board request succeeds, jiro returns the ordinary classified
Jira API error rather than an empty partial result.
