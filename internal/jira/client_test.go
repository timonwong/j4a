package jira

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/timonwong/j4a/internal/apperr"
)

func TestClientAuthHeadersAndBasePath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		config    func(string) Config
		wantAuth  string
		wantBasic bool
		wantAgent string
	}{
		{
			name:      "basic",
			config:    func(base string) Config { return Config{BaseURL: base + "/jira/", Username: "ada", Password: "secret"} },
			wantBasic: true, wantAgent: defaultUserAgent,
		},
		{
			name: "pat takes precedence",
			config: func(base string) Config {
				return Config{BaseURL: base + "/jira", Username: "ada", Password: "secret", PAT: "token", UserAgent: "j4a-test"}
			},
			wantAuth: "Bearer token", wantAgent: "j4a-test",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/jira/rest/api/2/myself" {
					t.Fatalf("path = %q", r.URL.Path)
				}
				if got := r.Header.Get("User-Agent"); got != test.wantAgent {
					t.Fatalf("User-Agent = %q, want %q", got, test.wantAgent)
				}
				if test.wantBasic {
					username, password, ok := r.BasicAuth()
					if !ok || username != "ada" || password != "secret" {
						t.Fatalf("BasicAuth = (%q, %q, %v)", username, password, ok)
					}
				} else if got := r.Header.Get("Authorization"); got != test.wantAuth {
					t.Fatalf("Authorization = %q, want %q", got, test.wantAuth)
				}
				_, _ = w.Write([]byte(`{"name":"ada","displayName":"Ada"}`))
			}))
			defer server.Close()
			client, err := NewClient(test.config(server.URL))
			if err != nil {
				t.Fatal(err)
			}
			user, err := client.Myself(context.Background())
			if err != nil || user.Username != "ada" {
				t.Fatalf("Myself() = %#v, %v", user, err)
			}
		})
	}
}

func TestHTTPStatusMappingPreservesRawBody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status int
		kind   apperr.Kind
	}{
		{http.StatusUnauthorized, apperr.KindAuth},
		{http.StatusForbidden, apperr.KindAuth},
		{http.StatusNotFound, apperr.KindNotFound},
		{http.StatusTooManyRequests, apperr.KindRateLimit},
		{http.StatusBadRequest, apperr.KindAPI},
		{http.StatusInternalServerError, apperr.KindAPI},
	}
	for _, test := range cases {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(`{"errorMessages":["nope"]}`))
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Myself(context.Background())
			if got := apperr.As(err); got.Kind != test.kind || got.StatusCode != test.status {
				t.Fatalf("apperr = %#v, want kind=%q status=%d", got, test.kind, test.status)
			}
			if !strings.Contains(err.Error(), "nope") {
				t.Fatalf("error message = %q", err)
			}
			var httpErr *HTTPError
			if !errors.As(err, &httpErr) || string(httpErr.Body) != `{"errorMessages":["nope"]}` {
				t.Fatalf("HTTPError = %#v", httpErr)
			}
		})
	}
}

func TestSearchPaginationAndFields(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/2/search" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["jql"] != "project = DEMO" || payload["startAt"] != float64(20) || payload["maxResults"] != float64(10) {
			t.Fatalf("payload = %#v", payload)
		}
		if got := payload["fields"].([]any); len(got) != 2 || got[0] != "summary" || got[1] != "customfield_1" {
			t.Fatalf("fields = %#v", got)
		}
		_, _ = w.Write([]byte(`{"startAt":20,"maxResults":10,"total":21,"issues":[{"id":"1","key":"DEMO-1","fields":{"summary":"Example","status":{"id":"3","name":"Done"}}}]}`))
	}))
	defer server.Close()
	client, _ := NewClient(Config{BaseURL: server.URL})
	result, err := client.Search(context.Background(), IssueListOptions{JQL: "project = DEMO", Page: Page{StartAt: 20, MaxResults: 10}, Fields: []string{"summary", "customfield_1"}})
	if err != nil || result.Total != 21 || len(result.Issues) != 1 || result.Issues[0].Status.Name != "Done" {
		t.Fatalf("Search() = %#v, %v", result, err)
	}
}

func TestRawRequest(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/serverInfo" || r.URL.Query().Get("expand") != "build" {
			t.Fatalf("request = %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"buildNumber":123}`))
	}))
	defer server.Close()
	client, _ := NewClient(Config{BaseURL: server.URL})
	raw, err := client.RawRequest(context.Background(), http.MethodGet, "/rest/api/2/serverInfo", url.Values{"expand": {"build"}}, nil)
	if err != nil || string(raw) != `{"buildNumber":123}` {
		t.Fatalf("RawRequest = %q, %v", raw, err)
	}
}
