package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/timonwong/j4a/internal/apperr"
	"github.com/timonwong/j4a/internal/jira"
)

func TestSearchAllRejectsNonProgressingPagination(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":1,"total":3,"issues":[{"id":"1","key":"OPS-1","fields":{}}]}`)
	}))
	defer server.Close()
	client, err := jira.NewClient(jira.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = searchAll(context.Background(), client, "project = OPS", 0, 1, nil)
	if err == nil || apperr.As(err).Kind != apperr.KindAPI || requests != 2 {
		t.Fatalf("searchAll() error=%v requests=%d", err, requests)
	}
}

func TestSearchAllDeduplicatesIssueKeysInFirstSeenOrder(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		switch payload["startAt"] {
		case nil:
			_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":2,"total":3,"issues":[{"id":"1","key":"OPS-1","fields":{}},{"id":"2","key":"OPS-2","fields":{}}]}`)
		case float64(2):
			_, _ = io.WriteString(writer, `{"startAt":2,"maxResults":2,"total":3,"issues":[{"id":"2","key":"OPS-2","fields":{}},{"id":"3","key":"OPS-3","fields":{}}]}`)
		default:
			t.Fatalf("payload = %#v", payload)
		}
	}))
	defer server.Close()
	client, err := jira.NewClient(jira.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := searchAll(context.Background(), client, "project = OPS", 0, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 3 || result.Issues[0].Key != "OPS-1" || result.Issues[1].Key != "OPS-2" || result.Issues[2].Key != "OPS-3" {
		t.Fatalf("issues = %#v", result.Issues)
	}
}

func TestSearchAllContinuesUntilEmptyWhenTotalIsOmitted(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		switch payload["startAt"] {
		case nil:
			_, _ = io.WriteString(writer, `{"startAt":0,"maxResults":1,"issues":[{"id":"1","key":"OPS-1","fields":{}}]}`)
		case float64(1):
			_, _ = io.WriteString(writer, `{"startAt":1,"maxResults":1,"issues":[{"id":"2","key":"OPS-2","fields":{}}]}`)
		case float64(2):
			_, _ = io.WriteString(writer, `{"startAt":2,"maxResults":1,"issues":[]}`)
		default:
			t.Fatalf("payload = %#v", payload)
		}
	}))
	defer server.Close()
	client, err := jira.NewClient(jira.Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := searchAll(context.Background(), client, "project = OPS", 0, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Issues) != 2 || result.Issues[1].Key != "OPS-2" {
		t.Fatalf("issues = %#v", result.Issues)
	}
}
