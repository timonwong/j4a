---
status: accepted
---

# Render Issue details as man-style sections

`jiro issue show` renders text as ordered man-page-style sections instead of a `FIELD` / `VALUE` table. Every existing field remains visible, including empty fields; uppercase headers are bold only for an actual or explicitly forced TTY using `github.com/muesli/termenv`, while non-TTY headers contain no ANSI styling. Each non-empty physical value line is indented by four spaces, blank lines remain blank, values are neither wrapped nor truncated, and terminal-safety sanitization remains presentation-only; normalized JSON is unchanged.
