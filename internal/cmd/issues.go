package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timonwong/jiro/internal/apperr"
	"github.com/timonwong/jiro/internal/jira"
	"github.com/timonwong/jiro/internal/output"
)

func (a *app) issuesCommand() *cobra.Command {
	command := &cobra.Command{Use: "issues", Aliases: []string{"issue"}, Short: "Work with Jira issues"}
	command.AddCommand(
		a.issuesListCommand(),
		a.issueShowCommand(),
		a.issueCreateCommand(),
		a.issueUpdateCommand(),
		a.issueCommentsCommand(),
		a.issueCommentCommand(),
		a.issueTransitionsCommand(),
		a.issueTransitionCommand(),
		a.issueAssignCommand(),
		a.issueMoveCommand(),
		a.issueLinksCommand(),
		a.issueLinkCommand(),
		a.issueUnlinkCommand(),
		a.issueLinkTypesCommand(),
		a.issueBulkTransitionCommand(),
		a.issueBulkAssignCommand(),
	)
	return command
}

func (a *app) issuesListCommand() *cobra.Command {
	var project, status, assignee, issueType, rawJQL string
	var resolution, reporter, sprint, parent, created, updated string
	var limit, offset int
	var all bool
	var fields, labels, components, fixVersions []string
	command := &cobra.Command{
		Use:   "list",
		Short: "List issues",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validatePagination(offset, limit); err != nil {
				return err
			}
			client, _, err := a.client()
			if err != nil {
				return err
			}
			jql, err := BuildIssueJQL(IssueListJQLOptions{
				RawJQL: rawJQL,
				Filters: IssueListJQLFilters{
					Project: project, Status: status, Assignee: assignee, IssueType: issueType,
					Resolution: resolution, Reporter: reporter, Labels: labels, Components: components,
					FixVersions: fixVersions, Sprint: sprint, Parent: parent, Created: created, Updated: updated,
				},
			})
			if err != nil {
				return err
			}
			if isRaw(a) {
				if all {
					return apperr.New(apperr.KindInvalidInput, "--all cannot be combined with --raw")
				}
				payload := searchPayload(jql, offset, limit, fields)
				return a.rawRequest(command.Context(), client, http.MethodPost, "rest/api/2/search", nil, payload)
			}
			var result jira.SearchResult
			if all {
				result, err = searchAll(command.Context(), client, jql, offset, limit, fields)
			} else {
				result, err = client.ListIssues(command.Context(), jira.IssueListOptions{
					JQL: jql, Page: jira.Page{StartAt: offset, MaxResults: limit}, Fields: fields,
				})
			}
			if err != nil {
				return err
			}
			return a.render(result, output.Table{
				Headers: []string{"KEY", "SUMMARY", "STATUS", "ASSIGNEE"},
				Rows:    issueRows(result.Issues),
			})
		},
	}
	flags := command.Flags()
	flags.StringVarP(&project, "project", "p", "", "filter by project key")
	flags.StringVar(&status, "status", "", "filter by status")
	flags.StringVar(&assignee, "assignee", "", "filter by assignee; use me for currentUser()")
	flags.StringVarP(&issueType, "type", "t", "", "filter by issue type")
	flags.StringVar(&resolution, "resolution", "", "filter by resolution; use unresolved for empty resolution")
	flags.StringVar(&reporter, "reporter", "", "filter by reporter; use me for currentUser()")
	flags.StringSliceVar(&labels, "label", nil, "filter by label; repeatable")
	flags.StringSliceVar(&components, "component", nil, "filter by component name; repeatable")
	flags.StringSliceVar(&fixVersions, "fix-version", nil, "filter by fix version name; repeatable")
	flags.StringVar(&sprint, "sprint", "", "filter by sprint ID, name, active/open, closed, or future")
	flags.StringVar(&parent, "parent", "", "filter by parent issue key")
	flags.StringVar(&created, "created", "", "filter by Jira absolute or relative created date")
	flags.StringVar(&updated, "updated", "", "filter by Jira absolute or relative updated date")
	flags.StringVarP(&rawJQL, "jql", "q", "", "additional raw JQL expression")
	flags.IntVarP(&limit, "limit", "n", 50, "maximum issues per page")
	flags.IntVar(&offset, "offset", 0, "zero-based result offset")
	flags.BoolVar(&all, "all", false, "fetch all result pages")
	flags.StringSliceVar(&fields, "fields", nil, "comma-separated Jira fields to request")
	return command
}

func (a *app) issueShowCommand() *cobra.Command {
	var fields []string
	command := &cobra.Command{
		Use:   "show ISSUE-KEY",
		Short: "Show an issue",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, _, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				query := url.Values{}
				if len(fields) > 0 {
					query.Set("fields", strings.Join(fields, ","))
				}
				return a.rawRequest(command.Context(), client, http.MethodGet, issuePath(args[0]), query, nil)
			}
			issue, err := client.ShowIssue(command.Context(), args[0], fields)
			if err != nil {
				return err
			}
			return a.render(issue, issueTable(issue))
		},
	}
	command.Flags().StringSliceVar(&fields, "fields", nil, "comma-separated Jira fields to request")
	return command
}

func (a *app) issueCreateCommand() *cobra.Command {
	var project, issueType, summary, description, descriptionFile, inputFormat, parent, sprint string
	var priority, assignee string
	var labels, components, fixVersions, fields []string
	command := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(project) == "" || strings.TrimSpace(issueType) == "" || strings.TrimSpace(summary) == "" {
				return apperr.New(apperr.KindInvalidInput, "project, type, and summary are required")
			}
			if strings.TrimSpace(sprint) != "" && isRaw(a) {
				return apperr.New(apperr.KindInvalidInput, "--raw is not available for issues create --sprint")
			}
			client, settings, err := a.writableClient()
			if err != nil {
				return err
			}
			format, err := parseInputFormat(inputFormat)
			if err != nil {
				return err
			}
			descriptionValue, err := readText(a.stdin, description, command.Flags().Changed("description"), descriptionFile)
			if err != nil {
				return err
			}
			descriptionValue, err = convertToJiraMarkup(descriptionValue, format)
			if err != nil {
				return err
			}
			resolvedFields, err := a.resolveFields(command.Context(), client, settings, fields)
			if err != nil {
				return err
			}
			applyStandardFields(resolvedFields, priority, assignee, labels, command.Flags().Changed("label"))
			if strings.TrimSpace(parent) != "" {
				resolvedFields["parent"] = map[string]string{"key": parent}
			}
			applyNamedIssueField(resolvedFields, "components", components, len(components) > 0, false)
			applyNamedIssueField(resolvedFields, "fixVersions", fixVersions, len(fixVersions) > 0, false)
			input := jira.CreateIssueInput{
				ProjectKey: project, IssueType: issueType, Summary: summary,
				Description: descriptionValue, Fields: resolvedFields,
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodPost, "rest/api/2/issue", nil, createPayload(input))
			}
			issue, err := client.CreateIssue(command.Context(), input)
			if err != nil {
				return err
			}
			result := map[string]any{"id": issue.ID, "key": issue.Key}
			if strings.TrimSpace(sprint) != "" {
				result["sprint"] = sprint
				if err := client.MoveIssueToSprint(command.Context(), issue.Key, jira.MoveIssueToSprintInput{Sprint: sprint}); err != nil {
					result["sprintMoved"] = false
					if renderErr := a.renderPartial(result, "Created "+issue.Key); renderErr != nil {
						return renderErr
					}
					return apperr.Wrap(apperr.KindPartialFailure, err, "created %s but failed to move it to sprint %q: %v", issue.Key, sprint, err)
				}
				result["sprintMoved"] = true
			}
			return a.renderMessage(result, "Created "+issue.Key)
		},
	}
	flags := command.Flags()
	flags.StringVarP(&project, "project", "p", "", "project key")
	flags.StringVarP(&issueType, "type", "t", "", "issue type")
	flags.StringVarP(&summary, "summary", "s", "", "issue summary")
	flags.StringVar(&description, "description", "", "description text")
	flags.StringVar(&descriptionFile, "description-file", "", "description file path, or - for stdin")
	flags.StringVar(&inputFormat, "input-format", "jira", "text input format: jira or markdown")
	flags.StringVar(&priority, "priority", "", "priority name")
	flags.StringVar(&assignee, "assignee", "", "assignee username; use none to clear")
	flags.StringSliceVar(&labels, "label", nil, "label; repeat or pass comma-separated values")
	flags.StringVar(&parent, "parent", "", "parent issue key")
	flags.StringSliceVar(&components, "component", nil, "component name; repeatable")
	flags.StringSliceVar(&fixVersions, "fix-version", nil, "fix version name; repeatable")
	flags.StringVar(&sprint, "sprint", "", "sprint ID, name substring, or active")
	flags.StringArrayVar(&fields, "field", nil, "custom field as key=value; repeatable")
	return command
}

func (a *app) issueUpdateCommand() *cobra.Command {
	var summary, description, descriptionFile, inputFormat string
	var priority, assignee string
	var labels, components, fixVersions, fields []string
	command := &cobra.Command{
		Use:   "update ISSUE-KEY",
		Short: "Update an issue",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, settings, err := a.writableClient()
			if err != nil {
				return err
			}
			format, err := parseInputFormat(inputFormat)
			if err != nil {
				return err
			}
			descriptionValue, err := readText(a.stdin, description, command.Flags().Changed("description"), descriptionFile)
			if err != nil {
				return err
			}
			descriptionValue, err = convertToJiraMarkup(descriptionValue, format)
			if err != nil {
				return err
			}
			resolvedFields, err := a.resolveFields(command.Context(), client, settings, fields)
			if err != nil {
				return err
			}
			applyStandardFields(resolvedFields, priority, assignee, labels, command.Flags().Changed("label"))
			applyNamedIssueField(resolvedFields, "components", components, command.Flags().Changed("component"), true)
			applyNamedIssueField(resolvedFields, "fixVersions", fixVersions, command.Flags().Changed("fix-version"), true)
			input := jira.UpdateIssueInput{Description: descriptionValue, Fields: resolvedFields}
			if command.Flags().Changed("summary") {
				input.Summary = &summary
			}
			if input.Summary == nil && input.Description == nil && len(input.Fields) == 0 {
				return apperr.New(apperr.KindInvalidInput, "at least one issue field is required")
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodPut, issuePath(args[0]), nil, updatePayload(input))
			}
			if err := client.UpdateIssue(command.Context(), args[0], input); err != nil {
				return err
			}
			result := map[string]any{"key": args[0], "updated": true}
			return a.renderMessage(result, "Updated "+args[0])
		},
	}
	flags := command.Flags()
	flags.StringVarP(&summary, "summary", "s", "", "new issue summary")
	flags.StringVar(&description, "description", "", "description text")
	flags.StringVar(&descriptionFile, "description-file", "", "description file path, or - for stdin")
	flags.StringVar(&inputFormat, "input-format", "jira", "text input format: jira or markdown")
	flags.StringVar(&priority, "priority", "", "priority name")
	flags.StringVar(&assignee, "assignee", "", "assignee username; use none to clear")
	flags.StringSliceVar(&labels, "label", nil, "replacement labels; repeat or pass comma-separated values")
	flags.StringSliceVar(&components, "component", nil, "replacement component names; use a single none to clear")
	flags.StringSliceVar(&fixVersions, "fix-version", nil, "replacement fix version names; use a single none to clear")
	flags.StringArrayVar(&fields, "field", nil, "custom field as key=value; repeatable")
	return command
}

func (a *app) issueCommentsCommand() *cobra.Command {
	var limit, offset int
	command := &cobra.Command{
		Use:   "comments ISSUE-KEY",
		Short: "List issue comments",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := validatePagination(offset, limit); err != nil {
				return err
			}
			client, _, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodGet, issuePath(args[0])+"/comment", pageQuery(offset, limit), nil)
			}
			comments, err := client.Comments(command.Context(), args[0], jira.Page{StartAt: offset, MaxResults: limit})
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(comments))
			for _, comment := range comments {
				author := ""
				if comment.Author != nil {
					author = comment.Author.DisplayName
				}
				rows = append(rows, []string{comment.ID, author, comment.Created, comment.Body})
			}
			data := map[string]any{"issueKey": args[0], "comments": comments}
			return a.render(data, output.Table{Headers: []string{"ID", "AUTHOR", "CREATED", "BODY"}, Rows: rows})
		},
	}
	command.Flags().IntVarP(&limit, "limit", "n", 50, "maximum comments")
	command.Flags().IntVar(&offset, "offset", 0, "zero-based result offset")
	return command
}

func (a *app) issueCommentCommand() *cobra.Command {
	var body, bodyFile, inputFormat string
	command := &cobra.Command{
		Use:   "comment ISSUE-KEY",
		Short: "Add an issue comment",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, _, err := a.writableClient()
			if err != nil {
				return err
			}
			format, err := parseInputFormat(inputFormat)
			if err != nil {
				return err
			}
			bodyValue, err := readText(a.stdin, body, command.Flags().Changed("body"), bodyFile)
			if err != nil {
				return err
			}
			if bodyValue == nil || *bodyValue == "" {
				return apperr.New(apperr.KindInvalidInput, "comment body is required")
			}
			bodyValue, err = convertToJiraMarkup(bodyValue, format)
			if err != nil {
				return err
			}
			input := jira.CommentInput{Body: *bodyValue}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodPost, issuePath(args[0])+"/comment", nil, input)
			}
			comment, err := client.Comment(command.Context(), args[0], input)
			if err != nil {
				return err
			}
			return a.renderMessage(comment, "Commented on "+args[0])
		},
	}
	flags := command.Flags()
	flags.StringVar(&body, "body", "", "comment body")
	flags.StringVar(&bodyFile, "body-file", "", "comment body file path, or - for stdin")
	flags.StringVar(&inputFormat, "input-format", "jira", "text input format: jira or markdown")
	return command
}

func (a *app) issueTransitionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list-transitions ISSUE-KEY",
		Short: "List available issue transitions",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, _, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodGet, issuePath(args[0])+"/transitions", nil, nil)
			}
			transitions, err := client.ListTransitions(command.Context(), args[0])
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(transitions))
			for _, transition := range transitions {
				to := ""
				if transition.To != nil {
					to = transition.To.Name
				}
				rows = append(rows, []string{transition.ID, transition.Name, to})
			}
			return a.render(map[string]any{"issueKey": args[0], "transitions": transitions}, output.Table{
				Headers: []string{"ID", "NAME", "TO"}, Rows: rows,
			})
		},
	}
}

func (a *app) issueTransitionCommand() *cobra.Command {
	var target string
	var fields []string
	command := &cobra.Command{
		Use:   "transition ISSUE-KEY",
		Short: "Transition an issue",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(target) == "" {
				return apperr.New(apperr.KindInvalidInput, "--to is required")
			}
			client, settings, err := a.writableClient()
			if err != nil {
				return err
			}
			transitions, err := client.ListTransitions(command.Context(), args[0])
			if err != nil {
				return err
			}
			transition, err := matchTransition(target, transitions)
			if err != nil {
				return err
			}
			resolvedFields, err := a.resolveFields(command.Context(), client, settings, fields)
			if err != nil {
				return err
			}
			input := jira.TransitionInput{ID: transition.ID, Fields: resolvedFields}
			if isRaw(a) {
				payload := map[string]any{"transition": map[string]string{"id": input.ID}}
				if len(input.Fields) > 0 {
					payload["fields"] = input.Fields
				}
				return a.rawRequest(command.Context(), client, http.MethodPost, issuePath(args[0])+"/transitions", nil, payload)
			}
			if err := client.Transition(command.Context(), args[0], input); err != nil {
				return err
			}
			result := map[string]any{"key": args[0], "transition": transition, "transitioned": true}
			return a.renderMessage(result, fmt.Sprintf("Transitioned %s to %s", args[0], transition.Name))
		},
	}
	command.Flags().StringVar(&target, "to", "", "transition ID or name")
	command.Flags().StringArrayVar(&fields, "field", nil, "transition field as key=value; repeatable")
	return command
}

func searchPayload(jql string, offset, limit int, fields []string) map[string]any {
	payload := map[string]any{"jql": jql}
	if offset > 0 {
		payload["startAt"] = offset
	}
	if limit > 0 {
		payload["maxResults"] = limit
	}
	if len(fields) > 0 {
		payload["fields"] = fields
	}
	return payload
}

func createPayload(input jira.CreateIssueInput) map[string]any {
	fields := make(map[string]any, len(input.Fields)+4)
	for key, value := range input.Fields {
		fields[key] = value
	}
	fields["project"] = map[string]string{"key": input.ProjectKey}
	fields["issuetype"] = map[string]string{"name": input.IssueType}
	fields["summary"] = input.Summary
	if input.Description != nil {
		fields["description"] = *input.Description
	}
	return map[string]any{"fields": fields}
}

func updatePayload(input jira.UpdateIssueInput) map[string]any {
	fields := make(map[string]any, len(input.Fields)+2)
	for key, value := range input.Fields {
		fields[key] = value
	}
	if input.Summary != nil {
		fields["summary"] = *input.Summary
	}
	if input.Description != nil {
		fields["description"] = *input.Description
	}
	return map[string]any{"fields": fields}
}

func applyStandardFields(fields map[string]any, priority, assignee string, labels []string, labelsSet bool) {
	if priority != "" {
		fields["priority"] = map[string]string{"name": priority}
	}
	if assignee != "" {
		if strings.EqualFold(assignee, "none") {
			fields["assignee"] = nil
		} else {
			fields["assignee"] = map[string]string{"name": assignee}
		}
	}
	if labelsSet {
		fields["labels"] = labels
	}
}

func applyNamedIssueField(fields map[string]any, key string, values []string, changed, allowClear bool) {
	if !changed {
		return
	}
	if allowClear && len(values) == 1 && strings.EqualFold(strings.TrimSpace(values[0]), "none") {
		fields[key] = []map[string]string{}
		return
	}
	named := make([]map[string]string, 0, len(values))
	for _, value := range values {
		named = append(named, map[string]string{"name": value})
	}
	fields[key] = named
}

func issueTable(issue jira.Issue) output.Table {
	status, issueType, priority, assignee, reporter, parent := "", "", "", "", "", ""
	if issue.Status != nil {
		status = issue.Status.Name
	}
	if issue.IssueType != nil {
		issueType = issue.IssueType.Name
	}
	if issue.Priority != nil {
		priority = issue.Priority.Name
	}
	if issue.Assignee != nil {
		assignee = issue.Assignee.DisplayName
	}
	if issue.Reporter != nil {
		reporter = issue.Reporter.DisplayName
	}
	if issue.Parent != nil {
		parent = issue.Parent.Key
	}
	components := make([]string, 0, len(issue.Components))
	for _, component := range issue.Components {
		components = append(components, component.Name)
	}
	fixVersions := make([]string, 0, len(issue.FixVersions))
	for _, version := range issue.FixVersions {
		fixVersions = append(fixVersions, version.Name)
	}
	rows := [][]string{
		{"Key", issue.Key}, {"Summary", issue.Summary}, {"Status", status},
		{"Type", issueType}, {"Priority", priority}, {"Assignee", assignee},
		{"Reporter", reporter}, {"Labels", joinNames(issue.Labels)},
		{"Parent", parent}, {"Components", joinNames(components)}, {"Fix Versions", joinNames(fixVersions)},
		{"Description", issue.Description},
	}
	return output.Table{Headers: []string{"FIELD", "VALUE"}, Rows: rows}
}

func matchTransition(target string, transitions []jira.Transition) (jira.Transition, error) {
	matches := make([]jira.Transition, 0, 1)
	for _, transition := range transitions {
		matchesTarget := transition.ID == target || strings.EqualFold(transition.Name, target)
		if transition.To != nil && strings.EqualFold(transition.To.Name, target) {
			matchesTarget = true
		}
		if matchesTarget {
			matches = append(matches, transition)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return jira.Transition{}, apperr.New(apperr.KindInvalidInput, fmt.Sprintf("transition %q was not found", target))
	}
	return jira.Transition{}, apperr.New(apperr.KindInvalidInput, fmt.Sprintf("transition %q is ambiguous", target))
}
