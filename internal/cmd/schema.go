package cmd

import (
	"github.com/spf13/cobra"
	"github.com/timonwong/jiro/internal/output"
)

type cliSchema struct {
	ContractVersion string            `json:"contractVersion"`
	Program         string            `json:"program"`
	Platform        string            `json:"platform"`
	GlobalFlags     []flagSchema      `json:"globalFlags"`
	Commands        []commandSchema   `json:"commands"`
	Types           map[string]any    `json:"types"`
	Output          outputSchema      `json:"output"`
	ExitCodes       map[string]string `json:"exitCodes"`
}

type commandSchema struct {
	Name                 string         `json:"name"`
	Aliases              []string       `json:"aliases,omitempty"`
	Auth                 bool           `json:"auth"`
	Mutating             bool           `json:"mutating"`
	MutationMode         string         `json:"mutationMode,omitempty"`
	ReadOnlyMethods      []string       `json:"readOnlyMethods,omitempty"`
	OutputMode           string         `json:"outputMode"`
	SupportsGlobalOutput bool           `json:"supportsGlobalOutput"`
	Args                 string         `json:"args,omitempty"`
	Flags                []flagSchema   `json:"flags,omitempty"`
	JSONData             map[string]any `json:"jsonData"`
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
	Default        string `json:"default"`
	JSONEnvelope   string `json:"jsonEnvelope"`
	ErrorEnvelope  string `json:"errorEnvelope"`
	SuccessStream  string `json:"successStream"`
	ErrorStream    string `json:"errorStream"`
	PartialFailure string `json:"partialFailure"`
	SchemaVersion  string `json:"schemaVersion"`
	Warnings       string `json:"warnings"`
}

func (a *app) schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Describe jiro's machine-readable CLI contract",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			renderer := output.New(a.stdout, a.stderr, output.FormatJSON, false)
			return renderer.Success(schemaDocument())
		},
	}
}

func schemaDocument() cliSchema {
	return cliSchema{
		ContractVersion: "3",
		Program:         "jiro",
		Platform:        "Jira Data Center/Server REST API v2",
		GlobalFlags: []flagSchema{
			flag("config", "c", "path"), flag("profile", "", "string"),
			flagDefault("output", "o", "enum:text|json", "text"),
			flag("quiet", "", "boolean"),
		},
		Commands: []commandSchema{
			commandWithFlags("cache refresh", nil, true, "", nil, object("refreshed", "path", "fieldCount", "fetchedAt", "expiresAt", "instance", "principal")),
			normalizedCommand(commandSchema{Name: "auth login", Auth: false, Mutating: true, Flags: []flagSchema{
				flagDefault("use-keyring", "", "boolean", true), flag("password-stdin", "", "boolean"), flag("token-stdin", "", "boolean"),
			}, JSONData: object("profile", "host", "authType", "credentialStore", "user")}),
			normalizedCommand(commandSchema{Name: "auth logout", Auth: false, Mutating: true, JSONData: object("profile", "credentialStore", "credentialRemoved", "environmentCredentialActive")}),
			normalizedCommand(commandSchema{Name: "auth status", Auth: true, Mutating: false, JSONData: object("profile", "instance", "authType", "user")}),
			{
				Name: "api", Auth: true, Mutating: true, MutationMode: "http_method",
				ReadOnlyMethods: append([]string(nil), apiReadOnlyMethodNames...), OutputMode: "raw_http", SupportsGlobalOutput: false,
				Args: "ENDPOINT", Flags: []flagSchema{
					flagDefault("method", "X", "string", "GET"),
					flag("input", "", "path-or-stdin"), repeatableFlag("string-field", "f", "key=value"), repeatableFlag("field", "F", "key=value"),
					repeatableFlag("form", "", "key=value"), repeatableFlag("header", "H", "header"),
					flagDefault("timeout", "", "duration", "30s"), flag("insecure", "", "boolean"), flag("include", "i", "boolean"),
				}, JSONData: nil,
			},
			commandWithFlags("issue list", nil, false, "", []flagSchema{
				flag("project", "p", "string"), flag("status", "", "string"), flag("assignee", "", "string"),
				flag("type", "t", "string"), flag("resolution", "", "string"), flag("reporter", "", "string"),
				repeatableFlag("label", "", "string"), repeatableFlag("component", "", "string"), repeatableFlag("fix-version", "", "string"),
				flag("sprint", "", "string"), flag("parent", "", "issue-key"), flag("created", "", "jira-date"), flag("updated", "", "jira-date"),
				flag("jql", "q", "string"), flagDefault("limit", "n", "integer", 50),
				flagDefault("offset", "", "integer", 0), flag("all", "", "boolean"), flag("fields", "", "string-list"),
			}, object("startAt", "maxResults", "total", "issues")),
			commandWithFlags("issue show", nil, false, "ISSUE-KEY", []flagSchema{flag("fields", "", "string-list")}, object("id", "key", "summary", "description", "status", "fields")),
			commandWithFlags("issue add", nil, true, "", []flagSchema{
				requiredFlag("project", "p", "string"), requiredFlag("type", "t", "string"), requiredFlag("summary", "s", "string"),
				flag("description", "", "string"), flag("description-file", "", "path-or-stdin"), flagDefault("input-format", "", "enum:jira|markdown", "jira"),
				flag("priority", "", "string"), flag("assignee", "", "string"), repeatableFlag("label", "", "string"), flag("parent", "", "issue-key"),
				repeatableFlag("component", "", "string"), repeatableFlag("fix-version", "", "string"), flag("sprint", "", "string"), repeatableFlag("field", "", "key=value"),
			}, object("id", "key", "sprint", "sprintMoved")),
			commandWithFlags("issue update", nil, true, "ISSUE-KEY", []flagSchema{
				flag("summary", "s", "string"), flag("description", "", "string"), flag("description-file", "", "path-or-stdin"),
				flagDefault("input-format", "", "enum:jira|markdown", "jira"), flag("priority", "", "string"), flag("assignee", "", "string"),
				repeatableFlag("label", "", "string"), repeatableFlag("component", "", "string"), repeatableFlag("fix-version", "", "string"), flag("sprint", "", "string"), repeatableFlag("field", "", "key=value"),
			}, object("key", "updated", "sprint", "sprintMoved")),
			commandWithFlags("issue comment list", nil, false, "ISSUE-KEY", []flagSchema{
				flagDefault("limit", "n", "integer", 50), flagDefault("offset", "", "integer", 0),
			}, object("issueKey", "comments")),
			commandWithFlags("issue comment add", nil, true, "ISSUE-KEY", []flagSchema{
				flag("body", "", "string"), flag("body-file", "", "path-or-stdin"), flagDefault("input-format", "", "enum:jira|markdown", "jira"),
			}, object("id", "body", "author", "created")),
			command("issue list-transitions", false, "ISSUE-KEY", object("issueKey", "transitions")),
			commandWithFlags("issue move", nil, true, "ISSUE-KEY", []flagSchema{
				requiredFlag("to", "", "string"), repeatableFlag("field", "", "key=value"),
			}, object("key", "transition", "transitioned")),
			commandWithFlags("issue assign", nil, true, "ISSUE-KEY", []flagSchema{
				requiredFlag("assignee", "", "string"),
			}, object("key", "assignee", "assigned")),
			command("issue link list", false, "ISSUE-KEY", object("issueKey", "links")),
			commandWithFlags("issue link add", nil, true, "FROM", []flagSchema{
				requiredFlag("to", "", "issue-key"), requiredFlag("type", "", "string"),
			}, object("from", "to", "type", "linked")),
			command("issue link delete", true, "LINK-ID", object("linkId", "unlinked")),
			command("issue link types", false, "", object("linkTypes")),
			commandWithFlags("issue bulk move", nil, true, "", []flagSchema{
				requiredFlag("jql", "", "string"), requiredFlag("to", "", "string"), repeatableFlag("field", "", "key=value"), flag("dry-run", "", "boolean"), flag("yes", "", "boolean"),
			}, batchObject()),
			commandWithFlags("issue bulk assign", nil, true, "", []flagSchema{
				requiredFlag("jql", "", "string"), requiredFlag("assignee", "", "string"), flag("dry-run", "", "boolean"), flag("yes", "", "boolean"),
			}, batchObject()),
			commandWithFlags("search", nil, false, "JQL", []flagSchema{
				flagDefault("limit", "n", "integer", 50), flagDefault("offset", "", "integer", 0), flag("all", "", "boolean"), flag("fields", "", "string-list"),
			}, object("startAt", "maxResults", "total", "issues")),
			commandWithFlags("project list", nil, false, "", []flagSchema{
				flagDefault("limit", "n", "integer", 50), flagDefault("offset", "", "integer", 0),
			}, object("projects")),
			commandWithFlags("project show", nil, false, "PROJECT-KEY", nil, object("id", "key", "name", "description", "projectType", "lead")),
			commandWithFlags("field list", nil, false, "", []flagSchema{flag("custom", "", "boolean")}, object("fields")),
			normalizedCommand(commandSchema{Name: "schema", Auth: false, Mutating: false, JSONData: object("contractVersion", "program", "platform", "globalFlags", "commands", "types", "output", "exitCodes")}),
		},
		Types: typeDefinitions(),
		Output: outputSchema{
			Default: "text", JSONEnvelope: `{"schemaVersion":"1","data":...,"warnings":[...]?}`,
			ErrorEnvelope:  `{"schemaVersion":"1","error":{"kind":...,"message":...}}`,
			SuccessStream:  "stdout",
			ErrorStream:    "stderr",
			PartialFailure: "on partial_failure, complete normalized result data is written to stdout before the structured error is written to stderr; exit code 7",
			SchemaVersion:  output.SchemaVersion, Warnings: "non-fatal success conditions; JSON envelope or text stderr",
		},
		ExitCodes: map[string]string{
			"0": "success", "1": "unexpected error", "2": "invalid input or config", "3": "authentication failed",
			"4": "resource not found", "5": "Jira API error", "6": "rate limited", "7": "partial failure",
		},
	}
}

func command(name string, mutating bool, args string, data map[string]any) commandSchema {
	return commandWithFlags(name, nil, mutating, args, nil, data)
}

func commandWithFlags(name string, aliases []string, mutating bool, args string, flags []flagSchema, data map[string]any) commandSchema {
	return normalizedCommand(commandSchema{Name: name, Aliases: aliases, Auth: true, Mutating: mutating, Args: args, Flags: flags, JSONData: data})
}

func normalizedCommand(command commandSchema) commandSchema {
	command.OutputMode = "normalized"
	command.SupportsGlobalOutput = true
	return command
}

func object(fields ...string) map[string]any {
	result := make(map[string]any, len(fields))
	for _, field := range fields {
		result[field] = "see command schema"
	}
	return result
}

func batchObject() map[string]any {
	return batchResultDefinition()
}

func typeDefinitions() map[string]any {
	return map[string]any{
		"Version":       object("id", "name", "archived", "released"),
		"Sprint":        object("id", "name", "state", "boardId", "goal", "startDate", "endDate", "completeDate"),
		"IssueLinkType": object("id", "name", "inward", "outward"),
		"IssueLink": map[string]any{
			"id": "Jira Link ID", "direction": "inward|outward", "relationship": "direction-relative description",
			"type": "IssueLinkType", "otherIssue": object("id", "key", "summary"),
		},
		"BatchCurrent": object("status", "assignee"),
		"BatchTarget":  object("transitionSpec", "transition", "assignee", "unassigned"),
		"BatchItem":    batchItemDefinition(),
		"BatchResult":  batchResultDefinition(),
	}
}

func batchResultDefinition() map[string]any {
	result := object("operation", "dryRun", "jql", "total", "ready", "succeeded", "failed", "notAttempted")
	result["items"] = []any{batchItemDefinition()}
	return result
}

func batchItemDefinition() map[string]any {
	return map[string]any{
		"issueKey": "Issue Key",
		"outcome":  "ready|succeeded|failed|not_attempted",
		"current":  "BatchCurrent",
		"target":   "BatchTarget",
		"error":    "present for failed and not_attempted outcomes",
	}
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
