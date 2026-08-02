package markup

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
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
	fence := strings.Repeat("`", max(3, run+1))
	opening := fence
	if block.Language != "" {
		opening += block.Language
	}
	return opening + "\n" + block.Body + ensureLiteralClosingSeparation(block.Body) + fence, nil
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
