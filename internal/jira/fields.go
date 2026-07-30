package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	"github.com/timonwong/j4a/internal/apperr"
)

var customFieldID = regexp.MustCompile(`^customfield_[0-9]+$`)

// ResolveCustomField resolves a j4a custom field alias against this Jira
// instance. A canonical custom field ID never causes a /field lookup.
func (c *Client) ResolveCustomField(ctx context.Context, alias string) (string, error) {
	if customFieldID.MatchString(alias) {
		return alias, nil
	}
	fields, err := c.ListFields(ctx)
	if err != nil {
		return "", err
	}
	return ResolveCustomField(alias, fields)
}

// ResolveCustomField resolves an alias using supplied live field definitions.
// It is useful when callers already fetched /field and want to avoid another
// request.
func ResolveCustomField(alias string, fields []Field) (string, error) {
	if customFieldID.MatchString(alias) {
		return alias, nil
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return "", apperr.New(apperr.KindInvalidInput, "custom field alias is required")
	}
	matches := make([]Field, 0, 1)
	for _, field := range fields {
		if (field.Custom || customFieldID.MatchString(field.ID)) && Slug(field.Name) == alias {
			matches = append(matches, field)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0].ID, nil
	case 0:
		return "", apperr.New(apperr.KindInvalidInput, fmt.Sprintf("custom field alias %q was not found", alias))
	default:
		ids := make([]string, len(matches))
		for i, field := range matches {
			ids[i] = field.ID
		}
		return "", apperr.New(apperr.KindInvalidInput, fmt.Sprintf("custom field alias %q is ambiguous: %s", alias, strings.Join(ids, ", ")))
	}
}

// Slug turns a Jira field display name into its j4a alias. Unicode letters and
// numbers are retained; every run of separators becomes one hyphen.
func Slug(name string) string {
	var result []rune
	separator := false
	for _, char := range strings.TrimSpace(name) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			if separator && len(result) > 0 {
				result = append(result, '-')
			}
			result = append(result, unicode.ToLower(char))
			separator = false
		} else {
			separator = true
		}
	}
	return strings.Trim(string(result), "-")
}

// ParseFieldValues parses repeated key=value flags. Values are decoded as JSON
// first with json.Number preserved, falling back to plain strings.
func ParseFieldValues(values []string) (map[string]any, error) {
	parsed := make(map[string]any, len(values))
	for _, value := range values {
		key, raw, found := strings.Cut(value, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, apperr.New(apperr.KindInvalidInput, fmt.Sprintf("field %q must be key=value", value))
		}
		parsedValue, err := decodeFieldValue(raw)
		if err != nil {
			return nil, err
		}
		parsed[key] = parsedValue
	}
	return parsed, nil
}

func decodeFieldValue(value string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err == nil {
		var extra any
		if decoder.Decode(&extra) == io.EOF {
			return decoded, nil
		}
	}
	return value, nil
}
