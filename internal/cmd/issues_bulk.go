package cmd

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timonwong/jiro/internal/apperr"
	"github.com/timonwong/jiro/internal/config"
	"github.com/timonwong/jiro/internal/jira"
	"github.com/timonwong/jiro/internal/output"
)

type issueBatchResult struct {
	Operation    string           `json:"operation"`
	DryRun       bool             `json:"dryRun"`
	JQL          string           `json:"jql"`
	Total        int              `json:"total"`
	Ready        int              `json:"ready"`
	Succeeded    int              `json:"succeeded"`
	Failed       int              `json:"failed"`
	NotAttempted int              `json:"notAttempted"`
	Items        []issueBatchItem `json:"items"`
}

type issueBatchItem struct {
	IssueKey string            `json:"issueKey"`
	Outcome  string            `json:"outcome"`
	Current  issueBatchCurrent `json:"current"`
	Target   issueBatchTarget  `json:"target"`
	Error    string            `json:"error,omitempty"`
}

type issueBatchCurrent struct {
	Status   *jira.Status `json:"status,omitempty"`
	Assignee *jira.User   `json:"assignee,omitempty"`
}

type issueBatchTarget struct {
	TransitionSpec string           `json:"transitionSpec,omitempty"`
	Transition     *jira.Transition `json:"transition,omitempty"`
	Assignee       *string          `json:"assignee,omitempty"`
	Unassigned     bool             `json:"unassigned,omitempty"`
}

func (a *app) issueBulkTransitionCommand() *cobra.Command {
	var jql, target string
	var fields []string
	var dryRun, yes bool
	command := &cobra.Command{
		Use:   "move",
		Short: "Transition every issue selected by JQL",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(jql) == "" || strings.TrimSpace(target) == "" {
				return apperr.New(apperr.KindInvalidInput, "--jql and --to are required")
			}
			if dryRun == yes {
				return apperr.New(apperr.KindInvalidInput, "exactly one of --dry-run or --yes is required")
			}
			var client *jira.Client
			var settings config.Settings
			var err error
			if dryRun {
				client, settings, err = a.client()
			} else {
				client, settings, err = a.writableClient()
			}
			if err != nil {
				return err
			}
			issues, err := searchAll(command.Context(), client, jql, 0, 50, []string{"status", "assignee"})
			if err != nil {
				return err
			}
			result := issueBatchResult{
				Operation: "transition", DryRun: dryRun, JQL: jql, Total: len(issues.Issues),
				Items: make([]issueBatchItem, 0, len(issues.Issues)),
			}
			if len(issues.Issues) == 0 {
				return a.finishIssueBatch(result)
			}
			resolvedFields, err := a.resolveFields(command.Context(), client, settings, fields)
			if err != nil {
				return err
			}
			for index, issue := range issues.Issues {
				item := issueBatchItem{
					IssueKey: issue.Key,
					Current:  issueBatchCurrent{Status: issue.Status},
					Target:   issueBatchTarget{TransitionSpec: target},
				}
				transitions, transitionErr := client.ListTransitions(command.Context(), issue.Key)
				if transitionErr == nil {
					var transition jira.Transition
					transition, transitionErr = matchTransition(target, transitions)
					if transitionErr == nil {
						item.Target.Transition = &transition
						result.Ready++
						if dryRun {
							item.Outcome = "ready"
						} else {
							transitionErr = client.Transition(command.Context(), issue.Key, jira.TransitionInput{ID: transition.ID, Fields: resolvedFields})
							if transitionErr == nil {
								item.Outcome = "succeeded"
								result.Succeeded++
							}
						}
					}
				}
				if transitionErr != nil {
					item.Outcome = "failed"
					item.Error = transitionErr.Error()
					result.Failed++
					result.Items = append(result.Items, item)
					if isSystemicIssueError(transitionErr) {
						appendNotAttempted(&result, issues.Issues[index+1:], issueBatchTarget{TransitionSpec: target}, transitionErr.Error())
						break
					}
					continue
				}
				result.Items = append(result.Items, item)
			}
			return a.finishIssueBatch(result)
		},
	}
	flags := command.Flags()
	flags.StringVar(&jql, "jql", "", "JQL selecting every issue to process")
	flags.StringVar(&target, "to", "", "transition ID, name, or destination status")
	flags.StringArrayVar(&fields, "field", nil, "transition field as key=value; repeatable")
	flags.BoolVar(&dryRun, "dry-run", false, "preflight every matching issue without changing Jira")
	flags.BoolVar(&yes, "yes", false, "confirm execution without prompting")
	return command
}

func (a *app) issueBulkAssignCommand() *cobra.Command {
	var jql, assignee string
	var dryRun, yes bool
	command := &cobra.Command{
		Use:   "assign",
		Short: "Assign every issue selected by JQL",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(jql) == "" || strings.TrimSpace(assignee) == "" {
				return apperr.New(apperr.KindInvalidInput, "--jql and --assignee are required")
			}
			if dryRun == yes {
				return apperr.New(apperr.KindInvalidInput, "exactly one of --dry-run or --yes is required")
			}
			var client *jira.Client
			var err error
			if dryRun {
				client, _, err = a.client()
			} else {
				client, _, err = a.writableClient()
			}
			if err != nil {
				return err
			}
			issues, err := searchAll(command.Context(), client, jql, 0, 50, []string{"status", "assignee"})
			if err != nil {
				return err
			}
			result := issueBatchResult{
				Operation: "assign", DryRun: dryRun, JQL: jql, Total: len(issues.Issues),
				Items: make([]issueBatchItem, 0, len(issues.Issues)),
			}
			if len(issues.Issues) == 0 {
				return a.finishIssueBatch(result)
			}
			resolved, _, err := resolveAssignee(command.Context(), client, assignee)
			if err != nil {
				return err
			}
			for index, issue := range issues.Issues {
				item := issueBatchItem{
					IssueKey: issue.Key,
					Current:  issueBatchCurrent{Assignee: issue.Assignee},
					Target:   assigneeBatchTarget(resolved),
				}
				result.Ready++
				if dryRun {
					item.Outcome = "ready"
					result.Items = append(result.Items, item)
					continue
				}
				assignErr := client.AssignIssue(command.Context(), issue.Key, resolved)
				if assignErr == nil {
					item.Outcome = "succeeded"
					result.Succeeded++
					result.Items = append(result.Items, item)
					continue
				}
				item.Outcome = "failed"
				item.Error = assignErr.Error()
				result.Failed++
				result.Items = append(result.Items, item)
				if isSystemicIssueError(assignErr) {
					appendNotAttempted(&result, issues.Issues[index+1:], assigneeBatchTarget(resolved), assignErr.Error())
					break
				}
			}
			return a.finishIssueBatch(result)
		},
	}
	flags := command.Flags()
	flags.StringVar(&jql, "jql", "", "JQL selecting every issue to process")
	flags.StringVar(&assignee, "assignee", "", "assignee username; use me or none")
	flags.BoolVar(&dryRun, "dry-run", false, "preflight every matching issue without changing Jira")
	flags.BoolVar(&yes, "yes", false, "confirm execution without prompting")
	return command
}

func assigneeBatchTarget(target jira.AssigneeTarget) issueBatchTarget {
	if target.Username == nil {
		return issueBatchTarget{Unassigned: true}
	}
	return issueBatchTarget{Assignee: target.Username}
}

func appendNotAttempted(result *issueBatchResult, issues []jira.Issue, target issueBatchTarget, reason string) {
	for _, issue := range issues {
		result.Items = append(result.Items, issueBatchItem{
			IssueKey: issue.Key, Outcome: "not_attempted",
			Current: issueBatchCurrent{Status: issue.Status, Assignee: issue.Assignee}, Target: target,
			Error: "not attempted after systemic failure: " + reason,
		})
		result.NotAttempted++
	}
}

func (a *app) finishIssueBatch(result issueBatchResult) error {
	if result.Failed > 0 || result.NotAttempted > 0 {
		if err := a.renderPartial(result, issueBatchTable(result)); err != nil {
			return err
		}
		return apperr.New(apperr.KindPartialFailure, fmt.Sprintf(
			"bulk %s completed with %d failed and %d not attempted", result.Operation, result.Failed, result.NotAttempted,
		))
	}
	return a.render(result, issueBatchTable(result))
}

func issueBatchTable(result issueBatchResult) output.Table {
	rows := make([][]string, 0, len(result.Items))
	for _, item := range result.Items {
		rows = append(rows, []string{item.IssueKey, item.Outcome, item.Error})
	}
	return output.Table{Headers: []string{"ISSUE", "OUTCOME", "ERROR"}, Rows: rows}
}

func isSystemicIssueError(err error) bool {
	appErr := apperr.As(err)
	switch appErr.Kind {
	case apperr.KindAuth, apperr.KindRateLimit, apperr.KindUnexpected, apperr.KindConfig:
		return true
	case apperr.KindAPI:
		return appErr.StatusCode == 0 || appErr.StatusCode >= http.StatusInternalServerError
	default:
		return false
	}
}
