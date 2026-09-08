package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type epathSQLModelCheck struct {
	Item                 epathRealOracleMetricRecipe
	Want                 epathRealOracleMetric
	Quantity             *epathSQLQuantity
	OptionalPresentation bool
	Conversion           *epathSQLConversionProof
	Allocation           *epathSQLAllocationProof
	ZoneService          *epathSQLZoneServiceProof
	DriverLink           *epathSQLDriverLinkProof
	DirectUse            *epathSQLDirectUseProof
	Reconciliation       *epathSQLReconciliationProof
	Quality              *epathSQLQualityProof
	SiteFlow             *epathSQLSiteFlowProof
	ZoneCarrier          *epathSQLZoneCarrierProof
	SiteResidual         *epathSQLSiteResidualProof
}
type epathSQLModelChecks struct {
	Rows            []epathSQLModelCheck
	Keys            map[string]bool
	RequireCoverage bool
}

func (checks *epathSQLModelChecks) add(group, scope, zone, period, key, unit string, q *epathSQLQuantity, target epathRealOracleTarget, status string, found, total *int) error {
	identity := strings.Join([]string{group, scope, strings.ToLower(zone), period, key}, "|")
	if checks.Keys == nil {
		checks.Keys = map[string]bool{}
	}
	if checks.Keys[identity] {
		return fmt.Errorf("duplicate required compiled target %s", identity)
	}
	checks.Keys[identity] = true
	want := epathRealOracleMetric{Key: identity, Group: group, Scope: scope, Zone: zone, Period: period, Unit: unit, Status: status, Found: found, Total: total}
	if q != nil {
		if !q.valid() {
			return fmt.Errorf("invalid compiled SQL interval %s", identity)
		}
		want.Value = epathOracleNumber(q.Value)
	} else if status == "" {
		want.Status = "unavailable"
	}
	if err := epathValidateOracleMetricIdentity(want); err != nil {
		return err
	}
	checks.Rows = append(checks.Rows, epathSQLModelCheck{Item: epathRealOracleMetricRecipe{Key: identity, Group: group, Scope: scope, Zone: zone, Period: period, Unit: unit, Target: target}, Want: want, Quantity: q})
	return nil
}

func epathSQLPeriodMonths(period string) []int {
	if period == "annual" {
		return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	}
	var month int
	fmt.Sscanf(period, "M%d", &month)
	return []int{month}
}
func epathSQLNodeTarget(level, category, service, domain string) epathRealOracleTarget {
	return epathRealOracleTarget{Collection: "nodes", Field: "value", Level: level, Category: category, Service: service, Unit: "kWh", ScaleDomain: domain, Aggregate: "sum"}
}

func epathSQLModelLoadDriverChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	zones := []string{}
	for _, zone := range frames.Zones {
		zones = append(zones, zone.Name)
	}
	sort.Strings(zones)
	periods := []string{"annual"}
	for month := 1; month <= 12; month++ {
		periods = append(periods, fmt.Sprintf("M%d", month))
	}
	contexts := append([]string{""}, zones...)
	for _, zone := range contexts {
		scope := "building"
		if zone != "" {
			scope = "zone"
		}
		for _, period := range periods {
			for _, service := range []string{"cooling", "heating"} {
				load := epathSQLQuantity{}
				for key := range frames.Zones {
					if zone != "" && !strings.EqualFold(zone, key) {
						continue
					}
					for _, month := range epathSQLPeriodMonths(period) {
						load = load.add(frames.Loads[epathSQLKey(key, service, month)])
					}
				}
				target := epathSQLNodeTarget("load", "", service, "thermal")
				target.Basis = "reported_variable"
				target.AllowPrunedZero = true
				if err := checks.add("loads", scope, zone, period, service, "kWh", &load, target, "", nil, nil); err != nil {
					return err
				}
				for _, component := range []string{"sensible", "latent"} {
					componentTarget := target
					componentTarget.Field = "loadBreakdown"
					componentTarget.Component = component
					componentTarget.AllowPrunedZero = false
					var componentValue *epathSQLQuantity
					if component == "sensible" {
						componentValue = &load
					}
					if err := checks.add("loads", scope, zone, period, service+"/"+component, "kWh", componentValue, componentTarget, "", nil, nil); err != nil {
						return err
					}
					if component == "sensible" && load.includesZero() {
						checks.Rows[len(checks.Rows)-1].OptionalPresentation = true
					}
				}
				categories := map[string]bool{}
				quantities := map[string]map[string]epathSQLQuantity{}
				for _, month := range epathSQLPeriodMonths(period) {
					interzone, totalLoad := epathSQLQuantity{}, epathSQLQuantity{}
					if scope == "building" {
						for key := range frames.Zones {
							totalLoad = totalLoad.add(frames.Loads[epathSQLKey(key, service, month)])
						}
						for _, cell := range frames.Cells {
							if cell.Month == month && cell.BuildingVisible && (cell.Category == "surface.interzone" || cell.Category == "air.interzone") {
								interzone = interzone.add(cell.Allocated[service])
							}
						}
					}
					for _, cell := range frames.Cells {
						if cell.Month != month || zone != "" && !strings.EqualFold(cell.Zone, zone) || scope == "building" && !cell.BuildingVisible {
							continue
						}
						contribution, pressureFamily := cell.Allocated[service]
						if !pressureFamily {
							continue
						}
						category := cell.Category
						if category == "internal.other" {
							category = "balance.storage_other"
						}
						if scope == "building" && (category == "surface.interzone" || category == "air.interzone") {
							category = "balance.storage_other"
							if totalLoad.Value > 0 && interzone.Value/totalLoad.Value >= .05 {
								category = "interzone.transfer"
							}
						}
						categories[category] = true
						if quantities[category] == nil {
							quantities[category] = map[string]epathSQLQuantity{}
						}
						quantities[category]["value"] = quantities[category]["value"].add(contribution)
						_, contributionHigh := contribution.bounds()
						if contributionHigh <= 0 {
							continue
						}
						raw, effective := cell.Raw, cell.Effective
						if service == "heating" {
							raw = raw.times(-1)
							effective = effective.times(-1)
						}
						raw, effective = raw.positive(), effective.positive()
						if contribution.includesZero() {
							raw, effective = raw.optionalPresentation(), effective.optionalPresentation()
						}
						quantities[category]["rawValue"] = quantities[category]["rawValue"].add(raw)
						quantities[category]["effectiveValue"] = quantities[category]["effectiveValue"].add(effective)
					}
				}
				ordered := []string{}
				for category := range categories {
					ordered = append(ordered, category)
				}
				sort.Strings(ordered)
				for _, category := range ordered {
					for _, field := range []string{"value", "rawValue", "effectiveValue"} {
						target := epathSQLNodeTarget("driver", category, service, "thermal")
						target.Field = field
						target.Basis = "heat_balance_share"
						q := quantities[category][field]
						value := &q
						if field == "value" {
							value = &q
							target.AllowPrunedZero = true
						}
						if err := checks.add("drivers", scope, zone, period, category+"/"+service+"/"+field, "kWh", value, target, "", nil, nil); err != nil {
							return err
						}
						if field != "value" && quantities[category]["value"].includesZero() {
							checks.Rows[len(checks.Rows)-1].OptionalPresentation = true
						}
					}
				}
			}
		}
	}
	return nil
}

func epathSQLModelSiteChecks(frames epathSQLFrames, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	periods := []string{"annual"}
	for month := 1; month <= 12; month++ {
		periods = append(periods, fmt.Sprintf("M%d", month))
	}
	for _, period := range periods {
		endUses, carriers, mapped := map[string]epathSQLQuantity{}, map[string]epathSQLQuantity{}, map[string]epathSQLQuantity{}
		for _, site := range model.Site {
			total := epathSQLQuantity{}
			known := true
			for _, month := range epathSQLPeriodMonths(period) {
				value := frames.Site[site.ID][month-1]
				if value == nil {
					known = false
					break
				}
				total = total.add(*value)
			}
			if !known {
				return fmt.Errorf("site %s/%s has unknown observations; partial known sums are forbidden", site.ID, period)
			}
			if site.Facility {
				carriers[site.Carrier] = carriers[site.Carrier].add(total)
			} else {
				endUses[site.EndUse] = endUses[site.EndUse].add(total)
				mapped[site.Carrier] = mapped[site.Carrier].add(total)
			}
		}
		keys := []string{}
		for endUse := range endUses {
			keys = append(keys, endUse)
		}
		sort.Strings(keys)
		for _, endUse := range keys {
			q := endUses[endUse]
			target := epathSQLNodeTarget("end_use", endUse, "", "site")
			target.Basis = "reported_meter"
			target.AllowPrunedZero = true
			if err := checks.add("endUses", "building", "", period, endUse, "kWh", &q, target, "", nil, nil); err != nil {
				return err
			}
		}
		keys = nil
		for carrier := range carriers {
			keys = append(keys, carrier)
		}
		sort.Strings(keys)
		for _, carrier := range keys {
			q := carriers[carrier]
			target := epathSQLNodeTarget("carrier", carrier, "", "site")
			target.Basis = "reported_meter"
			target.AllowPrunedZero = true
			if err := checks.add("carriers", "building", "", period, carrier, "kWh", &q, target, "", nil, nil); err != nil {
				return err
			}
			for field, value := range map[string]epathSQLQuantity{"expectedValue": q, "explainedValue": mapped[carrier], "residualValue": q.add(mapped[carrier].times(-1))} {
				row := epathRealOracleTarget{Collection: "reconciliation", ID: "reconcile.energy." + carrier + "." + period, Level: "energy", Field: field, Unit: "kWh"}
				v := value
				if err := checks.add("residuals", "building", "", period, carrier+"/"+field, "kWh", &v, row, "", nil, nil); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func epathSQLModelAvailabilityChecks(observed []epathRealSQLSource, plan *PurposeRunPlan, model epathRealSQLModel, checks *epathSQLModelChecks) error {
	if plan == nil {
		return fmt.Errorf("independent completeness requires the actual executed output plan")
	}
	requested := map[string]bool{}
	for _, object := range plan.OutputObjects {
		if !strings.EqualFold(object.ReportingFrequency, "Monthly") {
			continue
		}
		name := object.VariableName
		if name == "" {
			for _, field := range object.Fields {
				if strings.EqualFold(field.Name, "Key Name") || strings.EqualFold(field.Name, "Variable Name") {
					name = field.Value
					break
				}
			}
		}
		if name != "" {
			requested[strings.ToLower(name)] = true
		}
	}
	counts := map[string][2]int{}
	seen := map[string]bool{}
	for _, group := range model.Availability {
		if group.ID == "" || seen[group.ID] || !strings.Contains(" drivers loads endUses carriers ", " "+group.Stage+" ") || len(group.RequestedNames) == 0 {
			return fmt.Errorf("invalid/duplicate independent availability family %s", group.ID)
		}
		seen[group.ID] = true
		for _, name := range group.RequestedNames {
			if !requested[strings.ToLower(name)] {
				return fmt.Errorf("declared availability request absent from executed plan: %s", name)
			}
		}
		found := false
		for _, source := range observed {
			if source.IsMeter != group.IsMeter || !strings.EqualFold(source.ReportingFrequency, "Monthly") || source.Rows <= source.MissingRows {
				continue
			}
			if len(group.Observations) == 0 {
				for _, name := range group.RequestedNames {
					if strings.EqualFold(name, source.Name) {
						return fmt.Errorf("declared missing family %s has actual SQL observation %s/%s; reviewed unit identity required", group.ID, source.Name, source.SourceUnit)
					}
				}
			}
			for _, alternative := range group.Observations {
				if strings.EqualFold(source.Name, alternative.Name) && source.SourceUnit == alternative.Unit {
					found = true
				}
			}
		}
		pair := counts[group.Stage]
		pair[1]++
		if found {
			pair[0]++
		}
		counts[group.Stage] = pair
	}
	for _, stage := range []string{"drivers", "loads", "endUses", "carriers"} {
		pair := counts[stage]
		if pair[1] == 0 {
			return fmt.Errorf("missing independent availability stage %s", stage)
		}
		status := "partial"
		if pair[0] == 0 {
			status = "missing"
		} else if pair[0] == pair[1] {
			status = "complete"
		}
		q := epathSQLQuantity{Value: float64(pair[0])}
		found, total := pair[0], pair[1]
		if err := checks.add("completeness", "building", "", "annual", stage, "count", &q, epathRealOracleTarget{Collection: "quality", Field: stage}, status, &found, &total); err != nil {
			return err
		}
	}
	return nil
}

func epathCheckSQLModelQuantity(actual *float64, want *epathSQLQuantity) error {
	if want != nil && !want.valid() {
		return fmt.Errorf("invalid SQL interval configuration")
	}
	if actual == nil || want == nil {
		if actual == nil && want == nil {
			return nil
		}
		if want != nil {
			low, high := want.bounds()
			return fmt.Errorf("candidate unknown; independent SQL center %.12g interval [%.12g, %.12g]", want.Value, low, high)
		}
		return fmt.Errorf("candidate known %.12g; independent observation unavailable", *actual)
	}
	if !epathOracleFinite(*actual) {
		return fmt.Errorf("nonfinite SQL interval comparison")
	}
	low, high := want.bounds()
	slack := 1e-8 * math.Max(1, math.Abs(want.Value))
	if *actual < low-slack || *actual > high+slack {
		return fmt.Errorf("candidate %.12g vs independent SQL %.12g, proven interval [%.12g, %.12g]", *actual, want.Value, low, high)
	}
	return nil
}
