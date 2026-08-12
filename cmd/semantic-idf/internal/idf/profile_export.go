package idf

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
)

func ExportProfileGraphJSON(report ProfileReport) (string, error) {
	payload, err := json.MarshalIndent(report.GraphDataset, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(payload, '\n')), nil
}

func ExportProfileQAJSON(report ProfileReport) (string, error) {
	payload := struct {
		Outliers            []ProfileOutlierHint        `json:"outliers"`
		ParameterCandidates []ProfileParameterCandidate `json:"parameterCandidates"`
		Warnings            []ProfileWarning            `json:"warnings"`
	}{
		Outliers:            report.Outliers,
		ParameterCandidates: report.ParameterCandidates,
		Warnings:            report.Warnings,
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(content, '\n')), nil
}

func ExportProfileSchedulesJSON(report ProfileReport) (string, error) {
	payload := struct {
		Schedules []ScheduleSummary        `json:"schedules"`
		Clusters  []ProfileScheduleCluster `json:"clusters"`
	}{
		Schedules: report.Schedules,
		Clusters:  report.ScheduleClusters,
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(append(content, '\n')), nil
}

func ExportProfileQACSV(report ProfileReport) (string, error) {
	var b bytes.Buffer
	writer := csv.NewWriter(&b)
	_ = writer.Write([]string{"severity", "rule_id", "message", "zone", "group_id", "dimension", "schedule", "value", "median", "score"})
	for _, hint := range report.Outliers {
		_ = writer.Write([]string{
			hint.Severity,
			hint.RuleID,
			hint.Message,
			hint.ZoneName,
			hint.GroupID,
			hint.Dimension,
			hint.ScheduleName,
			formatSummaryNumber(hint.Value, 4),
			formatSummaryNumber(hint.Median, 4),
			formatSummaryNumber(hint.Score, 2),
		})
	}
	writer.Flush()
	return b.String(), writer.Error()
}
