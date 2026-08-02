package markup

import (
	"context"
	"fmt"
	"strings"
)

func renderJiraMarkup(ctx context.Context, document semanticDocument) (string, error) {
	blocks := make([]string, 0, len(document.Blocks))
	for _, block := range document.Blocks {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		switch typed := block.(type) {
		case headingBlock:
			content, err := renderJiraInlines(ctx, typed.Inlines, false)
			if err != nil {
				return "", err
			}
			heading := fmt.Sprintf("h%d.", typed.Level)
			if content != "" {
				heading += " " + content
			}
			blocks = append(blocks, heading)
		case paragraphBlock:
			content, err := renderJiraInlines(ctx, typed.Inlines, true)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		case thematicBreakBlock:
			blocks = append(blocks, "----")
		case quoteBlock:
			paragraphs := make([]string, 0, len(typed.Paragraphs))
			for _, paragraph := range typed.Paragraphs {
				content, err := renderJiraInlines(ctx, paragraph.Inlines, true)
				if err != nil {
					return "", err
				}
				paragraphs = append(paragraphs, content)
			}
			blocks = append(blocks, "{quote}\n"+strings.Join(paragraphs, "\n\n")+"\n{quote}")
		case listBlock:
			content, err := renderJiraList(ctx, typed, "")
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		case tableBlock:
			content, err := renderJiraTable(ctx, typed)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		case codeBlock:
			content, err := renderJiraCodeBlock(ctx, typed)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		case panelBlock:
			body, err := renderJiraMarkup(ctx, semanticDocument{Blocks: typed.Blocks})
			if err != nil {
				return "", err
			}
			header := "{panel}"
			if len(typed.Attributes) != 0 {
				parts := make([]string, 0, len(typed.Attributes))
				for _, attribute := range orderDirectiveAttributes(typed.Attributes, panelAttributeOrder()) {
					if attribute.Bare {
						parts = append(parts, attribute.Name)
					} else {
						value, err := escapeJiraDelimitedValueWithContext(ctx, attribute.Value, `\{}|`)
						if err != nil {
							return "", err
						}
						parts = append(parts, attribute.Name+"="+value)
					}
				}
				header = "{panel:" + strings.Join(parts, "|") + "}"
			}
			blocks = append(blocks, header+"\n"+body+ensureLiteralClosingSeparation(body)+"{panel}")
		case unsupportedMacroBlock:
			body, err := renderJiraMarkup(ctx, semanticDocument{Blocks: typed.Blocks})
			if err != nil {
				return "", err
			}
			blocks = append(blocks, typed.Opening+"\n"+body+ensureLiteralClosingSeparation(body)+typed.Closing)
		case literalBlock:
			content, err := escapeTextForJira(ctx, typed.Text, true)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		default:
			return "", fmt.Errorf("%w: unsupported semantic block in Jira renderer", ErrConversion)
		}
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n")), nil
}

func renderJiraInlines(ctx context.Context, inlines []semanticInline, atLineStart bool) (string, error) {
	var result strings.Builder
	for _, inline := range inlines {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		switch typed := inline.(type) {
		case textInline:
			content, err := escapeTextForJira(ctx, typed.Text, atLineStart)
			if err != nil {
				return "", err
			}
			result.WriteString(content)
		case codeInline:
			result.WriteString("{{" + escapeCodeSpan(typed.Text) + "}}")
		case hardBreakInline:
			result.WriteString("\\\\\n")
		case styledInline:
			children := typed.Children
			if combined, ok := combinedBoldItalic(typed); ok {
				content, err := renderJiraInlines(ctx, combined, false)
				if err != nil {
					return "", err
				}
				result.WriteString("*_" + content + "_*")
				break
			}
			content, err := renderJiraInlines(ctx, children, false)
			if err != nil {
				return "", err
			}
			switch typed.Style {
			case styleBold:
				result.WriteString("*" + content + "*")
			case styleItalic:
				result.WriteString("_" + content + "_")
			case styleStrike:
				result.WriteString("-" + content + "-")
			case styleInserted:
				result.WriteString("+" + content + "+")
			case styleSuper:
				result.WriteString("^" + content + "^")
			case styleSub:
				result.WriteString("~" + content + "~")
			case styleColor:
				value, err := escapeJiraDelimitedValueWithContext(ctx, typed.Value, `\{}|`)
				if err != nil {
					return "", err
				}
				result.WriteString("{color:" + value + "}" + content + "{color}")
			}
		case linkInline:
			label, err := renderJiraInlines(ctx, typed.Label, false)
			if err != nil {
				return "", err
			}
			target, err := escapeJiraDelimitedValueWithContext(ctx, typed.Target, `\[]|`)
			if err != nil {
				return "", err
			}
			if typed.Unnamed {
				result.WriteString("[" + target + "]")
			} else {
				result.WriteString("[" + label + "|" + target + "]")
			}
		case imageInline:
			source, err := escapeJiraDelimitedValueWithContext(ctx, typed.Source, `\!|`)
			if err != nil {
				return "", err
			}
			markup := "!" + source
			attributes := make([]string, 0, len(typed.Attributes)+1)
			if typed.Alt != "" {
				alt, err := escapeJiraDelimitedValueWithContext(ctx, typed.Alt, `\!|,=`)
				if err != nil {
					return "", err
				}
				attributes = append(attributes, "alt="+alt)
			}
			for _, attribute := range orderDirectiveAttributes(typed.Attributes, imageAttributeOrder()) {
				if attribute.Bare {
					attributes = append(attributes, attribute.Name)
				} else {
					value, err := escapeJiraDelimitedValueWithContext(ctx, attribute.Value, `\!|,=`)
					if err != nil {
						return "", err
					}
					attributes = append(attributes, attribute.Name+"="+value)
				}
			}
			if len(attributes) != 0 {
				markup += "|" + strings.Join(attributes, ",")
			}
			result.WriteString(markup + "!")
		case literalInline:
			content, err := escapeTextForJira(ctx, typed.Text, atLineStart)
			if err != nil {
				return "", err
			}
			result.WriteString(content)
		default:
			return "", fmt.Errorf("%w: unsupported semantic inline in Jira renderer", ErrConversion)
		}
		atLineStart = inlineEndsAtLineStart(inline)
	}
	return result.String(), nil
}

func escapeTextForJira(ctx context.Context, value string, atLineStart bool) (string, error) {
	var result strings.Builder
	for offset := 0; offset < len(value); {
		if offset&255 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		remaining := value[offset:]
		if atLineStart {
			if prefixLength := jiraLineControlPrefixLength(remaining); prefixLength != 0 {
				result.WriteString(remaining[:prefixLength-1])
				result.WriteString(`\.`)
				offset += prefixLength
				atLineStart = false
				continue
			}
		}
		character, size := utf8DecodeRune(remaining)
		if strings.ContainsRune(`\{}[]!*?_-+^~|#`, character) {
			result.WriteByte('\\')
		}
		result.WriteRune(character)
		offset += size
		atLineStart = character == '\n'
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return result.String(), nil
}

func escapeJiraDelimitedValueWithContext(ctx context.Context, value, delimiters string) (string, error) {
	return escapeSelectedRunes(ctx, value, delimiters)
}

func renderJiraList(ctx context.Context, list listBlock, parentMarkers string) (string, error) {
	marker := parentMarkers + "*"
	if list.Ordered {
		marker = parentMarkers + "#"
	}
	lines := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		content, err := renderJiraInlines(ctx, item.Inlines, false)
		if err != nil {
			return "", err
		}
		line := marker
		if content != "" {
			line += " " + content
		}
		lines = append(lines, line)
		for _, child := range item.Children {
			childText, err := renderJiraList(ctx, child, marker)
			if err != nil {
				return "", err
			}
			lines = append(lines, childText)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func renderJiraTable(ctx context.Context, table tableBlock) (string, error) {
	if table.Directive && table.Raw != "" {
		return table.Raw, nil
	}
	rows := make([]string, 0, len(table.Rows)+1)
	if len(table.Header) != 0 {
		header, err := renderJiraTableRow(ctx, table.Header, "||")
		if err != nil {
			return "", err
		}
		rows = append(rows, header)
	}
	for _, row := range table.Rows {
		value, err := renderJiraTableRow(ctx, row, "|")
		if err != nil {
			return "", err
		}
		rows = append(rows, value)
	}
	return strings.Join(rows, "\n"), nil
}

func renderJiraTableRow(ctx context.Context, cells []tableCell, delimiter string) (string, error) {
	values := make([]string, len(cells))
	for index, cell := range cells {
		value, err := renderJiraInlines(ctx, cell.Inlines, false)
		if err != nil {
			return "", err
		}
		values[index] = value
	}
	return delimiter + strings.Join(values, delimiter) + delimiter, nil
}

func renderJiraCodeBlock(ctx context.Context, block codeBlock) (string, error) {
	if block.NoFormat && block.Language == "" && !block.Directive {
		return "{noformat}\n" + block.Body + ensureLiteralClosingSeparation(block.Body) + "{noformat}", ctx.Err()
	}
	attributes := block.Attributes
	if block.Language != "" && !containsDirectiveAttribute(attributes, "language") {
		attributes = append([]directiveAttribute{{Name: "language", Value: block.Language}}, attributes...)
	}
	header := "{code}"
	if len(attributes) != 0 {
		parts := make([]string, 0, len(attributes))
		for _, attribute := range orderDirectiveAttributes(attributes, codeAttributeOrder()) {
			value, err := escapeJiraDelimitedValueWithContext(ctx, attribute.Value, `\{}|`)
			if err != nil {
				return "", err
			}
			parts = append(parts, attribute.Name+"="+value)
		}
		header = "{code:" + strings.Join(parts, "|") + "}"
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return header + "\n" + block.Body + ensureLiteralClosingSeparation(block.Body) + "{code}", nil
}
