package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/timonwong/jiro/internal/config"
)

type promptAnswer struct {
	label        string
	defaultValue string
	value        string
}

type fakeLoginTerminal struct {
	prompts []promptAnswer
	secret  string
	index   int
}

func (f *fakeLoginTerminal) IsTerminal() bool { return true }

func (f *fakeLoginTerminal) Prompt(label, defaultValue string) (string, error) {
	if f.index >= len(f.prompts) {
		return "", fmt.Errorf("unexpected prompt %q", label)
	}
	want := f.prompts[f.index]
	f.index++
	if label != want.label || defaultValue != want.defaultValue {
		return "", fmt.Errorf("prompt = %q default %q, want %q default %q", label, defaultValue, want.label, want.defaultValue)
	}
	if want.value == "" {
		return defaultValue, nil
	}
	return want.value, nil
}

func (f *fakeLoginTerminal) PromptSecret(label string) (string, error) {
	want := "Password"
	if len(f.prompts) > 1 && strings.Contains(f.prompts[1].value+f.prompts[1].defaultValue, "pat") {
		want = "Token"
	}
	if label != want {
		return "", fmt.Errorf("secret prompt = %q, want %q", label, want)
	}
	return f.secret, nil
}

type memorySecretStore struct {
	secrets     map[string]string
	setCalls    int
	deleteCalls int
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{secrets: map[string]string{}}
}

func (m *memorySecretStore) Get(service, account string) (string, error) {
	value, ok := m.secrets[service+"/"+account]
	if !ok {
		return "", config.ErrSecretNotFound
	}
	return value, nil
}

func (m *memorySecretStore) Set(service, account, secret string) error {
	m.setCalls++
	m.secrets[service+"/"+account] = secret
	return nil
}

func (m *memorySecretStore) Delete(service, account string) error {
	m.deleteCalls++
	key := service + "/" + account
	if _, ok := m.secrets[key]; !ok {
		return config.ErrSecretNotFound
	}
	delete(m.secrets, key)
	return nil
}

func TestLoginInteractiveBasicAndJSON(t *testing.T) {
	clearCommandEnv(t)
	server := authenticatedServer(t, config.AuthBasic, "alice", "fresh-password", http.StatusOK)
	defer server.Close()

	path := filepath.Join(t.TempDir(), "config.toml")
	terminal := &fakeLoginTerminal{prompts: []promptAnswer{
		{label: "Host", value: server.URL},
		{label: "Auth type (basic/pat)", value: "basic"},
		{label: "Username", value: "alice"},
	}, secret: "fresh-password"}
	store := newMemorySecretStore()
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	a := &app{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr, terminal: terminal, secretStore: store}
	if code := a.execute([]string{"--config", path, "-ojson", "auth", "login"}); code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		SchemaVersion string      `json:"schemaVersion"`
		Data          loginResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1" || envelope.Data.Profile != "default" || envelope.Data.CredentialStore != "keyring" || envelope.Data.User.Username != "alice" {
		t.Fatalf("result = %+v", envelope)
	}
	if store.setCalls != 1 || !storeContains(store, "fresh-password") {
		t.Fatalf("secret store = %+v", store)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("fresh-password")) {
		t.Fatalf("keyring login wrote plaintext credential: %s", data)
	}
}

func TestLoginInteractivePATUsesExistingDefaultsAndFreshSecret(t *testing.T) {
	clearCommandEnv(t)
	server := authenticatedServer(t, config.AuthPAT, "", "fresh-token", http.StatusOK)
	defer server.Close()
	path := writeAuthConfig(t, fmt.Sprintf("[default]\nhost = %q\nauth_type = \"pat\"\nuse_keyring = false\ntoken = \"old-token\"\n", server.URL))
	terminal := &fakeLoginTerminal{prompts: []promptAnswer{
		{label: "Host", defaultValue: server.URL},
		{label: "Auth type (basic/pat)", defaultValue: "pat"},
	}, secret: "fresh-token"}
	store := newMemorySecretStore()
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	a := &app{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr, terminal: terminal, secretStore: store}
	if code := a.execute([]string{"--config", path, "--quiet", "auth", "login"}); code != 0 || stdout.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !storeContains(store, "fresh-token") || storeContains(store, "old-token") {
		t.Fatalf("fresh credential not stored: %+v", store)
	}
}

func TestLoginNewProfileRequiresExplicitAuthSelection(t *testing.T) {
	clearCommandEnv(t)
	path := writeAuthConfig(t, "[default]\nhost = \"https://jira.example\"\nauth_type = \"basic\"\nusername = \"alice\"\n")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	terminal := &fakeLoginTerminal{prompts: []promptAnswer{
		{label: "Host", defaultValue: "https://jira.example"},
		{label: "Auth type (basic/pat)"},
	}}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	a := &app{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr, terminal: terminal, secretStore: newMemorySecretStore()}
	if code := a.execute([]string{"--config", path, "--profile", "new", "auth", "login"}); code != 2 || !strings.Contains(stderr.String(), "auth type") {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed login changed config")
	}
}

func TestLoginNonTTYEnvironmentAndStdin(t *testing.T) {
	t.Run("environment password wins", func(t *testing.T) {
		clearCommandEnv(t)
		server := authenticatedServer(t, config.AuthBasic, "alice", "env-password", http.StatusOK)
		defer server.Close()
		t.Setenv("JIRO_HOST", server.URL)
		t.Setenv("JIRO_AUTH_TYPE", "basic")
		t.Setenv("JIRO_USERNAME", "alice")
		t.Setenv("JIRO_PASSWORD", "env-password")
		path := filepath.Join(t.TempDir(), "config.toml")
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		code := Execute([]string{"--config", path, "-ojson", "auth", "login", "--use-keyring=false"}, strings.NewReader("stdin-password"), stdout, stderr)
		if code != 0 || stderr.Len() != 0 {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stdout.String(), `"credentialStore":"config"`) {
			t.Fatalf("stdout=%s", stdout.String())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte("env-password")) || bytes.Contains(data, []byte("stdin-password")) {
			t.Fatalf("config secret = %s", data)
		}
	})

	t.Run("PAT token from stdin", func(t *testing.T) {
		clearCommandEnv(t)
		server := authenticatedServer(t, config.AuthPAT, "", "stdin-token", http.StatusOK)
		defer server.Close()
		path := filepath.Join(t.TempDir(), "config.toml")
		store := newMemorySecretStore()
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		a := &app{stdin: strings.NewReader("stdin-token\n"), stdout: stdout, stderr: stderr, secretStore: store}
		code := a.execute([]string{"--config", path, "--host", server.URL, "--auth-type", "pat", "auth", "login"})
		if code != 0 || stderr.Len() != 0 || !storeContains(store, "stdin-token") {
			t.Fatalf("code=%d store=%+v stdout=%s stderr=%s", code, store, stdout.String(), stderr.String())
		}
	})
}

func TestLoginMissingInputAndUnauthorizedDoNotCommit(t *testing.T) {
	clearCommandEnv(t)
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls++
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	t.Run("missing username", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		code := Execute([]string{"--config", path, "--host", server.URL, "--auth-type", "basic", "auth", "login"}, strings.NewReader("secret"), stdout, stderr)
		if code != 2 || !strings.Contains(stderr.String(), "username is required") || calls != 0 {
			t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls, stdout.String(), stderr.String())
		}
	})

	t.Run("new profile missing explicit auth", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		code := Execute([]string{"--config", path, "--host", server.URL, "--username", "alice", "auth", "login"}, strings.NewReader("secret"), stdout, stderr)
		if code != 2 || !strings.Contains(stderr.String(), "auth type must be basic or pat") || calls != 0 {
			t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls, stdout.String(), stderr.String())
		}
	})

	t.Run("unsupported API version", func(t *testing.T) {
		path := writeAuthConfig(t, fmt.Sprintf("[default]\nhost = %q\nauth_type = \"pat\"\napi_version = 3\n", server.URL))
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		code := Execute([]string{"--config", path, "auth", "login"}, strings.NewReader("secret"), stdout, stderr)
		if code != 2 || !strings.Contains(stderr.String(), "REST API version 2") || calls != 0 {
			t.Fatalf("code=%d calls=%d stdout=%s stderr=%s", code, calls, stdout.String(), stderr.String())
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("unsupported API version changed config")
		}
	})

	t.Run("401", func(t *testing.T) {
		path := writeAuthConfig(t, fmt.Sprintf("[default]\nhost = %q\nauth_type = \"pat\"\nuse_keyring = true\n", server.URL))
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		store := newMemorySecretStore()
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		a := &app{stdin: strings.NewReader("bad-token"), stdout: stdout, stderr: stderr, secretStore: store}
		code := a.execute([]string{"--config", path, "-ojson", "auth", "login"})
		if code != 3 || !strings.Contains(stderr.String(), `"kind":"auth"`) || store.setCalls != 0 {
			t.Fatalf("code=%d store=%+v stdout=%s stderr=%s", code, store, stdout.String(), stderr.String())
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("unauthorized login changed config")
		}
	})
}

func TestLoginLogoutAndLogoutIdempotence(t *testing.T) {
	clearCommandEnv(t)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	server := authenticatedServer(t, config.AuthPAT, "", "token", http.StatusOK)
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.toml")
	store := newMemorySecretStore()
	stdout.Reset()
	stderr.Reset()
	a := &app{stdin: strings.NewReader("token"), stdout: stdout, stderr: stderr, secretStore: store}
	if code := a.execute([]string{"--config", path, "--host", server.URL, "--auth-type", "pat", "auth", "login"}); code != 0 {
		t.Fatalf("login code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	t.Setenv("JIRO_TOKEN", "environment-token")

	for index, wantRemoved := range []bool{true, false} {
		stdout.Reset()
		stderr.Reset()
		a = &app{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr, secretStore: store}
		code := a.execute([]string{"--config", path, "-ojson", "auth", "logout"})
		if code != 0 || stderr.Len() != 0 {
			t.Fatalf("logout %d code=%d stdout=%s stderr=%s", index, code, stdout.String(), stderr.String())
		}
		var envelope struct {
			Data logoutResult `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Data.Profile != "default" || envelope.Data.CredentialStore != "keyring" || envelope.Data.CredentialRemoved != wantRemoved || !envelope.Data.EnvironmentCredentialActive {
			t.Fatalf("logout %d = %+v", index, envelope.Data)
		}
	}

}

func TestLogoutMissingProfileIsConfigError(t *testing.T) {
	clearCommandEnv(t)
	path := writeAuthConfig(t, "[default]\nhost = \"https://jira.example\"\nauth_type = \"pat\"\nuse_keyring = true\n")
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	a := &app{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr, secretStore: newMemorySecretStore()}
	if code := a.execute([]string{"--config", path, "--profile", "missing", "-ojson", "auth", "logout"}); code != 2 || !strings.Contains(stderr.String(), `"kind":"config"`) {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestAuthStatusTextJSONQuietAndNamedProfile(t *testing.T) {
	clearCommandEnv(t)
	server := authenticatedServer(t, config.AuthBasic, "alice", "status-password", http.StatusOK)
	defer server.Close()

	path := writeAuthConfig(t, fmt.Sprintf(`[default]
host = "https://default.example"
username = "default-user"
auth_type = "basic"
use_keyring = false
password = "default-password"

[profiles.bot]
host = %q
username = "alice"
auth_type = "basic"
use_keyring = false
password = "status-password"
`, server.URL))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	args := []string{"--config", path, "--profile", "bot", "auth", "status"}
	if code := Execute(args, strings.NewReader(""), stdout, stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("text code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{"Profile", "bot", "Jira Instance", server.URL, "basic", "Test User (alice)", "true"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("text stdout=%q; missing %q", stdout.String(), want)
		}
	}
	if strings.Contains(stdout.String(), "status-password") {
		t.Fatalf("text status exposed credential: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	jsonArgs := []string{"--config", path, "--profile", "bot", "-ojson", "auth", "status"}
	if code := Execute(jsonArgs, strings.NewReader(""), stdout, stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("json code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var envelope struct {
		SchemaVersion string           `json:"schemaVersion"`
		Data          authStatusResult `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != "1" || envelope.Data.Profile != "bot" || envelope.Data.Instance != server.URL || envelope.Data.AuthType != config.AuthBasic || envelope.Data.User.Username != "alice" || !envelope.Data.User.Active {
		t.Fatalf("status = %+v", envelope)
	}
	if strings.Contains(stdout.String(), "status-password") {
		t.Fatalf("json status exposed credential: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	quietArgs := []string{"--config", path, "--profile", "bot", "--quiet", "auth", "status"}
	if code := Execute(quietArgs, strings.NewReader(""), stdout, stderr); code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("quiet code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("auth status modified the config")
	}
}

func TestAuthStatusUsesInjectedKeyringStore(t *testing.T) {
	clearCommandEnv(t)
	server := authenticatedServer(t, config.AuthPAT, "", "keyring-token", http.StatusOK)
	defer server.Close()
	path := writeAuthConfig(t, fmt.Sprintf("[default]\nhost = %q\nauth_type = \"pat\"\nuse_keyring = true\n", server.URL))
	store := newMemorySecretStore()
	store.secrets[config.KeyringService+"/"+config.KeyringAccount("")] = "keyring-token"
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	a := &app{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr, secretStore: store}
	if code := a.execute([]string{"--config", path, "-ojson", "auth", "status"}); code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"profile":"default"`) || strings.Contains(stdout.String(), "keyring-token") {
		t.Fatalf("stdout=%s", stdout.String())
	}
}

func TestAuthStatusFailures(t *testing.T) {
	t.Run("missing credential", func(t *testing.T) {
		clearCommandEnv(t)
		path := writeAuthConfig(t, "[default]\nhost = \"https://jira.example\"\nauth_type = \"pat\"\nuse_keyring = false\n")
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		code := Execute([]string{"--config", path, "-ojson", "auth", "status"}, strings.NewReader(""), stdout, stderr)
		if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"kind":"config"`) {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
	})

	t.Run("invalid credential", func(t *testing.T) {
		clearCommandEnv(t)
		server := authenticatedServer(t, config.AuthPAT, "", "bad-token", http.StatusUnauthorized)
		defer server.Close()
		path := writeAuthConfig(t, fmt.Sprintf("[default]\nhost = %q\nauth_type = \"pat\"\nuse_keyring = false\ntoken = \"bad-token\"\n", server.URL))
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		code := Execute([]string{"--config", path, "-ojson", "auth", "status"}, strings.NewReader(""), stdout, stderr)
		if code != 3 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"kind":"auth"`) {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
	})

	t.Run("network failure", func(t *testing.T) {
		clearCommandEnv(t)
		server := authenticatedServer(t, config.AuthPAT, "", "token", http.StatusOK)
		instance := server.URL
		server.Close()
		path := writeAuthConfig(t, fmt.Sprintf("[default]\nhost = %q\nauth_type = \"pat\"\nuse_keyring = false\ntoken = \"token\"\n", instance))
		stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
		code := Execute([]string{"--config", path, "-ojson", "auth", "status"}, strings.NewReader(""), stdout, stderr)
		if code != 5 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `"kind":"api_error"`) {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
		}
	})

}

func TestAuthCommandsOnlyAvailableUnderAuth(t *testing.T) {
	clearCommandEnv(t)
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	if code := Execute([]string{"--help"}, strings.NewReader(""), stdout, stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("root help code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "  auth ") || strings.Contains(stdout.String(), "\n  login ") || strings.Contains(stdout.String(), "\n  logout ") {
		t.Fatalf("root help exposes wrong auth commands:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"auth", "--help"}, strings.NewReader(""), stdout, stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("auth help code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	for _, command := range []string{"login", "logout", "status"} {
		if !strings.Contains(stdout.String(), "  "+command+" ") {
			t.Fatalf("auth help missing %q:\n%s", command, stdout.String())
		}
	}

	for _, command := range []string{"login", "logout"} {
		stdout.Reset()
		stderr.Reset()
		if code := Execute([]string{command}, strings.NewReader(""), stdout, stderr); code != 1 || !strings.Contains(stderr.String(), "unknown command") {
			t.Fatalf("top-level %s code=%d stdout=%s stderr=%s", command, code, stdout.String(), stderr.String())
		}
	}
}

func authenticatedServer(t *testing.T, auth config.AuthType, username, secret string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/rest/api/2/myself" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if auth == config.AuthPAT {
			if got := request.Header.Get("Authorization"); got != "Bearer "+secret {
				t.Errorf("Authorization = %q", got)
			}
		} else {
			gotUsername, gotPassword, ok := request.BasicAuth()
			if !ok || gotUsername != username || gotPassword != secret {
				t.Errorf("BasicAuth = %q/%q/%t", gotUsername, gotPassword, ok)
			}
		}
		writer.WriteHeader(status)
		if status >= 200 && status < 300 {
			_, _ = io.WriteString(writer, fmt.Sprintf(`{"name":%q,"displayName":"Test User","active":true}`, username))
		}
	}))
}

func writeAuthConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func storeContains(store *memorySecretStore, want string) bool {
	for _, value := range store.secrets {
		if value == want {
			return true
		}
	}
	return false
}

var _ config.SecretStore = (*memorySecretStore)(nil)
