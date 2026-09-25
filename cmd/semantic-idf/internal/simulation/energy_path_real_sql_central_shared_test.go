package simulation

// Finite extension of the existing shared-consumption oracle, not a new source
// registry. All expected roles come from literal native names and the separate
// original/executed Central proof; production resolvers/math are not called.
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

const epathSQLCentralSharedType = "CentralHeatPumpSystem"
const epathSQLCentralCoolingElectricity = "Chiller Heater System Cooling Electricity Energy"
const epathSQLCentralHeatingElectricity = "Chiller Heater System Heating Electricity Energy"

func epathSQLCentralSharedRole(m epathRealSQLHVACSharedMember) (string, error) {
	if strings.TrimSpace(m.ID) == "" || m.ObjectType != epathSQLCentralSharedType || strings.TrimSpace(m.ObjectName) == "" || strings.TrimSpace(m.PlantLoopName) == "" || m.Source.IsMeter || m.Source.AllowAbsent || len(m.Source.Keys) != 1 || !strings.EqualFold(strings.TrimSpace(m.Source.Keys[0]), m.ObjectName) || len(m.Source.Alternatives) != 1 || m.Source.Alternatives[0].Unit != "J" {
		return "", fmt.Errorf("Central shared source requires one exact original system and native Monthly/J selector")
	}
	role := ""
	switch m.Source.Alternatives[0].Name {
	case epathSQLCentralCoolingElectricity:
		role = "cooling"
	case epathSQLCentralHeatingElectricity:
		role = "heating"
	default:
		return "", fmt.Errorf("Central shared source is not one of the two paid system electricity energies")
	}
	loop := "Chilled Water Loop"
	if role == "heating" {
		loop = "Hot Water Loop"
	}
	// Retained role consistency is finite to the separately validated original;
	// these literal names never replace the native port/recipient proof.
	if !strings.EqualFold(m.ObjectName, "ChillerBank") || !strings.EqualFold(m.PlantLoopName, loop) {
		return "", fmt.Errorf("Central retained source changed its original system/circuit role")
	}
	if !epathSQLCentralSharedZoneSet(m.ServedZones) {
		return "", fmt.Errorf("Central shared source requires the complete five original recipients")
	}
	return role, nil
}

func epathSQLCentralSharedZoneSet(zones []string) bool {
	if len(zones) != 5 {
		return false
	}
	keys := make([]string, len(zones))
	for n, zone := range zones {
		keys[n] = strings.ToLower(strings.TrimSpace(zone))
	}
	sort.Strings(keys)
	return reflect.DeepEqual(keys, []string{"space1-1", "space2-1", "space3-1", "space4-1", "space5-1"})
}

// Existing Boiler-only callers need no executed text. Central is mandatory
// whenever the independently loaded original contains the native system, even
// when all pool declarations were deleted. The existing finite binder checks
// every physical/control field and permits only output/run-control differences.
func epathSQLCentralSharedOriginal(text string, pools []epathRealSQLHVACConsumptionPool, executed ...string) (idf.Document, idf.Document, error) {
	possible := strings.Contains(strings.ToLower(text), "centralheatpumpsystem")
	for _, pool := range pools {
		for _, member := range pool.Shared {
			possible = possible || member.ObjectType == epathSQLCentralSharedType
		}
	}
	if !possible {
		return idf.Document{}, idf.Document{}, nil
	}
	doc, err := idf.Parse(text)
	if err != nil {
		return doc, idf.Document{}, err
	}
	central := false
	for _, object := range doc.Objects {
		central = central || strings.EqualFold(object.Type, epathSQLCentralSharedType)
	}
	for _, pool := range pools {
		for _, member := range pool.Shared {
			central = central || member.ObjectType == epathSQLCentralSharedType
		}
	}
	if !central {
		return doc, idf.Document{}, nil
	}
	proof, err := epathSQLValidateCentralOriginal(text)
	if err != nil {
		return doc, idf.Document{}, err
	}
	if len(executed) != 1 || executed[0] == "" {
		return doc, idf.Document{}, fmt.Errorf("Central shared sources require the separately bound executed original")
	}
	if _, err := epathSQLBindCentralExecuted(text, executed[0], proof); err != nil {
		return doc, idf.Document{}, err
	}
	run, err := idf.Parse(executed[0])
	if err != nil {
		return doc, run, err
	}
	roles, ids := map[string]bool{}, map[string]bool{}
	for _, pool := range pools {
		for _, member := range pool.Shared {
			if member.ObjectType != epathSQLCentralSharedType {
				continue
			}
			role, err := epathSQLCentralSharedRole(member)
			if err != nil {
				return doc, run, err
			}
			loop := proof.CoolingLoop
			if role == "heating" {
				loop = proof.HeatingLoop
			}
			if pool.SiteID == "" || len(pool.Shared) != 1 || roles[role] || ids[member.ID] || !strings.EqualFold(member.ObjectName, epathSQLSharedField(doc.Objects[proof.System], 0)) || !strings.EqualFold(member.PlantLoopName, epathSQLSharedField(doc.Objects[loop], 0)) {
				return doc, run, fmt.Errorf("Central shared declaration swapped/reused native system, circuit, or paid role")
			}
			roles[role], ids[member.ID] = true, true
		}
	}
	if !roles["cooling"] || !roles["heating"] {
		return doc, run, fmt.Errorf("Central original requires both distinct paid service declarations")
	}
	return doc, run, nil
}

func epathSQLCentralSharedSite(member epathRealSQLHVACSharedMember, site epathRealSQLSite, frames epathSQLFrames) error {
	role, err := epathSQLCentralSharedRole(member)
	if err != nil {
		return err
	}
	name := "Cooling:Electricity"
	if role == "heating" {
		name = "Heating:Electricity"
	}
	if site.Facility || site.EndUse != role || site.Carrier != "electricity" || site.Tabular != nil || !site.Source.IsMeter || site.Source.AllowAbsent || len(site.Source.Alternatives) != 1 || site.Source.Alternatives[0] != (epathRealSQLAlternative{Name: name, Unit: "J"}) || len(frames.Site[site.ID]) != 12 || len(frames.SiteSources[site.ID]) != 1 {
		return fmt.Errorf("Central paid role requires its exact native Monthly electricity parent meter")
	}
	id := frames.SiteSources[site.ID][0]
	source, exists := frames.SourceIdentities[id]
	if !exists || id <= 0 || source.DictionaryIndex != id || !source.IsMeter || source.Name != name || source.SourceUnit != "J" || source.ReportingFrequency != "Monthly" {
		return fmt.Errorf("Central paid source borrowed another native parent meter")
	}
	values, err := epathSQLMonthly(source, epathRealSQLPrecision{DecimalPlaces: 3, SourceStages: 1, ContributionStages: 1})
	if err != nil {
		return err
	}
	for n, q := range values {
		if frames.Site[site.ID][n] == nil || !frames.Site[site.ID][n].valid() || !epathSQLFanPoolNear(frames.Site[site.ID][n].Value, q.Value) {
			return fmt.Errorf("Central parent month is absent or changed")
		}
	}
	return nil
}

// Add strict native dictionary metadata and orphan/row-identity checks around
// the existing independent 12-month calendar/value audit. Boiler audit remains
// byte-for-byte unchanged, and no Rate/Hourly companion is invented.
func epathSQLAuditCentralSharedObservation(db *sql.DB, source epathRealSQLSource, weather epathRealSQLWeather) error {
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,Name,KeyValue,Units,IsMeter,ReportingFrequency,Type,TimestepType,IndexGroup,ScheduleName FROM ReportDataDictionary WHERE ReportDataDictionaryIndex=? OR (LOWER(TRIM(Name))=LOWER(TRIM(?)) AND LOWER(TRIM(KeyValue))=LOWER(TRIM(?)) AND LOWER(TRIM(ReportingFrequency))='monthly')`, source.DictionaryIndex, source.Name, source.KeyValue)
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		var id, meter int
		var name, key, unit, frequency, kind, step, group string
		var schedule sql.NullString
		if err := rows.Scan(&id, &name, &key, &unit, &meter, &frequency, &kind, &step, &group, &schedule); err != nil {
			rows.Close()
			return err
		}
		count++
		if id != source.DictionaryIndex || name != source.Name || !strings.EqualFold(key, source.KeyValue) || unit != "J" || meter != 0 || frequency != "Monthly" || kind != "Sum" || step != "HVAC System" || group != "System" || strings.TrimSpace(schedule.String) != "" {
			rows.Close()
			return fmt.Errorf("Central paid dictionary is duplicate, filtered, or has a different native role")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("Central paid source requires exactly one native Monthly dictionary")
	}
	var total, distinct int
	var bad sql.NullInt64
	if err := db.QueryRow(`SELECT COUNT(*),COUNT(DISTINCT r.ReportDataIndex),SUM(CASE WHEN r.ReportDataIndex IS NULL OR r.ReportDataIndex<=0 OR t.TimeIndex IS NULL OR e.EnvironmentPeriodIndex IS NULL OR e.EnvironmentType IS NULL OR (e.EnvironmentType=3 AND t.WarmupFlag IS NOT NULL AND t.WarmupFlag NOT IN (0,1)) THEN 1 ELSE 0 END) FROM ReportData r LEFT JOIN "Time" t USING(TimeIndex) LEFT JOIN EnvironmentPeriods e USING(EnvironmentPeriodIndex) WHERE r.ReportDataDictionaryIndex=?`, source.DictionaryIndex).Scan(&total, &distinct, &bad); err != nil {
		return err
	}
	if total == 0 || total != distinct || !bad.Valid || bad.Int64 != 0 {
		return fmt.Errorf("Central paid rows have missing/duplicate/orphan native identities")
	}
	_, err = epathSQLAuditHVACSharedObservation(db, source, weather)
	return err
}

func epathSQLCompileCentralSharedMember(db *sql.DB, weather epathRealSQLWeather, member epathRealSQLHVACSharedMember, precision epathRealSQLPrecision, observed []epathRealSQLSource, original, executed idf.Document, plan *PurposeRunPlan) (epathSQLHVACSharedSourceIdentity, epathSQLHVACSharedSourceProof, error) {
	identity := epathSQLHVACSharedSourceIdentity{Member: member, Precision: precision}
	var proof epathSQLHVACSharedSourceProof
	if _, err := epathSQLCentralSharedRole(member); err != nil {
		return identity, proof, err
	}
	name := member.Source.Alternatives[0].Name
	index, err := epathSQLCentralSharedRequest(original, executed, plan, member)
	if err != nil {
		return identity, proof, err
	}
	count := 0
	for _, source := range observed {
		if strings.EqualFold(source.Name, name) && strings.EqualFold(source.KeyValue, member.ObjectName) && strings.EqualFold(source.ReportingFrequency, "Monthly") {
			identity.Source = source
			count++
		}
	}
	if count != 1 {
		return identity, proof, fmt.Errorf("Central paid role needs one observed native Monthly energy")
	}
	if err := epathSQLValidateHVACSharedSourceIdentity(identity); err != nil {
		return identity, proof, err
	}
	if err := epathSQLAuditCentralSharedObservation(db, identity.Source, weather); err != nil {
		return identity, proof, err
	}
	// Detach all declaration and native month pointers before registry retention.
	data, err := json.Marshal(identity)
	if err != nil {
		return identity, proof, err
	}
	var detached epathSQLHVACSharedSourceIdentity
	if err := json.Unmarshal(data, &detached); err != nil {
		return identity, proof, err
	}
	identity = detached
	proof = epathSQLHVACSharedSourceProof{Member: identity.Member, Source: identity.Source, Precision: precision, Canonical: true, ObjectIndex: index}
	return identity, proof, nil
}

func epathSQLCentralSharedCompilerDocuments(pools []epathRealSQLHVACConsumptionPool, texts []string) (idf.Document, idf.Document, error) {
	for _, pool := range pools {
		for _, member := range pool.Shared {
			if member.ObjectType == epathSQLCentralSharedType {
				if len(texts) != 2 {
					return idf.Document{}, idf.Document{}, fmt.Errorf("Central compiler needs independent original and executed text")
				}
				return epathSQLCentralSharedOriginal(texts[0], pools, texts[1])
			}
		}
	}
	return idf.Document{}, idf.Document{}, nil
}

func epathSQLCentralSharedRequest(original, executed idf.Document, plan *PurposeRunPlan, member epathRealSQLHVACSharedMember) (*int, error) {
	spec := epathSQLPVNativeSpec{Name: member.Source.Alternatives[0].Name, Key: member.ObjectName}
	if _, err := epathSQLPVRequestBinding(original, executed, plan, spec, "Monthly"); err != nil {
		return nil, err
	}
	fieldsOf := func(object idf.Object) []string {
		var fields []string
		for _, field := range object.Fields {
			fields = append(fields, strings.TrimSpace(field.Value))
		}
		if len(fields) == 4 && fields[3] == "" {
			fields = fields[:3]
		}
		return fields
	}
	count := 0
	var owner *int
	for _, output := range plan.OutputObjects {
		var fields []string
		for _, field := range output.Fields {
			fields = append(fields, strings.TrimSpace(field.Value))
		}
		if !epathSQLPVRequestCovers(output.ObjectType, fields, spec, "Monthly") {
			continue
		}
		if len(fields) == 4 && fields[3] == "" {
			fields = fields[:3]
		}
		count++
		runMatch, oldMatch := false, false
		for _, object := range executed.Objects {
			if strings.EqualFold(object.Type, "Output:Variable") && reflect.DeepEqual(fieldsOf(object), fields) {
				runMatch = true
			}
		}
		if output.State == "existing" && output.ObjectIndex != nil {
			for _, object := range original.Objects {
				if object.Index == *output.ObjectIndex && strings.EqualFold(object.Type, "Output:Variable") && reflect.DeepEqual(fieldsOf(object), fields) {
					oldMatch = true
				}
			}
			if !oldMatch {
				return nil, fmt.Errorf("Central source navigation does not name its actual literal original request")
			}
			copy := *output.ObjectIndex
			owner = &copy
		}
		if !runMatch {
			return nil, fmt.Errorf("Central source request is absent from the executed literal output roster")
		}
	}
	if count != 1 {
		return nil, fmt.Errorf("Central source requires one unfiltered actual plan request; scheduled decoys do not count")
	}
	return owner, nil
}

// The optional DirectHVAC constructor flag is enabled only for the reviewed
// shared-only Central service, never a missing Boiler/local source. Existing
// actual site/load validation and the complete source-local allocator run next.
func epathSQLCentralSharedOnlyService(frames epathSQLFrames, model epathRealSQLModel, service epathRealSQLService, selected []epathSQLHVACConsumptionPoolFrame) bool {
	if len(model.DirectHVACComponents) != 0 || len(frames.DirectHVAC) != 0 || len(frames.DirectHVACSourceIdentities) != 0 || len(selected) != 1 || len(selected[0].Shared) != 1 || len(service.SiteIDs) != 1 || service.SiteIDs[0] != selected[0].Declaration.SiteID || service.Basis != "service_path_allocation" || !epathSQLCentralSharedZoneSet(service.ServedZones) {
		return false
	}
	member := selected[0].Shared[0].Member
	role, err := epathSQLCentralSharedRole(member)
	if err != nil || role != service.Service {
		return false
	}
	for _, zone := range member.ServedZones {
		if frames.Zones[strings.ToLower(zone)].Name != zone || frames.Zones[strings.ToLower(zone)].Multiplier != 1 {
			return false
		}
	}
	for _, site := range model.Site {
		if site.ID == service.SiteIDs[0] {
			return epathSQLCentralSharedSite(member, site, frames) == nil
		}
	}
	return false
}
