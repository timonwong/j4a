# Preserve complete results on partial failure

When a composite command completes only part of its requested work, jiro writes the complete normalized result to stdout, writes a structured `partial_failure` error to stderr, and exits with code 7. This preserves every successful and failed item for agents while retaining a reliable non-zero process status; composite operations do not hide partial results inside an error or report success when work remains incomplete.
