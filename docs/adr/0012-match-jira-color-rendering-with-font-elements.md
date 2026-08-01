# Match Jira color rendering with font elements

Jiro Flavored Markdown renders Jira `{color:value}` notation as `<font color="value">...</font>` instead of dropping the color or modernizing it to a CSS-styled span. This deliberately matches Jira Data Center and Server's own HTML rendering shape: non-empty values pass through without a whitelist after HTML attribute escaping, while empty or malformed color macros remain visible and produce structured warnings.
