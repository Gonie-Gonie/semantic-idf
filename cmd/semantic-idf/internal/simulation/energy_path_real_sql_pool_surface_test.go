package simulation

// Independent acceptance support. Interpretation/identity proof, never a new numeric source,
// passive-surface exclusion, subtraction, or Zone HVAC load authority.
import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLPoolSurfaceSource struct {
	Name, Key, Unit, Frequency string
	Qualified, Required        bool
}

type epathSQLPoolSurfaceQualification struct {
	Original                             epathSQLPoolOriginalProof
	ExecutedSHA256                       string
	PoolIndex, SurfaceIndex, AuthorityID int
	Sources                              map[int]epathSQLPoolSurfaceSource
}

func epathSQLPoolSurfaceOutput(name string) string {
	switch name {
	case "Surface Inside Face Convection Heat Gain Energy":
		return "J"
	case "Surface Inside Face Convection Heat Gain Rate":
		return "W"
	}
	return ""
}

// Native observations can also carry an explanation of a downstream driver
// allocation. This is not a source transformation. Keep a finite literal
// contract independent of the production formatter; no arbitrary Formula is
// accepted merely because AllocationApplied is true or two strings agree.
func epathSQLPoolSurfaceNativeFormula(source EnergyDataSource) bool {
	if source.Formula == "" {
		return source.AllocationFormula == ""
	}
	if !source.AllocationApplied || source.Formula != source.AllocationFormula {
		return false
	}
	const proportional = "actual canonical service load * matching driver signed pressure / sum of matching signed pressures"
	return source.Formula == proportional || source.Formula == "sum(distinct directional driver allocatedValue with this sole allocation source); each allocation: "+proportional
}

// Call after ordinary surface and Hourly-companion frames and native Pool
// frames. Their independent numeric checks stay mandatory and unchanged.
func epathCompileSQLPoolSurfaceQualification(observed epathRealOracleEvidence, model epathRealSQLModel, frames epathSQLFrames) (*epathSQLPoolSurfaceQualification, error) {
	if len(model.PoolSystems) == 0 {
		return nil, nil
	}
	if len(frames.PoolSystems) != 1 || model.Surface.Mapping != "surface_class_boundary/v1" || model.Surface.Sign != -1 {
		return nil, fmt.Errorf("Pool surface qualification requires native frames and unchanged signed surface model")
	}
	originals, err := epathSQLValidatePoolOriginal(observed.originalText, model.PoolSystems)
	if err != nil || len(originals) != 1 {
		return nil, fmt.Errorf("Pool surface has no finite original proof: %v", err)
	}
	original := originals[0]
	if !reflect.DeepEqual(original, frames.PoolSystems[0].Original) {
		return nil, fmt.Errorf("Pool surface original differs from the native source proof")
	}
	doc, err := idf.Parse(observed.originalText)
	if err != nil {
		return nil, err
	}
	run, err := idf.Parse(observed.executedText)
	if err != nil || strings.TrimSpace(observed.executedText) == "" {
		return nil, fmt.Errorf("Pool surface requires separately hash-bound executed input")
	}
	d := original.Declaration
	lookup := epathSQLSharedOriginal{doc: doc}
	pool, err := lookup.one("SwimmingPool:Indoor", d.PoolName)
	if err != nil {
		return nil, err
	}
	surface, err := lookup.one("BuildingSurface:Detailed", d.SurfaceName)
	if err != nil {
		return nil, err
	}
	zone, err := lookup.one("Zone", d.ZoneName)
	if err != nil {
		return nil, err
	}
	zoneMultiplier, err := strconv.ParseFloat(epathSQLSharedField(zone, 6), 64)
	if err != nil || zoneMultiplier != 3 {
		return nil, fmt.Errorf("Pool surface original Zone multiplier is not the reviewed representative factor three")
	}
	if _, err := epathSQLPoolExecutedOwner(doc, run, epathSQLPoolOwner(zone, "", d.ZoneName)); err != nil {
		return nil, err
	}
	poolIndex, err := epathSQLPoolExecutedOwner(doc, run, epathSQLPoolOwner(pool, d.HotWaterLoopName, d.ZoneName))
	if err != nil {
		return nil, err
	}
	surfaceIndex, err := epathSQLPoolExecutedOwner(doc, run, epathSQLPoolOwner(surface, "", d.ZoneName))
	if err != nil {
		return nil, err
	}
	// Native geometry and representative scale are independent SQL evidence.
	// SQL SurfaceIndex is not an IDF object index and is never used as one.
	db, err := epathOpenOracleSQL(observed.sqlPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT s.SurfaceIndex,s.ClassName,s.ExtBoundCond,s.HeatTransferSurf,z.ZoneName,z.Multiplier,z.ListMultiplier FROM Surfaces s JOIN Zones z ON z.ZoneIndex=s.ZoneIndex WHERE lower(s.SurfaceName)=lower(?)`, d.SurfaceName)
	if err != nil {
		return nil, err
	}
	count := 0
	for rows.Next() {
		var index, boundary, heat int
		var class, zone string
		var multiplier, list float64
		if err := rows.Scan(&index, &class, &boundary, &heat, &zone, &multiplier, &list); err != nil {
			rows.Close()
			return nil, err
		}
		count++
		if index <= 0 || class != "Floor" || boundary != -1 || heat != 1 || !strings.EqualFold(zone, d.ZoneName) || multiplier != 3 || list != 1 || frames.Zones[strings.ToLower(zone)].Multiplier != 3 {
			rows.Close()
			return nil, fmt.Errorf("Pool surface lost exact native Ground/Floor/Zone representative multiplier")
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if count != 1 {
		return nil, fmt.Errorf("Pool floor has missing or ambiguous native SQL ownership")
	}
	out := &epathSQLPoolSurfaceQualification{Original: original, ExecutedSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(observed.executedText))), PoolIndex: poolIndex, SurfaceIndex: surfaceIndex, Sources: map[int]epathSQLPoolSurfaceSource{}}
	seen, qualified := map[string]bool{}, map[string]int{}
	for _, source := range observed.Sources {
		unit := epathSQLPoolSurfaceOutput(source.Name)
		if unit == "" || source.ReportingFrequency != "Monthly" && source.ReportingFrequency != "Hourly" {
			continue
		}
		key := source.Name + "|" + strings.ToLower(source.KeyValue) + "|" + source.ReportingFrequency
		if source.DictionaryIndex <= 0 || source.KeyValue == "" || source.IsMeter || source.SourceUnit != unit || seen[key] || out.Sources[source.DictionaryIndex].Name != "" {
			return nil, fmt.Errorf("Pool surface native source identity is missing/ambiguous: %d", source.DictionaryIndex)
		}
		seen[key] = true
		q := strings.EqualFold(source.KeyValue, d.SurfaceName)
		required := q && unit == "J"
		if q {
			wantRows := 12
			if source.ReportingFrequency == "Hourly" {
				wantRows = 8760
			}
			if source.Rows != wantRows || source.MissingRows != 0 || len(source.Months) != 12 {
				return nil, fmt.Errorf("Pool floor source %d is missing native observations, not known zero", source.DictionaryIndex)
			}
			for month, bucket := range source.Months {
				if bucket.Month != month+1 || bucket.MissingRows != 0 || bucket.EnergyKWh == nil || !epathOracleFinite(*bucket.EnergyKWh) {
					return nil, fmt.Errorf("Pool floor source %d lost exact known signed month", source.DictionaryIndex)
				}
			}
			// Actual surface requests are global keyed outputs, NOT a Pool or
			// ZoneHVAC scoped output; retain original wildcard duplicates.
			if _, err := epathSQLPoolSourceRequest(doc, run, observed.outputPlan, source, ""); err != nil {
				return nil, err
			}
			qualified[source.Name+"|"+source.ReportingFrequency] = source.DictionaryIndex
		}
		out.Sources[source.DictionaryIndex] = epathSQLPoolSurfaceSource{source.Name, source.KeyValue, unit, source.ReportingFrequency, q, required}
	}
	if len(qualified) != 4 {
		return nil, fmt.Errorf("Pool floor requires exact actual E/R Monthly/Hourly native identities")
	}
	monthlyID := qualified["Surface Inside Face Convection Heat Gain Energy|Monthly"]
	hourlyID := qualified["Surface Inside Face Convection Heat Gain Energy|Hourly"]
	out.AuthorityID = monthlyID
	if frames.SourceIdentities[monthlyID].DictionaryIndex != monthlyID || len(frames.SourceRaw[monthlyID]) != 12 || len(frames.SourceEffective[monthlyID]) != 12 || !strings.EqualFold(frames.SourceZone[monthlyID], d.ZoneName) {
		return nil, fmt.Errorf("Pool floor selected Monthly source was removed from ordinary quantity checks")
	}
	trace, ok := frames.TraceSourceIdentities[hourlyID]
	if !ok || trace.NativeCompanion == nil || trace.Source.DictionaryIndex != hourlyID || trace.Authority.DictionaryIndex != monthlyID {
		return nil, fmt.Errorf("Pool floor lost independently bound native Hourly companion")
	}
	for month := 1; month <= 12; month++ {
		cell := frames.Cells[epathSQLKey(d.ZoneName, "surface:surface.ground_floors", month)]
		found := 0
		if cell != nil {
			for _, id := range cell.SourceIDs {
				if id == monthlyID {
					found++
				}
			}
		}
		if cell == nil || found != 1 || cell.Category != "surface.ground_floors" || cell.Component != "sensible" || !cell.BuildingVisible || cell.ZeroPressureFallback {
			return nil, fmt.Errorf("Pool floor M%d was excluded or relabeled from its ordinary measured surface cell", month)
		}
	}
	return out, nil
}

// Supplemental to the same mandatory generic raw/effective, Hourly trace,
// driver and canonical Load checks. No numeric field is compared or mutated.
func epathSQLCheckPoolSurfaceQualification(sources []EnergyDataSource, proof *epathSQLPoolSurfaceQualification) error {
	if proof == nil || proof.PoolIndex < 0 || proof.SurfaceIndex < 0 || proof.PoolIndex == proof.SurfaceIndex || proof.AuthorityID <= 0 || len(proof.Original.OriginalSHA256) != 64 || len(proof.ExecutedSHA256) != 64 || proof.Sources[proof.AuthorityID].Required == false {
		return fmt.Errorf("missing independently bound Pool floor qualification proof")
	}
	poolID, surfaceID := fmt.Sprintf("component:%d", proof.PoolIndex), fmt.Sprintf("component:%d", proof.SurfaceIndex)
	seen := map[int]bool{}
	for _, source := range sources {
		unit := epathSQLPoolSurfaceOutput(source.Name)
		marker := strings.Contains(strings.ToLower(source.Explanation), "pool-bearing surface")
		if unit == "" || source.ReportingFrequency != "Monthly" && source.ReportingFrequency != "Hourly" {
			if marker {
				return fmt.Errorf("non-surface source %s acquired Pool-floor qualification", source.ID)
			}
			continue
		}
		var id int
		if _, err := fmt.Sscanf(source.ID, "sql-rdd-%d", &id); err != nil {
			return fmt.Errorf("Pool floor source lost native identity: %s", source.ID)
		}
		original, ok := proof.Sources[id]
		if !ok || seen[id] || source.ID != fmt.Sprintf("sql-rdd-%d", id) || source.SourceType != "sql_report_data" || source.IsMeter || source.Name != original.Name || !strings.EqualFold(source.KeyValue, original.Key) || source.SourceUnit != original.Unit || source.Units != original.Unit || source.ReportingFrequency != original.Frequency || len(source.InputSourceIDs) != 0 || !epathSQLPoolSurfaceNativeFormula(source) {
			return fmt.Errorf("Pool surface source %s differs from native exact identity", source.ID)
		}
		seen[id] = true
		parents := map[string]int{}
		for _, related := range source.RelatedEntityIDs {
			parents[related]++
		}
		if !original.Qualified {
			if marker || parents[poolID] != 0 || parents[surfaceID] != 0 {
				return fmt.Errorf("nonrecipient surface %s borrowed the original Pool floor", source.ID)
			}
			continue
		}
		if source.ZoneName != "" && !strings.EqualFold(source.ZoneName, proof.Original.Declaration.ZoneName) {
			return fmt.Errorf("Pool floor source %s borrowed a foreign Zone", source.ID)
		}
		if parents[poolID] != 1 || parents[surfaceID] != 1 {
			return fmt.Errorf("Pool floor source %s lost exact executed Pool/surface references", source.ID)
		}
		for related := range parents {
			if strings.HasPrefix(related, "component:") && related != poolID && related != surfaceID {
				return fmt.Errorf("Pool floor source %s acquired a foreign component parent", source.ID)
			}
		}
		lower := strings.ToLower(source.Explanation)
		for _, phrase := range []string{"pool-bearing surface", "reported surface transfer is retained", "not an isolated passive", "nonadditive context", "not an additional zone-air driver"} {
			if !strings.Contains(lower, phrase) {
				return fmt.Errorf("Pool floor source %s lost physical qualification %q", source.ID, phrase)
			}
		}
	}
	for id, source := range proof.Sources {
		if source.Required && !seen[id] {
			return fmt.Errorf("Pool floor lost required measured source sql-rdd-%d", id)
		}
	}
	return nil
}

// Keep the ordinary numeric source check untouched. Its already independently
// compiled quantity is repeated under a mandatory semantic key, not recomputed
// from a candidate or given a second precision allowance.
func epathSQLModelPoolSurfaceChecks(proof *epathSQLPoolSurfaceQualification, checks *epathSQLModelChecks) error {
	if proof == nil {
		return nil
	}
	if checks == nil {
		return fmt.Errorf("Pool surface requires existing mandatory generic source checks")
	}
	source, ok := proof.Sources[proof.AuthorityID]
	if !ok || !source.Required || !source.Qualified || source.Unit != "J" || source.Frequency != "Monthly" {
		return fmt.Errorf("Pool surface has no selected Monthly Energy authority")
	}
	target := epathRealOracleTarget{Collection: "sources", Field: "rawValue", SourceName: source.Name, SourceKey: source.Key, SourceUnit: source.Unit, Frequency: source.Frequency, Unit: "kWh"}
	var quantity *epathSQLQuantity
	count := 0
	for _, check := range checks.Rows {
		if check.Item.Group != "drivers" || check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Period != "annual" || !reflect.DeepEqual(check.Item.Target, target) || !strings.Contains(check.Item.Key, "|source/") {
			continue
		}
		if check.Quantity == nil || !check.Quantity.valid() || check.Want.Status != "" || check.OptionalPresentation {
			return fmt.Errorf("Pool surface generic anchor lost its mandatory original quantity")
		}
		copy := *check.Quantity
		if copy.Bounds != nil {
			bounds := *copy.Bounds
			copy.Bounds = &bounds
		}
		quantity, count = &copy, count+1
	}
	if count != 1 {
		return fmt.Errorf("Pool surface needs exactly one existing independent generic raw source anchor")
	}
	key := "pool_surface_source/" + source.Key + "/" + source.Name + "/Monthly/rawValue"
	if err := checks.add("drivers", "building", "", "annual", key, "kWh", quantity, target, "", nil, nil); err != nil {
		return err
	}
	checks.Rows[len(checks.Rows)-1].PoolSurface = proof
	return nil
}

// Root adds PoolSurface to epathSQLModelCheck and calls this in evaluation AND
// mandatory coverage. The prefix rejects removal of the supplemental proof.
func epathSQLCheckPoolSurfaceConsumer(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	marked := strings.Contains(check.Item.Key, "|pool_surface_source/") || strings.Contains(check.Want.Key, "|pool_surface_source/")
	if check.PoolSurface == nil {
		if marked {
			return fmt.Errorf("required Pool surface source check lost its original qualification proof")
		}
		return nil
	}
	p := check.PoolSurface
	source := p.Sources[p.AuthorityID]
	key := "drivers|building||annual|pool_surface_source/" + source.Key + "/" + source.Name + "/Monthly/rawValue"
	target := epathRealOracleTarget{Collection: "sources", Field: "rawValue", SourceName: source.Name, SourceKey: source.Key, SourceUnit: source.Unit, Frequency: source.Frequency, Unit: "kWh"}
	if !marked || check.Item.Key != key || check.Want.Key != key || check.Item.Group != "drivers" || check.Want.Group != "drivers" || check.Item.Scope != "building" || check.Want.Scope != "building" || check.Item.Zone != "" || check.Want.Zone != "" || check.Item.Period != "annual" || check.Want.Period != "annual" || check.Item.Unit != "kWh" || check.Want.Unit != "kWh" || check.Want.Status != "" || check.OptionalPresentation || check.Quantity == nil || !check.Quantity.valid() || check.Want.Value == nil || *check.Want.Value != check.Quantity.Value || !reflect.DeepEqual(check.Item.Target, target) {
		return fmt.Errorf("Pool surface proof escaped its exact mandatory generic source quantity and selector")
	}
	return epathSQLCheckPoolSurfaceQualification(bundle.EnergyExplanation.Sources, p)
}
