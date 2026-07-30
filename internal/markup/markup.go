// Package markup translates explicit Markdown input into Jira wiki markup.
package markup

import (
	"sync"

	confluence "github.com/kentaro-m/blackfriday-confluence"
	blackfriday "github.com/russross/blackfriday/v2"
)

// InputFormat selects whether a text value is already Jira markup or needs
// Markdown conversion.
type InputFormat string

const (
	// JiraMarkup preserves the value exactly. This is the default input format.
	JiraMarkup InputFormat = "jira"
	// Markdown converts CommonMark-compatible Markdown to Jira wiki markup.
	Markdown InputFormat = "markdown"
)

// ToJira returns Jira markup. Unsupported formats are left unchanged so the
// caller can validate its own command-level option before writing anything.
func ToJira(input string, format InputFormat) string {
	if format != Markdown || input == "" {
		return input
	}
	// blackfriday-confluence keeps list nesting in package-level state, so
	// serialize renders to make this safe for concurrent CLI/API use.
	renderMu.Lock()
	defer renderMu.Unlock()
	renderer := &confluence.Renderer{Flags: confluence.IgnoreMacroEscaping}
	parser := blackfriday.New(
		blackfriday.WithRenderer(renderer),
		blackfriday.WithExtensions(blackfriday.CommonExtensions),
	)
	return string(renderer.Render(parser.Parse([]byte(input))))
}

var renderMu sync.Mutex
