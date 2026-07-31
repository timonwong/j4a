# Target Jira Data Center first

jiro v1 treats Jira Data Center and Server through REST API v2 as the canonical platform because username/password Basic Auth and Jira wiki markup are explicit requirements. Cloud REST API v3 and ADF are deferred compatibility targets so the first release can provide a coherent auth, text, and testing contract without maintaining two materially different payload models.
