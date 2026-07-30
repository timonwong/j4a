package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/timonwong/j4a/internal/apperr"
)

func TestResolveCustomFieldPriorityAndAmbiguity(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`[
			{"id":"customfield_10001","name":"Story Points","custom":true},
			{"id":"customfield_10002","name":"Story-Points","custom":true}
		]`))
	}))
	defer server.Close()
	client, _ := NewClient(Config{BaseURL: server.URL})
	id, err := client.ResolveCustomField(context.Background(), "customfield_12345")
	if err != nil || id != "customfield_12345" || requests != 0 {
		t.Fatalf("direct resolution = %q, %v, requests=%d", id, err, requests)
	}
	_, err = client.ResolveCustomField(context.Background(), "story-points")
	if got := apperr.As(err).Kind; got != apperr.KindInvalidInput || requests != 1 {
		t.Fatalf("ambiguous resolution error = %v, requests=%d", err, requests)
	}
}

func TestResolveCustomFieldAndParseValues(t *testing.T) {
	t.Parallel()
	id, err := ResolveCustomField("故事-点数", []Field{{ID: "customfield_9", Name: "故事 点数", Custom: true}})
	if err != nil || id != "customfield_9" {
		t.Fatalf("ResolveCustomField = %q, %v", id, err)
	}
	values, err := ParseFieldValues([]string{
		"customfield_1=1.25",
		`customfield_2={"value":"High"}`,
		"customfield_3=not-json",
		"customfield_4=true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if number, ok := values["customfield_1"].(json.Number); !ok || number.String() != "1.25" {
		t.Fatalf("number = %#v", values["customfield_1"])
	}
	if object := values["customfield_2"].(map[string]any); object["value"] != "High" {
		t.Fatalf("object = %#v", object)
	}
	if values["customfield_3"] != "not-json" || values["customfield_4"] != true {
		t.Fatalf("values = %#v", values)
	}
}
