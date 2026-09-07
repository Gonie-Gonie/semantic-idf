package simulation

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type EnergyPathProjectionRequest struct {
	ResultPath string `json:"resultPath"`
	InputPath  string `json:"inputPath"`
	Scope      string `json:"scope,omitempty"`
	Zone       string `json:"zone,omitempty"`
	Period     string `json:"period,omitempty"`
	Service    string `json:"service,omitempty"`
}

type EnergyPathSelection struct {
	Scope   string `json:"scope"`
	Zone    string `json:"zone,omitempty"`
	Period  string `json:"period"`
	Service string `json:"service"`
}

type EnergyPathProjectionView struct {
	Scope          EnergyExplanationScope   `json:"scope"`
	Period         string                   `json:"period"`
	Service        string                   `json:"service"`
	Nodes          []EnergyExplanationNode  `json:"nodes"`
	Links          []EnergyPathLink         `json:"links"`
	Sources        []EnergyDataSource       `json:"sources"`
	Summary        EnergyExplanationSummary `json:"summary"`
	Quality        *EnergyPathQuality       `json:"quality,omitempty"`
	Reconciliation []EnergyReconciliation   `json:"reconciliation,omitempty"`
	Warnings       []EnergyWarning          `json:"warnings,omitempty"`
}

// PurposeResults is the unchanged canonical builder response. View is only an
// explicit selection; a monthly slice is never mislabeled as an annual graph.
type EnergyPathProjection struct {
	Selection      EnergyPathSelection             `json:"selection"`
	PurposeResults PurposeResultBundle             `json:"purposeResults"`
	View           EnergyPathProjectionView        `json:"view"`
	Provenance     *EnergyPathProjectionProvenance `json:"provenance,omitempty"`
}

// InputVerified binds the recorded model hash to this manifest's SQL file
// identity. It does not claim a cryptographic hash of the SQL's contents.
type EnergyPathProjectionProvenance struct {
	SQLPath         string   `json:"sqlPath"`
	InputPath       string   `json:"inputPath"`
	InputHash       string   `json:"inputHash"`
	InputVerified   bool     `json:"inputVerified"`
	OutputPlanKnown bool     `json:"outputPlanKnown"`
	Warnings        []string `json:"warnings,omitempty"`
}

func normalizeEnergyPathSelection(selection EnergyPathSelection) (EnergyPathSelection, error) {
	selection.Scope = strings.ToLower(strings.TrimSpace(selection.Scope))
	if selection.Scope == "" {
		selection.Scope = "building"
	}
	selection.Zone = strings.TrimSpace(selection.Zone)
	selection.Period = strings.ToUpper(strings.TrimSpace(selection.Period))
	if selection.Period == "" || selection.Period == "ANNUAL" {
		selection.Period = "annual"
	}
	selection.Service = strings.ToLower(strings.TrimSpace(selection.Service))
	if selection.Service == "" {
		selection.Service = "all"
	}
	if selection.Scope != "building" && selection.Scope != "zone" {
		return selection, fmt.Errorf("scope must be building or zone")
	}
	if selection.Scope == "zone" && selection.Zone == "" {
		return selection, fmt.Errorf("zone scope requires an exact zone name")
	}
	if selection.Scope == "building" && selection.Zone != "" {
		return selection, fmt.Errorf("a zone name requires zone scope")
	}
	if selection.Period != "annual" {
		month, err := strconv.Atoi(strings.TrimPrefix(selection.Period, "M"))
		if err != nil || month < 1 || month > 12 || selection.Period != fmt.Sprintf("M%d", month) {
			return selection, fmt.Errorf("period must be annual or M1 through M12")
		}
	}
	if selection.Service != "all" && selection.Service != "cooling" && selection.Service != "heating" {
		return selection, fmt.Errorf("service must be all, cooling, or heating")
	}
	return selection, nil
}

// LoadEnergyPathProjection never executes EnergyPlus, creates a model copy, or
// discovers substitute sibling outputs for an explicitly named SQL file.
func LoadEnergyPathProjection(request EnergyPathProjectionRequest) (EnergyPathProjection, error) {
	selection, err := normalizeEnergyPathSelection(EnergyPathSelection{Scope: request.Scope, Zone: request.Zone, Period: request.Period, Service: request.Service})
	if err != nil {
		return EnergyPathProjection{}, err
	}
	sqlPath, manifest, err := resolveEnergyPathSQL(request.ResultPath)
	if err != nil {
		return EnergyPathProjection{}, err
	}
	inputPath := strings.TrimSpace(request.InputPath)
	if inputPath == "" && manifest != nil {
		if strings.TrimSpace(manifest.InputHash) == "" {
			return EnergyPathProjection{}, fmt.Errorf("the run manifest has no input hash; provide an explicit IDF/epJSON input path to acknowledge an unverified pairing")
		}
		inputPath = manifest.InputPath
		if inputPath != "" && !filepath.IsAbs(inputPath) {
			inputPath = filepath.Join(filepath.Dir(sqlPath), inputPath)
		}
	}
	if inputPath == "" || inputPath == "-" {
		return EnergyPathProjection{}, fmt.Errorf("an existing IDF/epJSON input path is required; stdin model input is not supported")
	}
	inputPath, err = filepath.Abs(inputPath)
	if err != nil {
		return EnergyPathProjection{}, err
	}
	before, err := os.ReadFile(inputPath)
	if err != nil {
		return EnergyPathProjection{}, fmt.Errorf("read simulation input: %w", err)
	}
	inputHash := sha256.Sum256(before)
	if manifest != nil && manifest.InputHash != "" && !strings.EqualFold(manifest.InputHash, hex.EncodeToString(inputHash[:])) {
		return EnergyPathProjection{}, fmt.Errorf("input hash does not match this simulation run; use its recorded run-copy IDF/epJSON, including temporary output objects")
	}
	if _, err := simulationDocumentFromInput(inputPath); err != nil {
		return EnergyPathProjection{}, fmt.Errorf("parse simulation input: %w", err)
	}
	info, err := os.Stat(sqlPath)
	if err != nil {
		return EnergyPathProjection{}, err
	}
	// The builder reports unavailable outputs as data, but an unreadable or
	// corrupt SQLite file is an input error, not an empty successful projection.
	db, err := openSimulationSQLiteReadOnly(sqlPath)
	if err != nil {
		return EnergyPathProjection{}, err
	}
	var schemaVersion int
	err = db.QueryRow("PRAGMA schema_version").Scan(&schemaVersion)
	db.Close()
	if err != nil {
		return EnergyPathProjection{}, fmt.Errorf("read selected simulation SQL: %w", err)
	}
	run := SimulationRunResult{InputPath: inputPath, OutputDirectory: filepath.Dir(sqlPath), Files: []SimulationFileInfo{{Name: filepath.Base(sqlPath), Path: sqlPath, Kind: "sqlite", Size: info.Size()}}}
	purpose := SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath}
	if manifest != nil {
		run.RunID = manifest.RunID
		run.PurposeRunPlan = manifest.OutputPlan
		if plan := manifest.OutputPlan; plan != nil {
			purpose.AllocationPolicy = plan.AllocationPolicy
			purpose.BasicEnergyDetail = plan.BasicEnergyDetail
			purpose.Scope = SimulationPurposeScope{ZoneMode: plan.ZoneMode, ZoneNames: append([]string(nil), plan.ZoneNames...), PeriodMode: plan.PeriodMode, PeriodStart: plan.PeriodStart, PeriodEnd: plan.PeriodEnd}
		}
	}
	bundle := BuildPurposeResultBundle(&run, purpose)
	after, err := os.ReadFile(inputPath)
	if err != nil {
		return EnergyPathProjection{}, fmt.Errorf("recheck simulation input: %w", err)
	}
	if sha256.Sum256(after) != inputHash {
		return EnergyPathProjection{}, fmt.Errorf("simulation input changed while the result was being read")
	}
	finalInfo, err := os.Stat(sqlPath)
	if err != nil || !os.SameFile(info, finalInfo) || info.Size() != finalInfo.Size() || !info.ModTime().Equal(finalInfo.ModTime()) {
		return EnergyPathProjection{}, fmt.Errorf("selected simulation SQL changed while the result was being read")
	}
	projection, err := ProjectEnergyPath(bundle, selection)
	if err != nil {
		return EnergyPathProjection{}, err
	}
	projection.Provenance = &EnergyPathProjectionProvenance{SQLPath: sqlPath, InputPath: inputPath, InputHash: hex.EncodeToString(inputHash[:]), InputVerified: manifest != nil && manifest.InputHash != "", OutputPlanKnown: manifest != nil && manifest.OutputPlan != nil}
	if !projection.Provenance.InputVerified {
		projection.Provenance.Warnings = append(projection.Provenance.Warnings, "The selected input/SQL pairing has no matching recorded input hash; model-derived context is unverified.")
	}
	return projection, nil
}

func resolveEnergyPathSQL(path string) (string, *SimulationRunManifest, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "-" {
		return "", nil, fmt.Errorf("an existing run directory or exact SQL file is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", nil, fmt.Errorf("read requested result: %w", err)
	}
	directory := filepath.Dir(absolute)
	if info.IsDir() {
		directory = absolute
	} else if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("requested SQL must be a regular file")
	}
	var manifest *SimulationRunManifest
	manifestBytes, readErr := os.ReadFile(filepath.Join(directory, "semantic-idf-run.json"))
	if readErr == nil {
		var decoded SimulationRunManifest
		if err := json.Unmarshal(manifestBytes, &decoded); err != nil {
			return "", nil, fmt.Errorf("read run manifest: %w", err)
		}
		manifest = &decoded
	} else if !os.IsNotExist(readErr) {
		return "", nil, readErr
	}
	if info.IsDir() {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return "", nil, err
		}
		var candidates []string
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".sql") {
				candidates = append(candidates, filepath.Join(directory, entry.Name()))
			}
		}
		if len(candidates) != 1 {
			return "", nil, fmt.Errorf("run directory must contain exactly one SQL file; found %d; specify the exact SQL file", len(candidates))
		}
		absolute = candidates[0]
	}
	// A sibling manifest is not historical provenance for an unrelated SQL file.
	if manifest != nil {
		matched := false
		for _, file := range manifest.ResultFiles {
			if file.Kind != "sqlite" && !strings.EqualFold(filepath.Ext(file.Name), ".sql") {
				continue
			}
			candidate := file.Path
			if candidate == "" && filepath.Base(file.Name) == file.Name {
				candidate = file.Name
			}
			if candidate == "" {
				continue
			}
			if !filepath.IsAbs(candidate) {
				candidate = filepath.Join(directory, candidate)
			}
			left, leftErr := os.Stat(candidate)
			right, rightErr := os.Stat(absolute)
			if leftErr == nil && rightErr == nil && os.SameFile(left, right) {
				matched = true
				break
			}
		}
		if !matched {
			manifest = nil
		}
	}
	return absolute, manifest, nil
}

// ProjectEnergyPath selects existing canonical data. Browser-only Other groups,
// layout widths, and semantic-ID renaming are deliberately not written here.
func ProjectEnergyPath(bundle PurposeResultBundle, selection EnergyPathSelection) (EnergyPathProjection, error) {
	selection, err := normalizeEnergyPathSelection(selection)
	if err != nil {
		return EnergyPathProjection{}, err
	}
	explanation := bundle.EnergyExplanation
	if explanation.Schema != energyExplanationSchema {
		return EnergyPathProjection{}, fmt.Errorf("canonical Energy Path v2 result is unavailable")
	}
	selected := explanation
	summary := bundle.EnergyExplanationSummary
	if selection.Scope == "zone" {
		matches := 0
		if explanation.Scope.Kind == "zone" && strings.EqualFold(explanation.Scope.ZoneName, selection.Zone) {
			matches++
		}
		for _, zone := range explanation.ZoneResults {
			if zone.Scope.Kind == "zone" && strings.EqualFold(zone.Scope.ZoneName, selection.Zone) {
				matches++
				selected = EnergyExplanationResult{Schema: explanation.Schema, Purpose: explanation.Purpose, Scope: zone.Scope, AllocationPolicy: explanation.AllocationPolicy, Nodes: zone.Nodes, Links: zone.Links, Periods: zone.Periods, Sources: explanation.Sources, Completeness: zone.Completeness, Quality: zone.Quality, Reconciliation: zone.Reconciliation, Warnings: zone.Warnings, ZoneContributions: zone.ZoneContributions}
				summary = zone.Summary
			}
		}
		if matches != 1 {
			return EnergyPathProjection{}, fmt.Errorf("requested Zone is unavailable or ambiguous: %s", selection.Zone)
		}
		selection.Zone = selected.Scope.ZoneName
	} else if selected.Scope.Kind != "building" {
		return EnergyPathProjection{}, fmt.Errorf("Building scope is unavailable in this scoped result")
	}
	periodMatches := 0
	var matched *EnergyPeriod
	for index := range selected.Periods {
		if strings.EqualFold(selected.Periods[index].ID, selection.Period) {
			periodMatches++
			matched = &selected.Periods[index]
		}
	}
	if periodMatches > 1 {
		return EnergyPathProjection{}, fmt.Errorf("requested period is ambiguous: %s", selection.Period)
	}
	if matched != nil && (selection.Period != "annual" || len(matched.Nodes) > 0 || len(matched.Links) > 0) {
		selected.Nodes = matched.Nodes
		selected.Links = matched.Links
		selected.Reconciliation = matched.Reconciliation
		selected.Warnings = matched.Warnings
		selected.ZoneContributions = matched.ZoneContributions
		selected.Quality = matched.Quality
		if matched.Summary != nil {
			summary = *matched.Summary
		} else {
			summary = EnergyExplanationSummary{}
		}
	} else if selection.Period != "annual" {
		// A previously selected canonical graph may omit the wrapper, but every
		// node must explicitly prove the requested month. Empty/annual graphs do not.
		exactTopPeriod := len(selected.Nodes) > 0
		for _, node := range selected.Nodes {
			exactTopPeriod = exactTopPeriod && strings.EqualFold(node.Period, selection.Period)
		}
		if !exactTopPeriod {
			return EnergyPathProjection{}, fmt.Errorf("requested period is unavailable: %s; annual fallback is forbidden", selection.Period)
		}
		if !strings.EqualFold(summary.Period, selection.Period) {
			summary = EnergyExplanationSummary{}
		}
	}
	selected.Periods = nil
	selected.ZoneResults = nil
	selected.legacyNodes = nil
	if summary.Schema == energyExplanationSummarySchema {
		if (summary.Period != "" && !strings.EqualFold(summary.Period, selection.Period)) ||
			(summary.Scope.Kind != "" && !strings.EqualFold(summary.Scope.Kind, selected.Scope.Kind)) ||
			(summary.Scope.ZoneName != "" && !strings.EqualFold(summary.Scope.ZoneName, selected.Scope.ZoneName)) ||
			(len(selected.Nodes) == 0 && len(selected.Links) == 0) {
			// Rebuild only the view summary from the selected canonical graph;
			// stale wrapper metadata must not turn an empty month into annual data.
			summary = EnergyExplanationSummary{}
		}
	}
	for _, node := range selected.Nodes {
		if node.Period != "" && !strings.EqualFold(node.Period, selection.Period) {
			return EnergyPathProjection{}, fmt.Errorf("node period contradicts the selected graph")
		}
		if selection.Scope == "zone" && node.ZoneName != "" && !strings.EqualFold(node.ZoneName, selection.Zone) {
			return EnergyPathProjection{}, fmt.Errorf("node Zone contradicts the selected graph")
		}
		if _, ok := energyPathProjectionConsistentService(energyPathProjectionNodeServiceTokens(node)); !ok {
			return EnergyPathProjection{}, fmt.Errorf("node service metadata is contradictory")
		}
	}
	for _, link := range selected.Links {
		if link.Period != "" && !strings.EqualFold(link.Period, selection.Period) {
			return EnergyPathProjection{}, fmt.Errorf("link period contradicts the selected graph")
		}
		if selection.Scope == "zone" && link.ZoneName != "" && !strings.EqualFold(link.ZoneName, selection.Zone) {
			return EnergyPathProjection{}, fmt.Errorf("link Zone contradicts the selected graph")
		}
	}
	selected.Nodes = cloneEnergyExplanationNodes(selected.Nodes)
	selected.Links = append([]EnergyPathLink(nil), selected.Links...)
	ids := map[string]EnergyExplanationNode{}
	for _, node := range selected.Nodes {
		if node.ID == "" {
			return EnergyPathProjection{}, fmt.Errorf("empty canonical node ID")
		}
		if _, exists := ids[node.ID]; exists {
			return EnergyPathProjection{}, fmt.Errorf("ambiguous canonical node ID: %s", node.ID)
		}
		ids[node.ID] = node
	}
	seenLinks := map[string]bool{}
	for _, link := range selected.Links {
		if link.ID == "" || seenLinks[link.ID] {
			return EnergyPathProjection{}, fmt.Errorf("empty or ambiguous canonical link ID")
		}
		seenLinks[link.ID] = true
		if _, ok := ids[link.FromID]; !ok {
			return EnergyPathProjection{}, fmt.Errorf("canonical link source endpoint is unavailable")
		}
		if _, ok := ids[link.ToID]; !ok {
			return EnergyPathProjection{}, fmt.Errorf("canonical link target endpoint is unavailable")
		}
		if _, ok := energyPathProjectionConsistentService([]string{link.ServiceKind, energyPathProjectionNodeService(ids[link.FromID]), energyPathProjectionNodeService(ids[link.ToID])}); !ok {
			return EnergyPathProjection{}, fmt.Errorf("link service contradicts its canonical endpoints")
		}
	}
	quality := selected.Quality
	if quality == nil {
		quality = BuildEnergyPathQuality(selected, "")
	}
	if selection.Service != "all" {
		retained := map[string]bool{}
		keptLinks := []EnergyPathLink{}
		keptIDs := map[string]bool{}
		for _, link := range selected.Links {
			if energyPathProjectionLinkService(link, ids) == selection.Service {
				keptLinks = append(keptLinks, link)
				keptIDs[link.ID] = true
				retained[link.FromID] = true
				retained[link.ToID] = true
			}
		}
		for _, node := range selected.Nodes {
			if energyPathProjectionNodeService(node) == selection.Service {
				retained[node.ID] = true
			}
		}
		for _, link := range selected.Links {
			if link.Relation == "support_supply" && !keptIDs[link.ID] && (retained[link.FromID] || retained[link.ToID]) {
				keptLinks = append(keptLinks, link)
				retained[link.FromID] = true
				retained[link.ToID] = true
			}
		}
		visibleCarriers := map[string]bool{}
		for _, node := range selected.Nodes {
			if node.Level == "carrier" && retained[node.ID] && strings.TrimSpace(node.Carrier) != "" {
				visibleCarriers[strings.ToLower(strings.TrimSpace(node.Carrier))] = true
			}
		}
		// Storage charge is deliberately an isolated support observation, not
		// another consumption branch. Preserve its visible carrier's context.
		for _, node := range selected.Nodes {
			if node.Level == "support" && visibleCarriers[strings.ToLower(strings.TrimSpace(node.Carrier))] {
				retained[node.ID] = true
			}
		}
		keptNodes := []EnergyExplanationNode{}
		for _, node := range selected.Nodes {
			if retained[node.ID] {
				keptNodes = append(keptNodes, node)
			}
		}
		selected.Nodes, selected.Links = keptNodes, keptLinks
		summary = EnergyExplanationSummary{}
	}
	if summary.Schema != energyExplanationSummarySchema {
		summary = buildEnergyExplanationSummaryV2(selected)
		summary.Period = selection.Period
	}
	summary.Quality = quality
	view := EnergyPathProjectionView{Scope: selected.Scope, Period: selection.Period, Service: selection.Service, Nodes: selected.Nodes, Links: selected.Links, Sources: append([]EnergyDataSource(nil), explanation.Sources...), Summary: summary, Quality: quality, Reconciliation: append([]EnergyReconciliation(nil), selected.Reconciliation...), Warnings: append([]EnergyWarning(nil), selected.Warnings...)}
	if view.Nodes == nil {
		view.Nodes = []EnergyExplanationNode{}
	}
	if view.Links == nil {
		view.Links = []EnergyPathLink{}
	}
	if view.Sources == nil {
		view.Sources = []EnergyDataSource{}
	}
	return EnergyPathProjection{Selection: selection, PurposeResults: bundle, View: view}, nil
}

func energyPathProjectionNodeService(node EnergyExplanationNode) string {
	service, _ := energyPathProjectionConsistentService(energyPathProjectionNodeServiceTokens(node))
	return service
}

func energyPathProjectionNodeServiceTokens(node EnergyExplanationNode) []string {
	kind := strings.TrimPrefix(strings.TrimPrefix(node.Kind, "load."), "end_use.")
	return []string{node.ServiceKind, node.EndUse, kind}
}

func energyPathProjectionConsistentService(values []string) (string, bool) {
	found := ""
	for _, value := range values {
		if service := energyCanonicalServiceKind(value); service == "cooling" || service == "heating" {
			if found != "" && found != service {
				return "", false
			}
			found = service
		}
	}
	return found, true
}

func energyPathProjectionLinkService(link EnergyPathLink, nodes map[string]EnergyExplanationNode) string {
	if service := energyCanonicalServiceKind(link.ServiceKind); service == "cooling" || service == "heating" {
		return service
	}
	if service := energyPathProjectionNodeService(nodes[link.FromID]); service != "" {
		return service
	}
	return energyPathProjectionNodeService(nodes[link.ToID])
}

// CSV defaults to the selected canonical summary. Trace rows preserve their own
// units and paired quantities; annual source scalars never become monthly data.
func EnergyPathProjectionCSV(projection EnergyPathProjection, includeTrace bool) (string, error) {
	var out bytes.Buffer
	writer := csv.NewWriter(&out)
	headers := []string{"row_type", "stage", "category", "value", "unit", "basis", "aggregation_basis", "scope", "zone", "period", "service", "source_id", "link_id", "from_id", "to_id", "from_value", "from_unit", "to_value", "to_unit", "ratio", "ratio_kind", "trace_json", "status", "found", "total", "source_unit", "normalized_unit", "message"}
	if err := writer.Write(headers); err != nil {
		return "", err
	}
	number := func(value float64) (string, error) {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", fmt.Errorf("cannot export a non-finite energy value")
		}
		return strconv.FormatFloat(value, 'g', -1, 64), nil
	}
	base := func(kind string) []string {
		row := make([]string, len(headers))
		row[0] = kind
		row[7] = projection.Selection.Scope
		row[8] = projection.Selection.Zone
		row[9] = projection.Selection.Period
		row[10] = projection.Selection.Service
		return row
	}
	for _, group := range []struct {
		stage string
		items []EnergyExplanationSummaryItem
	}{{"drivers", projection.View.Summary.Drivers}, {"loads", projection.View.Summary.Loads}, {"endUses", projection.View.Summary.EndUses}, {"carriers", projection.View.Summary.Carriers}, {"ratios", projection.View.Summary.Ratios}, {"residuals", projection.View.Summary.Residuals}} {
		for _, item := range group.items {
			row := base("summary")
			row[1] = group.stage
			row[2] = item.Label
			var err error
			row[3], err = number(item.Value)
			if err != nil {
				return "", err
			}
			row[4] = item.Unit
			row[5] = item.Basis
			row[6] = item.AggregationBasis
			if err := writer.Write(row); err != nil {
				return "", err
			}
		}
	}
	if quality := projection.View.Quality; quality != nil {
		// Quality describes the full scope/period, not the displayed service.
		// A Zone's observed subtotal has no measured facility denominator and
		// cannot turn a stored partial/zero placeholder into a known 0% closure.
		contextNodes := projection.View.Nodes
		if projection.PurposeResults.EnergyExplanation.Schema == energyExplanationSchema {
			selection := projection.Selection
			selection.Service = "all"
			context, err := ProjectEnergyPath(projection.PurposeResults, selection)
			if err != nil {
				return "", err
			}
			contextNodes = context.View.Nodes
		}
		carrierDenominatorKnown := false
		for _, node := range contextNodes {
			value, valid := energyPathQualityEnergyValue(math.Abs(node.Value), node.Unit)
			carrierDenominatorKnown = carrierDenominatorKnown || (energyPathQualityReportedCarrier(node) && valid && value > 0)
		}
		for _, stage := range []struct {
			key   string
			level EnergyCompletenessLevel
		}{{"drivers", quality.Drivers}, {"loads", quality.Loads}, {"endUses", quality.EndUses}, {"carriers", quality.Carriers}, {"ratios", quality.Ratios}} {
			row := base("quality")
			row[1], row[2], row[22] = stage.key, "output_availability", stage.level.Status
			row[27] = stage.level.Message
			if stage.key == "ratios" {
				row[2] = "conversion_availability"
			}
			if stage.level.Total > 0 && stage.level.Status != "unavailable" && stage.level.Status != "not_requested" && stage.level.Status != "not_applicable" {
				row[23], row[24] = strconv.Itoa(stage.level.Found), strconv.Itoa(stage.level.Total)
			}
			if err := writer.Write(row); err != nil {
				return "", err
			}
		}
		for _, metric := range []struct {
			key, status string
			value       float64
		}{{"driver_to_load_closed_pct", quality.DriverToLoadStatus, quality.DriverToLoadClosedPct}, {"end_use_to_carrier_closed_pct", quality.EndUseToCarrierStatus, quality.EndUseToCarrierClosedPct}, {"zone_allocated_pct", quality.ZoneAllocationStatus, quality.ZoneAllocatedPct}, {"unassigned_pct", quality.ZoneAllocationStatus, quality.UnassignedPct}} {
			row := base("quality")
			row[2], row[4], row[22] = metric.key, "%", metric.status
			known := metric.status == "balanced" || metric.status == "complete" || metric.status == "partial" || metric.status == "overmapped"
			if metric.key == "end_use_to_carrier_closed_pct" && !carrierDenominatorKnown {
				known = false
				row[27] = "Facility denominator unavailable; observed or allocated subtotals are not measured facility totals."
			}
			if known {
				var err error
				row[3], err = number(metric.value)
				if err != nil {
					return "", err
				}
			}
			if err := writer.Write(row); err != nil {
				return "", err
			}
		}
	}
	if includeTrace {
		for _, source := range projection.View.Sources {
			row := base("source")
			row[2] = source.Name
			row[4] = source.NormalizedUnit
			row[9] = ""
			row[11] = source.ID
			row[25], row[26] = source.SourceUnit, source.NormalizedUnit
			encoded, err := json.Marshal(source)
			if err != nil {
				return "", err
			}
			row[21] = string(encoded)
			if err := writer.Write(row); err != nil {
				return "", err
			}
		}
		for _, link := range projection.View.Links {
			row := base("link")
			row[2] = link.Relation
			row[5] = link.Basis
			row[12] = link.ID
			row[13] = link.FromID
			row[14] = link.ToID
			var err error
			row[15], err = number(link.FromValue)
			if err != nil {
				return "", err
			}
			row[16] = link.FromUnit
			row[17], err = number(link.ToValue)
			if err != nil {
				return "", err
			}
			row[18] = link.ToUnit
			if link.RatioKind != "" {
				row[19], err = number(link.Ratio)
				if err != nil {
					return "", err
				}
			}
			row[20] = link.RatioKind
			encoded, err := json.Marshal(link)
			if err != nil {
				return "", err
			}
			row[21] = string(encoded)
			if err := writer.Write(row); err != nil {
				return "", err
			}
		}
	}
	writer.Flush()
	return out.String(), writer.Error()
}

// A missing historical plan is not evidence of requested or unrequested
// outputs. Keep the observed graph, but let the shared v2 quality helper expose
// unknown requested coverage rather than inventing an expectation denominator.
func markEnergyPathOutputPlanUnknown(explanation *EnergyExplanationV1) {
	explanation.Completeness.SourceAvailability = nil
	explanation.Completeness.HeatDrivers = EnergyCompletenessLevel{Level: "heat"}
	explanation.Completeness.DeliveredLoad = EnergyCompletenessLevel{Level: "load"}
	explanation.Completeness.EnergyUse = EnergyCompletenessLevel{Level: "energy"}
	explanation.Completeness.Items = nil
	explanation.Completeness.MissingCategories = nil
	explanation.Completeness.Status = "unavailable"
	explanation.Warnings = append(explanation.Warnings, EnergyWarning{Severity: "warning", Code: "energy_output_plan_unavailable", Message: "Historical Output request metadata is unavailable; observed values are retained, but requested-output coverage cannot be established."})
}
