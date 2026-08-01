package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/timonwong/jiro/internal/apperr"
)

type memoryStore struct{ secrets map[string]string }

func (s *memoryStore) key(service, account string) string { return service + "/" + account }
func (s *memoryStore) Get(service, account string) (string, error) {
	value, ok := s.secrets[s.key(service, account)]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}
func (s *memoryStore) Set(service, account, secret string) error {
	s.secrets[s.key(service, account)] = secret
	return nil
}
func (s *memoryStore) Delete(service, account string) error {
	key := s.key(service, account)
	if _, ok := s.secrets[key]; !ok {
		return ErrSecretNotFound
	}
	delete(s.secrets, key)
	return nil
}

func TestLoadPrecedence(t *testing.T) {
	clearJiroEnv(t)
	path := writeConfig(t, `
[default]
host = "https://default.example/jira/"
username = "default-user"
password = "default-password"
api_version = 1
read_only = true
use_keyring = false

[profiles.work]
host = "https://profile.example/jira"
username = "profile-user"
password = "profile-password"
api_version = 3
read_only = false
`)
	tests := []struct {
		name                             string
		env                              map[string]string
		options                          Options
		wantHost, wantUser, wantPassword string
		wantVersion                      int
		wantReadOnly                     bool
	}{
		{"profile overrides default", nil, Options{ConfigPath: path, Profile: "work"}, "https://profile.example/jira", "profile-user", "profile-password", 3, false},
		{"environment overrides profile", map[string]string{"JIRO_PROFILE": "work", "JIRA_HOST": "https://env.example", "JIRA_USERNAME": "env-user", "JIRA_PASSWORD": "env-password", "JIRA_API_VERSION": "4", "JIRO_READ_ONLY": "true"}, Options{ConfigPath: path}, "https://env.example", "env-user", "env-password", 4, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearJiroEnv(t)
			for name, value := range test.env {
				t.Setenv(name, value)
			}
			got, err := Load(test.options, &memoryStore{secrets: map[string]string{}})
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.Host != test.wantHost || got.Username != test.wantUser || got.Password != test.wantPassword || got.APIVersion != test.wantVersion || got.ReadOnly != test.wantReadOnly {
				t.Fatalf("Load() = %+v, want host=%q user=%q password=%q version=%d readOnly=%t", got, test.wantHost, test.wantUser, test.wantPassword, test.wantVersion, test.wantReadOnly)
			}
		})
	}
}

func TestLoadJiraEnvironmentCredentialContract(t *testing.T) {
	clearJiroEnv(t)
	path := writeConfig(t, `
[default]
host = "https://profile.example/jira"
username = "profile-user"
auth_type = "basic"
use_keyring = false
password = "profile-password"
`)

	t.Run("non-empty token selects PAT and ignores Basic variables", func(t *testing.T) {
		clearJiroEnv(t)
		t.Setenv("JIRA_TOKEN", "environment-token")
		t.Setenv("JIRA_USERNAME", "ignored-user")
		t.Setenv("JIRA_PASSWORD", "")

		got, err := Load(Options{ConfigPath: path}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.AuthType != AuthPAT || got.Token != "environment-token" || got.Username != "" || got.Password != "" {
			t.Fatalf("Load() = %+v, want atomic PAT credential", got)
		}
	})

	t.Run("complete Basic environment credential replaces the Profile credential", func(t *testing.T) {
		clearJiroEnv(t)
		t.Setenv("JIRA_USERNAME", "environment-user")
		t.Setenv("JIRA_PASSWORD", "environment-password")

		got, err := Load(Options{ConfigPath: path}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.AuthType != AuthBasic || got.Username != "environment-user" || got.Password != "environment-password" || got.Token != "" {
			t.Fatalf("Load() = %+v, want atomic Basic credential", got)
		}
	})

	for _, test := range []struct {
		name string
		env  map[string]string
	}{
		{name: "username without password", env: map[string]string{"JIRA_USERNAME": "environment-user"}},
		{name: "password without username", env: map[string]string{"JIRA_PASSWORD": "environment-password"}},
		{name: "explicitly empty password", env: map[string]string{"JIRA_USERNAME": "environment-user", "JIRA_PASSWORD": ""}},
	} {
		t.Run(test.name, func(t *testing.T) {
			clearJiroEnv(t)
			for name, value := range test.env {
				t.Setenv(name, value)
			}
			_, err := Load(Options{ConfigPath: path}, nil)
			if apperr.As(err).Kind != apperr.KindConfig || !strings.Contains(err.Error(), "JIRA_USERNAME and JIRA_PASSWORD") {
				t.Fatalf("Load() error = %v, want atomic Basic environment error", err)
			}
		})
	}

	t.Run("host override combines with the Profile credential", func(t *testing.T) {
		clearJiroEnv(t)
		t.Setenv("JIRA_HOST", "https://environment.example/jira/")

		got, err := Load(Options{ConfigPath: path}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Host != "https://environment.example/jira" || got.Username != "profile-user" || got.Password != "profile-password" {
			t.Fatalf("Load() = %+v, want host override with Profile credential", got)
		}
	})

	t.Run("legacy JIRO connection variables are ignored", func(t *testing.T) {
		clearJiroEnv(t)
		t.Setenv("JIRO_HOST", "https://legacy.example")
		t.Setenv("JIRO_USERNAME", "legacy-user")
		t.Setenv("JIRO_PASSWORD", "legacy-password")
		t.Setenv("JIRO_TOKEN", "legacy-token")
		t.Setenv("JIRO_AUTH_TYPE", "pat")
		t.Setenv("JIRO_API_VERSION", "9")

		got, err := Load(Options{ConfigPath: path}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Host != "https://profile.example/jira" || got.AuthType != AuthBasic || got.Username != "profile-user" || got.Password != "profile-password" || got.APIVersion != 2 {
			t.Fatalf("Load() = %+v, legacy connection environment must be ignored", got)
		}
	})
}

func TestLoadUserAgentPrecedence(t *testing.T) {
	clearJiroEnv(t)
	path := writeConfig(t, `
[default]
host = "https://default.example"
auth_type = "pat"
use_keyring = false
token = "default-token"
user_agent = "default-agent"

[profiles.inherited]
host = "https://inherited.example"
token = "inherited-token"

[profiles.overridden]
host = "https://overridden.example"
token = "overridden-token"
user_agent = "profile-agent"
`)

	tests := []struct {
		name, profile, environment, want string
	}{
		{name: "default Profile", want: "default-agent"},
		{name: "named Profile inherits default", profile: "inherited", want: "default-agent"},
		{name: "named Profile overrides default", profile: "overridden", want: "profile-agent"},
		{name: "environment overrides selected Profile", profile: "overridden", environment: "environment-agent", want: "environment-agent"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearJiroEnv(t)
			if test.environment != "" {
				t.Setenv("JIRA_USER_AGENT", test.environment)
			}
			got, err := Load(Options{ConfigPath: path, Profile: test.profile}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.UserAgent != test.want {
				t.Fatalf("UserAgent = %q, want %q", got.UserAgent, test.want)
			}
		})
	}
}

func TestDefaultPathUsesXDG(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(directory, "jiro", "config.toml")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestProfileNames(t *testing.T) {
	clearJiroEnv(t)
	path := writeConfig(t, `
[default]
host = "jira.example"

[profiles.zeta]
host = "zeta.example"

[profiles.alpha]
host = "alpha.example"
`)
	names, err := ProfileNames(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(names, ","); got != "alpha,zeta" {
		t.Fatalf("ProfileNames() = %q, want alpha,zeta", got)
	}

	names, err = ProfileNames(Options{ConfigPath: filepath.Join(t.TempDir(), "missing.toml")})
	if err != nil || len(names) != 0 {
		t.Fatalf("ProfileNames(missing) = %v, %v; want empty", names, err)
	}
}

func TestPlaintextSecretRequiresPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions use ACLs")
	}
	clearJiroEnv(t)
	path := writeConfig(t, "[default]\nhost = \"jira.example\"\nusername = \"user\"\npassword = \"secret\"")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(Options{ConfigPath: path}, &memoryStore{secrets: map[string]string{}})
	if apperr.As(err).Kind != apperr.KindConfig || !strings.Contains(err.Error(), "0600") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadSecrets(t *testing.T) {
	clearJiroEnv(t)
	path := writeConfig(t, `
[default]
host = "jira.example"
auth_type = "pat"
token = "toml-token"
use_keyring = true
`)
	settings := Settings{Host: "https://jira.example", AuthType: AuthPAT}
	store := &memoryStore{secrets: map[string]string{KeyringService + "/" + KeyringAccount(settings.Profile): "keyring-token"}}
	tests := []struct {
		name     string
		env      map[string]string
		store    SecretStore
		want     string
		wantKind apperr.Kind
	}{
		{"environment wins", map[string]string{"JIRA_TOKEN": "env-token"}, store, "env-token", ""},
		{"keyring wins over toml", nil, store, "keyring-token", ""},
		{"keyring miss is config error", nil, &memoryStore{secrets: map[string]string{}}, "", apperr.KindConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearJiroEnv(t)
			for name, value := range test.env {
				t.Setenv(name, value)
			}
			got, err := Load(Options{ConfigPath: path}, test.store)
			if test.wantKind != "" {
				if apperr.As(err).Kind != test.wantKind {
					t.Fatalf("Load() error kind = %v, want %v (err=%v)", apperr.As(err).Kind, test.wantKind, err)
				}
				return
			}
			if err != nil || got.Token != test.want {
				t.Fatalf("Load() = %+v, %v; want token %q", got, err, test.want)
			}
		})
	}
}

func TestCredentialValidationAndMaskedView(t *testing.T) {
	clearJiroEnv(t)
	tests := []struct {
		name, body string
		want       apperr.Kind
	}{
		{"basic needs username", "[default]\nhost = \"jira.example\"\npassword = \"password\"", apperr.KindConfig},
		{"basic needs password", "[default]\nhost = \"jira.example\"\nusername = \"user\"", apperr.KindConfig},
		{"pat needs token", "[default]\nhost = \"jira.example\"\nauth_type = \"pat\"", apperr.KindConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load(Options{ConfigPath: writeConfig(t, test.body)}, &memoryStore{secrets: map[string]string{}})
			if apperr.As(err).Kind != test.want {
				t.Fatalf("error kind = %v, want %v", apperr.As(err).Kind, test.want)
			}
		})
	}
	show, err := LoadForDisplay(Options{ConfigPath: writeConfig(t, "[default]\nhost = \"jira.example\"\nusername = \"user\"\npassword = \"secret\"")})
	if err != nil {
		t.Fatal(err)
	}
	if got := show.Masked().Password; got != "********" {
		t.Fatalf("masked password = %q", got)
	}
}

func TestConfigErrorsAndSecretHelpers(t *testing.T) {
	clearJiroEnv(t)
	tests := []struct {
		name, body string
		env        map[string]string
	}{
		{"invalid toml", "[wrong]\nhost = \"jira.example\"", nil},
		{"invalid env bool", "[default]\nhost = \"jira.example\"\nusername = \"user\"\npassword = \"password\"", map[string]string{"JIRO_READ_ONLY": "sometimes"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearJiroEnv(t)
			for name, value := range test.env {
				t.Setenv(name, value)
			}
			_, err := Load(Options{ConfigPath: writeConfig(t, test.body)}, nil)
			if apperr.As(err).Kind != apperr.KindConfig {
				t.Fatalf("error kind = %v, want config", apperr.As(err).Kind)
			}
		})
	}
	store := &memoryStore{secrets: map[string]string{}}
	settings := Settings{Host: "https://JIRA.example/", Username: "alice", AuthType: AuthBasic}
	if err := SetSecret(store, settings, "password"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(KeyringService, KeyringAccount(settings.Profile)); err != nil || got != "password" {
		t.Fatalf("stored secret = %q, %v", got, err)
	}
	if err := DeleteSecret(store, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(KeyringService, KeyringAccount(settings.Profile)); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("secret was not deleted: %v", err)
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func clearJiroEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"XDG_CONFIG_HOME", "JIRO_CONFIG_FILE", "JIRO_CONFIG", "JIRO_PROFILE", "JIRO_READ_ONLY", "JIRO_USE_KEYRING",
		"JIRA_HOST", "JIRA_API_VERSION", "JIRA_TOKEN", "JIRA_USERNAME", "JIRA_PASSWORD", "JIRA_USER_AGENT",
		"JIRO_HOST", "JIRO_API_VERSION", "JIRO_TOKEN", "JIRO_USERNAME", "JIRO_PASSWORD", "JIRO_AUTH_TYPE",
	} {
		old, existed := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(name, old)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}
