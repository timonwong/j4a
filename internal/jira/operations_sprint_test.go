package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMoveIssueToSprintUsesFirstNameMatchAcrossBoardAndSprintPages(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/agile/1.0/board":
			switch r.URL.Query().Get("startAt") {
			case "":
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1,"total":2,"values":[{"id":1,"name":"Team A","type":"scrum"}]}`))
			case "1":
				_, _ = w.Write([]byte(`{"startAt":1,"maxResults":1,"total":2,"values":[{"id":2,"name":"Team B","type":"scrum"}]}`))
			default:
				t.Fatalf("unexpected board page = %s", r.URL.String())
			}
		case "/rest/agile/1.0/board/1/sprint":
			switch r.URL.Query().Get("startAt") {
			case "":
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1,"total":2,"values":[{"id":10,"name":"Planning","state":"active","originBoardId":1}]}`))
			case "1":
				_, _ = w.Write([]byte(`{"startAt":1,"maxResults":1,"total":2,"values":[{"id":11,"name":"Release Candidate","state":"future","originBoardId":1}]}`))
			default:
				t.Fatalf("unexpected sprint page = %s", r.URL.String())
			}
		case "/rest/agile/1.0/sprint/11/issue":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s", r.Method)
			}
			var payload struct {
				Issues []string `json:"issues"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Issues) != 1 || payload.Issues[0] != "DEMO-1" {
				t.Fatalf("payload = %#v", payload)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request = %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.MoveIssueToSprint(context.Background(), "DEMO-1", MoveIssueToSprintInput{Sprint: "release"}); err != nil {
		t.Fatal(err)
	}
}

func TestResolveSprintFollowsIsLastWhenTotalIsOmitted(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/agile/1.0/board":
			switch r.URL.Query().Get("startAt") {
			case "":
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1,"isLast":false,"values":[{"id":1,"name":"Team A","type":"scrum"}]}`))
			case "1":
				_, _ = w.Write([]byte(`{"startAt":1,"maxResults":1,"isLast":true,"values":[{"id":2,"name":"Team B","type":"scrum"}]}`))
			default:
				t.Fatalf("unexpected board page = %s", r.URL.String())
			}
		case "/rest/agile/1.0/board/1/sprint":
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1,"isLast":true,"values":[{"id":10,"name":"Planning","state":"active","originBoardId":1}]}`))
		case "/rest/agile/1.0/board/2/sprint":
			switch r.URL.Query().Get("startAt") {
			case "":
				_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1,"isLast":false,"values":[{"id":20,"name":"Hardening","state":"future","originBoardId":2}]}`))
			case "1":
				_, _ = w.Write([]byte(`{"startAt":1,"maxResults":1,"isLast":true,"values":[{"id":21,"name":"Release Train","state":"future","originBoardId":2}]}`))
			default:
				t.Fatalf("unexpected sprint page = %s", r.URL.String())
			}
		default:
			t.Fatalf("unexpected request = %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	sprint, err := client.ResolveSprint(context.Background(), "release")
	if err != nil || sprint.ID != 21 {
		t.Fatalf("ResolveSprint() = %#v, %v", sprint, err)
	}
}
