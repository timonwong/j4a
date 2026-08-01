package markup

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode"
)

func renderJFM(ctx context.Context, document semanticDocument) (string, error) {
	blocks := make([]string, 0, len(document.Blocks))
	for _, block := range document.Blocks {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		switch typed := block.(type) {
		case headingBlock:
			content, err := renderJFMInlines(ctx, typed.Inlines, true)
			if err != nil {
				return "", err
			}
			heading := strings.Repeat("#", typed.Level)
			if content != "" {
				heading += " " + content
			}
			blocks = append(blocks, heading)
		case paragraphBlock:
			content, err := renderJFMInlines(ctx, typed.Inlines, true)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		case thematicBreakBlock:
			blocks = append(blocks, "---")
		case quoteBlock:
			paragraphs := make([]string, 0, len(typed.Paragraphs))
			for _, paragraph := range typed.Paragraphs {
				content, err := renderJFMInlines(ctx, paragraph.Inlines, true)
				if err != nil {
					return "", err
				}
				paragraphs = append(paragraphs, prefixQuotedLines(content))
			}
			blocks = append(blocks, strings.Join(paragraphs, "\n>\n"))
		case listBlock:
			content, err := renderJFMList(ctx, typed, 0)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		case tableBlock:
			content, err := renderJFMTable(ctx, typed)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		case codeBlock:
			content, err := renderJFMCodeBlock(ctx, typed)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		case panelBlock:
			body, err := renderJFM(ctx, semanticDocument{Blocks: typed.Blocks})
			if err != nil {
				return "", err
			}
			fence, err := safeContainerFence(ctx, body)
			if err != nil {
				return "", err
			}
			header := fence + "panel"
			if len(typed.Attributes) != 0 {
				serialized, err := serializeDirectiveAttributes(ctx, typed.Attributes, panelAttributeOrder())
				if err != nil {
					return "", err
				}
				header += "{" + serialized + "}"
			}
			blocks = append(blocks, header+"\n"+body+ensureLiteralClosingSeparation(body)+fence)
		case unsupportedMacroBlock:
			body, err := renderJFM(ctx, semanticDocument{Blocks: typed.Blocks})
			if err != nil {
				return "", err
			}
			blocks = append(blocks, typed.Opening+"\n"+body+ensureLiteralClosingSeparation(body)+typed.Closing)
		case literalBlock:
			content, err := escapeTextForJFM(ctx, typed.Text, true)
			if err != nil {
				return "", err
			}
			blocks = append(blocks, content)
		default:
			return "", fmt.Errorf("%w: unsupported semantic block in JFM renderer", ErrConversion)
		}
	}
	return strings.TrimSpace(strings.Join(blocks, "\n\n")), nil
}

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

func renderJFMInlines(ctx context.Context, inlines []semanticInline, atLineStart bool) (string, error) {
	var result strings.Builder
	for _, inline := range inlines {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		switch typed := inline.(type) {
		case textInline:
			content, err := escapeTextForJFM(ctx, typed.Text, atLineStart)
			if err != nil {
				return "", err
			}
			result.WriteString(content)
		case codeInline:
			content, err := renderJFMCodeSpan(ctx, typed.Text)
			if err != nil {
				return "", err
			}
			result.WriteString(content)
		case hardBreakInline:
			result.WriteString("\\\n")
		case styledInline:
			content, err := renderJFMInlines(ctx, typed.Children, false)
			if err != nil {
				return "", err
			}
			if combined, ok := combinedBoldItalic(typed); ok {
				content, err = renderJFMInlines(ctx, combined, false)
				if err != nil {
					return "", err
				}
				result.WriteString("***" + content + "***")
				break
			}
			switch typed.Style {
			case styleBold:
				result.WriteString("**" + content + "**")
			case styleItalic:
				result.WriteString("*" + content + "*")
			case styleStrike:
				result.WriteString("~~" + content + "~~")
			case styleInserted:
				result.WriteString("<ins>" + content + "</ins>")
			case styleSuper:
				result.WriteString("<sup>" + content + "</sup>")
			case styleSub:
				result.WriteString("<sub>" + content + "</sub>")
			case styleColor:
				result.WriteString(`<font color="` + html.EscapeString(typed.Value) + `">` + content + `</font>`)
			}
		case linkInline:
			label, err := renderJFMInlines(ctx, typed.Label, false)
			if err != nil {
				return "", err
			}
			if typed.Directive || typed.Dangerous {
				content, err := escapeDirectiveContent(ctx, label)
				if err != nil {
					return "", err
				}
				target, err := quoteDirectiveAttributeValue(ctx, typed.Target)
				if err != nil {
					return "", err
				}
				result.WriteString(":link[" + content + "]{target=" + target + "}")
				break
			}
			lowerTarget := strings.ToLower(typed.Target)
			if typed.Unnamed && (strings.HasPrefix(lowerTarget, "http://") || strings.HasPrefix(lowerTarget, "https://")) {
				result.WriteString("<" + typed.Target + ">")
			} else if typed.Unnamed && strings.HasPrefix(lowerTarget, "mailto:") {
				result.WriteString("<" + strings.TrimPrefix(typed.Target, typed.Target[:len("mailto:")]) + ">")
			} else {
				target, err := escapeMarkdownDestination(ctx, typed.Target)
				if err != nil {
					return "", err
				}
				result.WriteString("[" + label + "](" + target + ")")
			}
		case imageInline:
			if !typed.Directive && !typed.Dangerous {
				alt, err := escapeMarkdownLabelText(ctx, typed.Alt)
				if err != nil {
					return "", err
				}
				source, err := escapeMarkdownDestination(ctx, typed.Source)
				if err != nil {
					return "", err
				}
				result.WriteString("![" + alt + "](" + source + ")")
				break
			}
			attributes := []directiveAttribute{{Name: "src", Value: typed.Source}}
			for _, attribute := range typed.Attributes {
				attributes = append(attributes, attribute)
			}
			alt, err := escapeDirectiveContent(ctx, typed.Alt)
			if err != nil {
				return "", err
			}
			serialized, err := serializeDirectiveAttributes(ctx, attributes, imageAttributeOrder())
			if err != nil {
				return "", err
			}
			result.WriteString(":image[" + alt + "]{" + serialized + "}")
		case literalInline:
			content, err := escapeTextForJFM(ctx, typed.Text, atLineStart)
			if err != nil {
				return "", err
			}
			result.WriteString(content)
		default:
			return "", fmt.Errorf("%w: unsupported semantic inline in JFM renderer", ErrConversion)
		}
		atLineStart = inlineEndsAtLineStart(inline)
	}
	return result.String(), nil
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

func combinedBoldItalic(inline styledInline) ([]semanticInline, bool) {
	if len(inline.Children) != 1 || inline.Style != styleBold && inline.Style != styleItalic {
		return nil, false
	}
	nested, ok := inline.Children[0].(styledInline)
	if !ok || inline.Style == nested.Style || nested.Style != styleBold && nested.Style != styleItalic {
		return nil, false
	}
	return nested.Children, true
}

func renderJFMCodeSpan(ctx context.Context, value string) (string, error) {
	run, err := longestRunWithContext(ctx, value, '`')
	if err != nil {
		return "", err
	}
	delimiter := strings.Repeat("`", run+1)
	if delimiter == "" {
		delimiter = "`"
	}
	padding := ""
	if value != "" && (value[0] == ' ' || value[len(value)-1] == ' ' || value[0] == '`' || value[len(value)-1] == '`') && strings.Trim(value, " ") != "" {
		padding = " "
	}
	return delimiter + padding + value + padding + delimiter, nil
}

func escapeTextForJFM(ctx context.Context, value string, atLineStart bool) (string, error) {
	var result strings.Builder
	for index := 0; index < len(value); {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		character, size := utf8DecodeRune(value[index:])
		if character == '\\' || strings.ContainsRune("*_~[]`<>", character) {
			result.WriteByte('\\')
		}
		if atLineStart && strings.ContainsRune("#>+-", character) {
			result.WriteByte('\\')
		}
		result.WriteRune(character)
		atLineStart = character == '\n'
		index += size
	}
	if err := ctx.Err(); err != nil {
		return "", err
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

func utf8DecodeRune(value string) (rune, int) {
	for _, character := range value {
		return character, len(string(character))
	}
	return unicode.ReplacementChar, 1
}

func escapeMarkdownDestination(ctx context.Context, value string) (string, error) {
	var result strings.Builder
	for offset, character := range value {
		if offset&255 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		if strings.ContainsRune(`\()<> `, character) {
			result.WriteByte('\\')
		}
		result.WriteRune(character)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return result.String(), nil
}

func escapeMarkdownDestinationUnchecked(value string) string {
	result, _ := escapeMarkdownDestination(context.Background(), value)
	return result
}

func escapeMarkdownLabelText(ctx context.Context, value string) (string, error) {
	return escapeSelectedRunes(ctx, value, `\[]`)
}

func escapeDirectiveContent(ctx context.Context, value string) (string, error) {
	return escapeSelectedRunes(ctx, value, `\]`)
}

func imageAttributeOrder() []string {
	return []string{"src", "thumbnail", "align", "border", "bordercolor", "hspace", "vspace", "width", "height", "title"}
}

func serializeDirectiveAttributes(ctx context.Context, attributes []directiveAttribute, order []string) (string, error) {
	ordered := orderDirectiveAttributes(attributes, order)
	parts := make([]string, 0, len(ordered))
	for index, attribute := range ordered {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		if attribute.Bare {
			parts = append(parts, attribute.Name)
		} else {
			value, err := quoteDirectiveAttributeValue(ctx, attribute.Value)
			if err != nil {
				return "", err
			}
			parts = append(parts, attribute.Name+"="+value)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return strings.Join(parts, " "), nil
}

func escapeSelectedRunes(ctx context.Context, value, selected string) (string, error) {
	var result strings.Builder
	for offset, character := range value {
		if offset&255 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		if strings.ContainsRune(selected, character) {
			result.WriteByte('\\')
		}
		result.WriteRune(character)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return result.String(), nil
}

func quoteDirectiveAttributeValue(ctx context.Context, value string) (string, error) {
	var result strings.Builder
	result.WriteByte('"')
	for offset, character := range value {
		if offset&255 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		quoted := strconv.Quote(string(character))
		result.WriteString(quoted[1 : len(quoted)-1])
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	result.WriteByte('"')
	return result.String(), nil
}

func escapeJiraDelimitedValueWithContext(ctx context.Context, value, delimiters string) (string, error) {
	return escapeSelectedRunes(ctx, value, delimiters)
}

func orderDirectiveAttributes(attributes []directiveAttribute, order []string) []directiveAttribute {
	result := make([]directiveAttribute, 0, len(attributes))
	used := make([]bool, len(attributes))
	for _, name := range order {
		for index, attribute := range attributes {
			if !used[index] && strings.EqualFold(attribute.Name, name) {
				attribute.Name = name
				result = append(result, attribute)
				used[index] = true
			}
		}
	}
	for index, attribute := range attributes {
		if !used[index] {
			result = append(result, attribute)
		}
	}
	return result
}

func prefixQuotedLines(value string) string {
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if line == "" {
			lines[index] = ">"
		} else {
			lines[index] = "> " + line
		}
	}
	return strings.Join(lines, "\n")
}

func renderJFMList(ctx context.Context, list listBlock, depth int) (string, error) {
	lines := make([]string, 0, len(list.Items))
	indent := strings.Repeat(" ", depth*4)
	marker := "-"
	if list.Ordered {
		marker = "1."
	}
	for _, item := range list.Items {
		content, err := renderJFMInlines(ctx, item.Inlines, false)
		if err != nil {
			return "", err
		}
		line := indent + marker
		if content != "" {
			line += " " + content
		}
		lines = append(lines, line)
		for _, child := range item.Children {
			childText, err := renderJFMList(ctx, child, depth+1)
			if err != nil {
				return "", err
			}
			lines = append(lines, childText)
		}
	}
	return strings.Join(lines, "\n"), nil
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

func renderJFMTable(ctx context.Context, table tableBlock) (string, error) {
	if table.Directive || len(table.Header) == 0 {
		return ":::table\n" + table.Raw + "\n:::", nil
	}
	rows := make([]string, 0, len(table.Rows)+2)
	header, err := renderJFMTableRow(ctx, table.Header)
	if err != nil {
		return "", err
	}
	rows = append(rows, header)
	separator := make([]string, len(table.Header))
	for index := range separator {
		separator[index] = "---"
	}
	rows = append(rows, "| "+strings.Join(separator, " | ")+" |")
	for _, row := range table.Rows {
		value, err := renderJFMTableRow(ctx, row)
		if err != nil {
			return "", err
		}
		rows = append(rows, value)
	}
	return strings.Join(rows, "\n"), nil
}

func renderJFMTableRow(ctx context.Context, cells []tableCell) (string, error) {
	values := make([]string, len(cells))
	for index, cell := range cells {
		value, err := renderJFMInlines(ctx, cell.Inlines, false)
		if err != nil {
			return "", err
		}
		values[index] = strings.ReplaceAll(value, "|", "\\|")
	}
	return "| " + strings.Join(values, " | ") + " |", nil
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

func renderJFMCodeBlock(ctx context.Context, block codeBlock) (string, error) {
	if block.Directive {
		attributes := block.Attributes
		if block.Language != "" && !containsDirectiveAttribute(attributes, "language") {
			attributes = append([]directiveAttribute{{Name: "language", Value: block.Language}}, attributes...)
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		serialized, err := serializeCodeDirectiveAttributes(ctx, attributes)
		if err != nil {
			return "", err
		}
		return ":::code{" + serialized + "}\n" + block.Body + ensureLiteralClosingSeparation(block.Body) + ":::", nil
	}
	run, err := longestRunWithContext(ctx, block.Body, '`')
	if err != nil {
		return "", err
	}
	fence := strings.Repeat("`", maxInt(3, run+1))
	opening := fence
	if block.Language != "" {
		opening += block.Language
	}
	return opening + "\n" + block.Body + ensureLiteralClosingSeparation(block.Body) + fence, nil
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

func ensureLiteralClosingSeparation(body string) string {
	if body == "" || strings.HasSuffix(body, "\n") || strings.HasSuffix(body, "\r") {
		return ""
	}
	return "\n"
}

func serializeCodeDirectiveAttributes(ctx context.Context, attributes []directiveAttribute) (string, error) {
	ordered := orderDirectiveAttributes(attributes, codeAttributeOrder())
	parts := make([]string, 0, len(ordered))
	for _, attribute := range ordered {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if strings.EqualFold(attribute.Name, "collapse") || strings.EqualFold(attribute.Name, "linenumbers") {
			if value := strings.ToLower(attribute.Value); value == "true" || value == "false" {
				parts = append(parts, attribute.Name+"="+value)
				continue
			}
		}
		value, err := quoteDirectiveAttributeValue(ctx, attribute.Value)
		if err != nil {
			return "", err
		}
		parts = append(parts, attribute.Name+"="+value)
	}
	return strings.Join(parts, " "), ctx.Err()
}

func containsDirectiveAttribute(attributes []directiveAttribute, name string) bool {
	for _, attribute := range attributes {
		if strings.EqualFold(attribute.Name, name) {
			return true
		}
	}
	return false
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func inlineEndsAtLineStart(inline semanticInline) bool {
	switch typed := inline.(type) {
	case hardBreakInline:
		return true
	case textInline:
		return strings.HasSuffix(typed.Text, "\n")
	case literalInline:
		return strings.HasSuffix(typed.Text, "\n")
	default:
		return false
	}
}

func safeContainerFence(ctx context.Context, body string) (string, error) {
	longest := 2
	atLinePrefix := true
	current := 0
	for index := 0; index < len(body); index++ {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
		if atLinePrefix {
			if body[index] == ':' {
				current++
				continue
			}
			if current > longest {
				longest = current
			}
			atLinePrefix = false
		}
		if body[index] == '\n' {
			atLinePrefix = true
			current = 0
		}
	}
	if current > longest {
		longest = current
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return strings.Repeat(":", longest+1), nil
}

func longestRunWithContext(ctx context.Context, value string, target byte) (int, error) {
	longest, current := 0, 0
	for index := 0; index < len(value); index++ {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		if value[index] == target {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	return longest, ctx.Err()
}
