// Package config loads jiro connection profiles and resolves credentials.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/timonwong/jiro/internal/apperr"
)

const (
	// KeyringService is the service name used for jiro credentials.
	KeyringService = "jiro"
)

// AuthType identifies the authentication scheme used for a Jira Instance.
type AuthType string

const (
	AuthBasic AuthType = "basic"
	AuthPAT   AuthType = "pat"
)

// Options select the config source and Profile. Bool options use pointers so
// callers can explicitly override persisted jiro behavior where needed.
type Options struct {
	ConfigPath string
	Profile    string
	ReadOnly   *bool
	UseKeyring *bool
}

// Settings is the fully resolved connection configuration.
type Settings struct {
	Profile    string   `json:"profile,omitempty"`
	Host       string   `json:"host"`
	Username   string   `json:"username,omitempty"`
	AuthType   AuthType `json:"authType"`
	APIVersion int      `json:"apiVersion"`
	ReadOnly   bool     `json:"readOnly"`
	UseKeyring bool     `json:"useKeyring"`
	UserAgent  string   `json:"userAgent,omitempty"`
	Password   string   `json:"-"`
	Token      string   `json:"-"`
}

// Draft is a possibly incomplete login candidate plus an immutable snapshot
// of the config source it was read from. Exists reports whether the selected
// [default] or [profiles.NAME] table already exists.
type Draft struct {
	Settings Settings
	Exists   bool

	source              configSource
	persistedUseKeyring bool
}

// LogoutResult reports which persistent credential store was selected and
// whether a credential was actually removed.
type LogoutResult struct {
	CredentialStore   string `json:"credentialStore"`
	CredentialRemoved bool   `json:"credentialRemoved"`
}

// Secret returns the credential appropriate for the configured auth type.
func (s Settings) Secret() string {
	if s.AuthType == AuthPAT {
		return s.Token
	}
	return s.Password
}

// MaskedSettings is a non-secret view that is safe to render or inspect.
type MaskedSettings struct {
	Profile    string   `json:"profile,omitempty"`
	Host       string   `json:"host"`
	Username   string   `json:"username,omitempty"`
	AuthType   AuthType `json:"authType"`
	APIVersion int      `json:"apiVersion"`
	ReadOnly   bool     `json:"readOnly"`
	UseKeyring bool     `json:"useKeyring"`
	UserAgent  string   `json:"userAgent,omitempty"`
	Password   string   `json:"password,omitempty"`
	Token      string   `json:"token,omitempty"`
}

// Masked returns a view that never exposes credential values.
func (s Settings) Masked() MaskedSettings {
	return MaskedSettings{
		Profile: s.Profile, Host: s.Host, Username: s.Username, AuthType: s.AuthType,
		APIVersion: s.APIVersion, ReadOnly: s.ReadOnly, UseKeyring: s.UseKeyring, UserAgent: s.UserAgent,
		Password: mask(s.Password), Token: mask(s.Token),
	}
}

func mask(value string) string {
	if value == "" {
		return ""
	}
	return "********"
}

// SecretStore abstracts the OS credential store for production and tests.
type SecretStore interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

// ErrSecretNotFound indicates that an account does not exist in a SecretStore.
var ErrSecretNotFound = errors.New("secret not found")

// DefaultPath returns the XDG-style jiro configuration path.
func DefaultPath() (string, error) {
	if directory := os.Getenv("XDG_CONFIG_HOME"); directory != "" {
		return filepath.Join(directory, "jiro", "config.toml"), nil
	}
	if runtime.GOOS == "windows" {
		directory, err := os.UserConfigDir()
		if err != nil {
			return "", apperr.Wrap(apperr.KindConfig, err, "determine config directory")
		}
		return filepath.Join(directory, "jiro", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", apperr.Wrap(apperr.KindConfig, err, "determine home directory")
	}
	return filepath.Join(home, ".config", "jiro", "config.toml"), nil
}

// Load reads TOML configuration and resolves settings with this precedence:
// JIRA_* connection overrides, selected Profile, then [default]. jiro-owned
// behavior continues to use JIRO_* environment variables.
// A nil store uses the operating system keyring when use_keyring is enabled.
func Load(options Options, store SecretStore) (Settings, error) {
	return load(options, store, true)
}

// LoadForDisplay resolves non-secret settings without accessing the OS keyring.
// It is intended for local validation paths that must work before a credential
// has been saved.
func LoadForDisplay(options Options) (Settings, error) {
	return load(options, nil, false)
}

// ProfileNames returns the sorted named Profiles from the selected config
// file without resolving credentials or accessing the OS keyring.
func ProfileNames(options Options) ([]string, error) {
	path, err := configPath(options)
	if err != nil {
		return nil, err
	}
	file, err := readFile(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(file.Profiles))
	for name := range file.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func load(options Options, store SecretStore, resolveSecrets bool) (Settings, error) {
	path, err := configPath(options)
	if err != nil {
		return Settings{}, err
	}
	file, err := readFile(path)
	if err != nil {
		return Settings{}, err
	}

	profile := firstNonEmpty(options.Profile, os.Getenv("JIRO_PROFILE"))
	values := file.Default
	if profile != "" {
		selected, ok := file.Profiles[profile]
		if !ok {
			return Settings{}, apperr.New(apperr.KindConfig, fmt.Sprintf("profile %q not found", profile))
		}
		values = mergeProfile(values, selected)
	}
	environment, err := environmentValues()
	if err != nil {
		return Settings{}, err
	}
	values = merge(values, environment)
	values = merge(values, optionValues(options))

	settings, err := resolve(profile, values)
	if err != nil {
		return Settings{}, err
	}
	environmentCredential, active, err := LoadEnvironmentCredential()
	if err != nil {
		return Settings{}, err
	}
	if active {
		environmentCredential.ApplyTo(&settings)
	}
	if !resolveSecrets {
		return settings, nil
	}
	if !active {
		if err := resolveSecret(&settings, store); err != nil {
			return Settings{}, err
		}
	}
	if err := validateCredential(settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func configPath(options Options) (string, error) {
	if options.ConfigPath != "" {
		return options.ConfigPath, nil
	}
	if path := os.Getenv("JIRO_CONFIG_FILE"); path != "" {
		return path, nil
	}
	if path := os.Getenv("JIRO_CONFIG"); path != "" {
		return path, nil
	}
	return DefaultPath()
}

func readFile(path string) (fileConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileConfig{Profiles: map[string]values{}}, nil
	}
	if err != nil {
		return fileConfig{}, apperr.Wrap(apperr.KindConfig, err, "read config %q", path)
	}
	file, err := parseTOML(string(data))
	if err != nil {
		return fileConfig{}, apperr.Wrap(apperr.KindConfig, err, "parse config %q", path)
	}
	if runtime.GOOS != "windows" && file.hasPlaintextSecret() {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return fileConfig{}, apperr.Wrap(apperr.KindConfig, statErr, "inspect config %q", path)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fileConfig{}, apperr.New(apperr.KindConfig, fmt.Sprintf("config %q contains a plaintext secret and must have permissions 0600", path))
		}
	}
	return file, nil
}

func resolve(profile string, values values) (Settings, error) {
	apiVersion := 2
	if values.apiVersion != nil {
		apiVersion = *values.apiVersion
	}
	auth := AuthBasic
	if values.authType != nil {
		auth = AuthType(strings.ToLower(*values.authType))
	}
	if auth != AuthBasic && auth != AuthPAT {
		return Settings{}, apperr.New(apperr.KindConfig, "auth_type must be basic or pat")
	}
	if apiVersion <= 0 {
		return Settings{}, apperr.New(apperr.KindConfig, "api_version must be positive")
	}
	if values.host == nil || *values.host == "" {
		return Settings{}, apperr.New(apperr.KindConfig, "host is required")
	}
	host, err := NormalizeHost(*values.host)
	if err != nil {
		return Settings{}, err
	}
	setting := Settings{Profile: profile, Host: host, AuthType: auth, APIVersion: apiVersion}
	if values.username != nil {
		setting.Username = *values.username
	}
	if values.readOnly != nil {
		setting.ReadOnly = *values.readOnly
	}
	if values.useKeyring != nil {
		setting.UseKeyring = *values.useKeyring
	}
	if values.userAgent != nil {
		setting.UserAgent = *values.userAgent
	}
	if values.password != nil {
		setting.Password = *values.password
	}
	if values.token != nil {
		setting.Token = *values.token
	}
	return setting, nil
}

func resolveSecret(settings *Settings, store SecretStore) error {
	if !settings.UseKeyring {
		return nil
	}
	if store == nil {
		store = OSSecretStore{}
	}
	secret, err := store.Get(KeyringService, KeyringAccount(settings.Profile))
	if errors.Is(err, ErrSecretNotFound) {
		return apperr.New(apperr.KindConfig, "credential is not present in the OS keyring")
	}
	if err != nil {
		return apperr.Wrap(apperr.KindConfig, err, "read credential from OS keyring")
	}
	if settings.AuthType == AuthPAT {
		settings.Token = secret
	} else {
		settings.Password = secret
	}
	return nil
}

func validateCredential(settings Settings) error {
	if settings.AuthType == AuthPAT {
		if settings.Token == "" {
			return apperr.New(apperr.KindConfig, "token is required for pat authentication")
		}
		return nil
	}
	if settings.Username == "" {
		return apperr.New(apperr.KindConfig, "username is required for basic authentication")
	}
	if settings.Password == "" {
		return apperr.New(apperr.KindConfig, "password is required for basic authentication")
	}
	return nil
}

// NormalizeHost turns a Jira base URL into a canonical, stable form.
func NormalizeHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", apperr.New(apperr.KindConfig, "host is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", apperr.New(apperr.KindConfig, "host must be an absolute base URL")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", apperr.New(apperr.KindConfig, "host must use http or https")
	}
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	if u.Path == "/" {
		u.Path = ""
	}
	return u.String(), nil
}

// KeyringAccount identifies the credential owned by one Profile. The empty
// Profile name is the default Profile. Legacy host|username|auth accounts are
// deliberately neither read nor migrated.
func KeyringAccount(profile string) string {
	if profile == "" {
		return "default"
	}
	return "profile:" + profile
}

// SetSecret writes the current auth credential to the configured secret store.
func SetSecret(store SecretStore, settings Settings, secret string) error {
	if store == nil {
		store = OSSecretStore{}
	}
	if secret == "" {
		return apperr.New(apperr.KindInvalidInput, "secret must not be empty")
	}
	if err := store.Set(KeyringService, KeyringAccount(settings.Profile), secret); err != nil {
		return apperr.Wrap(apperr.KindConfig, err, "store credential in OS keyring")
	}
	return nil
}

// DeleteSecret removes the current auth credential from the configured secret store.
func DeleteSecret(store SecretStore, settings Settings) error {
	if store == nil {
		store = OSSecretStore{}
	}
	if err := store.Delete(KeyringService, KeyringAccount(settings.Profile)); err != nil {
		return apperr.Wrap(apperr.KindConfig, err, "delete credential from OS keyring")
	}
	return nil
}

type values struct {
	host, username, authType, password, token, userAgent *string
	apiVersion                                           *int
	readOnly, useKeyring                                 *bool
}

type fileConfig struct {
	Default       values
	DefaultExists bool
	Profiles      map[string]values
}

func (f fileConfig) hasPlaintextSecret() bool {
	if f.Default.hasPlaintextSecret() {
		return true
	}
	for _, profile := range f.Profiles {
		if profile.hasPlaintextSecret() {
			return true
		}
	}
	return false
}

func (v values) hasPlaintextSecret() bool {
	return (v.password != nil && *v.password != "") || (v.token != nil && *v.token != "")
}

func merge(base, override values) values {
	result := base
	if override.host != nil {
		result.host = override.host
	}
	if override.username != nil {
		result.username = override.username
	}
	if override.authType != nil {
		result.authType = override.authType
	}
	if override.password != nil {
		result.password = override.password
	}
	if override.token != nil {
		result.token = override.token
	}
	if override.userAgent != nil {
		result.userAgent = override.userAgent
	}
	if override.apiVersion != nil {
		result.apiVersion = override.apiVersion
	}
	if override.readOnly != nil {
		result.readOnly = override.readOnly
	}
	if override.useKeyring != nil {
		result.useKeyring = override.useKeyring
	}
	return result
}

func mergeProfile(base, override values) values {
	// A named Profile may inherit non-secret defaults, but credentials belong
	// to the Profile itself and must never fall through from [default].
	base.password = nil
	base.token = nil
	return merge(base, override)
}

func environmentValues() (values, error) {
	var result values
	result.host = envString("JIRA_HOST")
	result.userAgent = envString("JIRA_USER_AGENT")
	var err error
	if result.apiVersion, err = envInt("JIRA_API_VERSION"); err != nil {
		return values{}, err
	}
	if result.readOnly, err = envBool("JIRO_READ_ONLY"); err != nil {
		return values{}, err
	}
	if result.useKeyring, err = envBool("JIRO_USE_KEYRING"); err != nil {
		return values{}, err
	}
	return result, nil
}

func optionValues(options Options) values {
	result := values{readOnly: options.ReadOnly, useKeyring: options.UseKeyring}
	return result
}

// EnvironmentCredential is one complete runtime credential resolved from the
// JIRA_* environment contract.
type EnvironmentCredential struct {
	AuthType AuthType
	Username string
	Password string
	Token    string
}

// LoadEnvironmentCredential resolves the atomic JIRA_* credential override.
// A non-empty token selects PAT and ignores Basic variables. Otherwise setting
// either Basic variable requires both values to be non-empty.
func LoadEnvironmentCredential() (EnvironmentCredential, bool, error) {
	if token := os.Getenv("JIRA_TOKEN"); token != "" {
		return EnvironmentCredential{AuthType: AuthPAT, Token: token}, true, nil
	}
	username, usernameSet := os.LookupEnv("JIRA_USERNAME")
	password, passwordSet := os.LookupEnv("JIRA_PASSWORD")
	if !usernameSet && !passwordSet {
		return EnvironmentCredential{}, false, nil
	}
	if username == "" || password == "" {
		return EnvironmentCredential{}, false, apperr.New(apperr.KindConfig, "JIRA_USERNAME and JIRA_PASSWORD must both be non-empty for Basic authentication")
	}
	return EnvironmentCredential{AuthType: AuthBasic, Username: username, Password: password}, true, nil
}

// ApplyTo replaces the complete Credential portion of settings atomically.
func (credential EnvironmentCredential) ApplyTo(settings *Settings) {
	settings.AuthType = credential.AuthType
	settings.Username = credential.Username
	settings.Password = credential.Password
	settings.Token = credential.Token
}

func envString(name string) *string {
	if value, ok := os.LookupEnv(name); ok {
		return &value
	}
	return nil
}

func envBool(name string) (*bool, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, apperr.New(apperr.KindConfig, name+" must be true or false")
	}
	return &parsed, nil
}

func envInt(name string) (*int, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, apperr.New(apperr.KindConfig, name+" must be an integer")
	}
	return &parsed, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
