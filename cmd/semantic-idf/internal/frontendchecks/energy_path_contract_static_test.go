package frontendchecks

import (
	"strings"
	"testing"
)

func TestFrontendEnergyPathV2HeaderAndControlContract(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")

	for _, required := range []string{
		`semantic-idf.energy-explanation/v2`,
		`export function renderEnergyPathHeader`,
		`export function renderEnergyPathControls`,
		`label: "Load Drivers"`,
		`label: "Thermal Load"`,
		`label: "End-use Energy"`,
		`label: "Energy Source"`,
		`scaleDomain: "thermal"`,
		`scaleDomain: "site"`,
		`unitLabel: "kWh thermal"`,
		`unitLabel: "kWh site"`,
		`Load drivers → Thermal load → End-use energy → Energy sources`,
		`data-simulation-energy-scope`,
		`data-simulation-energy-path-period`,
		`data-simulation-energy-service`,
		`viewState.simulationEnergyScopeKind === "zone"`,
		`type="search"`,
		`list="simulationEnergyPathZones"`,
		`data-simulation-energy-zone-name`,
		`["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"]`,
		"value: `M${index + 1}`",
		`value: "all"`,
		`value: "cooling"`,
		`value: "heating"`,
		`energyPathServiceOptions`,
		`energyPathItemService(link) === service`,
		`class="energy-path-conversion-divider"`,
		`class="energy-path-domain-legend"`,
		`export function renderEnergyPathNodeInspector`,
		`data-energy-path-inspector-value="${key}"`,
		`"rawValue"`,
		`"effectiveValue"`,
		`"allocatedValue"`,
		`source.multiplierApplication`,
		`export function renderEnergyPathSourceDetails`,
		`data-energy-path-inspector-section=`,
		`source.driverComponent`,
		`source.heatDirection`,
		`source.inspectorSection`,
		`source.formula`,
		`source.inputSourceIds`,
		`source.relatedEntityIds`,
		`data-energy-path-source-status=`,
		`renderEnergyPathWarnings(graph.warnings)`,
		`data-energy-path-warnings`,
		`data-energy-path-warning-severity=`,
		`data-energy-path-topology-air-coupling-id=`,
		`data-entity-kind="thermal_air_coupling"`,
		`data-energy-path-offset-effects`,
		`data-energy-path-offset-effect=`,
		`data-energy-path-simultaneous-load-ratio=`,
		`data-energy-path-simultaneous-load-numerator=`,
		`data-energy-path-simultaneous-load-denominator=`,
		`simultaneous_min_over_max`,
		`signed_heat_balance_offset`,
		`never create reverse main ribbons`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("Energy Path v2 contract missing %q", required)
		}
	}

	stageOrder := []string{`level: "driver"`, `level: "load"`, `level: "end_use"`, `level: "carrier"`}
	previous := -1
	for _, stage := range stageOrder {
		index := strings.Index(view, stage)
		if index < 0 || index <= previous {
			t.Fatalf("Energy Path stages are not fixed in canonical order at %q", stage)
		}
		previous = index
	}

	controls := sliceBetween(view, "function renderEnergyPathControls", "function renderEnergyPathStage")
	for _, legacy := range []string{
		"sankey-mode",
		"sign-mode",
		"node-limit",
		"focus-mode",
		"allocation-policy",
		"period-kind",
		"period-index",
	} {
		if strings.Contains(controls, legacy) {
			t.Fatalf("Energy Path v2 primary controls retain legacy control %q", legacy)
		}
	}
}

func TestFrontendEnergyPathV2DefaultsAndLegacyResultGuidance(t *testing.T) {
	state := readTestFile(t, "frontend/src/js/state.js")
	for _, required := range []string{
		`simulationEnergyScopeKind: "building"`,
		`simulationEnergyZoneName: ""`,
		`simulationEnergyPeriod: "annual"`,
		`simulationEnergyService: "all"`,
	} {
		if !strings.Contains(state, required) {
			t.Fatalf("Energy Path default state missing %q", required)
		}
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`from "./energy-path-view.js"`,
		`const useEnergyPathV2 = isEnergyPathV2(explanation)`,
		`if (useEnergyPathV2)`,
		`renderEnergyPathView(explanation, state, {`,
		`outputObjects: result?.purposeRunPlan?.outputObjects || []`,
		`inspectorActionsForNode:`,
		`updateEnergyPathControlState(event, state, explanation)`,
		`"simulation.energyPathUpgradeUnavailable"`,
		`available variables remain in Series`,
		`openSimulationEnergyPathTopologyAirCoupling`,
		`openSelectionInView("topology"`,
		`targetId: couplingID`,
	} {
		if !strings.Contains(simulation, required) {
			t.Fatalf("Energy Path v2 integration or legacy-result guidance missing %q", required)
		}
	}
}

func TestFrontendEnergyPathNamingDocumentationAndStyles(t *testing.T) {
	i18n := readTestFile(t, "frontend/src/js/i18n.js")
	for _, required := range []string{
		`"simulation.energyPathName": "Energy Path"`,
		`"simulation.energyPathStageSources": "Energy Source"`,
		`"simulation.energySankey": "Energy Path"`,
		`"simulation.energyPathInspectorSources": "Source details"`,
		`"simulation.energyPathQualityWarnings": "Energy Path quality warnings"`,
		`"simulation.energyPathOpenTopologyAirCoupling": "Open Topology air coupling {id}"`,
		`"simulation.energyPathSourceComponent": "Component"`,
		`"simulation.energyPathSourceHeatDirection": "Heat direction"`,
		`"simulation.energyPathSourceFormula": "Formula"`,
		`"simulation.energyPathName": "에너지 경로"`,
		`"simulation.energyPathStageSources": "에너지원"`,
		`"simulation.energySankey": "에너지 경로"`,
	} {
		if !strings.Contains(i18n, required) {
			t.Fatalf("Energy Path translation contract missing %q", required)
		}
	}

	guide := readTestFile(t, "frontend/src/guide.html")
	for _, required := range []string{
		`href="#energy-path"`,
		`id="energy-path"`,
		`Load drivers → Thermal loads → End-use energy → Energy sources`,
		`Scope, Period, and Service`,
		`defaulting to Building,`,
		`Annual, and All`,
	} {
		if !strings.Contains(guide, required) {
			t.Fatalf("Energy Path Guide contract missing %q", required)
		}
	}

	styles := readTestFile(t, "frontend/src/styles/simulation.css")
	for _, required := range []string{
		`.energy-path-header`,
		`grid-template-columns: minmax(120px, 1fr) auto`,
		`.energy-path-controls`,
		`.energy-path-stage-grid`,
		`.energy-path-stage-grid .energy-path-node`,
		`.energy-path-plot-divider`,
		`.energy-path-conversion-divider`,
		`.energy-path-domain-legend`,
		`.energy-path-node-inspector`,
		`grid-template-columns: repeat(5, minmax(110px, 1fr))`,
	} {
		if !strings.Contains(styles, required) {
			t.Fatalf("Energy Path style contract missing %q", required)
		}
	}

	releaseNotes := readTestFile(t, "../../docs/release-notes/unreleased.md")
	if !strings.Contains(releaseNotes, "Renamed the Simulation Sankey result to Energy Path") {
		t.Fatal("unreleased notes do not record the Energy Path naming and direction change")
	}
}

func TestFrontendEnergyPathV2SummaryConsumersAndV1Adapter(t *testing.T) {
	adapter := readTestFile(t, "frontend/src/js/energy-path-summary.js")
	for _, required := range []string{
		`semantic-idf.energy-explanation-summary/v2`,
		`export function isEnergyPathSummaryV2`,
		`export function energyPathSummaryGroups`,
		`export function energyPathSummaryKPIValues`,
		`key: "drivers"`,
		`key: "loads"`,
		`key: "endUses"`,
		`key: "carriers"`,
		`key: "ratios"`,
		`key: "residuals"`,
		`key: "topZones"`,
		`ENERGY_PATH_SUMMARY_V1_GROUPS`,
		`key: "energyByCarrier"`,
		`key: "energyByEndUse"`,
		`key: "deliveredLoadByService"`,
		`key: "heatDrivers"`,
	} {
		if !strings.Contains(adapter, required) {
			t.Fatalf("Energy Path summary adapter missing %q", required)
		}
	}

	canonicalOrder := []string{
		`key: "drivers"`,
		`key: "loads"`,
		`key: "endUses"`,
		`key: "carriers"`,
		`key: "ratios"`,
		`key: "residuals"`,
		`key: "topZones"`,
	}
	v2Definitions := sliceBetween(adapter, "const ENERGY_PATH_SUMMARY_V2_GROUPS", "const ENERGY_PATH_SUMMARY_V1_GROUPS")
	previous := -1
	for _, key := range canonicalOrder {
		index := strings.Index(v2Definitions, key)
		if index < 0 || index <= previous {
			t.Fatalf("v2 summary groups are not canonical and ordered at %q", key)
		}
		previous = index
	}
	for _, legacy := range []string{"energyByCarrier", "energyByEndUse", "deliveredLoadByService", "derivedKpis", "heatDrivers", "topHeatDrivers"} {
		if strings.Contains(v2Definitions, legacy) {
			t.Fatalf("v2 summary definition reads legacy key %q", legacy)
		}
	}

	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		`export function renderEnergyPathKPI`,
		`export function renderEnergyPathSummaryOverview`,
		`energyPathSummaryGroups(summary)`,
		`energyPathKPIItems(summary, options.graph || {}, { ...options, knownZoneOnly })`,
		`"rawValue"`,
		`"allocatedValue"`,
		`item.basis`,
		`item.aggregationBasis`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("Energy Path v2 KPI/overview missing %q", required)
		}
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`renderEnergyPathKPI(scopedSummary, kpiOptions)`,
		`energyPathSummaryGroups(summary).map((group) => [group.label, group.items])`,
		`energyPathLegacyDerivedKPIItems(explanationSummary)`,
	} {
		if !strings.Contains(simulation, required) {
			t.Fatalf("Simulation v2 summary path or v1 fallback missing %q", required)
		}
	}

	batch := readTestFile(t, "frontend/src/js/batch/batch-simulation.js")
	for _, required := range []string{
		`from "../energy-path-summary.js"`,
		`const v2 = isEnergyPathSummaryV2(summary)`,
		`const groups = energyPathSummaryGroups(summary).map((group) => [group.type, group.items])`,
		`rawValue: item.rawValue`,
		`allocatedValue: item.allocatedValue`,
		`aggregationBasis: item.aggregationBasis`,
		`energyExplanationSummaryComparisonGroups`,
		`energyPathSummaryGroups(v2Summary || leftSummary, { comparison: true })`,
	} {
		if !strings.Contains(batch, required) {
			t.Fatalf("Batch v2 summary consumer or v1 fallback missing %q", required)
		}
	}

	for _, consumer := range []struct {
		name   string
		source string
	}{
		{name: "Simulation", source: simulation},
		{name: "Batch", source: batch},
	} {
		for _, legacyRead := range []string{
			"summary.energyByCarrier",
			"summary.energyByEndUse",
			"summary.deliveredLoadByService",
			"summary.derivedKpis",
			"summary.heatDrivers",
			"summary.topHeatDrivers",
		} {
			if strings.Contains(consumer.source, legacyRead) {
				t.Fatalf("%s reads legacy summary key outside the v1 adapter: %q", consumer.name, legacyRead)
			}
		}
	}
}

func TestFrontendEnergyPathUsesPrecomputedZoneResults(t *testing.T) {
	view := readTestFile(t, "frontend/src/js/views/energy-path-view.js")
	for _, required := range []string{
		`Array.isArray(explanation.zoneResults)`,
		`export function energyPathResultForState`,
		`energyPathZoneResults(explanation)`,
		`result?.scope?.zoneName`,
		`const scopedResult = energyPathResultForState(explanation, viewState)`,
		`scopedResult.periods`,
		`export function energyPathSummaryForState`,
		`scopedResult.summary || fallbackSummary`,
		`period?.summary`,
		`const rootIsZoneOnly = energyPathToken(explanation.scope?.kind) === "zone"`,
		`simulationEnergyService: "all"`,
		`energyPathServiceOptions(explanation, viewState)`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("precomputed Energy Path zone-result contract missing %q", required)
		}
	}
	for _, staleFilter := range []string{
		`links.filter((link) => !link.zoneName`,
		`energyPathToken(node.zoneName) === wantedZone`,
	} {
		if strings.Contains(view, staleFilter) {
			t.Fatalf("Energy Path still derives Zone scope from the aggregated Building graph: %q", staleFilter)
		}
	}

	simulation := readTestFile(t, "frontend/src/js/views/simulation-views.js")
	for _, required := range []string{
		`const scopedSummary = energyPathSummaryForState(explanation, explanationSummary, state)`,
		`renderEnergyPathKPI(scopedSummary, kpiOptions)`,
	} {
		if !strings.Contains(simulation, required) {
			t.Fatalf("Simulation does not align Energy Path summary scope/period with its graph: %q", required)
		}
	}
	if strings.Contains(simulation, `renderEnergyPathSummaryOverview(scopedSummary)`) {
		t.Fatal("single Energy view duplicates its graph with the old summary overview")
	}
	changeHandler := sliceBetween(simulation, "function handleSimulationEnergyDashboardChange", "function handleSimulationHVACResultsInput")
	if !strings.Contains(changeHandler, "updateEnergyPathControlState(event, state, explanation)") {
		t.Fatal("Simulation change handler does not route Energy Path controls to the local v2 state adapter")
	}
	for _, backendCall := range []string{"postJSON(", "fetch(", "backend."} {
		if strings.Contains(changeHandler, backendCall) {
			t.Fatalf("Energy Path control change may trigger a backend call: %q", backendCall)
		}
	}
}
