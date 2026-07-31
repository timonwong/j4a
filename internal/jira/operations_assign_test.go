package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssignIssueResolvesMeAndClearsWithNullUsername(t *testing.T) {
	t.Parallel()
	var writes []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/myself":
			_, _ = w.Write([]byte(`{"name":"ada","displayName":"Ada"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/rest/api/2/issue/DEMO-1/assignee":
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			writes = append(writes, payload)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request = %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	me, err := client.ResolveAssignee(context.Background(), "me")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.AssignIssue(context.Background(), "DEMO-1", me); err != nil {
		t.Fatal(err)
	}
	none, err := client.ResolveAssignee(context.Background(), "none")
	if err != nil {
		t.Fatal(err)
	}
	if err := client.AssignIssue(context.Background(), "DEMO-1", none); err != nil {
		t.Fatal(err)
	}
	if len(writes) != 2 || writes[0]["name"] != "ada" || writes[1]["name"] != nil {
		t.Fatalf("assignment payloads = %#v", writes)
	}
}
