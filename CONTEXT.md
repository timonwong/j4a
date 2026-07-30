# j4a Jira CLI

j4a provides a scriptable command-line interface for working with Jira Data Center and Server. Its language distinguishes Jira-owned identifiers and markup from the friendlier aliases and input formats accepted by the CLI.

## Language

**Jira Instance**:
A Jira Data Center or Server deployment addressed by one base URL.
_Avoid_: Site, server, host

**Profile**:
A named set of connection and authentication settings for one Jira Instance. A Profile may own one persisted Credential.
_Avoid_: Account, environment

**Credential**:
A secret that proves a Profile may act as a Jira user, represented by either a Basic Auth password or a Personal Access Token.
_Avoid_: Session, account

**Login**:
The local operation that verifies a fresh Credential with a Jira Instance and associates it with a Profile. It does not create a persistent Jira server session.
_Avoid_: Session creation, browser login

**Logout**:
The local removal of a Profile's persisted Credential. It does not revoke a Jira Personal Access Token or unset credentials supplied by the environment.
_Avoid_: Token revocation, session expiry

**Issue Key**:
The human-readable Jira identifier for an issue, such as `PROJ-123`.
_Avoid_: Ticket ID, issue ID

**Custom Field ID**:
The Jira-owned, instance-specific identifier for a custom field, such as `customfield_10006`.
_Avoid_: Custom field key

**Custom Field Alias**:
A j4a-friendly slug derived from a custom field's display name, such as `story-points`.
_Avoid_: Custom Field ID, field name

**Principal**:
The Jira user identity authenticated for one operation, independent of the local Profile and Credential used to prove it.
_Avoid_: Profile, Credential, account

**Field Metadata Snapshot**:
A time-bounded copy of the custom fields visible to one Principal on one Jira Instance. It is disposable and is not the source of truth.
_Avoid_: Field configuration, field registry

**Jira Markup**:
Jira's wiki-style text notation accepted directly by Jira Data Center and Server rich-text fields.
_Avoid_: Jira Markdown, wiki Markdown

**Markdown Input**:
CommonMark-compatible text that j4a converts to Jira Markup only when explicitly requested.
_Avoid_: Jira Markup
