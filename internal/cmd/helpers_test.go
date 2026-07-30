package cmd

import "testing"

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
