package cmd

import (
	"fmt"

	"github.com/timonwong/jiro/internal/markup"
	"github.com/timonwong/jiro/internal/output"
)

type jfmConversionDirection struct {
	label string
	value string
}

var (
	jfmToJiraDirection = jfmConversionDirection{label: "JFM to Jira", value: "jfm_to_jira"}
	jiraToJFMDirection = jfmConversionDirection{label: "Jira to JFM", value: "jira_to_jfm"}
)

func jfmConversionWarnings(
	direction jfmConversionDirection,
	warnings []markup.ConversionWarning,
	extraDetails map[string]any,
) []output.Warning {
	converted := make([]output.Warning, len(warnings))
	for index, warning := range warnings {
		details := make(map[string]any, 5+len(extraDetails))
		details["direction"] = direction.value
		details["line"] = warning.Line
		details["column"] = warning.Column
		details["construct"] = warning.Construct
		details["reason"] = warning.Reason
		for key, value := range extraDetails {
			details[key] = value
		}
		converted[index] = output.Warning{
			Code: "jfm_conversion",
			Message: fmt.Sprintf(
				"%s conversion at line %d, column %d (%s): %s",
				direction.label, warning.Line, warning.Column, warning.Construct, warning.Reason,
			),
			Details: details,
		}
	}
	return converted
}
