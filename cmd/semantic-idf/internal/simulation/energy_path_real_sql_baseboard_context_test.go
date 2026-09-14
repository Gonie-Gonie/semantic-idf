package simulation

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathRealSQLBaseboardContext struct {
	ZoneName  string               `json:"zoneName"`
	OwnerName string               `json:"ownerName"`
	Service   string               `json:"service"`
	Frequency string               `json:"frequency"`
	Source    epathRealSQLSelector `json:"source"`
}

type epathSQLBaseboardContextIdentity struct {
	ZoneName, OwnerName, Service string
	Source                       epathRealSQLSource
	Precision                    epathRealSQLPrecision
	ObjectIndex                  *int
	RequestBound                 bool
	RecipientQualification       *epathSQLBaseboardRecipientQualification
	// Wire values use the explicit per-reported-row 0.001 kWh transport
	// grid. Source above retains independent unrounded native SQL sums.
	ReportedScalar float64
	HourlyLabels   []string
	HourlyValues   []float64
}

func epathSQLBaseboardContextClass(name, frequency string) (unit, category, component string, effective bool, err error) {
	if frequency != "Monthly" && frequency != "Hourly" {
		return "", "", "", false, fmt.Errorf("baseboard context requires actual Monthly/Hourly frequency")
	}
	switch name {
	case "Baseboard Total Heating Energy":
		return "J", "load.heating", "load.baseboard_response.combined", true, nil
	case "Baseboard Total Heating Rate":
		return "W", "load.heating", "load.baseboard_response.combined", true, nil
	case "Baseboard Electricity Rate":
		return "W", "energy.heating", "energy.baseboard_electricity_rate", true, nil
	case "Baseboard Electricity Energy":
		if frequency == "Hourly" {
			return "J", "", "", false, nil
		}
	}
	return "", "", "", false, fmt.Errorf("unreviewed native baseboard context name/frequency; Monthly Electricity Energy is direct only")
}

func epathSQLBaseboardContextOpener(doc idf.Document, plan *PurposeRunPlan, declaration epathRealSQLBaseboardContext, executed ...idf.Document) (*int, error) {
	if plan == nil || plan.BasicEnergyDetail != "energy_path" || len(executed) > 1 {
		return nil, fmt.Errorf("baseboard source proof requires the actual Energy Path plan")
	}
	name := declaration.Source.Alternatives[0].Name
	monthly, actual := 0, 0
	exactIndices, wildcardIndices := map[int]bool{}, map[int]bool{}
	for _, output := range plan.OutputObjects {
		if !strings.EqualFold(output.ObjectType, "Output:Variable") || !strings.EqualFold(output.VariableName, name) {
			continue
		}
		key := strings.TrimSpace(output.KeyValue)
		exact := strings.EqualFold(key, declaration.OwnerName)
		wildcard := key == "" || key == "*"
		if !exact && !wildcard {
			continue
		}
		basic := false
		for _, purpose := range output.PurposeIDs {
			basic = basic || purpose == "basic_energy"
		}
		if exact && (output.ReportingFrequency == "Monthly" || output.ReportingFrequency == declaration.Frequency) {
			if !basic || !strings.EqualFold(output.ScopeZoneName, declaration.ZoneName) {
				return nil, fmt.Errorf("baseboard actual request has contradictory owner/purpose")
			}
			if output.ReportingFrequency == "Monthly" {
				monthly++
			}
			if output.ReportingFrequency == declaration.Frequency {
				actual++
			}
		}
		if output.ReportingFrequency != declaration.Frequency || !basic || output.ObjectIndex == nil {
			continue
		}
		if exact {
			exactIndices[*output.ObjectIndex] = true
		} else if output.ScopeZoneName == "" || strings.EqualFold(output.ScopeZoneName, declaration.ZoneName) {
			wildcardIndices[*output.ObjectIndex] = true
		}
	}
	if monthly != 1 || actual != 1 {
		return nil, fmt.Errorf("baseboard context requires one exact actual-frequency and Monthly intent request")
	}
	originalExact, originalWildcard := 0, 0
	for _, object := range doc.Objects {
		if !strings.EqualFold(object.Type, "Output:Variable") || !strings.EqualFold(epathSQLBaseboardField(object, 1), name) || !strings.EqualFold(epathSQLBaseboardField(object, 2), declaration.Frequency) {
			continue
		}
		key := epathSQLBaseboardField(object, 0)
		if strings.EqualFold(key, declaration.OwnerName) {
			originalExact++
		} else if key == "" || key == "*" {
			originalWildcard++
		}
	}
	if len(executed) == 1 {
		// A capture plan describes temporary requests before application; those
		// entries legitimately have no index. Navigation in LoadProjection opens
		// the separately hash-bound executed model, where the exact requests are
		// now real objects. The original census still proves preserved ambiguity.
		runExact, runWildcard := map[int]bool{}, map[int]bool{}
		for _, object := range executed[0].Objects {
			if !strings.EqualFold(object.Type, "Output:Variable") || !strings.EqualFold(epathSQLBaseboardField(object, 1), name) || !strings.EqualFold(epathSQLBaseboardField(object, 2), declaration.Frequency) {
				continue
			}
			key := epathSQLBaseboardField(object, 0)
			if strings.EqualFold(key, declaration.OwnerName) {
				runExact[object.Index] = true
			} else if key == "" || key == "*" {
				runWildcard[object.Index] = true
			}
		}
		if len(runExact) < originalExact || len(runWildcard) < originalWildcard {
			return nil, fmt.Errorf("executed baseboard opener lost an untouched original output identity")
		}
		for index := range exactIndices {
			if !runExact[index] {
				return nil, fmt.Errorf("baseboard actual-frequency exact plan index is not its executed output")
			}
		}
		for index := range wildcardIndices {
			if !runWildcard[index] {
				return nil, fmt.Errorf("baseboard actual-frequency wildcard plan index is not its executed output")
			}
		}
		indices := runExact
		if len(indices) == 0 {
			indices = runWildcard
		}
		if len(indices) == 0 {
			return nil, fmt.Errorf("baseboard actual-frequency request has no executed output identity")
		}
		if len(indices) != 1 {
			return nil, nil
		}
		for index := range indices {
			copy := index
			return &copy, nil
		}
	}
	// Census establishes ambiguity; only the actual executed plan establishes
	// an opener number, because annualization can move original object indexes.
	indices := exactIndices
	originalCount := originalExact
	if originalCount == 0 && len(indices) == 0 {
		indices = wildcardIndices
		originalCount = originalWildcard
	}
	if originalCount > 1 || len(indices) > 1 {
		return nil, nil
	}
	if originalCount == 1 && len(indices) == 0 {
		return nil, fmt.Errorf("unique original-only baseboard opener lacks an executed index proof")
	}
	if originalCount == 0 && len(indices) > 0 {
		return nil, fmt.Errorf("baseboard actual plan opener has no preserved original output identity")
	}
	for index := range indices {
		copy := index
		return &copy, nil
	}
	return nil, nil
}

func epathCompileSQLBaseboardContexts(sqlPath, originalText string, plan *PurposeRunPlan, observed []epathRealSQLSource, model epathRealSQLModel, frames *epathSQLFrames, executedText ...string) error {
	if len(model.BaseboardContexts) == 0 {
		return nil
	}
	if frames == nil || len(frames.BaseboardContextIdentities) > 0 || strings.TrimSpace(originalText) == "" {
		return fmt.Errorf("baseboard contexts require fresh frames and hash-bound original text")
	}
	doc, err := idf.Parse(originalText)
	if err != nil {
		return err
	}
	if len(executedText) > 1 || model.Surface.Mapping != "" && (len(executedText) != 1 || strings.TrimSpace(executedText[0]) == "") {
		return fmt.Errorf("real baseboard recipient trace requires the separately hash-bound executed input")
	}
	runDoc := doc
	if len(executedText) == 1 && strings.TrimSpace(executedText[0]) != "" {
		runDoc, err = idf.Parse(executedText[0])
		if err != nil {
			return err
		}
	}
	recipients, err := epathSQLCompileBaseboardRecipientQualification(doc, runDoc, observed, model.BaseboardContexts)
	if err != nil {
		return err
	}
	db, err := epathOpenOracleSQL(sqlPath)
	if err != nil {
		return err
	}
	defer db.Close()
	identities := map[int]epathSQLBaseboardContextIdentity{}
	seen := map[string]bool{}
	for _, declaration := range model.BaseboardContexts {
		if declaration.Service != "heating" || declaration.Source.IsMeter || declaration.Source.AllowAbsent || len(declaration.Source.Keys) != 1 || len(declaration.Source.Alternatives) != 1 || declaration.Source.Keys[0] != declaration.OwnerName {
			return fmt.Errorf("baseboard context must declare one exact native observed equipment source")
		}
		alternative := declaration.Source.Alternatives[0]
		unit, _, _, _, err := epathSQLBaseboardContextClass(alternative.Name, declaration.Frequency)
		if err != nil || alternative.Unit != unit {
			return fmt.Errorf("invalid native context unit/frequency: %v", err)
		}
		if err := epathSQLValidateNativeBaseboardOriginalOwner(doc, epathSQLBaseboardOwner(declaration.ZoneName, declaration.OwnerName)); err != nil {
			return err
		}
		zone, ok := frames.Zones[strings.ToLower(declaration.ZoneName)]
		if !ok || zone.Name != declaration.ZoneName {
			return fmt.Errorf("native context has no independently observed exact SQL Zone")
		}
		if alternative.Name == "Baseboard Electricity Energy" {
			companions := 0
			for _, direct := range frames.DirectHVACSourceIdentities {
				if direct.FamilyID != "heating.baseboard.electricity" || !strings.EqualFold(direct.Owner.KeyValue, declaration.OwnerName) || !strings.EqualFold(direct.Owner.ZoneName, declaration.ZoneName) {
					continue
				}
				if err := epathSQLValidateDirectHVACSourceIdentity(direct); err != nil {
					return err
				}
				companions++
			}
			if companions != 1 {
				return fmt.Errorf("Hourly baseboard electricity requires its independently observed unique Monthly direct counterpart")
			}
		}
		identityKey := strings.ToLower(declaration.OwnerName) + "|" + alternative.Name + "|" + declaration.Frequency
		if seen[identityKey] {
			return fmt.Errorf("duplicate native baseboard context declaration")
		}
		seen[identityKey] = true
		var openerDocuments []idf.Document
		if len(executedText) == 1 && strings.TrimSpace(executedText[0]) != "" {
			openerDocuments = append(openerDocuments, runDoc)
		}
		index, err := epathSQLBaseboardContextOpener(doc, plan, declaration, openerDocuments...)
		if err != nil {
			return err
		}
		var selected epathRealSQLSource
		count := 0
		for _, source := range observed {
			if strings.EqualFold(source.Name, alternative.Name) && strings.EqualFold(source.KeyValue, declaration.OwnerName) && strings.EqualFold(source.ReportingFrequency, declaration.Frequency) {
				selected = source
				count++
			}
		}
		if count != 1 || selected.IsMeter || selected.SourceUnit != unit || selected.MissingRows != 0 || selected.EnergyKWh == nil || selected.RawSum == nil {
			return fmt.Errorf("baseboard context missing/duplicate/NULL native SQL observation")
		}
		var dictionaryCount, dictionaryID int
		if err := db.QueryRow(`SELECT COUNT(*),COALESCE(MIN(ReportDataDictionaryIndex),0) FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE AND KeyValue=? COLLATE NOCASE AND ReportingFrequency=? COLLATE NOCASE`, alternative.Name, declaration.OwnerName, declaration.Frequency).Scan(&dictionaryCount, &dictionaryID); err != nil {
			return err
		}
		if dictionaryCount != 1 || dictionaryID != selected.DictionaryIndex {
			return fmt.Errorf("baseboard native dictionary-only duplicates cannot be hidden by absent ReportData")
		}
		identity := epathSQLBaseboardContextIdentity{ZoneName: declaration.ZoneName, OwnerName: declaration.OwnerName, Service: declaration.Service, Source: selected, Precision: model.Precision, ObjectIndex: index, RequestBound: true, RecipientQualification: recipients}
		if err := epathSQLReadBaseboardContextRows(db, &identity); err != nil {
			return err
		}
		if _, exists := frames.SourceIdentities[selected.DictionaryIndex]; exists {
			return fmt.Errorf("native context is already registered as an additive source authority")
		}
		identities[selected.DictionaryIndex] = identity
	}
	// No load, pressure, direct consumption or generic source frames receive
	// these records. Their only consumer is the separate inspector proof.
	frames.BaseboardContextIdentities = identities
	return nil
}

func epathSQLReadBaseboardContextRows(db *sql.DB, identity *epathSQLBaseboardContextIdentity) error {
	source := identity.Source
	expectedRows, expectedIntervalType := 12, 3
	if source.ReportingFrequency == "Hourly" {
		expectedRows, expectedIntervalType = 8760, 1
	}
	var environments, timeRows, distinctTimes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM EnvironmentPeriods WHERE EnvironmentType=3`).Scan(&environments); err != nil {
		return err
	}
	if environments != 1 {
		return fmt.Errorf("native baseboard context has ambiguous weather environments")
	}
	if err := db.QueryRow(`SELECT COUNT(*),COUNT(DISTINCT t.TimeIndex) FROM "Time" t JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex WHERE e.EnvironmentType=3 AND t.IntervalType=? AND `+epathOracleNonWarmupSQL, expectedIntervalType).Scan(&timeRows, &distinctTimes); err != nil {
		return err
	}
	if timeRows != expectedRows || distinctTimes != expectedRows {
		return fmt.Errorf("native baseboard source lacks an exact original shared Time axis")
	}
	rows, err := db.Query(`SELECT t.TimeIndex,t.EnvironmentPeriodIndex,t.Year,t.Month,t.Day,t.Hour,t.Minute,t."Interval",t.IntervalType,r.Value
FROM ReportData r JOIN "Time" t ON t.TimeIndex=r.TimeIndex JOIN EnvironmentPeriods e ON e.EnvironmentPeriodIndex=t.EnvironmentPeriodIndex
WHERE r.ReportDataDictionaryIndex=? AND e.EnvironmentType=3 AND `+epathOracleNonWarmupSQL+` ORDER BY t.TimeIndex,r.ReportDataIndex`, source.DictionaryIndex)
	if err != nil {
		return err
	}
	defer rows.Close()
	count, environment := 0, 0
	var last int64
	rawSum, energySum, wireSum := 0.0, 0.0, 0.0
	for rows.Next() {
		var index int64
		var env, year, month, day, hour, minute, intervalType int
		var interval, value sql.NullFloat64
		if err := rows.Scan(&index, &env, &year, &month, &day, &hour, &minute, &interval, &intervalType, &value); err != nil {
			return err
		}
		if index <= last || year != 2017 || !interval.Valid || interval.Float64 <= 0 || !value.Valid || !epathOracleFinite(value.Float64) || !epathOracleFinite(interval.Float64) || env <= 0 || environment != 0 && env != environment {
			return fmt.Errorf("invalid/duplicate/NULL baseboard native observation or weather axis")
		}
		if strings.HasPrefix(source.Name, "Baseboard Electricity ") && value.Float64 < 0 {
			return fmt.Errorf("native baseboard electricity cannot be a negative consumption observation")
		}
		last, environment = index, env
		if source.ReportingFrequency == "Monthly" {
			lastDay := time.Date(2017, time.Month(count+2), 0, 0, 0, 0, 0, time.UTC)
			if count >= 12 || intervalType != 3 || month != count+1 || day != lastDay.Day() || hour != 24 || minute != 0 || interval.Float64 != float64(lastDay.Day()*1440) {
				return fmt.Errorf("native baseboard Monthly row does not cover exact original calendar month")
			}
		} else {
			stamp := time.Date(2017, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(count) * time.Hour)
			if count >= 8760 || intervalType != 1 || month != int(stamp.Month()) || day != stamp.Day() || hour != stamp.Hour()+1 || minute != 0 || interval.Float64 != 60 {
				return fmt.Errorf("native baseboard Hourly observation lost its full 8760-row shared weather axis")
			}
			identity.HourlyLabels = append(identity.HourlyLabels, fmt.Sprintf("%02d-%02d %02d:00", month, day, hour))
		}
		energy := value.Float64 * (1.0 / 3600000)
		if source.SourceUnit == "W" {
			energy = value.Float64 * (interval.Float64 / 60) / 1000
		}
		if !epathOracleFinite(energy) {
			return fmt.Errorf("native baseboard conversion is not finite")
		}
		wire := math.Round(energy*1000) / 1000
		if source.ReportingFrequency == "Hourly" {
			identity.HourlyValues = append(identity.HourlyValues, wire)
		}
		rawSum += value.Float64
		energySum += energy
		wireSum += wire
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != expectedRows || source.Rows != count || source.MissingRows != 0 || source.RawSum == nil || source.EnergyKWh == nil {
		return fmt.Errorf("baseboard context has incomplete native observations, never measured zero")
	}
	// Independent observation aggregation may divide J rather than multiply
	// its conversion factor. Permit binary arithmetic roundoff, not rounding.
	close := func(a, b float64) bool {
		return epathOracleFinite(a) && epathOracleFinite(b) && math.Abs(a-b) <= float64(count)*4*(math.Nextafter(math.Max(1, math.Abs(a)), math.Inf(1))-math.Max(1, math.Abs(a)))
	}
	if !close(rawSum, *source.RawSum) || !close(energySum, *source.EnergyKWh) {
		return fmt.Errorf("baseboard context row proof differs from independently collected native SQL sums")
	}
	identity.ReportedScalar = math.Round(wireSum*1000) / 1000
	return epathSQLValidateBaseboardContextIdentity(*identity)
}

func epathSQLValidateBaseboardContextIdentity(identity epathSQLBaseboardContextIdentity) error {
	if identity.RecipientQualification == nil {
		return fmt.Errorf("baseboard source lost independently bound recipient metadata proof")
	}
	source := identity.Source
	unit, _, _, _, err := epathSQLBaseboardContextClass(source.Name, source.ReportingFrequency)
	if err != nil || !identity.RequestBound || identity.Service != "heating" || identity.ZoneName == "" || identity.OwnerName == "" || !strings.EqualFold(identity.OwnerName, source.KeyValue) || source.SourceUnit != unit || source.IsMeter || source.DictionaryIndex <= 0 || source.MissingRows != 0 || source.RawSum == nil || source.EnergyKWh == nil || !epathOracleFinite(*source.RawSum) || !epathOracleFinite(*source.EnergyKWh) || !epathOracleFinite(identity.ReportedScalar) || identity.Precision.DecimalPlaces != 3 {
		return fmt.Errorf("baseboard context lacks exact original owner/request/native observation proof")
	}
	want := 12
	if source.ReportingFrequency == "Hourly" {
		want = 8760
	}
	if source.Rows != want {
		return fmt.Errorf("baseboard context native row count is incomplete")
	}
	if source.ReportingFrequency == "Monthly" && (len(identity.HourlyLabels) != 0 || len(identity.HourlyValues) != 0) {
		return fmt.Errorf("Monthly context fabricated an Hourly chart")
	}
	if source.ReportingFrequency == "Hourly" {
		if len(identity.HourlyLabels) != 8760 || len(identity.HourlyValues) != 8760 {
			return fmt.Errorf("baseboard context Hourly chart is not native/full")
		}
		for _, value := range identity.HourlyValues {
			if !epathOracleFinite(value) {
				return fmt.Errorf("baseboard context chart contains unknown/nonfinite values")
			}
		}
	}
	return nil
}

func epathSQLModelBaseboardContextSourceChecks(frames epathSQLFrames, checks *epathSQLModelChecks) error {
	ids := []int{}
	for id := range frames.BaseboardContextIdentities {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		identity := frames.BaseboardContextIdentities[id]
		if err := epathSQLValidateBaseboardContextIdentity(identity); err != nil {
			return err
		}
		if id != identity.Source.DictionaryIndex {
			return fmt.Errorf("baseboard context source identity changed")
		}
		_, _, _, effective, _ := epathSQLBaseboardContextClass(identity.Source.Name, identity.Source.ReportingFrequency)
		fields := []string{"rawValue"}
		if effective {
			fields = append(fields, "effectiveValue")
		}
		for _, scope := range []string{"building", "zone"} {
			zone := ""
			if scope == "zone" {
				zone = identity.ZoneName
			}
			for _, field := range fields {
				q := epathSQLQuantity{Value: identity.ReportedScalar}
				target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: identity.Source.Name, SourceKey: identity.Source.KeyValue, SourceUnit: identity.Source.SourceUnit, Frequency: identity.Source.ReportingFrequency, Unit: "kWh"}
				key := "baseboard_context/" + identity.OwnerName + "/" + identity.Source.Name + "/" + identity.Source.ReportingFrequency + "/" + field
				if err := checks.add("loads", scope, zone, "annual", key, "kWh", &q, target, "", nil, nil); err != nil {
					return err
				}
				copy := identity
				checks.Rows[len(checks.Rows)-1].BaseboardContextSource = &copy
			}
		}
	}
	return nil
}

func epathCheckSQLBaseboardContextSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	identity := check.BaseboardContextSource
	if identity == nil {
		return fmt.Errorf("missing independent native baseboard context source proof")
	}
	if err := epathSQLValidateBaseboardContextIdentity(*identity); err != nil {
		return err
	}
	if err := epathSQLCheckBaseboardRecipientQualification(bundle.EnergyExplanation.Sources, identity.RecipientQualification); err != nil {
		return err
	}
	target := check.Item.Target
	unit, category, component, effective, _ := epathSQLBaseboardContextClass(identity.Source.Name, identity.Source.ReportingFrequency)
	if check.Item.Group != "loads" || check.Item.Period != "annual" || check.Item.Unit != "kWh" || check.Quantity == nil || *check.Quantity != (epathSQLQuantity{Value: identity.ReportedScalar}) || target.Collection != "sources" || target.Field != "rawValue" && target.Field != "effectiveValue" || target.Field == "effectiveValue" && !effective || target.SourceName != identity.Source.Name || target.SourceKey != identity.Source.KeyValue || target.SourceUnit != unit || target.Frequency != identity.Source.ReportingFrequency || target.Unit != "kWh" || check.Item.Scope != "building" && check.Item.Scope != "zone" || check.Item.Scope == "building" && check.Item.Zone != "" || check.Item.Scope == "zone" && !strings.EqualFold(check.Item.Zone, identity.ZoneName) {
		return fmt.Errorf("baseboard context proof escaped its exact source/owner/quantity target")
	}
	id := fmt.Sprintf("sql-rdd-%d", identity.Source.DictionaryIndex)
	count := 0
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID != id && !(strings.EqualFold(source.Name, identity.Source.Name) && strings.EqualFold(source.KeyValue, identity.Source.KeyValue) && source.ReportingFrequency == identity.Source.ReportingFrequency) {
			continue
		}
		count++
		method := "sum_report_data"
		if unit == "W" {
			method = "integrate_rate_by_time_interval"
		}
		if !reflect.DeepEqual(source.ObjectIndex, identity.ObjectIndex) {
			actual, expected := -1, -1
			if source.ObjectIndex != nil {
				actual = *source.ObjectIndex
			}
			if identity.ObjectIndex != nil {
				expected = *identity.ObjectIndex
			}
			return fmt.Errorf("native baseboard source %s %s opener=%d, independently bound executed opener=%d (-1 is unknown)", id, identity.Source.ReportingFrequency, actual, expected)
		}
		if source.ID != id || source.SourceType != "sql_report_data" || source.Name != identity.Source.Name || !strings.EqualFold(source.KeyValue, identity.OwnerName) || !strings.EqualFold(source.ZoneName, identity.ZoneName) || source.IsMeter || source.Units != unit || source.SourceUnit != unit || source.NormalizedUnit != "kWh" || source.ReportingFrequency != identity.Source.ReportingFrequency || source.AggregationMethod != method || source.AggregationBasis != "model_total" || len(source.InputSourceIDs) != 0 || source.Formula != "" || source.AllocationApplied {
			return fmt.Errorf("native baseboard source lost original identity/owner/frequency/opener/nonadditivity")
		}
		if !source.inspectorDecodedFromJSON || source.inspectorValuePresence&1 == 0 || source.RawValue != identity.ReportedScalar {
			return fmt.Errorf("baseboard native scalar is absent or differs from independently calculated row-quantized transport")
		}
		if effective {
			if source.inspectorValuePresence&2 == 0 || source.EffectiveValue != identity.ReportedScalar || source.DriverRole != "context" || source.InspectorSection != "Context" || source.DriverCategory != category || source.DriverComponent != component || source.HeatDirection != "heating" || source.EffectiveMultiplier != 1 || source.MultiplierApplication != "already_model_total" {
				return fmt.Errorf("baseboard context effective source changed signed value, known zero, classification or multiplier")
			}
		} else if source.inspectorValuePresence&2 == 0 || source.EffectiveValue != identity.ReportedScalar || source.EffectiveMultiplier != 1 || source.MultiplierApplication != "already_model_total" {
			// The full projection normalizes this observed component source to
			// model total even though the Hourly series is never allocated. That
			// measured effective scalar is not an effective allocation amount.
			return fmt.Errorf("Hourly direct companion lost known native model-total scalar or multiplied it twice")
		}
		if identity.Source.ReportingFrequency == "Hourly" {
			if source.HourlyEnergy == nil || source.HourlyEnergy.Unit != "kWh" || source.HourlyEnergy.Basis != "reported_source" || !reflect.DeepEqual(source.HourlyEnergy.Values, identity.HourlyValues) || !reflect.DeepEqual(bundle.EnergyExplanation.HourlyLabels, identity.HourlyLabels) {
				return fmt.Errorf("baseboard native Hourly chart lost exact original row values or shared axis")
			}
		} else if source.HourlyEnergy != nil {
			return fmt.Errorf("Monthly context source fabricated Hourly values")
		}
	}
	if count != 1 {
		return fmt.Errorf("baseboard context needs exactly one original candidate source identity")
	}
	// Context is allowed in the inspector only. Follow source dependencies to
	// ensure a renamed derived source cannot smuggle it into a primary graph.
	sources := map[string]EnergyDataSource{}
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, duplicate := sources[source.ID]; duplicate {
			return fmt.Errorf("duplicate candidate source identity")
		}
		sources[source.ID] = source
	}
	var reaches func(string, map[string]bool) bool
	reaches = func(current string, seen map[string]bool) bool {
		if current == id {
			return true
		}
		if seen[current] {
			return false
		}
		seen[current] = true
		for _, parent := range sources[current].InputSourceIDs {
			if reaches(parent, seen) {
				return true
			}
		}
		return false
	}
	coverage := epathSQLModelCoverageReport{}
	for _, context := range epathSQLCoverageContexts(bundle, &coverage) {
		for _, node := range context.nodes {
			for _, sourceID := range node.SourceIDs {
				if reaches(sourceID, map[string]bool{}) {
					return fmt.Errorf("non-additive baseboard context became a graph node input")
				}
			}
		}
		for _, link := range context.links {
			for _, sourceID := range link.SourceIDs {
				if reaches(sourceID, map[string]bool{}) {
					return fmt.Errorf("non-additive baseboard context became a graph link input")
				}
			}
		}
	}
	return nil
}
