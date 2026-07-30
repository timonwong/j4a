package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/timonwong/j4a/internal/apperr"
	"github.com/timonwong/j4a/internal/jira"
	"github.com/timonwong/j4a/internal/markup"
)

var directCustomFieldID = regexp.MustCompile(`^customfield_[0-9]+$`)

func readText(reader io.Reader, inline string, inlineSet bool, file string) (*string, error) {
	if inlineSet && file != "" {
		return nil, apperr.New(apperr.KindInvalidInput, "inline text and file input are mutually exclusive")
	}
	if inlineSet {
		value := inline
		return &value, nil
	}
	if file == "" {
		return nil, nil
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(reader)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.KindInvalidInput, err, "read text input")
	}
	value := string(data)
	return &value, nil
}

func parseInputFormat(value string) (markup.InputFormat, error) {
	switch markup.InputFormat(strings.ToLower(value)) {
	case "", markup.JiraMarkup:
		return markup.JiraMarkup, nil
	case markup.Markdown:
		return markup.Markdown, nil
	default:
		return "", apperr.New(apperr.KindInvalidInput, "input format must be jira or markdown")
	}
}

func resolveFields(ctx context.Context, client *jira.Client, values []string) (map[string]any, error) {
	parsed, err := jira.ParseFieldValues(values)
	if err != nil {
		return nil, err
	}
	if len(parsed) == 0 {
		return map[string]any{}, nil
	}
	needsMetadata := false
	for key := range parsed {
		if !directCustomFieldID.MatchString(key) {
			needsMetadata = true
			break
		}
	}
	var metadata []jira.Field
	if needsMetadata {
		metadata, err = client.ListFields(ctx)
		if err != nil {
			return nil, err
		}
	}
	resolved := make(map[string]any, len(parsed))
	for key, value := range parsed {
		id, err := jira.ResolveCustomField(key, metadata)
		if err != nil {
			return nil, err
		}
		resolved[id] = value
	}
	return resolved, nil
}

func jqlLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func buildIssueJQL(raw, project, status, assignee, issueType string) string {
	raw = strings.TrimSpace(raw)
	order := ""
	if index := strings.LastIndex(strings.ToLower(raw), " order by "); index >= 0 {
		order = strings.TrimSpace(raw[index:])
		raw = strings.TrimSpace(raw[:index])
	} else if strings.HasPrefix(strings.ToLower(raw), "order by ") {
		order = raw
		raw = ""
	}
	clauses := make([]string, 0, 5)
	hasFilters := project != "" || status != "" || assignee != "" || issueType != ""
	if raw != "" {
		if hasFilters {
			clauses = append(clauses, "("+raw+")")
		} else {
			clauses = append(clauses, raw)
		}
	}
	if project != "" {
		clauses = append(clauses, "project = "+jqlLiteral(project))
	}
	if status != "" {
		clauses = append(clauses, "status = "+jqlLiteral(status))
	}
	if assignee != "" {
		if strings.EqualFold(assignee, "me") {
			clauses = append(clauses, "assignee = currentUser()")
		} else {
			clauses = append(clauses, "assignee = "+jqlLiteral(assignee))
		}
	}
	if issueType != "" {
		clauses = append(clauses, "issuetype = "+jqlLiteral(issueType))
	}
	query := strings.Join(clauses, " AND ")
	if order != "" {
		if query == "" {
			return order
		}
		return query + " " + order
	}
	if query == "" {
		return "ORDER BY updated DESC"
	}
	return query + " ORDER BY updated DESC"
}

func searchAll(ctx context.Context, client *jira.Client, jql string, startAt, maxResults int, fields []string) (jira.SearchResult, error) {
	if maxResults <= 0 {
		maxResults = 50
	}
	combined := jira.SearchResult{StartAt: startAt, MaxResults: maxResults}
	next := startAt
	for {
		page, err := client.Search(ctx, jira.IssueListOptions{
			JQL:    jql,
			Page:   jira.Page{StartAt: next, MaxResults: maxResults},
			Fields: fields,
		})
		if err != nil {
			return jira.SearchResult{}, err
		}
		combined.Issues = append(combined.Issues, page.Issues...)
		combined.Total = page.Total
		if len(page.Issues) == 0 || next+len(page.Issues) >= page.Total {
			break
		}
		next += len(page.Issues)
	}
	return combined, nil
}

func issueRows(issues []jira.Issue) [][]string {
	rows := make([][]string, 0, len(issues))
	for _, issue := range issues {
		status := ""
		if issue.Status != nil {
			status = issue.Status.Name
		}
		assignee := ""
		if issue.Assignee != nil {
			assignee = issue.Assignee.DisplayName
			if assignee == "" {
				assignee = issue.Assignee.Username
			}
		}
		rows = append(rows, []string{issue.Key, issue.Summary, status, assignee})
	}
	return rows
}

func sortedFieldKeys(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
