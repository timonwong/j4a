package cmd

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timonwong/jiro/internal/apperr"
	"github.com/timonwong/jiro/internal/jira"
	"github.com/timonwong/jiro/internal/output"
)

func (a *app) issueAssignCommand() *cobra.Command {
	var assignee string
	command := &cobra.Command{
		Use:   "assign ISSUE-KEY",
		Short: "Assign or unassign an issue",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(assignee) == "" {
				return apperr.New(apperr.KindInvalidInput, "--assignee is required")
			}
			client, _, err := a.writableClient()
			if err != nil {
				return err
			}
			resolved, resultValue, err := resolveAssignee(command.Context(), client, assignee)
			if err != nil {
				return err
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodPut, issuePath(args[0])+"/assignee", nil, map[string]any{"name": resultValue})
			}
			if err := client.AssignIssue(command.Context(), args[0], resolved); err != nil {
				return err
			}
			result := map[string]any{"key": args[0], "assignee": resultValue, "assigned": resultValue != nil}
			message := "Assigned " + args[0] + " to " + fmt.Sprint(resultValue)
			if resultValue == nil {
				message = "Unassigned " + args[0]
			}
			return a.renderMessage(result, message)
		},
	}
	command.Flags().StringVar(&assignee, "assignee", "", "assignee username; use me or none")
	return command
}

func (a *app) issueLinksCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "links ISSUE-KEY",
		Short: "List links for an issue",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, _, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				query := url.Values{"fields": {"issuelinks"}}
				return a.rawRequest(command.Context(), client, http.MethodGet, issuePath(args[0]), query, nil)
			}
			links, err := client.ListIssueLinks(command.Context(), args[0])
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(links))
			for _, link := range links {
				rows = append(rows, []string{link.ID, link.Direction, link.Relationship, link.Type.Name, link.OtherIssue.Key})
			}
			return a.render(map[string]any{"issueKey": args[0], "links": links}, output.Table{
				Headers: []string{"ID", "DIRECTION", "RELATIONSHIP", "TYPE", "ISSUE"}, Rows: rows,
			})
		},
	}
}

func (a *app) issueLinkCommand() *cobra.Command {
	var to, linkType string
	command := &cobra.Command{
		Use:   "link FROM",
		Short: "Link one issue outward to another issue",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(to) == "" || strings.TrimSpace(linkType) == "" {
				return apperr.New(apperr.KindInvalidInput, "--to and --type are required")
			}
			client, _, err := a.writableClient()
			if err != nil {
				return err
			}
			types, err := client.ListIssueLinkTypes(command.Context())
			if err != nil {
				return err
			}
			resolved, err := matchIssueLinkType(strings.TrimSpace(linkType), types)
			if err != nil {
				return err
			}
			input := jira.IssueLinkInput{From: args[0], To: to, TypeID: resolved.ID}
			if isRaw(a) {
				payload := map[string]any{
					"type":         map[string]string{"id": resolved.ID},
					"outwardIssue": map[string]string{"key": args[0]},
					"inwardIssue":  map[string]string{"key": to},
				}
				return a.rawRequest(command.Context(), client, http.MethodPost, "rest/api/2/issueLink", nil, payload)
			}
			if err := client.CreateIssueLink(command.Context(), input); err != nil {
				return err
			}
			result := map[string]any{"from": args[0], "to": to, "type": resolved, "linked": true}
			relationship := resolved.Outward
			if relationship == "" {
				relationship = "using link type " + resolved.ID
			}
			return a.renderMessage(result, fmt.Sprintf("Linked %s %s %s", args[0], relationship, to))
		},
	}
	command.Flags().StringVar(&to, "to", "", "target issue key")
	command.Flags().StringVar(&linkType, "type", "", "link type ID or unique name")
	return command
}

func matchIssueLinkType(target string, types []jira.IssueLinkType) (jira.IssueLinkType, error) {
	matches := make([]jira.IssueLinkType, 0, 1)
	for _, candidate := range types {
		if candidate.ID == target || strings.EqualFold(candidate.Name, target) {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return jira.IssueLinkType{}, apperr.New(apperr.KindInvalidInput, fmt.Sprintf("issue link type %q was not found", target))
	default:
		return jira.IssueLinkType{}, apperr.New(apperr.KindInvalidInput, fmt.Sprintf("issue link type %q is ambiguous", target))
	}
}

func (a *app) issueUnlinkCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unlink LINK-ID",
		Short: "Delete an issue link by Jira link ID",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if id, parseErr := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64); parseErr != nil || id <= 0 {
				return apperr.New(apperr.KindInvalidInput, "LINK-ID must be a positive Jira link ID")
			}
			client, _, err := a.writableClient()
			if err != nil {
				return err
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodDelete, "rest/api/2/issueLink/"+url.PathEscape(args[0]), nil, nil)
			}
			if err := client.DeleteIssueLink(command.Context(), args[0]); err != nil {
				return err
			}
			return a.renderMessage(map[string]any{"linkId": args[0], "unlinked": true}, "Unlinked "+args[0])
		},
	}
}

func (a *app) issueLinkTypesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "link-types",
		Short: "List Jira issue link types",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			client, _, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodGet, "rest/api/2/issueLinkType", nil, nil)
			}
			types, err := client.ListIssueLinkTypes(command.Context())
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(types))
			for _, linkType := range types {
				rows = append(rows, []string{linkType.ID, linkType.Name, linkType.Inward, linkType.Outward})
			}
			return a.render(map[string]any{"linkTypes": types}, output.Table{
				Headers: []string{"ID", "NAME", "INWARD", "OUTWARD"}, Rows: rows,
			})
		},
	}
}

func resolveAssignee(ctx context.Context, client *jira.Client, value string) (jira.AssigneeTarget, any, error) {
	target, err := client.ResolveAssignee(ctx, value)
	if err != nil {
		return jira.AssigneeTarget{}, nil, err
	}
	if target.Username == nil {
		return target, nil, nil
	}
	return target, *target.Username, nil
}

func (a *app) issueMoveCommand() *cobra.Command {
	var sprint string
	command := &cobra.Command{
		Use:   "move ISSUE-KEY",
		Short: "Move an issue to a Sprint",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(sprint) == "" {
				return apperr.New(apperr.KindInvalidInput, "--sprint is required")
			}
			client, _, err := a.writableClient()
			if err != nil {
				return err
			}
			if isRaw(a) {
				resolved, err := client.ResolveSprint(command.Context(), sprint)
				if err != nil {
					return err
				}
				path := fmt.Sprintf("rest/agile/1.0/sprint/%d/issue", resolved.ID)
				return a.rawRequest(command.Context(), client, http.MethodPost, path, nil, map[string][]string{"issues": {args[0]}})
			}
			if err := client.MoveIssueToSprint(command.Context(), args[0], jira.MoveIssueToSprintInput{Sprint: sprint}); err != nil {
				return err
			}
			result := map[string]any{"key": args[0], "sprint": sprint, "moved": true}
			return a.renderMessage(result, fmt.Sprintf("Moved %s to sprint %s", args[0], sprint))
		},
	}
	command.Flags().StringVar(&sprint, "sprint", "", "sprint ID, name substring, or active")
	return command
}
