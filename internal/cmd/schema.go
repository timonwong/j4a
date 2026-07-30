package cmd

import (
	"github.com/spf13/cobra"
	"github.com/timonwong/j4a/internal/apperr"
	"github.com/timonwong/j4a/internal/output"
)

type cliSchema struct {
	ContractVersion string            `json:"contractVersion"`
	Program         string            `json:"program"`
	Platform        string            `json:"platform"`
	GlobalFlags     []flagSchema      `json:"globalFlags"`
	Commands        []commandSchema   `json:"commands"`
	Output          outputSchema      `json:"output"`
	ExitCodes       map[string]string `json:"exitCodes"`
}

type commandSchema struct {
	Name     string         `json:"name"`
	Aliases  []string       `json:"aliases,omitempty"`
	Auth     bool           `json:"auth"`
	Mutating bool           `json:"mutating"`
	Args     string         `json:"args,omitempty"`
	Flags    []flagSchema   `json:"flags,omitempty"`
	JSONData map[string]any `json:"jsonData"`
}

type flagSchema struct {
	Name       string `json:"name"`
	Short      string `json:"short,omitempty"`
	Type       string `json:"type"`
	Required   bool   `json:"required,omitempty"`
	Repeatable bool   `json:"repeatable,omitempty"`
	Default    any    `json:"default,omitempty"`
}

type outputSchema struct {
	Default       string `json:"default"`
	JSONEnvelope  string `json:"jsonEnvelope"`
	ErrorEnvelope string `json:"errorEnvelope"`
	SuccessStream string `json:"successStream"`
	ErrorStream   string `json:"errorStream"`
	Raw           string `json:"raw"`
	SchemaVersion string `json:"schemaVersion"`
}

func (a *app) schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Describe j4a's machine-readable CLI contract",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if a.raw {
				return apperr.New(apperr.KindInvalidInput, "--raw is not available for schema")
			}
			renderer := output.New(a.stdout, a.stderr, output.FormatJSON, false)
			return renderer.Success(schemaDocument())
		},
	}
}

func schemaDocument() cliSchema {
	return cliSchema{
		ContractVersion: "1",
		Program:         "j4a",
		Platform:        "Jira Data Center/Server REST API v2",
		GlobalFlags: []flagSchema{
			flag("config", "c", "path"), flag("profile", "", "string"), flag("host", "", "url"),
			flag("username", "", "string"), flag("auth-type", "", "enum:basic|pat"),
			flagDefault("output", "o", "enum:text|json", "text"), flag("json", "", "boolean"),
			flag("raw", "", "boolean"), flag("quiet", "", "boolean"),
		},
		Commands: []commandSchema{
			command("myself", false, "", object("accountId", "username", "displayName", "emailAddress", "active")),
			commandWithFlags("issues list", []string{"issue list"}, false, "", []flagSchema{
				flag("project", "p", "string"), flag("status", "", "string"), flag("assignee", "", "string"),
				flag("type", "t", "string"), flag("jql", "q", "string"), flagDefault("limit", "n", "integer", 50),
				flagDefault("offset", "", "integer", 0), flag("all", "", "boolean"), flag("fields", "", "string-list"),
			}, object("startAt", "maxResults", "total", "issues")),
			commandWithFlags("issues show", []string{"issue show"}, false, "ISSUE-KEY", []flagSchema{flag("fields", "", "string-list")}, object("id", "key", "summary", "description", "status", "fields")),
			commandWithFlags("issues create", []string{"issue create"}, true, "", []flagSchema{
				requiredFlag("project", "p", "string"), requiredFlag("type", "t", "string"), requiredFlag("summary", "s", "string"),
				flag("description", "", "string"), flag("description-file", "", "path-or-stdin"), flagDefault("input-format", "", "enum:jira|markdown", "jira"),
				flag("priority", "", "string"), flag("assignee", "", "string"), repeatableFlag("label", "", "string"), repeatableFlag("field", "", "key=value"),
			}, object("id", "key")),
			commandWithFlags("issues update", []string{"issue update"}, true, "ISSUE-KEY", []flagSchema{
				flag("summary", "s", "string"), flag("description", "", "string"), flag("description-file", "", "path-or-stdin"),
				flagDefault("input-format", "", "enum:jira|markdown", "jira"), flag("priority", "", "string"), flag("assignee", "", "string"),
				repeatableFlag("label", "", "string"), repeatableFlag("field", "", "key=value"),
			}, object("key", "updated")),
			commandWithFlags("issues comments", []string{"issue comments"}, false, "ISSUE-KEY", []flagSchema{
				flagDefault("limit", "n", "integer", 50), flagDefault("offset", "", "integer", 0),
			}, object("issueKey", "comments")),
			commandWithFlags("issues comment", []string{"issue comment"}, true, "ISSUE-KEY", []flagSchema{
				flag("body", "", "string"), flag("body-file", "", "path-or-stdin"), flagDefault("input-format", "", "enum:jira|markdown", "jira"),
			}, object("id", "body", "author", "created")),
			command("issues list-transitions", false, "ISSUE-KEY", object("issueKey", "transitions")),
			commandWithFlags("issues transition", []string{"issue transition"}, true, "ISSUE-KEY", []flagSchema{
				requiredFlag("to", "", "string"), repeatableFlag("field", "", "key=value"),
			}, object("key", "transition", "transitioned")),
			commandWithFlags("search", nil, false, "JQL", []flagSchema{
				flagDefault("limit", "n", "integer", 50), flagDefault("offset", "", "integer", 0), flag("all", "", "boolean"), flag("fields", "", "string-list"),
			}, object("startAt", "maxResults", "total", "issues")),
			commandWithFlags("projects list", []string{"project list"}, false, "", []flagSchema{
				flagDefault("limit", "n", "integer", 50), flagDefault("offset", "", "integer", 0),
			}, object("projects")),
			commandWithFlags("projects show", []string{"project show"}, false, "PROJECT-KEY", nil, object("id", "key", "name", "description", "projectType", "lead")),
			commandWithFlags("fields list", []string{"field list"}, false, "", []flagSchema{flag("custom", "", "boolean")}, object("fields")),
			{Name: "config show", Auth: false, Mutating: false, JSONData: object("profile", "host", "username", "authType", "apiVersion", "readOnly", "useKeyring")},
			{Name: "config path", Auth: false, Mutating: false, JSONData: object("path")},
			{Name: "config set-secret", Auth: false, Mutating: true, JSONData: object("stored", "profile")},
			{Name: "config delete-secret", Auth: false, Mutating: true, JSONData: object("deleted", "profile")},
			{Name: "schema", Auth: false, Mutating: false, JSONData: object("contractVersion", "program", "platform", "globalFlags", "commands", "output", "exitCodes")},
		},
		Output: outputSchema{
			Default: "text", JSONEnvelope: `{"schemaVersion":"1","data":...}`,
			ErrorEnvelope: `{"schemaVersion":"1","error":{"kind":...,"message":...}}`,
			SuccessStream: "stdout", ErrorStream: "stderr", Raw: "unmodified Jira REST response",
			SchemaVersion: output.SchemaVersion,
		},
		ExitCodes: map[string]string{
			"0": "success", "1": "unexpected error", "2": "invalid input or config", "3": "authentication failed",
			"4": "resource not found", "5": "Jira API error", "6": "rate limited",
		},
	}
}

func command(name string, mutating bool, args string, data map[string]any) commandSchema {
	return commandWithFlags(name, nil, mutating, args, nil, data)
}

func commandWithFlags(name string, aliases []string, mutating bool, args string, flags []flagSchema, data map[string]any) commandSchema {
	return commandSchema{Name: name, Aliases: aliases, Auth: true, Mutating: mutating, Args: args, Flags: flags, JSONData: data}
}

func object(fields ...string) map[string]any {
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		result[field] = "see command schema"
	}
	return result
}

func flag(name, short, kind string) flagSchema {
	return flagSchema{Name: name, Short: short, Type: kind}
}

func requiredFlag(name, short, kind string) flagSchema {
	value := flag(name, short, kind)
	value.Required = true
	return value
}

func repeatableFlag(name, short, kind string) flagSchema {
	value := flag(name, short, kind)
	value.Repeatable = true
	return value
}

func flagDefault(name, short, kind string, defaultValue any) flagSchema {
	value := flag(name, short, kind)
	value.Default = defaultValue
	return value
}
