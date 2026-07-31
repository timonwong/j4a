package cmd

import "testing"

func TestBuildIssueJQLTypedFilters(t *testing.T) {
	tests := []struct {
		name    string
		options IssueListJQLOptions
		want    string
	}{
		{
			name: "combines filters and repeats values within fields",
			options: IssueListJQLOptions{Filters: IssueListJQLFilters{
				Project: "OPS", Status: "Open", Assignee: "me", IssueType: "Bug",
				Resolution: "unresolved", Reporter: "me",
				Labels: []string{"agent", "cli"}, Components: []string{"API", "UI"}, FixVersions: []string{"4.4", "4.5"},
				Sprint: "active", Parent: "OPS-42",
			}},
			want: `project = "OPS" AND status = "Open" AND assignee = currentUser() AND issuetype = "Bug" AND resolution IS EMPTY AND reporter = currentUser() AND labels IN ("agent", "cli") AND component IN ("API", "UI") AND fixVersion IN ("4.4", "4.5") AND sprint IN openSprints() AND parent = "OPS-42" ORDER BY updated DESC`,
		},
		{
			name: "sprint named and numeric equality",
			options: IssueListJQLOptions{Filters: IssueListJQLFilters{
				Sprint: "Release Sprint",
			}},
			want: `sprint = "Release Sprint" ORDER BY updated DESC`,
		},
		{
			name: "sprint ID equality",
			options: IssueListJQLOptions{Filters: IssueListJQLFilters{
				Sprint: "42",
			}},
			want: `sprint = 42 ORDER BY updated DESC`,
		},
		{
			name: "closed sprint shortcut",
			options: IssueListJQLOptions{Filters: IssueListJQLFilters{
				Sprint: "closed",
			}},
			want: `sprint IN closedSprints() ORDER BY updated DESC`,
		},
		{
			name: "future sprint shortcut",
			options: IssueListJQLOptions{Filters: IssueListJQLFilters{
				Sprint: "future",
			}},
			want: `sprint IN futureSprints() ORDER BY updated DESC`,
		},
		{
			name: "absolute dates select their whole calendar day",
			options: IssueListJQLOptions{Filters: IssueListJQLFilters{
				Created: "2026-07-31", Updated: "2026/02/28",
			}},
			want: `created >= "2026-07-31" AND created < "2026-08-01" AND updated >= "2026-02-28" AND updated < "2026-03-01" ORDER BY updated DESC`,
		},
		{
			name: "relative values and Jira date functions use greater than or equal",
			options: IssueListJQLOptions{Filters: IssueListJQLFilters{
				Created: "-4w 2d", Updated: "startOfWeek()",
			}},
			want: `created >= "-4w 2d" AND updated >= startOfWeek() ORDER BY updated DESC`,
		},
		{
			name: "preserves raw JQL and its explicit order",
			options: IssueListJQLOptions{
				RawJQL:  `priority = High ORDER BY created DESC`,
				Filters: IssueListJQLFilters{Labels: []string{"agent"}},
			},
			want: `(priority = High) AND labels IN ("agent") ORDER BY created DESC`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := BuildIssueJQL(test.options)
			if err != nil {
				t.Fatalf("BuildIssueJQL() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("BuildIssueJQL() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildIssueJQLEscapesQuotedLiterals(t *testing.T) {
	got, err := BuildIssueJQL(IssueListJQLOptions{Filters: IssueListJQLFilters{
		Labels: []string{`agent" OR project = ADMIN`},
	}})
	if err != nil {
		t.Fatalf("BuildIssueJQL() error = %v", err)
	}
	const want = `labels IN ("agent\" OR project = ADMIN") ORDER BY updated DESC`
	if got != want {
		t.Fatalf("BuildIssueJQL() = %q, want %q", got, want)
	}
}

func TestBuildIssueJQLRejectsUnsafeOperands(t *testing.T) {
	tests := []IssueListJQLOptions{
		{Filters: IssueListJQLFilters{Labels: []string{"agent\nproject = ADMIN"}}},
		{Filters: IssueListJQLFilters{Created: `startOfWeek() OR project = ADMIN`}},
		{Filters: IssueListJQLFilters{Updated: "today"}},
		{Filters: IssueListJQLFilters{Created: "2026-02-30"}},
	}
	for _, options := range tests {
		if _, err := BuildIssueJQL(options); err == nil {
			t.Fatalf("BuildIssueJQL(%+v) error = nil, want invalid operand error", options)
		}
	}
}

func TestBuildIssueJQLPreservesExplicitOrder(t *testing.T) {
	tests := []struct {
		name                                string
		raw, project, status, assignee, typ string
		want                                string
	}{
		{"default order", "", "OPS", "", "", "", `project = "OPS" ORDER BY updated DESC`},
		{"raw order", `project = OPS ORDER BY rank ASC`, "", "", "", "", `project = OPS ORDER BY rank ASC`},
		{"filters before raw order", `labels = agent ORDER BY created DESC`, "OPS", "", "", "", `(labels = agent) AND project = "OPS" ORDER BY created DESC`},
		{"order only", `ORDER BY created DESC`, "OPS", "", "", "", `project = "OPS" ORDER BY created DESC`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := buildIssueJQL(test.raw, test.project, test.status, test.assignee, test.typ); got != test.want {
				t.Fatalf("buildIssueJQL() = %q, want %q", got, test.want)
			}
		})
	}
}
