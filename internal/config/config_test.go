package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/timonwong/j4a/internal/apperr"
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
	clearJ4AEnv(t)
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
	falseValue := false
	tests := []struct {
		name                             string
		env                              map[string]string
		options                          Options
		wantHost, wantUser, wantPassword string
		wantVersion                      int
		wantReadOnly                     bool
	}{
		{"profile overrides default", nil, Options{ConfigPath: path, Profile: "work"}, "https://profile.example/jira", "profile-user", "profile-password", 3, false},
		{"environment overrides profile", map[string]string{"J4A_PROFILE": "work", "J4A_HOST": "https://env.example", "J4A_USERNAME": "env-user", "J4A_PASSWORD": "env-password", "J4A_API_VERSION": "4", "J4A_READ_ONLY": "true"}, Options{ConfigPath: path}, "https://env.example", "env-user", "env-password", 4, true},
		{"options override environment", map[string]string{"J4A_PROFILE": "work", "J4A_HOST": "https://env.example", "J4A_USERNAME": "env-user", "J4A_PASSWORD": "env-password", "J4A_API_VERSION": "4", "J4A_READ_ONLY": "true"}, Options{ConfigPath: path, Host: "https://cli.example/", Username: "cli-user", Password: "cli-password", APIVersion: 5, ReadOnly: &falseValue}, "https://cli.example", "cli-user", "cli-password", 5, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearJ4AEnv(t)
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

func TestDefaultPathUsesXDG(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(directory, "j4a", "config.toml")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestPlaintextSecretRequiresPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions use ACLs")
	}
	clearJ4AEnv(t)
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
	clearJ4AEnv(t)
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
		{"environment wins", map[string]string{"J4A_TOKEN": "env-token"}, store, "env-token", ""},
		{"keyring wins over toml", nil, store, "keyring-token", ""},
		{"keyring miss is config error", nil, &memoryStore{secrets: map[string]string{}}, "", apperr.KindConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearJ4AEnv(t)
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
	clearJ4AEnv(t)
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
	clearJ4AEnv(t)
	tests := []struct {
		name, body string
		env        map[string]string
	}{
		{"invalid toml", "[wrong]\nhost = \"jira.example\"", nil},
		{"invalid env bool", "[default]\nhost = \"jira.example\"\nusername = \"user\"\npassword = \"password\"", map[string]string{"J4A_READ_ONLY": "sometimes"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearJ4AEnv(t)
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

func clearJ4AEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"XDG_CONFIG_HOME", "J4A_CONFIG_FILE", "J4A_CONFIG", "J4A_PROFILE", "J4A_HOST", "J4A_USERNAME", "J4A_AUTH_TYPE", "J4A_API_VERSION", "J4A_READ_ONLY", "J4A_USE_KEYRING", "J4A_PASSWORD", "J4A_TOKEN"} {
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
