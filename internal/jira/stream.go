package jira

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/timonwong/jiro/internal/apperr"
)

// StreamRequest describes one raw authenticated request relative to the
// Client's Jira Instance. The caller owns the returned response body.
type StreamRequest struct {
	Method          string
	Endpoint        string
	Query           []QueryParameter
	Header          http.Header
	Body            io.Reader
	ContentLength   int64
	Timeout         time.Duration
	Insecure        bool
	FollowRedirects bool
}

const maximumStreamRedirects = 5

// QueryParameter is one ordered query entry appended after the endpoint's
// existing query string. Repeated keys are preserved.
type QueryParameter struct {
	Key   string
	Value string
}

// ValidateRelativeEndpoint rejects endpoints that could escape or ambiguously
// rewrite the configured Jira Instance path.
func ValidateRelativeEndpoint(endpoint string) error {
	if endpoint == "" {
		return apperr.New(apperr.KindInvalidInput, "API endpoint must not be empty")
	}
	if strings.HasPrefix(endpoint, "//") {
		return apperr.New(apperr.KindInvalidInput, "API endpoint must be relative to the Jira Instance")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return apperr.Wrap(apperr.KindInvalidInput, err, "invalid API endpoint")
	}
	if parsed.IsAbs() || parsed.Host != "" || parsed.Opaque != "" {
		return apperr.New(apperr.KindInvalidInput, "API endpoint must be relative to the Jira Instance")
	}
	if parsed.Fragment != "" || strings.Contains(endpoint, "#") {
		return apperr.New(apperr.KindInvalidInput, "API endpoint must not contain a fragment")
	}
	if parsed.Path == "" {
		return apperr.New(apperr.KindInvalidInput, "API endpoint path must not be empty")
	}

	if err := validateRecursivelyDecodedEndpoint(endpointPath(endpoint), "path", url.PathUnescape, validateDecodedEndpointPath); err != nil {
		return err
	}
	if err := validateRecursivelyDecodedEndpoint(parsed.RawQuery, "query", url.QueryUnescape, validateDecodedEndpointQuery); err != nil {
		return err
	}
	return nil
}

func validateRecursivelyDecodedEndpoint(value, component string, decode func(string) (string, error), validate func(string) error) error {
	decoded := value
	for depth := 0; depth < 8; depth++ {
		unescaped, err := decode(decoded)
		if err != nil {
			return apperr.Wrap(apperr.KindInvalidInput, err, "invalid API endpoint %s encoding", component)
		}
		if err := validate(unescaped); err != nil {
			return err
		}
		if unescaped == decoded {
			return nil
		}
		decoded = unescaped
	}
	return apperr.New(apperr.KindInvalidInput, "API endpoint "+component+" encoding is too deeply nested")
}

func endpointPath(endpoint string) string {
	path, _, _ := strings.Cut(endpoint, "?")
	return path
}

func validateDecodedEndpointPath(path string) error {
	if err := validateDecodedEndpointCharacters(path, "path"); err != nil {
		return err
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return apperr.New(apperr.KindInvalidInput, "API endpoint path must not contain traversal segments")
		}
	}
	return nil
}

func validateDecodedEndpointQuery(query string) error {
	return validateDecodedEndpointCharacters(query, "query")
}

func validateDecodedEndpointCharacters(value, component string) error {
	if strings.ContainsRune(value, '\\') {
		return apperr.New(apperr.KindInvalidInput, "API endpoint "+component+" must not contain backslashes")
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return apperr.New(apperr.KindInvalidInput, "API endpoint "+component+" must not contain control characters")
		}
	}
	return nil
}

// Stream sends one authenticated request without buffering its request or
// response body. It disables automatic decompression and connection retries.
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
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if !input.FollowRedirects {
			return fmt.Errorf("redirects are not allowed for this request")
		}
		if len(via) > maximumStreamRedirects {
			return fmt.Errorf("stopped after %d redirects", maximumStreamRedirects)
		}
		if !sameOrigin(via[0].URL, request.URL) {
			return fmt.Errorf("redirect changed Jira Instance origin")
		}
		return nil
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
	clone.DisableCompression = true
	clone.DisableKeepAlives = true
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

func sameOrigin(first, second *url.URL) bool {
	return strings.EqualFold(first.Scheme, second.Scheme) &&
		strings.EqualFold(first.Hostname(), second.Hostname()) &&
		originPort(first) == originPort(second)
}

func originPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	return "80"
}
