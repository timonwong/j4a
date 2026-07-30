package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timonwong/j4a/internal/config"
)

func (a *app) configureCompletions(root *cobra.Command) {
	root.CompletionOptions.SetDefaultShellCompDirective(cobra.ShellCompDirectiveNoFileComp)
	must(root.MarkPersistentFlagFilename("config"))
	mustRegisterFlagCompletion(root, "profile", func(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		names, err := config.ProfileNames(a.configOptions())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})
	mustRegisterFlagCompletion(root, "auth-type", fixedCompletions(
		cobra.CompletionWithDesc("basic", "Username and password authentication"),
		cobra.CompletionWithDesc("pat", "Personal Access Token authentication"),
	))
	mustRegisterFlagCompletion(root, "output", fixedCompletions(
		cobra.CompletionWithDesc("text", "Human-readable output"),
		cobra.CompletionWithDesc("json", "Versioned machine-readable output"),
	))

	issuesList := mustFindCommand(root, "issues", "list")
	mustRegisterFlagCompletion(issuesList, "assignee", fixedCompletions(
		cobra.CompletionWithDesc("me", "Use Jira currentUser()"),
	))

	for _, path := range [][]string{{"issues", "create"}, {"issues", "update"}} {
		command := mustFindCommand(root, path...)
		mustRegisterFlagCompletion(command, "input-format", inputFormatCompletions())
		mustRegisterFlagCompletion(command, "assignee", fixedCompletions(
			cobra.CompletionWithDesc("none", "Clear the assignee"),
		))
		mustRegisterFlagCompletion(command, "description-file", fileOrStdinCompletions)
	}

	issueComment := mustFindCommand(root, "issues", "comment")
	mustRegisterFlagCompletion(issueComment, "input-format", inputFormatCompletions())
	mustRegisterFlagCompletion(issueComment, "body-file", fileOrStdinCompletions)
}

func inputFormatCompletions() cobra.CompletionFunc {
	return fixedCompletions(
		cobra.CompletionWithDesc("jira", "Jira wiki markup"),
		cobra.CompletionWithDesc("markdown", "Convert Markdown to Jira markup"),
	)
}

func fixedCompletions(values ...cobra.Completion) cobra.CompletionFunc {
	return cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp)
}

func fileOrStdinCompletions(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	return []cobra.Completion{cobra.CompletionWithDesc("-", "Read from stdin")}, cobra.ShellCompDirectiveDefault
}

func mustFindCommand(root *cobra.Command, path ...string) *cobra.Command {
	command, remaining, err := root.Find(path)
	if err != nil || len(remaining) != 0 || command == root {
		panic(fmt.Sprintf("completion command path %q is invalid", strings.Join(path, " ")))
	}
	return command
}

func mustRegisterFlagCompletion(command *cobra.Command, name string, completion cobra.CompletionFunc) {
	must(command.RegisterFlagCompletionFunc(name, completion))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
