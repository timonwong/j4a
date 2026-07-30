// Package jira provides a normalized client for Jira Data Center and Server's
// REST API v2. It deliberately keeps Jira's wire payloads private.
package jira

// Page identifies a page in Jira list results.
type Page struct {
	StartAt    int `json:"startAt"`
	MaxResults int `json:"maxResults"`
}

// User is a Jira user in j4a's stable representation.
type User struct {
	AccountID    string `json:"accountId"`
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	Active       bool   `json:"active"`
}

// Project identifies a Jira project.
type Project struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
	ProjectType string `json:"projectType,omitempty"`
	Lead        *User  `json:"lead,omitempty"`
}

// IssueType identifies an issue type.
type IssueType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Status identifies an issue status.
type Status struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
}

// Priority identifies an issue priority.
type Priority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Component identifies an issue component.
type Component struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Issue is a normalized issue. Fields holds Jira field values keyed by their
// Jira field IDs, without exposing the rest of Jira's wire document.
type Issue struct {
	ID          string         `json:"id"`
	Key         string         `json:"key"`
	Summary     string         `json:"summary"`
	Description string         `json:"description,omitempty"`
	Project     *Project       `json:"project,omitempty"`
	IssueType   *IssueType     `json:"issueType,omitempty"`
	Status      *Status        `json:"status,omitempty"`
	Priority    *Priority      `json:"priority,omitempty"`
	Assignee    *User          `json:"assignee,omitempty"`
	Reporter    *User          `json:"reporter,omitempty"`
	Labels      []string       `json:"labels,omitempty"`
	Components  []Component    `json:"components,omitempty"`
	Created     string         `json:"created,omitempty"`
	Updated     string         `json:"updated,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
}

// IssueListOptions controls a JQL issue list query.
type IssueListOptions struct {
	JQL    string   `json:"jql"`
	Page   Page     `json:"page"`
	Fields []string `json:"fields,omitempty"`
}

// SearchResult is a normalized Jira search response.
type SearchResult struct {
	StartAt    int     `json:"startAt"`
	MaxResults int     `json:"maxResults"`
	Total      int     `json:"total"`
	Issues     []Issue `json:"issues"`
}

// CreateIssueInput is the writable normalized form for creating an issue.
type CreateIssueInput struct {
	ProjectKey  string         `json:"projectKey"`
	IssueType   string         `json:"issueType"`
	Summary     string         `json:"summary"`
	Description *string        `json:"description,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
}

// UpdateIssueInput is the writable normalized form for updating an issue.
// Nil pointers leave standard fields unchanged; Fields are sent as supplied.
type UpdateIssueInput struct {
	Summary     *string        `json:"summary,omitempty"`
	Description *string        `json:"description,omitempty"`
	Fields      map[string]any `json:"fields,omitempty"`
}

// Comment is a normalized Jira issue comment.
type Comment struct {
	ID      string `json:"id"`
	Body    string `json:"body"`
	Author  *User  `json:"author,omitempty"`
	Created string `json:"created,omitempty"`
	Updated string `json:"updated,omitempty"`
}

// CommentInput describes a new or replacement comment body.
type CommentInput struct {
	Body string `json:"body"`
}

// Transition identifies a transition available for an issue.
type Transition struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	To     *Status `json:"to,omitempty"`
	Fields []Field `json:"fields,omitempty"`
}

// TransitionInput moves an issue using a Jira transition ID. Fields may
// contain values required by that transition screen.
type TransitionInput struct {
	ID     string         `json:"id"`
	Fields map[string]any `json:"fields,omitempty"`
}

// Field is a normalized Jira field definition.
type Field struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Custom bool   `json:"custom"`
	Type   string `json:"type,omitempty"`
}
