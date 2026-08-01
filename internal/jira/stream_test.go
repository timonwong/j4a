package jira

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidateRelativeEndpoint(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{
		"rest/api/2/myself",
		"/rest/api/2/search?jql=project%20%3D%20OPS&expand=names",
		"plugins/servlet/example/%E2%9C%93",
		"rest/api/2/myself#profile",
		"rest/./api/../myself",
		"rest/%252e%252e/api/2/myself",
		`rest\api\2\myself`,
		"rest/%0a/api/2/myself",
		"rest/%zz/api/2/myself",
		"rest/api/2/search?q=%25250a",
	} {
		endpoint := endpoint
		t.Run("accept "+endpoint, func(t *testing.T) {
			t.Parallel()
			if err := ValidateRelativeEndpoint(endpoint); err != nil {
				t.Fatalf("ValidateRelativeEndpoint(%q) error = %v", endpoint, err)
			}
		})
	}

	for _, endpoint := range []string{
		"",
		"https://other.example/rest/api/2/myself",
		"//other.example/rest/api/2/myself",
	} {
		endpoint := endpoint
		t.Run("reject "+endpoint, func(t *testing.T) {
			t.Parallel()
			if err := ValidateRelativeEndpoint(endpoint); err == nil {
				t.Fatalf("ValidateRelativeEndpoint(%q) succeeded", endpoint)
			}
		})
	}
}

func TestStreamRequestPreservesContextPathQueryAndAuthentication(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.EscapedPath() != "/jira/rest/api/2/search/%E2%9C%93" {
			t.Fatalf("escaped path = %q", request.URL.EscapedPath())
		}
		if request.URL.RawQuery != "jql=project%20%3D%20OPS&expand=names&field=one&field=two" {
			t.Fatalf("raw query = %q", request.URL.RawQuery)
		}
		username, password, ok := request.BasicAuth()
		if !ok || username != "alice" || password != "secret" {
			t.Fatalf("basic auth = %q/%q/%t", username, password, ok)
		}
		if got := request.Header.Get("User-Agent"); got != "jiro/test" {
			t.Fatalf("User-Agent = %q", got)
		}
		if got := request.Header.Get("X-Test"); got != "kept" {
			t.Fatalf("X-Test = %q", got)
		}
		_, _ = io.WriteString(writer, "ok")
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL + "/jira", Username: "alice", Password: "secret", UserAgent: "jiro/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Stream(context.Background(), StreamRequest{
		Method:   http.MethodGet,
		Endpoint: "/rest/api/2/search/%E2%9C%93?jql=project%20%3D%20OPS&expand=names",
		Query:    []QueryParameter{{Key: "field", Value: "one"}, {Key: "field", Value: "two"}},
		Header:   http.Header{"X-Test": []string{"kept"}},
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}

func TestStreamRequestUsesPATAndCallerContentLength(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		if request.ContentLength != 4 {
			t.Fatalf("ContentLength = %d", request.ContentLength)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "data" {
			t.Fatalf("body = %q", body)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, PAT: "token"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Stream(context.Background(), StreamRequest{
		Method:        http.MethodPost,
		Endpoint:      "rest/api/2/example",
		Body:          strings.NewReader("data"),
		ContentLength: 4,
		Timeout:       5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

func TestStreamRequestRepresentsKnownEmptyBodyExplicitly(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Body != http.NoBody || request.ContentLength != 0 {
			t.Fatalf("Body=%T ContentLength=%d", request.Body, request.ContentLength)
		}
		return &http.Response{
			StatusCode: http.StatusNoContent, Status: "204 No Content", Proto: "HTTP/1.1",
			Header: make(http.Header), Body: http.NoBody, Request: request,
		}, nil
	})
	client, err := NewClient(Config{BaseURL: "https://jira.example", HTTPClient: &http.Client{Transport: transport}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Stream(context.Background(), StreamRequest{
		Method: http.MethodPost, Endpoint: "rest/api/2/import", Body: strings.NewReader(""), ContentLength: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

func TestStreamRequestInsecureTLSIsInvocationScoped(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "ok")
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}

	if response, requestErr := client.Stream(context.Background(), StreamRequest{
		Method: http.MethodGet, Endpoint: "rest/api/2/example", Timeout: time.Second,
	}); requestErr == nil {
		_ = response.Body.Close()
		t.Fatal("verified TLS request unexpectedly succeeded")
	}

	response, err := client.Stream(context.Background(), StreamRequest{
		Method: http.MethodGet, Endpoint: "rest/api/2/example", Timeout: time.Second, Insecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "ok" {
		t.Fatalf("body=%q error=%v", body, err)
	}
}

func TestStreamRequestUsesHTTP11(t *testing.T) {
	t.Parallel()

	protocol := make(chan int, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		protocol <- request.ProtoMajor
		writer.WriteHeader(http.StatusNoContent)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Stream(context.Background(), StreamRequest{
		Method: http.MethodGet, Endpoint: "rest/api/2/example", Timeout: time.Second, Insecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if got := <-protocol; got != 1 {
		t.Fatalf("HTTP protocol major = %d, want 1", got)
	}
}

func TestStreamRequestReturnsRedirectWithoutFollowingIt(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		http.Redirect(writer, request, "/next", http.StatusFound)
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Stream(context.Background(), StreamRequest{
		Method: http.MethodGet, Endpoint: "start", Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound || calls.Load() != 1 {
		t.Fatalf("status=%d calls=%d", response.StatusCode, calls.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
