package jira

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/timonwong/jiro/internal/apperr"
)

// StreamRequest describes one raw authenticated request relative to the
// Client's Jira Instance. The caller owns the returned response body.
type StreamRequest struct {
	Method        string
	Endpoint      string
	Query         []QueryParameter
	Header        http.Header
	Body          io.Reader
	ContentLength int64
	Timeout       time.Duration
	Insecure      bool
}

// QueryParameter is one ordered query entry appended after the endpoint's
// existing query string. Repeated keys are preserved.
type QueryParameter struct {
	Key   string
	Value string
}

// ValidateRelativeEndpoint rejects only endpoints that select another origin.
// All other URL validation is delegated to Go's HTTP stack.
func ValidateRelativeEndpoint(endpoint string) error {
	if endpoint == "" {
		return apperr.New(apperr.KindInvalidInput, "API endpoint must not be empty")
	}
	if strings.HasPrefix(endpoint, "//") {
		return apperr.New(apperr.KindInvalidInput, "API endpoint must be relative to the Jira Instance")
	}
	parsed, err := url.Parse(endpoint)
	if err == nil && (parsed.IsAbs() || parsed.Host != "" || parsed.Opaque != "") {
		return apperr.New(apperr.KindInvalidInput, "API endpoint must be relative to the Jira Instance")
	}
	return nil
}

// Stream sends one authenticated request without buffering its request or
// response body. It uses Go's standard compression and connection semantics.
func (c *Client) Stream(ctx context.Context, input StreamRequest) (*http.Response, error) {
	if err := ValidateRelativeEndpoint(input.Endpoint); err != nil {
		return nil, err
	}
	if input.Timeout < 0 {
		return nil, apperr.New(apperr.KindInvalidInput, "timeout must not be negative")
	}

	requestURL := c.streamEndpoint(input.Endpoint, input.Query)
	body := input.Body
	if body != nil && input.ContentLength == 0 {
		body = http.NoBody
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, requestURL, body)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindInvalidInput, err, "create Jira API request")
	}
	request.Header = input.Header.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.ContentLength = input.ContentLength
	request.Header.Set("User-Agent", c.userAgent)
	request.Header.Del("Authorization")
	if c.pat != "" {
		request.Header.Set("Authorization", "Bearer "+c.pat)
	} else if c.username != "" || c.password != "" {
		request.SetBasicAuth(c.username, c.password)
	}

	httpClient, err := c.streamHTTPClient(input)
	if err != nil {
		return nil, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindAPI, err, "request Jira API: %v", err)
	}
	return response, nil
}

func (c *Client) streamEndpoint(endpoint string, parameters []QueryParameter) string {
	path, query, hasQuery := strings.Cut(endpoint, "?")
	path = strings.TrimPrefix(path, "/")
	basePath := strings.TrimRight(c.baseURL.EscapedPath(), "/")
	result := c.baseURL.Scheme + "://" + c.baseURL.Host + basePath + "/" + path
	if hasQuery || len(parameters) > 0 {
		result += "?" + query
	}
	separator := ""
	if hasQuery {
		separator = "&"
	}
	for _, parameter := range parameters {
		result += separator + url.QueryEscape(parameter.Key) + "=" + url.QueryEscape(parameter.Value)
		separator = "&"
	}
	return result
}

func (c *Client) streamHTTPClient(input StreamRequest) (*http.Client, error) {
	client := *c.httpClient
	client.Timeout = input.Timeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	base, ok := transport.(*http.Transport)
	if !ok {
		if input.Insecure {
			return nil, apperr.New(apperr.KindInvalidInput, "--insecure requires the standard HTTP transport")
		}
		return &client, nil
	}
	clone := base.Clone()
	clone.ForceAttemptHTTP2 = false
	clone.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	clone.Protocols = protocols
	tlsConfig := &tls.Config{NextProtos: []string{"http/1.1"}}
	if clone.TLSClientConfig != nil {
		tlsConfig = clone.TLSClientConfig.Clone()
		tlsConfig.NextProtos = []string{"http/1.1"}
	}
	if input.Insecure {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec -- explicit --insecure behavior
	}
	clone.TLSClientConfig = tlsConfig
	client.Transport = clone
	return &client, nil
}
