# j4a Jira CLI

j4a provides a scriptable command-line interface for working with Jira Data Center and Server. Its language distinguishes Jira-owned identifiers and markup from the friendlier aliases and input formats accepted by the CLI.

## Language

**Jira Instance**:
A Jira Data Center or Server deployment addressed by one base URL.
_Avoid_: Site, server, host

**Profile**:
A named set of connection and authentication settings for one Jira Instance.
_Avoid_: Account, environment

**Issue Key**:
The human-readable Jira identifier for an issue, such as `PROJ-123`.
_Avoid_: Ticket ID, issue ID

**Custom Field ID**:
The Jira-owned, instance-specific identifier for a custom field, such as `customfield_10006`.
_Avoid_: Custom field key

**Custom Field Alias**:
A j4a-friendly slug derived from a custom field's display name, such as `story-points`.
_Avoid_: Custom Field ID, field name

**Jira Markup**:
Jira's wiki-style text notation accepted directly by Jira Data Center and Server rich-text fields.
_Avoid_: Jira Markdown, wiki Markdown

**Markdown Input**:
CommonMark-compatible text that j4a converts to Jira Markup only when explicitly requested.
_Avoid_: Jira Markup
