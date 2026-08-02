package markup_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/timonwong/jiro/internal/markup"
	"golang.org/x/tools/txtar"
)

func TestToJiraGolden(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("testdata/markdown_input")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".txtar" {
			continue
		}
		entry := entry
		t.Run(strings.TrimSuffix(entry.Name(), ".txtar"), func(t *testing.T) {
			t.Parallel()
			archive, err := txtar.ParseFile(filepath.Join("testdata/markdown_input", entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			sections := make(map[string][]byte, len(archive.Files))
			for _, file := range archive.Files {
				sections[file.Name] = file.Data
			}
			for _, required := range []string{"input.md", "want.jira"} {
				if _, ok := sections[required]; !ok {
					t.Fatalf("missing required txtar section %q", required)
				}
			}
			got, err := markup.ToJira(string(sections["input.md"]), markup.Markdown)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.TrimSuffix(string(sections["want.jira"]), "\n")
			if got != want {
				t.Fatalf("ToJira() = %q, want %q", got, want)
			}
		})
	}
}

func TestToJiraCoversRelevantMarkdownToJiraScenariosUsingJiroContract(t *testing.T) {
	t.Parallel()

	// These inputs are curated from jadujoel/markdown-to-jira's formatting,
	// list, table, and document scenarios. Expected output comes from jiro's
	// Issue #3 contract. HTML preview/debug, renderer internals, input repair,
	// bare-autolink, task-list semantics, and live Jira E2E scenarios are
	// intentionally not represented here.
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strong with underscores",
			input: "__bold__",
			want:  "*bold*",
		},
		{
			name:  "emphasis with stars",
			input: "*italic*",
			want:  "_italic_",
		},
		{
			name:  "italic wrapping strong",
			input: "*italic and **bold** text*",
			want:  "_italic and *bold* text_",
		},
		{
			name:  "asterisk list markers",
			input: "* one\n* two",
			want:  "* one\n* two",
		},
		{
			name:  "ordered list with formatted unordered children",
			input: "1. **Step one**\n   - `sub a`\n   - _sub b_\n2. Step two",
			want:  "# *Step one*\n#* {{sub a}}\n#* _sub b_\n# Step two",
		},
		{
			name:  "table followed by paragraph",
			input: "| A | B |\n| --- | --- |\n| 1 | 2 |\n\nAfter table.",
			want:  "||A||B||\n|1|2|\n\nAfter table.",
		},
		{
			name:  "consecutive code blocks use canonical separators",
			input: "```js\nconst x = 1\n```\n\n```unknown\nx = 1\n```",
			want:  "{code:language=javascript}\nconst x = 1\n{code}\n\n{code}\nx = 1\n{code}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := markup.ToJira(test.input, markup.Markdown)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("ToJira() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestToJiraConvertsConcurrentlyWithoutSharedState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		format  markup.InputFormat
		want    string
		wantErr *markup.ConversionError
	}{
		{
			input:  "# Release\n\n- one\n- two",
			format: markup.Markdown,
			want:   "h1. Release\n\n* one\n* two",
		},
		{
			input:  "| A | B |\n| --- | --- |\n| `x` | [docs](../docs) |",
			format: markup.Markdown,
			want:   "||A||B||\n|{{x}}|[docs|../docs]|",
		},
		{
			input:  "{panel}\r\n* existing Jira Markup *\n{panel}\n",
			format: markup.JiraMarkup,
			want:   "{panel}\r\n* existing Jira Markup *\n{panel}\n",
		},
		{
			input:  "safe\n\n| H |\n| --- |\n| ![alt](image.png) |",
			format: markup.Markdown,
			wantErr: &markup.ConversionError{
				Line:     5,
				Column:   3,
				NodeType: "Image",
				Reason:   "images are not supported in table cells",
			},
		},
	}

	const workers = 32
	const iterations = 20
	start := make(chan struct{})
	problems := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := range workers {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			<-start
			for iteration := range iterations {
				test := tests[(worker+iteration)%len(tests)]
				got, err := markup.ToJira(test.input, test.format)
				if test.wantErr == nil {
					if err != nil || got != test.want {
						problems <- fmt.Errorf("worker %d iteration %d: output=%q error=%v, want output=%q", worker, iteration, got, err, test.want)
						return
					}
					continue
				}
				if got != "" {
					problems <- fmt.Errorf("worker %d iteration %d: partial output=%q", worker, iteration, got)
					return
				}
				var conversionErr *markup.ConversionError
				if !errors.As(err, &conversionErr) || *conversionErr != *test.wantErr {
					problems <- fmt.Errorf("worker %d iteration %d: error=%T %v, want %+v", worker, iteration, err, err, test.wantErr)
					return
				}
			}
		}(worker)
	}
	close(start)
	wait.Wait()
	close(problems)
	for problem := range problems {
		t.Error(problem)
	}
}
