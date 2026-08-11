package frontendchecks

import (
	"strings"
	"testing"
)

func TestFrontendSimulationEnergySystemsCrossJumpContracts(t *testing.T) {
	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	if strings.Contains(strings.ToLower(simulation), "confidence") {
		t.Fatalf("simulation energy renderer should describe basis without confidence vocabulary")
	}
	for _, term := range []string{
		`["systems", t("simulation.systems"`,
		"renderEnergySystemsSubview",
		"simulationEnergyServiceAggregates",
		"renderSimulationEnergyConnectedSystems",
		"renderSimulationEnergySupportingAssets",
		"simulationServicePathLoopRefs",
		"simulationServicePathSupportingAssets",
		"simulationServicePathSupportingAssetRefs",
		"simulation.sourceEnergy",
		"data-simulation-hvac-path-id",
		"data-simulation-hvac-loop-name",
		"data-simulation-hvac-coupling-id",
		"openSimulationHVACLoopRef",
		"openSimulationHVACCoupling",
		"simulationHVACLoopRefGraphKey",
		"simulationRelatedServicePathsForEnergySelection",
		"simulationHVACServicePathsByIDs",
		"renderSimulationEnergyRelatedHVACLinks",
		"simulation.energyHVACJumps",
		"simulation.openLoopInHVAC",
		"simulation.openAssetInHVAC",
		"relatedPathIds",
		"focusedEnergyExplanationGraph",
		"data-simulation-energy-focus-mode",
		"data-simulation-energy-service-path-focus",
		"data-simulation-energy-loop-focus",
		"data-simulation-energy-loop-focus-jump",
		"data-simulation-energy-service-path-jump",
		"simulationEnergyLoopFocusOptions",
		"simulationEnergyServicePathsForLoopFocus",
		"renderSimulationEnergyLoopFocusButton",
		"simulationEnergyNodeMatchesServicePaths",
		"state.simulationEnergyLoopFocus",
		"data-simulation-energy-sankey-mode",
		"data-simulation-energy-sign-mode",
		"energyExplanationSignModeGraph",
		"cooling_pressure",
		"heating_pressure",
		"categoryGroupedEnergyExplanationGraph",
		"energyHeatCategoryNodeLabel",
		"heat.category.",
		"simulation.energySankeyCategoryGrouped",
		"heatCategoryGroupedCount",
		"groupedEnergyExplanationGraph",
		"data-simulation-energy-node-limit",
		"data-simulation-energy-show-all-nodes",
		"renderEnergyExplanationGroupingNotice",
		"otherRelatedPathIDs",
		"omittedHeatNodeByID",
		"existing.relatedPathIds",
		"renderEnergySignConventionNote",
		"renderEnergySankeyModeControls",
		"energyExplanationSankeyMode",
		"energyExplanationSankeyColumnConfig",
		"energyExplanationSankeyDisplayGraph",
		"simulation.energySankeyMode",
		"simulation.energySankeyGrouped",
		"simulation.energySankeyGroupedAllHeat",
		"simulation.energySignConvention",
		"maxDefaultNodes = 100",
		"maxDefaultNodes - nonHeatCount - 1",
		"heat.other_grouped",
		"otherSignedValue",
		"otherDisplayValue",
		`sign: otherSigns.length === 1 ? otherSigns[0] : "mixed"`,
		"energyAllocationPolicyLabel",
		"energy-source-availability",
		"simulation-source-output-jump",
		"data-jump-object-index",
		"sourceOutputForEnergySource",
		"findPurposeOutputObjectByIndex",
		"energyMeterOutputKeysMatch",
		"energyMeterAliasGroupKey",
		"electricityproducedfacility",
		"renderEnergySourceSeriesInspectButton",
		"energySourceSeriesRef",
		"data-simulation-series-meter",
		"findSimulationSeriesForMeter",
		"source.objectIndex",
		"source.aggregationMethod",
		"source.sourceUnit",
		"source.normalizedUnit",
		"source.tableName",
		"source.rowName",
		"source.columnName",
		"ruleId",
		"relationshipRules",
		"relationshipRule",
		"energyExplanationRelationshipRuleLabel",
		"simulation.relatedPathIds",
		"simulation.missingSourceMetadata",
		"selection.relation",
		"selection.pathType",
		"state.simulationEnergySelection === edge.id",
		"energyExplanationEdgeClassTokens",
		"connectedNodeIDs.has(node.id)",
		"energyExplanationNodeClassTokens",
		"data-simulation-energy-period-jump",
		"data-simulation-energy-period-kind",
		"data-simulation-energy-period-index",
		"renderEnergyPeriodControls",
		"energyExplanationPeriodKinds",
		"energyExplanationPeriodKindLabel",
		"energyPointPeriodID",
		"simulation-energy-chart-period",
		"renderPurposeHTMLEnergyExplanation",
		"purposeHTMLEnergySummaryRows",
		"purposeHTMLAnnualEnergyGraph",
		"Energy Explanation Annual Nodes",
		"Energy Explanation Annual Edges",
		"edge.fromId",
		"edge.toId",
		"Energy Explanation Reconciliation",
		"Energy Explanation Sources",
		"Energy Explanation Source Availability",
		"Energy Explanation Relationship Rules",
		"Energy Explanation Warnings",
		"Energy Explanation Monthly Ledger",
		"purposeHTMLSourceMetadataFields",
		"purposeHTMLSourceValueSummary",
		"purposeHTMLEnergyMonthlyRows",
		"purposeHTMLEnergyWarningRows",
		"period.warnings",
		"Source IDs",
		"Output Object",
		"Related Paths",
		"node.relatedPathIds",
		"edge.relatedPathIds",
		"completeness.sourceAvailability",
		"explanation.relationshipRules",
		"renderEnergyDerivedKPISection",
		"energyExplanationDerivedKPIItems",
		"energyExplanationGraphDerivedKPIItems",
		"explanationSummary.derivedKpis",
		"formatOptionalValueWithUnit",
		"firstPositiveNumber",
		"item.numeratorValue",
		"item.denominatorValue",
		"renderEnergyUseBreakdownSection",
		"renderSimulationEnergyRelatedZones",
		"simulationRelatedZoneNamesForEnergySelection",
		"renderSimulationEnergyServicePathFocusButton",
		"energyUseTotalBasisNote",
		"energyMeterHierarchyLabel",
		"simulation.energyUseBreakdown",
		"simulation.energyUseTotalBasis",
		"renderEnergyExplanationMonthlyLevelChart",
		"simulation.energyExplanationMonthlyLevels",
		"renderEnergyZoneBreakdownSection",
		"energyZoneBreakdownRows",
		"data-simulation-energy-zone-jump",
		"data-simulation-energy-heatflow-zone-jump",
		"data-simulation-energy-profile-zone-jump",
		"openSimulationProfileZone",
		"simulationProfileZoneName",
		"basicEnergyDetail",
		"basicEnergyDetailLabel",
		"currentBasicEnergyDetail",
		"hasDetailTierGap",
		"simulation.energyDetailTier",
		"simulation.basicEnergyDetail",
		"simulation.energyDetailTierHint",
		"simulation.energyOutputShortageHint",
		"simulation.energyAccountingCoverageHint",
		"renderEnergySourceAvailabilitySummary",
		"result.purposeResults?.zoneHeatFlow",
		"simulation.energyZoneBreakdown",
		"simulation.openServicePathInSankey",
		"simulation.openZoneInSankey",
		"simulation.openZoneHeatFlow",
		"simulation.relation",
		"selection.meterHierarchyLevel",
		"meterHierarchy",
		"missingCategories",
		"renderEnergyReconciliationSources",
		"energy-reconciliation-sources",
		"energyReconciliationStatus",
		"energyReconciliationStatusLabel",
		"energy-reconciliation-status",
		"renderEnergyZoneResidualRanking",
		"zoneHeatResidualRanking",
		"item.zoneName",
		"item.serviceKind",
		"renderSourceOutputCell(object, { compact: true })",
		"navigateHVAC(",
	} {
		if !strings.Contains(simulation, term) {
			t.Fatalf("simulation energy systems contract missing %q", term)
		}
	}
	hvac := readTestFile(t, "frontend/src/js/views/hvac-views.js")
	if !strings.Contains(hvac, "export function navigateHVAC") {
		t.Fatalf("hvac navigation should remain exportable for simulation energy cross-jumps")
	}
	indexHTML := readTestFile(t, "frontend/src/index.html")
	for _, removed := range []string{
		`id="simulationPurposeAllocationPolicy"`,
		`id="simulationPurposeApplyMode"`,
		`id="simulationPurposeFrequencyPolicy"`,
		`id="simulationPurposeZoneHeatFlowDetail"`,
		`id="simulationPurposeEnergyDetail"`,
		`id="simulationPurposePeriodMode"`,
		`id="simulationPurposeZoneMode"`,
	} {
		if strings.Contains(indexHTML, removed) {
			t.Fatalf("simplified Simulation setup still exposes %q", removed)
		}
	}
	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, term := range []string{".energy-related-zones", ".energy-related-zone-chip", ".energy-related-service-paths", ".energy-related-hvac-links", ".energy-service-path-chip", ".energy-service-path-action-row", ".simulation-energy-system-links", ".simulation-energy-system-chip", ".energy-explanation-drilldown-actions", ".energy-use-total-basis", ".simulation-energy-focus-controls", ".simulation-energy-period-row", ".simulation-energy-period-slider", ".simulation-energy-zone-paths", ".simulation-energy-zone-actions", ".simulation-energy-chart-period", ".energy-explanation-output-actions", ".energy-source-availability-summary", ".energy-source-availability", ".energy-source-availability-status.missing", ".energy-source-availability-status.not_applicable", ".simulation-source-output-jump", ".energy-reconciliation-sources", ".energy-reconciliation-status", ".energy-sankey-grouping-notice", ".energy-sankey-sign-note", ".energy-sankey-edge.measured_meter", ".energy-sankey-edge.measured_energy_variable", ".energy-sankey-edge.integrated_rate_variable", ".energy-sankey-edge.selected", ".energy-sankey-node.connected", ".energy-sankey-node.electricity", ".energy-sankey-node.district_cooling", ".energy-sankey-node.fans", ".energy-sankey-node.pumps", ".energy-sankey-node.heat_recovery", ".energy-sankey-node.water_systems", ".energy-sankey-node.refrigeration", ".energy-sankey-node.generators", ".energy-sankey-node.storage_charge", ".energy-sankey-node.storage_discharge", ".energy-sankey-node.other", ".energy-sankey-legend i.node", ".energy-sankey-legend i.measured_meter", ".energy-sankey-legend i.measured_energy_variable", ".energy-sankey-legend i.integrated_rate_variable"} {
		if !strings.Contains(styles, term) {
			t.Fatalf("simulation energy cross-jump style missing %q", term)
		}
	}
	if !strings.Contains(simulation, "function energyEndUseLabel") || !strings.Contains(simulation, "energyEndUseGenerators") || !strings.Contains(simulation, "energyEndUseStorageCharge") {
		t.Fatalf("simulation energy end-use label mapping is missing")
	}
	if !strings.Contains(simulation, "function energyExplanationBasisLabel") || !strings.Contains(simulation, "basisMeasuredEnergyVariable") || !strings.Contains(simulation, "basisIntegratedRateVariable") {
		t.Fatalf("simulation energy basis label mapping is missing")
	}
	if !strings.Contains(simulation, "function renderSimulationEnergyDrilldownActions") || !strings.Contains(simulation, "data-simulation-energy-heatflow-zone-jump") {
		t.Fatalf("simulation energy drilldown action mapping is missing")
	}
}

func TestFrontendSimulationUsesSimplifiedDefaultsAndAutomaticEnergyPlus(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	for _, removed := range []string{
		`data-simulation-purpose="integrity_check"`,
		`data-simulation-purpose="custom_outputs"`,
		`data-simulation-result-view-button="integrity"`,
		`data-simulation-result-view="integrity"`,
		`id="simulationIntegrityFilter"`,
		`id="simulationCustomOutputs"`,
		`id="simulationOutputDiscoveryFilter"`,
		`id="simulationOutputDiscoveryRefresh"`,
		`id="simulationOutputDiscoveryStats"`,
		`id="simulationOutputDiscoveryList"`,
		`id="simulationCustomSeries"`,
		`id="simulationPurposeZoneNames"`,
		`id="simulationPurposePeriodStart"`,
		`id="simulationPurposePeriodEnd"`,
		`class="simulation-plan-panel"`,
		`id="simulationRunPlanStats"`,
		`id="simulationRunPlan"`,
		`class="simulation-run-options"`,
		`id="simulationAutoRunOnOpen"`,
		`id="simulationApplyStandardOutput"`,
		`id="simulationEnergyPlusSelect"`,
		`id="simulationRefreshEnv"`,
	} {
		if strings.Contains(markup, removed) {
			t.Fatalf("simplified Simulation markup still exposes %q", removed)
		}
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	definitions := sliceBetween(simulation, "const simulationPurposeDefinitions", "function simulationSemanticNavigationIndex")
	for _, removed := range []string{`id: "integrity_check"`, `id: "custom_outputs"`} {
		if strings.Contains(definitions, removed) {
			t.Fatalf("removed Simulation purpose remains in the UI definition: %q", removed)
		}
	}

	defaults := sliceBetween(simulation, "const simulationPurposeDefaults", "function simulationSemanticNavigationIndex")
	for _, required := range []string{
		`zoneMode: "all"`,
		`periodMode: "full"`,
		`basicEnergyDetail: "heat_drivers"`,
		`zoneHeatFlowDetail: "surface"`,
		`frequencyPolicy: "purpose_default"`,
		`allocationPolicy: "direct_only"`,
		`outputApplyMode: "add_missing_only"`,
	} {
		if !strings.Contains(defaults, required) {
			t.Fatalf("simplified Simulation defaults are missing %q", required)
		}
	}

	requestBuilder := sliceBetween(simulation, "function buildSimulationPurposeRequest", "function simulationHVACPurposeScope")
	for _, required := range []string{
		`zoneMode: simulationPurposeDefaults.zoneMode`,
		`zoneNames: []`,
		`periodMode: simulationPurposeDefaults.periodMode`,
		`periodStart: ""`,
		`periodEnd: ""`,
		`customOutputs: []`,
		`basicEnergyDetail: simulationPurposeDefaults.basicEnergyDetail`,
		`zoneHeatFlowDetail: simulationPurposeDefaults.zoneHeatFlowDetail`,
		`frequencyPolicy: simulationPurposeDefaults.frequencyPolicy`,
		`allocationPolicy: simulationPurposeDefaults.allocationPolicy`,
		`outputApplyMode: simulationPurposeDefaults.outputApplyMode`,
	} {
		if !strings.Contains(requestBuilder, required) {
			t.Fatalf("simplified Simulation request is missing fixed default reference %q", required)
		}
	}
	for _, removed := range []string{
		"simulationPurposeZoneMode",
		"simulationPurposeZoneNames",
		"simulationPurposePeriodMode",
		"simulationPurposePeriodStart",
		"simulationPurposePeriodEnd",
		"simulationPurposeEnergyDetail",
		"simulationPurposeZoneHeatFlowDetail",
		"simulationPurposeFrequencyPolicy",
		"simulationPurposeAllocationPolicy",
		"simulationPurposeApplyMode",
		"simulationCustomOutputs",
	} {
		if strings.Contains(requestBuilder, removed) {
			t.Fatalf("simplified Simulation request still reads removed control %q", removed)
		}
	}

	resultViews := sliceBetween(simulation, "function ensureActiveSimulationResultView", "function toggleSimulationResultSections")
	if strings.Contains(resultViews, `"integrity"`) || strings.Contains(resultViews, "integrity:") {
		t.Fatal("removed Integrity result view remains in result-view availability")
	}

	environmentRenderer := sliceBetween(simulation, "function renderSimulationEnvironment", "function recommendedEnergyPlusInstallPath")
	if strings.Contains(environmentRenderer, "simulationEnergyPlusSelect") {
		t.Fatal("Simulation environment renderer still depends on the removed EnergyPlus selector")
	}
	for _, required := range []string{"simulationWeatherSelect", "weatherFolders", "currentWeather"} {
		if !strings.Contains(environmentRenderer, required) {
			t.Fatalf("Simulation environment renderer must keep weather selection after automatic EnergyPlus selection: missing %q", required)
		}
	}

	installResolver := sliceBetween(simulation, "function selectedEnergyPlusInstall", "function currentInputEnergyPlusVersion")
	if strings.Contains(installResolver, "simulationEnergyPlusSelect") {
		t.Fatal("automatic EnergyPlus resolution still reads the removed selector")
	}
	for _, required := range []string{
		"state.simulationEnvironment?.installations",
		"recommendedEnergyPlusInstallPath(installs",
		"install.executablePath",
	} {
		if !strings.Contains(installResolver, required) {
			t.Fatalf("automatic EnergyPlus resolver is missing %q", required)
		}
	}

	installPolicy := sliceBetween(simulation, "function recommendedEnergyPlusInstallPath", "function renderSimulationProgress")
	for _, required := range []string{
		"currentInputEnergyPlusVersion()",
		"normalizedVersionKey(install.version) === requiredVersion",
		"installs[0].executablePath",
	} {
		if !strings.Contains(installPolicy, required) {
			t.Fatalf("automatic EnergyPlus matching/fallback policy is missing %q", required)
		}
	}

	blockingIssue := sliceBetween(simulation, "function simulationBlockingIssue", "function currentInputRequiresWeatherFile")
	if strings.Contains(blockingIssue, "simulationEnergyPlusSelect") {
		t.Fatal("Simulation blocker still reads the removed EnergyPlus selector")
	}
	for _, required := range []string{"selectedEnergyPlusInstall()", "simulation.energyPlusBlockedTitle", "simulationVersionIssue()"} {
		if !strings.Contains(blockingIssue, required) {
			t.Fatalf("automatic EnergyPlus/no-install blocker is missing %q", required)
		}
	}

	run := sliceBetween(simulation, "async function runCurrentSimulation", "async function maybeAutoRunSimulation")
	if strings.Contains(run, "simulationEnergyPlusSelect") || strings.Contains(run, "env?.installations?.[0]?.executablePath") {
		t.Fatal("Simulation run still bypasses automatic version matching")
	}
	for _, required := range []string{"selectedEnergyPlusInstall()", "energyPlusExecutablePath: installPath"} {
		if !strings.Contains(run, required) {
			t.Fatalf("Simulation run is missing automatic EnergyPlus contract %q", required)
		}
	}
}

func TestFrontendSimulationRefreshRemovalAndWeatherControlStyle(t *testing.T) {
	markup := readTestFile(t, "frontend/src/index.html")
	if strings.Contains(markup, `id="simulationRefreshEnv"`) {
		t.Fatal("main Simulation must not expose the environment Refresh button")
	}
	if !strings.Contains(markup, `id="simulationWeatherSelect"`) {
		t.Fatal("main Simulation must retain the Weather selector")
	}

	stateSource := readTestFile(t, "frontend/src/js/state.js")
	if strings.Contains(stateSource, "simulationRefreshEnv") {
		t.Fatal("state element registry still retains the removed Simulation Refresh button")
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	if strings.Contains(simulation, "simulationRefreshEnv") {
		t.Fatal("Simulation event wiring still retains the removed environment Refresh listener")
	}

	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	controls := sliceBetween(styles, ".simulation-controls {", ".simulation-purpose-panel {")
	for _, required := range []string{
		"grid-template-columns: minmax(260px, 520px)",
		".simulation-controls select",
		"width: 100%",
		"min-width: 0",
		"min-height: var(--control-height)",
		"border: 1px solid var(--line)",
		"border-radius: var(--radius-sm)",
		"background: var(--control)",
		"color: var(--ink)",
		"padding: 0 9px",
	} {
		if !strings.Contains(controls, required) {
			t.Fatalf("Simulation Weather selector must use the common control style and bounded width: missing %q", required)
		}
	}
}

func TestFrontendSimulationAutomaticEnergyPlusVersionEdges(t *testing.T) {
	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")

	versionReader := sliceBetween(simulation, "function currentInputEnergyPlusVersion", "function normalizedVersionKey")
	for _, required := range []string{
		`extractInputEnergyPlusVersion(elements.idfInput?.value || "")`,
		`JSON.parse(trimmed)`,
		`key.toLowerCase() === "version"`,
		`key.toLowerCase() === "version_identifier"`,
		`return normalizedVersionKey(identifier)`,
	} {
		if !strings.Contains(versionReader, required) {
			t.Fatalf("automatic EnergyPlus version detection is missing the epJSON Version/version_identifier contract %q", required)
		}
	}

	installPolicy := sliceBetween(simulation, "function recommendedEnergyPlusInstallPath", "function renderSimulationProgress")
	exactMatch := `installs.find((install) => normalizedVersionKey(install.version) === requiredVersion)`
	exactIndex := strings.Index(installPolicy, exactMatch)
	fallbackIndex := strings.Index(installPolicy, "installs[0].executablePath")
	if exactIndex < 0 || fallbackIndex < 0 || exactIndex > fallbackIndex {
		t.Fatal("automatic EnergyPlus resolver must prefer an exact normalized input-version match before its fallback install")
	}

	versionIssue := sliceBetween(simulation, "function simulationVersionIssue", "function simulationBlockingIssue")
	for _, required := range []string{
		"const requiredVersion = currentInputEnergyPlusVersion()",
		"const selectedInstall = selectedEnergyPlusInstall()",
		"const selectedVersion = normalizedVersionKey(selectedInstall?.version)",
		"selectedInstall?.version || \"\"",
		"unknown version",
		"simulation.versionMismatch",
	} {
		if !strings.Contains(versionIssue, required) {
			t.Fatalf("automatic EnergyPlus version blocker is missing %q", required)
		}
	}
	compactVersionIssue := strings.Join(strings.Fields(versionIssue), " ")
	for _, forbidden := range []string{
		"if (!selectedInstall?.version) { return null; }",
		"if (!selectedInstall?.version) return null;",
		"if (!selectedVersion) { return null; }",
		"if (!selectedVersion) return null;",
	} {
		if strings.Contains(compactVersionIssue, forbidden) {
			t.Fatalf("versioned input must remain blocked when the automatically selected install has a blank or unknown version: found %q", forbidden)
		}
	}
	if strings.Count(versionIssue, "return null;") != 2 {
		t.Fatal("simulationVersionIssue may return no blocker only for an unversioned input or an exact version match")
	}
}

func TestFrontendBatchEnergyExplanationDeltaContracts(t *testing.T) {
	batch := readTestFile(t, "frontend/src/js/batch/batch-simulation.js")
	for _, term := range []string{
		"renderEnergyExplanationDeltaRanking",
		"renderEnergyExplanationEdgeDeltaRanking",
		"energyExplanationDeltaMetricCell",
		"energyExplanationDeltaRatioSideDetail",
		"energyExplanationDeltaSourceCell",
		"energyExplanationDeltaSourceSummary",
		"energyExplanationDeltaRows",
		"energyExplanationEdgeDeltaRows",
		"energyExplanationAnnualEdgeItems",
		"energyExplanationDeltaStatus",
		"renderEnergyExplanationEdgeDeltaBars",
		"batch-energy-edge-delta-view",
		"energyExplanationDeltaValue",
		"energyExplanationDeltaPercent",
		"energyExplanationComparisonValue",
		"Residual\", \"residuals",
		"zero baseline",
		"zero comparison",
		"leftMissing",
		"rightMissing",
		"leftSourceSummary",
		"rightSourceSummary",
		"common.missing",
		"renderEnergyExplanationCompletenessDelta",
		"energyExplanationSourceAvailabilitySummary",
		"renderEnergyCompareSelects",
		"selectedEnergyCompareResults",
		"handleEnergyCompareSelectChange",
		"energyExplanationMissingCategorySummary",
		"exportMultiSimulationCSV",
		"exportMultiSimulationXLSX",
		"exportMultiSimulationJSON",
		"multiSimulationComparisonContext",
		"multiSimulationExportContext",
		"context: multiSimulationExportContext(result)",
		"SaveBatchSimulationXLSX({",
		"context: exportContext",
		"semantic-idf.batch-simulation/v1",
		"baselineRowId",
		"targetRowId",
		"purposeRequest: batchPurposeRequest()",
		"workerCount: Number(elements.multiSimulationWorkers?.value || 0)",
		"weatherMode: elements.multiSimulationWeatherMode?.value",
		"energyExplanationSummaryExportItems",
		"derivedKpis",
		"energy_explanation.derived_kpi",
		"energyExplanationSourceExportItems",
		"energyExplanationSourceAvailabilityExportItems",
		"energyExplanationNodeExportItems",
		"energyExplanationEdgeExportItems",
		"energyExplanationWarningExportItems",
		"energyExplanationBatchExportPeriods",
		"energyExplanationSourceTableExportFieldsForIDs",
		"energyExplanationSourceUnitExportFieldsForIDs",
		"energyExplanationSummaryEdgeExportFields",
		"energyExplanationRatioExportFields",
		"energyExplanationSourceObjectIndexes",
		"energyExplanationSourceTableExportFieldsForIDs(explanation, metric.sourceIds || [])",
		"energyExplanationSourceUnitExportFieldsForIDs(explanation, metric.sourceIds || [])",
		"energyExplanationSourceTableExportFieldsForIDs(explanation, edge.sourceIds || [])",
		"energyExplanationSourceUnitExportFieldsForIDs(explanation, edge.sourceIds || [])",
		"energyExplanationSourceTableExportFieldsForIDs(explanation, reconciliation.sourceIds || [])",
		"energyExplanationSourceUnitExportFieldsForIDs(explanation, reconciliation.sourceIds || [])",
		"sourceIds: item.sourceIds",
		"sourceIds: item.sourceIds || []",
		"energyExplanationSourceObjectIndexes(explanation, availability.sourceIds || [])",
		"emptyEnergyExplanationEdgeExportFields(availability.sourceIds || [])",
		"formula: item.formula",
		"numeratorValue: item.numeratorValue",
		"denominatorValue: item.denominatorValue",
		"heatCategory: item.heatCategory",
		"sign: item.sign",
		"reconciliation.zoneName",
		"reconciliation.status",
		"energy_explanation.source",
		"energy_explanation.source_availability",
		"energy_explanation.node",
		"energy_explanation.edge",
		"energy_explanation.warning",
		"source_frequency",
		"source_aggregation",
		"source_table",
		"source_row",
		"source_column",
		"source_unit",
		"normalized_unit",
		"path_type",
		"heat_category",
		"numerator_value",
		"denominator_value",
		"source_object_index",
		"rule_id",
		"source_ids",
		"related_path_ids",
		"Largest Energy Explanation Changes",
		"Sankey Edge Delta",
		"Basis</th><th>Edge",
		"Sources</th><th>Status",
		"missing in baseline",
	} {
		if !strings.Contains(batch, term) {
			t.Fatalf("batch energy explanation delta contract missing %q", term)
		}
	}
	html := readTestFile(t, "frontend/src/batch.html")
	if !strings.Contains(html, "multiSimulationExport") || !strings.Contains(html, "multiSimulationExportXLSX") || !strings.Contains(html, "multiSimulationExportJSON") {
		t.Fatalf("batch simulation export button is missing")
	}
	if !strings.Contains(html, "multiSimulationCompareBaseline") || !strings.Contains(html, "multiSimulationCompareTarget") {
		t.Fatalf("batch simulation energy comparison selectors are missing")
	}
	for _, removed := range []string{
		`data-batch-purpose="integrity_check"`,
		`id="multiSimulationEnergyPlus"`,
		`id="multiSimulationViewMode"`,
		`id="multiSimulationEnergyDetail"`,
		`id="multiSimulationAllocationPolicy"`,
		`id="multiSimulationFrequencyPolicy"`,
		`id="batchSimulationPlanPreview"`,
	} {
		if strings.Contains(html, removed) {
			t.Fatalf("simplified Batch Simulation markup still exposes %q", removed)
		}
	}

	batchShell := readTestFile(t, "frontend/src/js/batch.js")
	for _, removed := range []string{
		"multiSimulationEnergyPlus",
		"multiSimulationViewMode",
		"multiSimulationEnergyDetail",
		"multiSimulationAllocationPolicy",
		"multiSimulationFrequencyPolicy",
		"batchSimulationPlanPreview",
	} {
		if strings.Contains(batchShell, removed) || strings.Contains(batch, removed) {
			t.Fatalf("Batch scripts still retain removed control %q", removed)
		}
	}

	purposeRequest := sliceBetween(batch, "function batchPurposeRequest", "function bindEvents")
	for _, required := range []string{
		`basicEnergyDetail: "heat_drivers"`,
		`zoneHeatFlowDetail: "surface"`,
		`frequencyPolicy: "purpose_default"`,
		`allocationPolicy: "direct_only"`,
		`outputApplyMode: "add_missing_only"`,
		`zoneMode: "all"`,
		`zoneNames: []`,
		`periodMode: "full"`,
		`periodStart: ""`,
		`periodEnd: ""`,
		`loopMode: "all"`,
		`customOutputs: []`,
	} {
		if !strings.Contains(purposeRequest, required) {
			t.Fatalf("simplified Batch purpose request is missing fixed default %q", required)
		}
	}
	for _, removed := range []string{
		"multiSimulationEnergyDetail",
		"multiSimulationAllocationPolicy",
		"multiSimulationFrequencyPolicy",
	} {
		if strings.Contains(purposeRequest, removed) {
			t.Fatalf("simplified Batch purpose request still reads removed control %q", removed)
		}
	}

	runRequest := sliceBetween(batch, "async function run()", "async function callRunAPI")
	if !strings.Contains(runRequest, `energyPlusExecutablePath: ""`) {
		t.Fatal("Batch Simulation must leave the executable blank for backend per-file automatic selection")
	}
	if strings.Contains(runRequest, "multiSimulationEnergyPlus") {
		t.Fatal("Batch Simulation run still reads the removed manual EnergyPlus selector")
	}

	exportContext := sliceBetween(batch, "function multiSimulationExportContext", "function energyExplanationSummaryExportItems")
	if !strings.Contains(exportContext, `viewMode: "purpose"`) {
		t.Fatal("Batch Simulation export context must use the fixed purpose result view")
	}
	if strings.Contains(exportContext, "multiSimulationViewMode") {
		t.Fatal("Batch Simulation export context still reads the removed result-view selector")
	}
	for _, removed := range []string{"schedulePlanPreview", "refreshPlanPreview", "renderPlanPreview", "PreviewBatchSimulationPlan"} {
		if strings.Contains(batch, removed) {
			t.Fatalf("Batch Simulation still retains removed plan preview behavior %q", removed)
		}
	}
	batchApp := readTestFile(t, "batch_app.go")
	for _, term := range []string{"batchSimulationEnergyNodeSection", "Energy Nodes", "energy_nodes", "batchSimulationEnergyWarningSection", "batchSimulationEnergyWarningRows", "Energy Warnings", "energy_warnings"} {
		if !strings.Contains(batchApp, term) {
			t.Fatalf("batch simulation energy workbook export missing %q", term)
		}
	}
	if !strings.Contains(batchApp, "reconciliation_status") || !strings.Contains(batchApp, "row.Status") {
		t.Fatalf("batch simulation reconciliation workbook status export is missing")
	}
	if !strings.Contains(batchApp, "basic_energy_detail") || !strings.Contains(batchApp, "purposeRequest.BasicEnergyDetail") {
		t.Fatalf("batch simulation run context should preserve Basic Energy detail")
	}
	styles := readTestFile(t, "frontend/src/styles/workspace.css")
	if !strings.Contains(styles, ".batch-energy-edge-delta-view") || !strings.Contains(styles, ".batch-energy-edge-delta-track") {
		t.Fatalf("batch energy edge delta styles are missing")
	}
	if !strings.Contains(styles, ".batch-energy-delta-sources") {
		t.Fatalf("batch energy delta source styles are missing")
	}
}
