// Package cmd defines j4a's public Cobra command tree.
package cmd

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timonwong/j4a/internal/apperr"
	"github.com/timonwong/j4a/internal/config"
	"github.com/timonwong/j4a/internal/fieldcache"
	"github.com/timonwong/j4a/internal/jira"
	"github.com/timonwong/j4a/internal/output"
)

type app struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	terminal    loginTerminal
	secretStore config.SecretStore
	fieldStore  fieldcache.Store
	warnings    []output.Warning

	configPath string
	profile    string
	host       string
	username   string
	authType   string
	output     string
	raw        bool
	quiet      bool
}

// Execute runs j4a and returns a stable process exit code.
func Execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	a := &app{stdin: stdin, stdout: stdout, stderr: stderr}
	return a.execute(args)
}

func (a *app) execute(args []string) int {
	a.warnings = nil
	if a.terminal == nil {
		a.terminal = newLoginTerminal(a.stdin, a.stderr)
	}
	root := a.rootCommand()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		renderer, renderErr := a.renderer()
		if renderErr != nil {
			err = renderErr
			renderer = output.New(a.stdout, a.stderr, output.FormatText, false)
		}
		_ = renderer.Error(err)
		return apperr.ExitCode(err)
	}
	return 0
}

func (a *app) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "j4a",
		Short:         "A scriptable Jira CLI for humans and AI agents",
		Version:       "dev",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			_, err := a.renderer()
			return err
		},
	}
	root.SetIn(a.stdin)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return apperr.Wrap(apperr.KindInvalidInput, err, "invalid command flags")
	})

	flags := root.PersistentFlags()
	flags.StringVarP(&a.configPath, "config", "c", "", "config file path")
	flags.StringVar(&a.profile, "profile", "", "config profile name")
	flags.StringVar(&a.host, "host", "", "Jira base URL")
	flags.StringVar(&a.username, "username", "", "Jira username for Basic Auth")
	flags.StringVar(&a.authType, "auth-type", "", "authentication type: basic or pat")
	flags.StringVarP(&a.output, "output", "o", "text", "output format: text or json")
	flags.BoolVar(&a.raw, "raw", false, "emit the unmodified Jira REST response")
	flags.BoolVar(&a.quiet, "quiet", false, "suppress successful text output")

	root.AddCommand(
		a.cacheCommand(),
		a.loginCommand(),
		a.logoutCommand(),
		a.issuesCommand(),
		a.projectsCommand(),
		a.fieldsCommand(),
		a.searchCommand(),
		a.myselfCommand(),
		a.schemaCommand(),
	)
	return root
}

func (a *app) renderer() (output.Renderer, error) {
	if a.output == "raw" && !a.raw {
		return output.Renderer{}, apperr.New(apperr.KindInvalidInput, "use --raw instead of --output=raw")
	}
	formatValue := a.output
	if a.raw {
		if formatValue != "" && formatValue != "text" && formatValue != "raw" {
			return output.Renderer{}, apperr.New(apperr.KindInvalidInput, "--raw conflicts with --output")
		}
		formatValue = "raw"
	}
	format, err := output.ParseFormat(formatValue)
	if err != nil {
		return output.Renderer{}, err
	}
	return output.New(a.stdout, a.stderr, format, a.quiet).WithWarnings(a.warnings...), nil
}

func (a *app) addWarning(warning output.Warning) {
	a.warnings = append(a.warnings, warning)
}

func (a *app) configOptions() config.Options {
	return config.Options{
		ConfigPath: a.configPath,
		Profile:    a.profile,
		Host:       a.host,
		Username:   a.username,
		AuthType:   a.authType,
	}
}

func (a *app) settings() (config.Settings, error) {
	settings, err := config.Load(a.configOptions(), nil)
	if err != nil {
		return config.Settings{}, err
	}
	if settings.APIVersion != 2 {
		return config.Settings{}, apperr.New(apperr.KindConfig, "j4a v1 supports Jira REST API version 2 only")
	}
	return settings, nil
}

func (a *app) client() (*jira.Client, config.Settings, error) {
	settings, err := a.settings()
	if err != nil {
		return nil, config.Settings{}, err
	}
	clientConfig := jira.Config{BaseURL: settings.Host, Username: settings.Username}
	if settings.AuthType == config.AuthPAT {
		clientConfig.PAT = settings.Token
	} else {
		clientConfig.Password = settings.Password
	}
	client, err := jira.NewClient(clientConfig)
	if err != nil {
		return nil, config.Settings{}, err
	}
	return client, settings, nil
}

func (a *app) writableClient() (*jira.Client, config.Settings, error) {
	preview, err := config.LoadForDisplay(a.configOptions())
	if err != nil {
		return nil, config.Settings{}, err
	}
	if preview.APIVersion != 2 {
		return nil, config.Settings{}, apperr.New(apperr.KindConfig, "j4a v1 supports Jira REST API version 2 only")
	}
	if err := a.requireWritable(preview); err != nil {
		return nil, config.Settings{}, err
	}
	return a.client()
}

func (a *app) requireWritable(settings config.Settings) error {
	if settings.ReadOnly {
		return apperr.New(apperr.KindInvalidInput, "write operation blocked by read_only configuration")
	}
	return nil
}

func (a *app) render(data any, table output.Table) error {
	renderer, err := a.renderer()
	if err != nil {
		return err
	}
	if renderer.Format == output.FormatText {
		return renderer.Success(table)
	}
	return renderer.Success(data)
}

func (a *app) renderMessage(data any, message string) error {
	renderer, err := a.renderer()
	if err != nil {
		return err
	}
	if renderer.Format == output.FormatText {
		return renderer.Success(message)
	}
	return renderer.Success(data)
}

func (a *app) rawRequest(ctx context.Context, client *jira.Client, method, path string, query url.Values, input any) error {
	renderer, err := a.renderer()
	if err != nil {
		return err
	}
	raw, err := client.RawRequest(ctx, method, path, query, input)
	if err != nil {
		return err
	}
	return renderer.Raw(raw)
}

func isRaw(a *app) bool {
	renderer, err := a.renderer()
	return err == nil && renderer.Format == output.FormatRaw
}

func exactArgs(count int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != count {
			return apperr.New(apperr.KindInvalidInput, fmt.Sprintf("expected %d argument(s), got %d", count, len(args)))
		}
		return nil
	}
}

func optionalArgs(max int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > max {
			return apperr.New(apperr.KindInvalidInput, fmt.Sprintf("expected at most %d argument(s), got %d", max, len(args)))
		}
		return nil
	}
}

func pageQuery(startAt, maxResults int) url.Values {
	query := url.Values{}
	if startAt > 0 {
		query.Set("startAt", fmt.Sprint(startAt))
	}
	if maxResults > 0 {
		query.Set("maxResults", fmt.Sprint(maxResults))
	}
	return query
}

func validatePagination(offset, limit int) error {
	if offset < 0 {
		return apperr.New(apperr.KindInvalidInput, "offset must not be negative")
	}
	if limit < 0 {
		return apperr.New(apperr.KindInvalidInput, "limit must not be negative")
	}
	return nil
}

func issuePath(key string) string {
	return "rest/api/2/issue/" + url.PathEscape(key)
}

func joinNames(values []string) string {
	return strings.Join(values, ", ")
}
