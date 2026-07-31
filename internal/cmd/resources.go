package cmd

import (
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/timonwong/jiro/internal/apperr"
	"github.com/timonwong/jiro/internal/jira"
	"github.com/timonwong/jiro/internal/output"
)

func (a *app) searchCommand() *cobra.Command {
	var limit, offset int
	var all bool
	var fields []string
	command := &cobra.Command{
		Use:   "search JQL",
		Short: "Search issues with JQL",
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
				if all {
					return apperr.New(apperr.KindInvalidInput, "--all cannot be combined with --raw")
				}
				return a.rawRequest(command.Context(), client, http.MethodPost, "rest/api/2/search", nil, searchPayload(args[0], offset, limit, fields))
			}
			var result jira.SearchResult
			if all {
				result, err = searchAll(command.Context(), client, args[0], offset, limit, fields)
			} else {
				result, err = client.Search(command.Context(), jira.IssueListOptions{
					JQL: args[0], Page: jira.Page{StartAt: offset, MaxResults: limit}, Fields: fields,
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
	command.Flags().IntVarP(&limit, "limit", "n", 50, "maximum issues per page")
	command.Flags().IntVar(&offset, "offset", 0, "zero-based result offset")
	command.Flags().BoolVar(&all, "all", false, "fetch all result pages")
	command.Flags().StringSliceVar(&fields, "fields", nil, "comma-separated Jira fields to request")
	return command
}

func (a *app) myselfCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "myself",
		Short: "Show the authenticated Jira user",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			client, _, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodGet, "rest/api/2/myself", nil, nil)
			}
			user, err := client.Myself(command.Context())
			if err != nil {
				return err
			}
			return a.render(user, output.Table{
				Headers: []string{"FIELD", "VALUE"},
				Rows: [][]string{
					{"Username", user.Username}, {"Display Name", user.DisplayName},
					{"Email", user.EmailAddress}, {"Active", stringValue(user.Active)},
				},
			})
		},
	}
}

func (a *app) projectsCommand() *cobra.Command {
	command := &cobra.Command{Use: "projects", Aliases: []string{"project"}, Short: "Work with Jira projects"}
	command.AddCommand(a.projectsListCommand(), a.projectShowCommand())
	return command
}

func (a *app) projectsListCommand() *cobra.Command {
	var limit, offset int
	command := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validatePagination(offset, limit); err != nil {
				return err
			}
			client, _, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodGet, "rest/api/2/project", pageQuery(offset, limit), nil)
			}
			projects, err := client.ListProjects(command.Context(), jira.Page{StartAt: offset, MaxResults: limit})
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(projects))
			for _, project := range projects {
				rows = append(rows, []string{project.Key, project.Name, project.ProjectType})
			}
			return a.render(map[string]any{"projects": projects}, output.Table{
				Headers: []string{"KEY", "NAME", "TYPE"}, Rows: rows,
			})
		},
	}
	command.Flags().IntVarP(&limit, "limit", "n", 50, "maximum projects")
	command.Flags().IntVar(&offset, "offset", 0, "zero-based result offset")
	return command
}

func (a *app) projectShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show PROJECT-KEY",
		Short: "Show a project",
		Args:  exactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, _, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				return a.rawRequest(command.Context(), client, http.MethodGet, "rest/api/2/project/"+url.PathEscape(args[0]), nil, nil)
			}
			project, err := client.ShowProject(command.Context(), args[0])
			if err != nil {
				return err
			}
			lead := ""
			if project.Lead != nil {
				lead = project.Lead.DisplayName
			}
			return a.render(project, output.Table{
				Headers: []string{"FIELD", "VALUE"},
				Rows: [][]string{
					{"Key", project.Key}, {"Name", project.Name}, {"Type", project.ProjectType},
					{"Lead", lead}, {"Description", project.Description},
				},
			})
		},
	}
}

func (a *app) fieldsCommand() *cobra.Command {
	command := &cobra.Command{Use: "fields", Aliases: []string{"field"}, Short: "Inspect Jira fields"}
	command.AddCommand(a.fieldsListCommand())
	return command
}

func (a *app) fieldsListCommand() *cobra.Command {
	var customOnly bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List Jira fields",
		Args:  exactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			client, settings, err := a.client()
			if err != nil {
				return err
			}
			if isRaw(a) {
				if customOnly {
					return apperr.New(apperr.KindInvalidInput, "--custom cannot be combined with --raw")
				}
				return a.rawRequest(command.Context(), client, http.MethodGet, "rest/api/2/field", nil, nil)
			}
			var fields []jira.Field
			if customOnly {
				metadata, err := a.loadCustomFieldMetadata(command.Context(), client, settings)
				if err != nil {
					return err
				}
				fields = metadata.fields
			} else {
				fields, err = client.ListFields(command.Context())
				if err != nil {
					return err
				}
			}
			type fieldView struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Alias  string `json:"alias,omitempty"`
				Custom bool   `json:"custom"`
				Type   string `json:"type,omitempty"`
			}
			views := make([]fieldView, 0, len(fields))
			rows := make([][]string, 0, len(fields))
			for _, field := range fields {
				alias := ""
				if field.Custom {
					alias = jira.Slug(field.Name)
				}
				views = append(views, fieldView{ID: field.ID, Name: field.Name, Alias: alias, Custom: field.Custom, Type: field.Type})
				rows = append(rows, []string{field.ID, field.Name, alias, field.Type})
			}
			return a.render(map[string]any{"fields": views}, output.Table{
				Headers: []string{"ID", "NAME", "ALIAS", "TYPE"}, Rows: rows,
			})
		},
	}
	command.Flags().BoolVar(&customOnly, "custom", false, "show custom fields only")
	return command
}
