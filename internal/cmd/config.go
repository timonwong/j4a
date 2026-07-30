package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/timonwong/j4a/internal/apperr"
	"github.com/timonwong/j4a/internal/config"
	"github.com/timonwong/j4a/internal/output"
	"golang.org/x/term"
)

func (a *app) configCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Inspect and manage j4a configuration"}
	command.AddCommand(
		a.configShowCommand(),
		a.configPathCommand(),
		a.configSetSecretCommand(),
		a.configDeleteSecretCommand(),
	)
	return command
}

func (a *app) configShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show resolved non-secret configuration",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if isRaw(a) {
				return apperr.New(apperr.KindInvalidInput, "--raw is not available for config commands")
			}
			settings, err := config.LoadForDisplay(a.configOptions())
			if err != nil {
				return err
			}
			masked := settings.Masked()
			return a.render(masked, output.Table{
				Headers: []string{"FIELD", "VALUE"},
				Rows: [][]string{
					{"Profile", masked.Profile}, {"Host", masked.Host}, {"Username", masked.Username},
					{"Auth Type", string(masked.AuthType)}, {"API Version", fmt.Sprint(masked.APIVersion)},
					{"Read Only", fmt.Sprint(masked.ReadOnly)}, {"Use Keyring", fmt.Sprint(masked.UseKeyring)},
					{"Password", masked.Password}, {"Token", masked.Token},
				},
			})
		},
	}
}

func (a *app) configPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the resolved config file path",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if isRaw(a) {
				return apperr.New(apperr.KindInvalidInput, "--raw is not available for config commands")
			}
			path, err := a.resolvedConfigPath()
			if err != nil {
				return err
			}
			return a.renderMessage(map[string]string{"path": path}, path)
		},
	}
}

func (a *app) configSetSecretCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-secret",
		Short: "Store the active profile credential in the OS keyring",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if isRaw(a) {
				return apperr.New(apperr.KindInvalidInput, "--raw is not available for config commands")
			}
			settings, err := config.LoadForDisplay(a.configOptions())
			if err != nil {
				return err
			}
			if !settings.UseKeyring {
				return apperr.New(apperr.KindConfig, "set use_keyring = true before storing a keyring credential")
			}
			secret, err := a.readSecret(settings.AuthType)
			if err != nil {
				return err
			}
			if err := config.SetSecret(nil, settings, secret); err != nil {
				return err
			}
			return a.renderMessage(map[string]any{"stored": true, "profile": settings.Profile}, "Credential stored in OS keyring")
		},
	}
}

func (a *app) configDeleteSecretCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete-secret",
		Short: "Remove the active profile credential from the OS keyring",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if isRaw(a) {
				return apperr.New(apperr.KindInvalidInput, "--raw is not available for config commands")
			}
			settings, err := config.LoadForDisplay(a.configOptions())
			if err != nil {
				return err
			}
			if err := config.DeleteSecret(nil, settings); err != nil {
				return err
			}
			return a.renderMessage(map[string]any{"deleted": true, "profile": settings.Profile}, "Credential removed from OS keyring")
		},
	}
}

func (a *app) resolvedConfigPath() (string, error) {
	if a.configPath != "" {
		return a.configPath, nil
	}
	if path := os.Getenv("J4A_CONFIG_FILE"); path != "" {
		return path, nil
	}
	if path := os.Getenv("J4A_CONFIG"); path != "" {
		return path, nil
	}
	return config.DefaultPath()
}

func (a *app) readSecret(auth config.AuthType) (string, error) {
	label := "Password"
	if auth == config.AuthPAT {
		label = "Token"
	}
	if file, ok := a.stdin.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		_, _ = fmt.Fprintf(a.stderr, "%s: ", label)
		secret, err := term.ReadPassword(int(file.Fd()))
		_, _ = fmt.Fprintln(a.stderr)
		if err != nil {
			return "", apperr.Wrap(apperr.KindInvalidInput, err, "read credential")
		}
		return string(secret), nil
	}
	secret, err := io.ReadAll(a.stdin)
	if err != nil {
		return "", apperr.Wrap(apperr.KindInvalidInput, err, "read credential from stdin")
	}
	value := strings.TrimRight(string(secret), "\r\n")
	if value == "" {
		return "", apperr.New(apperr.KindInvalidInput, strings.ToLower(label)+" must not be empty")
	}
	return value, nil
}
