package config

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type tomlFile struct {
	Default  *tomlValues           `toml:"default"`
	Profiles map[string]tomlValues `toml:"profiles"`
}

type tomlValues struct {
	Host       *string `toml:"host"`
	Username   *string `toml:"username"`
	AuthType   *string `toml:"auth_type"`
	APIVersion *int    `toml:"api_version"`
	ReadOnly   *bool   `toml:"read_only"`
	UseKeyring *bool   `toml:"use_keyring"`
	Password   *string `toml:"password"`
	Token      *string `toml:"token"`
}

func parseTOML(input string) (fileConfig, error) {
	var decoded tomlFile
	decoder := toml.NewDecoder(strings.NewReader(input)).DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fileConfig{}, err
	}
	result := fileConfig{DefaultExists: decoded.Default != nil, Profiles: make(map[string]values, len(decoded.Profiles))}
	if decoded.Default != nil {
		result.Default = decoded.Default.values()
	}
	for name, profile := range decoded.Profiles {
		result.Profiles[name] = profile.values()
	}
	return result, nil
}

func (v tomlValues) values() values {
	return values{
		host: v.Host, username: v.Username, authType: v.AuthType,
		apiVersion: v.APIVersion, readOnly: v.ReadOnly, useKeyring: v.UseKeyring,
		password: v.Password, token: v.Token,
	}
}

type tomlValueKind uint8

const (
	tomlString tomlValueKind = iota
	tomlBool
)

type tomlUpdate struct {
	delete bool
	kind   tomlValueKind
	text   string
	flag   bool
}

type tomlField struct {
	lineStart, lineEnd int
	valueStart         int
	valueEnd           int
	kind               unstable.Kind
	data               string
}

type tomlSection struct {
	found        bool
	insertOffset int
	fields       map[string]tomlField
}

type textEdit struct {
	start, end  int
	replacement []byte
}

// updateProfileTOML edits only the requested table and keys. All untouched
// source bytes, including comments and formatting, are copied verbatim.
func updateProfileTOML(source []byte, profile string, updates map[string]tomlUpdate) ([]byte, error) {
	target := []string{"default"}
	if profile != "" {
		target = []string{"profiles", profile}
	}
	section, err := inspectTOMLSection(source, target)
	if err != nil {
		return nil, err
	}

	if !section.found {
		return appendTOMLSection(source, profile, updates), nil
	}

	var edits []textEdit
	var additions []string
	keys := sortedUpdateKeys(updates)
	for _, key := range keys {
		update := updates[key]
		field, exists := section.fields[key]
		if update.delete {
			if exists {
				edits = append(edits, textEdit{start: field.lineStart, end: field.lineEnd})
			}
			continue
		}

		encoded := encodeTOMLUpdate(update)
		if exists {
			if tomlFieldMatches(field, update) {
				continue
			}
			edits = append(edits, textEdit{start: field.valueStart, end: field.valueEnd, replacement: []byte(encoded)})
			continue
		}
		additions = append(additions, key+" = "+encoded+"\n")
	}
	if len(additions) > 0 {
		prefix := ""
		if section.insertOffset > 0 && source[section.insertOffset-1] != '\n' {
			prefix = "\n"
		}
		edits = append(edits, textEdit{
			start:       section.insertOffset,
			end:         section.insertOffset,
			replacement: []byte(prefix + strings.Join(additions, "")),
		})
	}

	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].start == edits[j].start {
			return edits[i].end > edits[j].end
		}
		return edits[i].start > edits[j].start
	})
	result := append([]byte(nil), source...)
	for _, edit := range edits {
		result = append(result[:edit.start], append(edit.replacement, result[edit.end:]...)...)
	}
	return result, nil
}

func inspectTOMLSection(source []byte, target []string) (tomlSection, error) {
	section := tomlSection{fields: map[string]tomlField{}}
	var parser unstable.Parser
	parser.KeepComments = true
	parser.Reset(source)
	var current []string
	for parser.NextExpression() {
		node := parser.Expression()
		switch node.Kind {
		case unstable.Table, unstable.ArrayTable:
			current = nodeKey(node)
			if slicesEqual(current, target) {
				key := node.Child()
				_, lineEnd := sourceLineBounds(source, int(key.Raw.Offset), int(key.Raw.Offset+key.Raw.Length))
				section.found = true
				section.insertOffset = lineEnd
			}
		case unstable.KeyValue:
			if !slicesEqual(current, target) {
				continue
			}
			keys := nodeKey(node)
			if len(keys) != 1 {
				continue
			}
			value := node.Value()
			lineStart, lineEnd := sourceLineBounds(source, int(node.Raw.Offset), int(node.Raw.Offset+node.Raw.Length))
			section.fields[keys[0]] = tomlField{
				lineStart: lineStart, lineEnd: lineEnd,
				valueStart: int(value.Raw.Offset), valueEnd: int(value.Raw.Offset + value.Raw.Length),
				kind: value.Kind, data: string(value.Data),
			}
			section.insertOffset = lineEnd
		}
	}
	if err := parser.Error(); err != nil {
		return tomlSection{}, err
	}
	return section, nil
}

func nodeKey(node *unstable.Node) []string {
	var result []string
	iterator := node.Key()
	for iterator.Next() {
		result = append(result, string(iterator.Node().Data))
	}
	return result
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sourceLineBounds(source []byte, startOffset, endOffset int) (int, int) {
	start := bytes.LastIndexByte(source[:startOffset], '\n') + 1
	end := bytes.IndexByte(source[endOffset:], '\n')
	if end < 0 {
		return start, len(source)
	}
	return start, endOffset + end + 1
}

func tomlFieldMatches(field tomlField, update tomlUpdate) bool {
	switch update.kind {
	case tomlString:
		return field.kind == unstable.String && field.data == update.text
	case tomlBool:
		return field.kind == unstable.Bool && field.data == strconv.FormatBool(update.flag)
	default:
		return false
	}
}

func encodeTOMLUpdate(update tomlUpdate) string {
	if update.kind == tomlBool {
		return strconv.FormatBool(update.flag)
	}
	return encodeTOMLString(update.text)
}

func encodeTOMLString(value string) string {
	var result strings.Builder
	result.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\b':
			result.WriteString(`\b`)
		case '\t':
			result.WriteString(`\t`)
		case '\n':
			result.WriteString(`\n`)
		case '\f':
			result.WriteString(`\f`)
		case '\r':
			result.WriteString(`\r`)
		case '"':
			result.WriteString(`\"`)
		case '\\':
			result.WriteString(`\\`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&result, `\u%04X`, r)
			} else {
				result.WriteRune(r)
			}
		}
	}
	result.WriteByte('"')
	return result.String()
}

func appendTOMLSection(source []byte, profile string, updates map[string]tomlUpdate) []byte {
	result := append([]byte(nil), source...)
	if len(result) > 0 && result[len(result)-1] != '\n' {
		result = append(result, '\n')
	}
	if len(result) > 0 && !bytes.HasSuffix(result, []byte("\n\n")) {
		result = append(result, '\n')
	}
	header := "[default]\n"
	if profile != "" {
		header = "[profiles." + encodeTOMLKey(profile) + "]\n"
	}
	result = append(result, header...)
	for _, key := range sortedUpdateKeys(updates) {
		update := updates[key]
		if update.delete {
			continue
		}
		result = append(result, key+" = "+encodeTOMLUpdate(update)+"\n"...)
	}
	return result
}

func encodeTOMLKey(value string) string {
	if value != "" {
		bare := true
		for _, r := range value {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
				bare = false
				break
			}
		}
		if bare {
			return value
		}
	}
	return encodeTOMLString(value)
}

func sortedUpdateKeys(updates map[string]tomlUpdate) []string {
	order := map[string]int{
		"host": 0, "username": 1, "auth_type": 2,
		"use_keyring": 3, "password": 4, "token": 5,
	}
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, lok := order[keys[i]]
		right, rok := order[keys[j]]
		if lok && rok {
			return left < right
		}
		if lok != rok {
			return lok
		}
		return keys[i] < keys[j]
	})
	return keys
}
