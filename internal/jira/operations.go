package jira

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/timonwong/j4a/internal/apperr"
)

// Myself returns the authenticated Jira user.
func (c *Client) Myself(ctx context.Context) (User, error) {
	var wire wireUser
	if err := c.do(ctx, http.MethodGet, "rest/api/2/myself", nil, nil, &wire); err != nil {
		return User{}, err
	}
	return normalizeUser(wire), nil
}

// Search searches issues using JQL, pagination, and an optional field list.
func (c *Client) Search(ctx context.Context, opts IssueListOptions) (SearchResult, error) {
	if strings.TrimSpace(opts.JQL) == "" {
		return SearchResult{}, apperr.New(apperr.KindInvalidInput, "JQL is required")
	}
	payload := map[string]any{"jql": opts.JQL}
	if opts.Page.StartAt > 0 {
		payload["startAt"] = opts.Page.StartAt
	}
	if opts.Page.MaxResults > 0 {
		payload["maxResults"] = opts.Page.MaxResults
	}
	if len(opts.Fields) > 0 {
		payload["fields"] = opts.Fields
	}
	var wire wireSearch
	if err := c.do(ctx, http.MethodPost, "rest/api/2/search", nil, payload, &wire); err != nil {
		return SearchResult{}, err
	}
	return normalizeSearch(wire), nil
}

// ListIssues is an explicit alias for Search.
func (c *Client) ListIssues(ctx context.Context, opts IssueListOptions) (SearchResult, error) {
	return c.Search(ctx, opts)
}

// ShowIssue returns one issue. fields selects Jira fields when non-empty.
func (c *Client) ShowIssue(ctx context.Context, key string, fields []string) (Issue, error) {
	if strings.TrimSpace(key) == "" {
		return Issue{}, apperr.New(apperr.KindInvalidInput, "issue key is required")
	}
	query := url.Values{}
	if len(fields) > 0 {
		query.Set("fields", strings.Join(fields, ","))
	}
	var wire wireIssue
	if err := c.do(ctx, http.MethodGet, "rest/api/2/issue/"+url.PathEscape(key), query, nil, &wire); err != nil {
		return Issue{}, err
	}
	return normalizeIssue(wire), nil
}

// CreateIssue creates and returns an issue.
func (c *Client) CreateIssue(ctx context.Context, input CreateIssueInput) (Issue, error) {
	if strings.TrimSpace(input.ProjectKey) == "" || strings.TrimSpace(input.IssueType) == "" || strings.TrimSpace(input.Summary) == "" {
		return Issue{}, apperr.New(apperr.KindInvalidInput, "project key, issue type, and summary are required")
	}
	fields := cloneFields(input.Fields)
	fields["project"] = map[string]string{"key": input.ProjectKey}
	fields["issuetype"] = map[string]string{"name": input.IssueType}
	fields["summary"] = input.Summary
	if input.Description != nil {
		fields["description"] = *input.Description
	}
	var wire wireIssue
	if err := c.do(ctx, http.MethodPost, "rest/api/2/issue", nil, map[string]any{"fields": fields}, &wire); err != nil {
		return Issue{}, err
	}
	return normalizeIssue(wire), nil
}

// UpdateIssue updates fields on an issue.
func (c *Client) UpdateIssue(ctx context.Context, key string, input UpdateIssueInput) error {
	if strings.TrimSpace(key) == "" {
		return apperr.New(apperr.KindInvalidInput, "issue key is required")
	}
	fields := cloneFields(input.Fields)
	if input.Summary != nil {
		fields["summary"] = *input.Summary
	}
	if input.Description != nil {
		fields["description"] = *input.Description
	}
	if len(fields) == 0 {
		return apperr.New(apperr.KindInvalidInput, "at least one issue field is required")
	}
	return c.do(ctx, http.MethodPut, "rest/api/2/issue/"+url.PathEscape(key), nil, map[string]any{"fields": fields}, nil)
}

// ListComments returns comments on an issue.
func (c *Client) ListComments(ctx context.Context, key string, page Page) ([]Comment, error) {
	if strings.TrimSpace(key) == "" {
		return nil, apperr.New(apperr.KindInvalidInput, "issue key is required")
	}
	query := pageQuery(page)
	var wire wireComments
	if err := c.do(ctx, http.MethodGet, "rest/api/2/issue/"+url.PathEscape(key)+"/comment", query, nil, &wire); err != nil {
		return nil, err
	}
	comments := make([]Comment, len(wire.Comments))
	for i, comment := range wire.Comments {
		comments[i] = normalizeComment(comment)
	}
	return comments, nil
}

// Comments is an explicit alias for ListComments.
func (c *Client) Comments(ctx context.Context, key string, page Page) ([]Comment, error) {
	return c.ListComments(ctx, key, page)
}

// ShowComment returns a single issue comment.
func (c *Client) ShowComment(ctx context.Context, key, id string) (Comment, error) {
	if strings.TrimSpace(key) == "" || strings.TrimSpace(id) == "" {
		return Comment{}, apperr.New(apperr.KindInvalidInput, "issue key and comment ID are required")
	}
	var wire wireComment
	if err := c.do(ctx, http.MethodGet, "rest/api/2/issue/"+url.PathEscape(key)+"/comment/"+url.PathEscape(id), nil, nil, &wire); err != nil {
		return Comment{}, err
	}
	return normalizeComment(wire), nil
}

// Comment creates an issue comment.
func (c *Client) Comment(ctx context.Context, key string, input CommentInput) (Comment, error) {
	if strings.TrimSpace(key) == "" || input.Body == "" {
		return Comment{}, apperr.New(apperr.KindInvalidInput, "issue key and comment body are required")
	}
	var wire wireComment
	if err := c.do(ctx, http.MethodPost, "rest/api/2/issue/"+url.PathEscape(key)+"/comment", nil, input, &wire); err != nil {
		return Comment{}, err
	}
	return normalizeComment(wire), nil
}

// AddComment is an explicit alias for Comment.
func (c *Client) AddComment(ctx context.Context, key string, input CommentInput) (Comment, error) {
	return c.Comment(ctx, key, input)
}

// ListTransitions returns transitions currently available for an issue.
func (c *Client) ListTransitions(ctx context.Context, key string) ([]Transition, error) {
	if strings.TrimSpace(key) == "" {
		return nil, apperr.New(apperr.KindInvalidInput, "issue key is required")
	}
	var wire struct {
		Transitions []wireTransition `json:"transitions"`
	}
	if err := c.do(ctx, http.MethodGet, "rest/api/2/issue/"+url.PathEscape(key)+"/transitions", nil, nil, &wire); err != nil {
		return nil, err
	}
	result := make([]Transition, len(wire.Transitions))
	for i, transition := range wire.Transitions {
		result[i] = normalizeTransition(transition)
	}
	return result, nil
}

// Transition moves an issue through a transition.
func (c *Client) Transition(ctx context.Context, key string, input TransitionInput) error {
	if strings.TrimSpace(key) == "" || strings.TrimSpace(input.ID) == "" {
		return apperr.New(apperr.KindInvalidInput, "issue key and transition ID are required")
	}
	payload := map[string]any{"transition": map[string]string{"id": input.ID}}
	if len(input.Fields) > 0 {
		payload["fields"] = input.Fields
	}
	return c.do(ctx, http.MethodPost, "rest/api/2/issue/"+url.PathEscape(key)+"/transitions", nil, payload, nil)
}

// ListProjects returns Jira projects. Some Jira Server versions return an
// array while newer Data Center versions return a paged object; both are
// normalized here.
func (c *Client) ListProjects(ctx context.Context, page Page) ([]Project, error) {
	query := pageQuery(page)
	var raw []byte
	if err := c.do(ctx, http.MethodGet, "rest/api/2/project", query, nil, &rawJSON{target: &raw}); err != nil {
		return nil, err
	}
	return decodeProjects(raw)
}

// ShowProject returns a project by key or ID.
func (c *Client) ShowProject(ctx context.Context, key string) (Project, error) {
	if strings.TrimSpace(key) == "" {
		return Project{}, apperr.New(apperr.KindInvalidInput, "project key is required")
	}
	var wire wireProject
	if err := c.do(ctx, http.MethodGet, "rest/api/2/project/"+url.PathEscape(key), nil, nil, &wire); err != nil {
		return Project{}, err
	}
	return normalizeProject(wire), nil
}

// ListFields returns live Jira field definitions.
func (c *Client) ListFields(ctx context.Context) ([]Field, error) {
	var wire []wireField
	if err := c.do(ctx, http.MethodGet, "rest/api/2/field", nil, nil, &wire); err != nil {
		return nil, err
	}
	fields := make([]Field, len(wire))
	for i, field := range wire {
		fields[i] = normalizeField(field)
	}
	return fields, nil
}

func pageQuery(page Page) url.Values {
	query := url.Values{}
	if page.StartAt > 0 {
		query.Set("startAt", strconv.Itoa(page.StartAt))
	}
	if page.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(page.MaxResults))
	}
	return query
}

func cloneFields(fields map[string]any) map[string]any {
	copy := make(map[string]any, len(fields)+3)
	for key, value := range fields {
		copy[key] = value
	}
	return copy
}
