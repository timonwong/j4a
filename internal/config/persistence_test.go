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

type faultStore struct {
	secrets      map[string]string
	getErrors    []error
	setErrors    []error
	deleteErrors []error
	afterSet     func()
	afterDelete  func()
}

func (s *faultStore) key(service, account string) string { return service + "/" + account }

func (s *faultStore) Get(service, account string) (string, error) {
	if err := popError(&s.getErrors); err != nil {
		return "", err
	}
	value, ok := s.secrets[s.key(service, account)]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

func (s *faultStore) Set(service, account, secret string) error {
	s.secrets[s.key(service, account)] = secret
	if s.afterSet != nil {
		after := s.afterSet
		s.afterSet = nil
		after()
	}
	return popError(&s.setErrors)
}

func (s *faultStore) Delete(service, account string) error {
	key := s.key(service, account)
	if _, ok := s.secrets[key]; !ok {
		return ErrSecretNotFound
	}
	delete(s.secrets, key)
	if s.afterDelete != nil {
		after := s.afterDelete
		s.afterDelete = nil
		after()
	}
	return popError(&s.deleteErrors)
}

func popError(errors *[]error) error {
	if len(*errors) == 0 {
		return nil
	}
	err := (*errors)[0]
	*errors = (*errors)[1:]
	return err
}

func TestLoadDraftAllowsIncompleteAndReportsTargetExistence(t *testing.T) {
	clearJ4AEnv(t)
	missingPath := filepath.Join(t.TempDir(), "config.toml")
	draft, err := LoadDraft(Options{ConfigPath: missingPath})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Exists || draft.Settings.Host != "" || draft.Settings.AuthType != AuthBasic || !draft.Settings.UseKeyring {
		t.Fatalf("new default draft = %+v", draft)
	}

	path := writeConfig(t, `# defaults
[default]
host = "jira.example/base/"
api_version = 2
read_only = true
password = "must-not-be-inherited"

[profiles.work]
read_only = false
`)
	draft, err = LoadDraft(Options{ConfigPath: path, Profile: "work"})
	if err != nil {
		t.Fatal(err)
	}
	if !draft.Exists || draft.Settings.Host != "https://jira.example/base" || draft.Settings.APIVersion != 2 || draft.Settings.ReadOnly || draft.Settings.Password != "" {
		t.Fatalf("existing named draft = %+v", draft)
	}

	draft, err = LoadDraft(Options{ConfigPath: path, Profile: "new"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Exists || draft.Settings.Host != "https://jira.example/base" || !draft.Settings.ReadOnly || draft.Settings.Password != "" {
		t.Fatalf("new named draft = %+v", draft)
	}
}

func TestCommitLoginPreservesSourceOutsideAuthenticationFields(t *testing.T) {
	clearJ4AEnv(t)
	path := writeConfig(t, `# file header
[default]
host = "https://default.example" # keep default formatting
api_version = 2

[profiles.work] # work profile
host  = 'https://old.example/' # old host
auth_type = "pat"
token = "old-token"
read_only = true # preserve me

# untouched profile comment
[profiles.other]
host = "https://other.example"
auth_type = "pat"
token = "other-token"
use_keyring = false
`)
	draft, err := LoadDraft(Options{ConfigPath: path, Profile: "work"})
	if err != nil {
		t.Fatal(err)
	}
	candidate := draft.Settings
	candidate.Host = "https://NEW.example/jira/"
	candidate.AuthType = AuthBasic
	candidate.Username = "alice"
	candidate.UseKeyring = false
	candidate.Password = "new-password"
	if err := CommitLogin(draft, candidate, &memoryStore{secrets: map[string]string{}}); err != nil {
		t.Fatal(err)
	}

	want := `# file header
[default]
host = "https://default.example" # keep default formatting
api_version = 2

[profiles.work] # work profile
host  = "https://new.example/jira" # old host
auth_type = "basic"
read_only = true # preserve me
username = "alice"
use_keyring = false
password = "new-password"

# untouched profile comment
[profiles.other]
host = "https://other.example"
auth_type = "pat"
token = "other-token"
use_keyring = false
`
	if got := readConfigText(t, path); got != want {
		t.Fatalf("updated TOML mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	settings, err := Load(Options{ConfigPath: path, Profile: "work"}, &memoryStore{secrets: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Password != "new-password" || !settings.ReadOnly || settings.APIVersion != 2 {
		t.Fatalf("persisted settings = %+v", settings)
	}
}

func TestCommitLoginBasicPATSwitchAndPrivateMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permissions use ACLs")
	}
	clearJ4AEnv(t)
	path := writeConfig(t, `[default]
host = "jira.example"
username = "obsolete"
auth_type = "pat"
token = "old-token"
use_keyring = false
`)
	draft, err := LoadDraft(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{secrets: map[string]string{}}
	basic := draft.Settings
	basic.AuthType = AuthBasic
	basic.Username = "alice"
	basic.Password = "basic-secret"
	basic.UseKeyring = true
	if err := CommitLogin(draft, basic, store); err != nil {
		t.Fatal(err)
	}
	if text := readConfigText(t, path); strings.Contains(text, "token") || strings.Contains(text, "password") {
		t.Fatalf("keyring Basic config retained secret: %s", text)
	}
	if got, err := store.Get(KeyringService, "default"); err != nil || got != "basic-secret" {
		t.Fatalf("default keyring secret = %q, %v", got, err)
	}

	draft, err = LoadDraft(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	pat := draft.Settings
	pat.AuthType = AuthPAT
	pat.Token = "pat-secret"
	pat.UseKeyring = false
	if err := CommitLogin(draft, pat, store); err != nil {
		t.Fatal(err)
	}
	text := readConfigText(t, path)
	if strings.Contains(text, "username") || strings.Contains(text, "password") || !strings.Contains(text, `token = "pat-secret"`) {
		t.Fatalf("plaintext PAT config = %s", text)
	}
	if _, err := store.Get(KeyringService, "default"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("stale keyring credential remains: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestProfileScopedKeyringIsolationAndLegacyIgnored(t *testing.T) {
	clearJ4AEnv(t)
	if KeyringAccount("") != "default" || KeyringAccount("work") != "profile:work" {
		t.Fatalf("unexpected keyring accounts: %q %q", KeyringAccount(""), KeyringAccount("work"))
	}
	path := writeConfig(t, `[default]
host = "jira.example"
username = "alice"
auth_type = "basic"
use_keyring = true

[profiles.work]
host = "jira.example"
username = "alice"
auth_type = "basic"
use_keyring = true
`)
	legacy := "https://jira.example|alice|basic"
	store := &memoryStore{secrets: map[string]string{
		KeyringService + "/default":      "default-secret",
		KeyringService + "/profile:work": "work-secret",
		KeyringService + "/" + legacy:    "legacy-secret",
	}}
	defaultSettings, err := Load(Options{ConfigPath: path}, store)
	if err != nil {
		t.Fatal(err)
	}
	workSettings, err := Load(Options{ConfigPath: path, Profile: "work"}, store)
	if err != nil {
		t.Fatal(err)
	}
	if defaultSettings.Password != "default-secret" || workSettings.Password != "work-secret" {
		t.Fatalf("isolated secrets = default %q work %q", defaultSettings.Password, workSettings.Password)
	}
	delete(store.secrets, KeyringService+"/profile:work")
	_, err = Load(Options{ConfigPath: path, Profile: "work"}, store)
	if apperr.As(err).Kind != apperr.KindConfig || !strings.Contains(err.Error(), "not present") {
		t.Fatalf("legacy key should be ignored, got %v", err)
	}
}

func TestLoginAndLogoutAreIdempotent(t *testing.T) {
	clearJ4AEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	store := &memoryStore{secrets: map[string]string{}}
	for i := 0; i < 2; i++ {
		draft, err := LoadDraft(Options{ConfigPath: path, Profile: "bot"})
		if err != nil {
			t.Fatal(err)
		}
		candidate := draft.Settings
		candidate.Host = "jira.example"
		candidate.AuthType = AuthPAT
		candidate.Token = "same-token"
		candidate.UseKeyring = true
		if err := CommitLogin(draft, candidate, store); err != nil {
			t.Fatalf("login %d: %v", i+1, err)
		}
	}
	first, err := Logout(Options{ConfigPath: path, Profile: "bot"}, store)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Logout(Options{ConfigPath: path, Profile: "bot"}, store)
	if err != nil {
		t.Fatal(err)
	}
	if first != (LogoutResult{CredentialStore: "keyring", CredentialRemoved: true}) || second != (LogoutResult{CredentialStore: "keyring"}) {
		t.Fatalf("logout results = %+v then %+v", first, second)
	}
	text := readConfigText(t, path)
	if !strings.Contains(text, "[profiles.bot]") || strings.Contains(text, "token") {
		t.Fatalf("logout removed non-sensitive profile or retained secret: %s", text)
	}

	plainPath := writeConfig(t, `[default]
host = "jira.example"
auth_type = "pat"
use_keyring = false
token = "plain-token"
read_only = true
`)
	first, err = Logout(Options{ConfigPath: plainPath}, store)
	if err != nil {
		t.Fatal(err)
	}
	second, err = Logout(Options{ConfigPath: plainPath}, store)
	if err != nil {
		t.Fatal(err)
	}
	if !first.CredentialRemoved || second.CredentialRemoved || !strings.Contains(readConfigText(t, plainPath), "read_only = true") {
		t.Fatalf("plaintext logout results = %+v then %+v; config=%s", first, second, readConfigText(t, plainPath))
	}
}

func TestLogoutUsesPersistedStoreAndMissingProfileIsConfigError(t *testing.T) {
	clearJ4AEnv(t)
	path := writeConfig(t, `[default]
host = "jira.example"
auth_type = "pat"
use_keyring = false
token = "default-token"

[profiles.work]
host = "jira.example"
auth_type = "pat"
use_keyring = true
`)
	t.Setenv("J4A_USE_KEYRING", "false")
	t.Setenv("J4A_AUTH_TYPE", "basic")
	store := &memoryStore{secrets: map[string]string{KeyringService + "/profile:work": "work-token"}}
	result, err := Logout(Options{ConfigPath: path, Profile: "work"}, store)
	if err != nil {
		t.Fatal(err)
	}
	if result.CredentialStore != "keyring" || !result.CredentialRemoved {
		t.Fatalf("Logout() = %+v", result)
	}
	if !strings.Contains(readConfigText(t, path), `token = "default-token"`) {
		t.Fatal("logout modified a different profile's plaintext credential")
	}
	_, err = Logout(Options{ConfigPath: path, Profile: "missing"}, store)
	if apperr.As(err).Kind != apperr.KindConfig {
		t.Fatalf("missing profile error = %v", err)
	}
}

func TestCommitLoginRollsBackCredentialOnWriteFailure(t *testing.T) {
	clearJ4AEnv(t)
	path := writeConfig(t, `[default]
host = "old.example"
username = "alice"
auth_type = "basic"
use_keyring = true
`)
	draft, err := LoadDraft(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	before := readConfigText(t, path)
	store := &faultStore{secrets: map[string]string{KeyringService + "/default": "old-secret"}}
	originalRename := renameConfigFile
	renameConfigFile = func(_, _ string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { renameConfigFile = originalRename })
	candidate := draft.Settings
	candidate.Host = "new.example"
	candidate.Password = "new-secret"
	err = CommitLogin(draft, candidate, store)
	if apperr.As(err).Kind != apperr.KindConfig || !strings.Contains(err.Error(), "replace config") {
		t.Fatalf("CommitLogin() error = %v", err)
	}
	if got := readConfigText(t, path); got != before {
		t.Fatalf("config changed after failed commit: %s", got)
	}
	if got := store.secrets[KeyringService+"/default"]; got != "old-secret" {
		t.Fatalf("credential after rollback = %q", got)
	}
}

func TestCommitLoginRollsBackPartialSecretStoreFailure(t *testing.T) {
	clearJ4AEnv(t)
	path := writeConfig(t, `[default]
host = "jira.example"
username = "alice"
auth_type = "basic"
use_keyring = true
`)
	draft, err := LoadDraft(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	store := &faultStore{
		secrets:   map[string]string{KeyringService + "/default": "old-secret"},
		setErrors: []error{errors.New("injected keyring failure"), nil},
	}
	candidate := draft.Settings
	candidate.Password = "new-secret"
	err = CommitLogin(draft, candidate, store)
	if apperr.As(err).Kind != apperr.KindConfig || !strings.Contains(err.Error(), "store credential") {
		t.Fatalf("CommitLogin() error = %v", err)
	}
	if got := store.secrets[KeyringService+"/default"]; got != "old-secret" {
		t.Fatalf("credential after rollback = %q", got)
	}
}

func TestCommitLoginRejectsConcurrentConfigChanges(t *testing.T) {
	clearJ4AEnv(t)
	path := writeConfig(t, `[default]
host = "jira.example"
username = "alice"
auth_type = "basic"
use_keyring = true
`)
	draft, err := LoadDraft(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# concurrent edit\n"+readConfigText(t, path)), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{secrets: map[string]string{}}
	candidate := draft.Settings
	candidate.Password = "new-secret"
	err = CommitLogin(draft, candidate, store)
	if apperr.As(err).Kind != apperr.KindConfig || !strings.Contains(err.Error(), "changed concurrently") {
		t.Fatalf("CommitLogin() error = %v", err)
	}
	if len(store.secrets) != 0 {
		t.Fatalf("credential written despite concurrent config change: %+v", store.secrets)
	}
}

func TestCommitLoginRollsBackWhenConfigChangesAfterKeyringWrite(t *testing.T) {
	clearJ4AEnv(t)
	path := writeConfig(t, `[default]
host = "jira.example"
username = "alice"
auth_type = "basic"
use_keyring = true
`)
	draft, err := LoadDraft(Options{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	store := &faultStore{
		secrets: map[string]string{KeyringService + "/default": "old-secret"},
		afterSet: func() {
			if err := os.WriteFile(path, []byte("# concurrent edit\n"+readConfigText(t, path)), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	candidate := draft.Settings
	candidate.Host = "new.example"
	candidate.Password = "new-secret"
	err = CommitLogin(draft, candidate, store)
	if apperr.As(err).Kind != apperr.KindConfig || !strings.Contains(err.Error(), "changed concurrently") {
		t.Fatalf("CommitLogin() error = %v", err)
	}
	if got := store.secrets[KeyringService+"/default"]; got != "old-secret" {
		t.Fatalf("credential after rollback = %q", got)
	}
}

func TestLogoutWriteFailureLeavesPlaintextCredential(t *testing.T) {
	clearJ4AEnv(t)
	path := writeConfig(t, `[default]
host = "jira.example"
auth_type = "pat"
use_keyring = false
token = "keep-me"
`)
	before := readConfigText(t, path)
	originalRename := renameConfigFile
	renameConfigFile = func(_, _ string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { renameConfigFile = originalRename })
	_, err := Logout(Options{ConfigPath: path}, &memoryStore{secrets: map[string]string{}})
	if apperr.As(err).Kind != apperr.KindConfig {
		t.Fatalf("Logout() error = %v", err)
	}
	if got := readConfigText(t, path); got != before {
		t.Fatalf("config changed after failed logout: %s", got)
	}
}

func readConfigText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
