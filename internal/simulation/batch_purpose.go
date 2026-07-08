package simulation

import (
	"fmt"
	"os"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/internal/epinput"
	"github.com/Gonie-Gonie/semantic-idf/internal/idf"
)

func prepareBatchPurposeSimulationRequest(request SimulationRunRequest, purposeRequest SimulationPurposeRequest) (SimulationRunRequest, error) {
	text := request.Text
	if strings.TrimSpace(text) == "" && strings.TrimSpace(request.InputPath) != "" {
		content, err := os.ReadFile(request.InputPath)
		if err != nil {
			return request, err
		}
		text = string(content)
	}
	if strings.TrimSpace(text) == "" {
		return request, fmt.Errorf("purpose batch run needs input text or an input path")
	}
	model, err := epinput.Parse(request.InputPath, []byte(text))
	if err != nil {
		return request, err
	}
	doc := epinput.ToIDFDocument(model)
	normalized := NormalizeSimulationPurposeRequest(&purposeRequest)
	plan := BuildPurposeRunPlan(doc, normalized)
	updated, preview := idf.ApplyOutput(doc, PurposeRunPlanApplyRequest(plan, PurposeOutputApplyModeKeepExistingAdd))
	if !preview.CanApply {
		return request, fmt.Errorf("purpose output plan has blocking warnings")
	}
	request.Text = writeBatchPurposeDocument(updated, model)
	request.PurposeRequest = &normalized
	request.PurposeRunPlan = &plan
	request.TemporaryOutputDiff = PurposeRunPlanTemporaryOutputDiff(plan)
	request.ResultMode = PurposeSQLModeSQLFirst
	return request, nil
}

func writeBatchPurposeDocument(doc idf.Document, original *epinput.Model) string {
	if original != nil && original.Format == epinput.FormatEPJSON {
		model := epinput.FromIDFDocument(doc, epinput.FormatEPJSON)
		output, err := epinput.Write(model, epinput.FormatEPJSON)
		if err == nil {
			return output
		}
	}
	return doc.String()
}

func SummarizePurposeMetrics(bundle *PurposeResultBundle) []PurposeMetric {
	if bundle == nil {
		return nil
	}
	var metrics []PurposeMetric
	for _, total := range bundle.Energy.Totals {
		metrics = append(metrics, PurposeMetric{
			ID:           "energy." + metricID(total.Name),
			Label:        total.Name,
			PurposeID:    SimulationPurposeBasicEnergy,
			Value:        total.Value,
			Unit:         total.Unit,
			DisplayValue: formatPurposeMetric(total.Value, total.Unit),
			Status:       "ok",
		})
	}
	metrics = append(metrics, energyExplanationPurposeMetrics(bundle.EnergyExplanation)...)
	if bundle.Integrity.Completed || bundle.Integrity.Status != "" {
		metrics = append(metrics,
			PurposeMetric{
				ID:           "integrity.err_warnings",
				Label:        "ERR warnings",
				PurposeID:    SimulationPurposeIntegrity,
				Value:        float64(bundle.Integrity.ERR.Warnings),
				DisplayValue: fmt.Sprintf("%d", bundle.Integrity.ERR.Warnings),
				Status:       "ok",
			},
			PurposeMetric{
				ID:           "integrity.err_severe_fatal",
				Label:        "ERR severe/fatal",
				PurposeID:    SimulationPurposeIntegrity,
				Value:        float64(bundle.Integrity.ERR.Severe + bundle.Integrity.ERR.Fatal),
				DisplayValue: fmt.Sprintf("%d", bundle.Integrity.ERR.Severe+bundle.Integrity.ERR.Fatal),
				Status:       "ok",
			},
			PurposeMetric{
				ID:           "integrity.missing_outputs",
				Label:        "Missing purpose outputs",
				PurposeID:    SimulationPurposeIntegrity,
				Value:        float64(missingCompletenessCount(bundle.Completeness) + missingCompletenessCount(bundle.Energy.Completeness) + missingCompletenessCount(bundle.ZoneHeatFlow.Completeness)),
				DisplayValue: fmt.Sprintf("%d", missingCompletenessCount(bundle.Completeness)+missingCompletenessCount(bundle.Energy.Completeness)+missingCompletenessCount(bundle.ZoneHeatFlow.Completeness)),
				Status:       "ok",
			},
		)
	}
	for _, loop := range bundle.HVACLoops {
		alertCount := len(loop.Alerts)
		metrics = append(metrics, PurposeMetric{
			ID:           "hvac." + metricID(loop.LoopType+"."+loop.Name+".alerts"),
			Label:        loop.Name + " alerts",
			PurposeID:    SimulationPurposeHVACLoopCheck,
			Value:        float64(alertCount),
			DisplayValue: fmt.Sprintf("%d", alertCount),
			Status:       loop.Status,
		})
		for _, metric := range loop.DerivedMetrics {
			metrics = append(metrics, PurposeMetric{
				ID:           "hvac." + metricID(loop.LoopType+"."+loop.Name+"."+metric.Name),
				Label:        loop.Name + " " + metric.Name,
				PurposeID:    SimulationPurposeHVACLoopCheck,
				Value:        metric.Value,
				Unit:         metric.Unit,
				DisplayValue: formatPurposeMetric(metric.Value, metric.Unit),
				Status:       metric.Status,
			})
		}
	}
	return metrics
}

func energyExplanationPurposeMetrics(explanation EnergyExplanationResult) []PurposeMetric {
	if explanation.Schema == "" || len(explanation.Nodes) == 0 {
		return nil
	}
	summary := buildEnergyExplanationSummary(explanation)
	if summary.Schema == "" {
		return nil
	}
	metrics := []PurposeMetric{}
	metrics = appendEnergyExplanationSummaryTotalMetric(metrics, "energy_use", "Energy explanation use", summary.EnergyByCarrier)
	metrics = appendEnergyExplanationSummaryTotalMetric(metrics, "delivered_load", "Delivered load total", summary.DeliveredLoadByService)
	metrics = appendEnergyExplanationSummaryTotalMetric(metrics, "heat_drivers", "Heat driver total", summary.HeatDrivers)
	metrics = appendEnergyExplanationSummaryTotalMetric(metrics, "residual", "Energy explanation residual", summary.Residuals)
	if explanation.Completeness.MappedPercent > 0 {
		metrics = append(metrics, PurposeMetric{
			ID:           "energy_explanation.mapped_percent",
			Label:        "Mapped energy percent",
			PurposeID:    SimulationPurposeBasicEnergy,
			Value:        explanation.Completeness.MappedPercent,
			Unit:         "%",
			DisplayValue: formatPurposeMetric(explanation.Completeness.MappedPercent, "%"),
			Status:       explanation.Completeness.Status,
		})
	}
	for _, item := range summary.TopHeatDrivers {
		if item.Value == 0 {
			continue
		}
		label := strings.TrimSpace("Heat driver: " + firstNonEmpty(item.Label, item.Kind, item.ID))
		metrics = append(metrics, PurposeMetric{
			ID:           "energy_explanation.heat." + metricID(item.ID),
			Label:        label,
			PurposeID:    SimulationPurposeBasicEnergy,
			Value:        item.Value,
			Unit:         item.Unit,
			DisplayValue: formatPurposeMetric(item.Value, item.Unit),
			Status:       "ok",
		})
	}
	return metrics
}

func appendEnergyExplanationSummaryTotalMetric(metrics []PurposeMetric, id string, label string, items []EnergyExplanationSummaryItem) []PurposeMetric {
	value, unit := energyExplanationSummaryItemsTotal(items)
	return appendEnergyExplanationTotalMetric(metrics, id, label, value, unit)
}

func energyExplanationSummaryItemsTotal(items []EnergyExplanationSummaryItem) (float64, string) {
	total := 0.0
	unit := "kWh"
	for _, item := range items {
		total += item.Value
		if item.Unit != "" {
			unit = item.Unit
		}
	}
	return roundedEnergyNumber(total), unit
}

func appendEnergyExplanationTotalMetric(metrics []PurposeMetric, id string, label string, value float64, unit string) []PurposeMetric {
	if value == 0 {
		return metrics
	}
	return append(metrics, PurposeMetric{
		ID:           "energy_explanation." + id,
		Label:        label,
		PurposeID:    SimulationPurposeBasicEnergy,
		Value:        value,
		Unit:         unit,
		DisplayValue: formatPurposeMetric(value, unit),
		Status:       "ok",
	})
}

func missingCompletenessCount(items []PurposeCompletenessItem) int {
	count := 0
	for _, item := range items {
		if !item.Found {
			count++
		}
	}
	return count
}

func metricID(value string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func formatPurposeMetric(value float64, unit string) string {
	if strings.TrimSpace(unit) == "" {
		return fmt.Sprintf("%.3g", value)
	}
	return strings.TrimSpace(fmt.Sprintf("%.3g %s", value, unit))
}
