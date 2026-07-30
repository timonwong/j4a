package markup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestToJiraFixtures(t *testing.T) {
	t.Parallel()
	input, err := os.ReadFile(filepath.Join("testdata", "markdown.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "markdown.wiki"))
	if err != nil {
		t.Fatal(err)
	}
	// The renderer always terminates a document with a final blank line; the
	// fixture itself has the conventional single trailing newline.
	wantOutput := string(want) + "\n"
	if got := ToJira(string(input), Markdown); got != wantOutput {
		t.Fatalf("ToJira() = %q, want %q", got, wantOutput)
	}
}

func TestToJiraDefaultsToPassThrough(t *testing.T) {
	t.Parallel()
	input := "* existing Jira markup *"
	if got := ToJira(input, JiraMarkup); got != input {
		t.Fatalf("ToJira() = %q, want %q", got, input)
	}
}
