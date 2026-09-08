package simulation

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathRealSQLRadiantSurfaceContext struct {
	Policy string `json:"policy"`
}

type epathSQLRadiantSurfaceContextIdentity struct {
	Owner          epathSQLRadiantOwnerProof
	Source         epathRealSQLSource
	Kind, Category string
	Precision      epathRealSQLPrecision
	Raw, Effective [12]epathSQLQuantity
}

type epathSQLRadiantSurfaceContextPlan struct {
	Identities  map[int]epathSQLRadiantSurfaceContextIdentity
	FamilyZones map[string]bool
}

// This optional declaration is not an allow-list: every active surface is
// independently bound to the original parent, SQL geometry, selected Monthly
// source and explicit H/C roster. Existing non-radiant declarations are no-op.
func epathSQLRadiantSurfaceContexts(db *sql.DB, observed []epathRealSQLSource, model epathRealSQLModel, zones map[string]epathSQLZone, surfaces map[string]epathSQLSurface, original []string) (epathSQLRadiantSurfaceContextPlan, error) {
	out := epathSQLRadiantSurfaceContextPlan{}
	native := 0
	for _, load := range model.Loads {
		if load.NativeRadiant != nil {
			native++
		}
	}
	if model.Surface.RadiantContext == nil && native == 0 {
		return out, nil
	}
	if model.Surface.RadiantContext == nil || model.Surface.RadiantContext.Policy != "original_direct_surface_context/v1" || native != 2 || len(original) != 1 || strings.TrimSpace(original[0]) == "" {
		return out, fmt.Errorf("active radiant surface context requires an explicit policy, both native services and original input")
	}
	var owners map[string]epathSQLRadiantOwnerProof
	services := map[string]bool{}
	for _, load := range model.Loads {
		if load.NativeRadiant == nil || services[load.Service] {
			return out, fmt.Errorf("radiant surface context cannot mix or duplicate load ownership authorities")
		}
		found, err := epathSQLRadiantLoadDeclaration(load, original[0])
		if err != nil {
			return out, err
		}
		if owners != nil && !reflect.DeepEqual(owners, found) {
			return out, fmt.Errorf("radiant H/C declarations disagree on original surface ownership")
		}
		owners, services[load.Service] = found, true
	}
	doc, err := idf.Parse(original[0])
	if err != nil {
		return out, err
	}
	bySurface, byZone := map[string]epathSQLRadiantOwnerProof{}, map[string]epathSQLRadiantOwnerProof{}
	for _, owner := range owners {
		zoneKey, surfaceKey := strings.ToLower(owner.Owner.ZoneName), strings.ToLower(owner.Owner.SurfaceName)
		surface, known := surfaces[surfaceKey]
		if !known || surface.Zone != zoneKey || zones[zoneKey].Name == "" || zones[zoneKey].Multiplier != owner.ZoneMultiplier*owner.ZoneListMultiplier {
			return out, fmt.Errorf("active radiant surface has missing/foreign SQL geometry or contradictory multiplier")
		}
		if err := epathSQLRadiantSurfaceGeometry(db, doc, owner); err != nil {
			return out, err
		}
		bySurface[surfaceKey], byZone[zoneKey] = owner, owner
	}
	out.Identities, out.FamilyZones = map[int]epathSQLRadiantSurfaceContextIdentity{}, map[string]bool{}
	register := func(source epathRealSQLSource, owner epathSQLRadiantOwnerProof, kind, category string) error {
		if _, exists := out.Identities[source.DictionaryIndex]; exists {
			return fmt.Errorf("radiant context source was declared twice")
		}
		identity := epathSQLRadiantSurfaceContextIdentity{Owner: owner, Source: source, Kind: kind, Category: category, Precision: model.Precision}
		months, err := epathSQLMonthly(source, model.Precision)
		if err != nil {
			return err
		}
		for month, q := range months {
			identity.Raw[month], identity.Effective[month] = q, q.times(owner.ZoneMultiplier*owner.ZoneListMultiplier)
		}
		if err := epathSQLValidateRadiantSurfaceContextIdentity(identity); err != nil {
			return err
		}
		out.Identities[source.DictionaryIndex] = identity
		return nil
	}
	selected, err := epathSQLSelect(observed, model.Surface.Source)
	if err != nil {
		return out, err
	}
	seenSurfaces := map[string]bool{}
	for _, source := range selected {
		key := strings.ToLower(source.KeyValue)
		if owner, active := bySurface[key]; active {
			if err := register(source, owner, "inside_face", surfaces[key].Category); err != nil {
				return out, err
			}
			seenSurfaces[key] = true
		}
	}
	if len(seenSurfaces) != len(bySurface) {
		return out, fmt.Errorf("active surface observation is missing, not zero or a passive surface")
	}
	for _, family := range model.Families {
		for _, term := range family.Terms {
			selector := term.Source
			if len(selector.Keys) == 0 {
				selector.Keys = family.Keys
			}
			selector.AllowAbsent = false
			sources, err := epathSQLSelect(observed, selector)
			if err != nil {
				return out, err
			}
			for _, source := range sources {
				if _, already := out.Identities[source.DictionaryIndex]; already {
					return out, fmt.Errorf("context observation cannot be reused as another declared driver term")
				}
				zoneKey := strings.ToLower(source.KeyValue)
				owner, active := byZone[zoneKey]
				if !active || source.Name != "Zone Air Heat Balance Surface Convection Rate" {
					continue
				}
				if family.Role != "context" || family.Category != "balance.storage_other" || family.Component != "sensible" || len(family.Terms) != 1 || term.Sign != 1 || len(family.Subtract) != 0 || len(family.TraceSources) != 0 {
					return out, fmt.Errorf("active Zone surface aggregate requires an explicit context-only family without passive/storage subtraction")
				}
				if err := register(source, owner, "zone_aggregate", "balance.storage_other"); err != nil {
					return out, err
				}
				out.FamilyZones[family.ID+"|"+zoneKey] = true
			}
		}
	}
	for _, family := range model.Families {
		for _, zone := range family.Keys {
			zone = strings.ToLower(zone)
			if _, active := byZone[zone]; !active {
				continue
			}
			for _, dependency := range family.Subtract {
				if dependency == "surface.total" || out.FamilyZones[dependency+"|"+zone] {
					return out, fmt.Errorf("active Zone cannot infer passive/storage pressure by subtracting surface context")
				}
			}
			for _, trace := range family.TraceSources {
				for _, alias := range trace.Source.Alternatives {
					if alias.Name == "Zone Air Heat Balance Surface Convection Rate" {
						return out, fmt.Errorf("active surface aggregate cannot become a temporal pressure trace")
					}
				}
			}
		}
	}
	// An observed aggregate must not disappear merely because its family was
	// omitted from the recipe. Absence in SQL, unlike observed zero, adds none.
	for _, source := range observed {
		if _, active := byZone[strings.ToLower(source.KeyValue)]; active && source.Name == "Zone Air Heat Balance Surface Convection Rate" && source.ReportingFrequency == "Monthly" {
			if _, retained := out.Identities[source.DictionaryIndex]; !retained {
				return out, fmt.Errorf("observed active Zone aggregate lacks its explicit context source declaration")
			}
		}
	}
	return out, nil
}

func epathSQLRadiantSurfaceGeometry(db *sql.DB, doc idf.Document, owner epathSQLRadiantOwnerProof) error {
	var original *idf.Object
	for index := range doc.Objects {
		if doc.Objects[index].Index == owner.SurfaceIndex {
			original = &doc.Objects[index]
		}
	}
	if original == nil || !strings.EqualFold(original.Type, "BuildingSurface:Detailed") {
		return fmt.Errorf("radiant context lacks its original direct surface index")
	}
	var index, boundary int
	var class string
	if err := db.QueryRow(`SELECT SurfaceIndex,ClassName,ExtBoundCond FROM Surfaces WHERE lower(SurfaceName)=lower(?) AND HeatTransferSurf=1`, owner.Owner.SurfaceName).Scan(&index, &class, &boundary); err != nil || !strings.EqualFold(class, epathSQLRadiantField(*original, 1)) {
		return fmt.Errorf("radiant SQL surface class disagrees with original input: %v", err)
	}
	want := 0
	switch strings.ToLower(epathSQLRadiantField(*original, 5)) {
	case "ground":
		want = -1
	case "outdoors":
		want = 0
	case "adiabatic":
		want = index
	case "surface":
		if err := db.QueryRow(`SELECT SurfaceIndex FROM Surfaces WHERE lower(SurfaceName)=lower(?)`, epathSQLRadiantField(*original, 6)).Scan(&want); err != nil {
			return fmt.Errorf("radiant surface has an unbound original adjacent surface")
		}
	default:
		return fmt.Errorf("unreviewed original radiant surface boundary")
	}
	if boundary != want {
		return fmt.Errorf("radiant SQL surface boundary disagrees with original input")
	}
	return nil
}

func epathSQLValidateRadiantSurfaceContextIdentity(proof epathSQLRadiantSurfaceContextIdentity) error {
	owner, source := proof.Owner, proof.Source
	if owner.Owner.EquipmentType != "ZoneHVAC:LowTemperatureRadiant:ConstantFlow" || owner.Owner.EquipmentName == "" || owner.Owner.ZoneName == "" || owner.Owner.SurfaceName == "" || owner.SurfaceIndex < 0 || owner.ZoneMultiplier <= 0 || owner.ZoneListMultiplier <= 0 || !epathOracleFinite(owner.ZoneMultiplier*owner.ZoneListMultiplier) || source.DictionaryIndex <= 0 || source.IsMeter || source.ReportingFrequency != "Monthly" || source.Rows != 12 || source.MissingRows != 0 {
		return fmt.Errorf("invalid independent radiant surface context identity")
	}
	switch proof.Kind {
	case "inside_face":
		if !strings.EqualFold(source.KeyValue, owner.Owner.SurfaceName) || !(source.Name == "Surface Inside Face Convection Heat Gain Energy" && source.SourceUnit == "J" || source.Name == "Surface Inside Face Convection Heat Gain Rate" && source.SourceUnit == "W") || proof.Category == "" {
			return fmt.Errorf("radiant surface context requires the exact original convection observation")
		}
	case "zone_aggregate":
		if !strings.EqualFold(source.KeyValue, owner.Owner.ZoneName) || source.Name != "Zone Air Heat Balance Surface Convection Rate" || source.SourceUnit != "W" || proof.Category != "balance.storage_other" {
			return fmt.Errorf("radiant aggregate context requires the exact original Zone rate")
		}
	default:
		return fmt.Errorf("unknown radiant surface context kind")
	}
	months, err := epathSQLMonthly(source, proof.Precision)
	if err != nil {
		return err
	}
	for month, q := range months {
		bucket := source.Months[month]
		if bucket.MissingRows != 0 || bucket.RawSum == nil || !epathOracleFinite(*bucket.RawSum) || !reflect.DeepEqual(proof.Raw[month], q) || !reflect.DeepEqual(proof.Effective[month], q.times(owner.ZoneMultiplier*owner.ZoneListMultiplier)) {
			return fmt.Errorf("radiant context value/multiplier differs from its independently observed month")
		}
	}
	return nil
}

func epathSQLValidateRadiantSurfaceContextFrames(frames epathSQLFrames) error {
	for id, proof := range frames.RadiantSurfaceContextIdentities {
		if err := epathSQLValidateRadiantSurfaceContextIdentity(proof); err != nil {
			return err
		}
		if id != proof.Source.DictionaryIndex || !reflect.DeepEqual(frames.SourceIdentities[id], proof.Source) || !reflect.DeepEqual(frames.SourceRaw[id], proof.Raw[:]) || !reflect.DeepEqual(frames.SourceEffective[id], proof.Effective[:]) || !strings.EqualFold(frames.SourceZone[id], proof.Owner.Owner.ZoneName) {
			return fmt.Errorf("radiant context source frame is missing or changed")
		}
		for _, cell := range frames.Cells {
			for _, sourceID := range cell.SourceIDs {
				if sourceID == id {
					return fmt.Errorf("radiant context source leaked into a passive/pressure/storage cell")
				}
			}
		}
	}
	return nil
}

// An explicitly retained active-Zone aggregate has original source frames but
// deliberately no thermal cell. Only its exact typed context binding can
// discharge the ordinary family-cell presence obligation.
func epathSQLRadiantContextFamilyOwner(frames epathSQLFrames, family epathRealSQLFamily, zone string) (bool, error) {
	matched := 0
	for _, proof := range frames.RadiantSurfaceContextIdentities {
		if proof.Kind != "zone_aggregate" || !strings.EqualFold(proof.Owner.Owner.ZoneName, zone) {
			continue
		}
		for _, term := range family.Terms {
			for _, alternative := range term.Source.Alternatives {
				if alternative.Name != proof.Source.Name || alternative.Unit != proof.Source.SourceUnit {
					continue
				}
				if family.Role != "context" || family.Category != proof.Category || family.Component != "sensible" || len(family.Terms) != 1 || len(term.Source.Alternatives) != 1 || term.Sign != 1 || term.Source.IsMeter || len(family.Subtract) != 0 || len(family.TraceSources) != 0 {
					return false, fmt.Errorf("radiant aggregate family escaped its exact context contract")
				}
				if len(term.Source.Keys) != 0 {
					return false, fmt.Errorf("radiant aggregate family requires its explicit declared Zone keys")
				}
				matched++
			}
		}
	}
	if matched > 1 {
		return false, fmt.Errorf("ambiguous radiant aggregate family context")
	}
	return matched == 1, nil
}

// Geometry's surface-N aliases use the currently rendered document's object
// indices. Annualization can shift those without changing the original surface.
// The public stable entity format instead binds its exact original type/name:
// lowercase trimmed UTF-8, percent-encoding all bytes except a-z/0-9/._-.
// Do not call the production semantic-ID builder to derive oracle expectations.
func epathSQLRadiantStableSurfaceID(name string) string {
	var token strings.Builder
	for _, b := range []byte(strings.ToLower(strings.TrimSpace(name))) {
		if b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '.' || b == '_' || b == '-' {
			token.WriteByte(b)
		} else {
			fmt.Fprintf(&token, "%%%02x", b)
		}
	}
	return "surface:buildingsurface%3adetailed:" + token.String()
}

func epathSQLRadiantSurfaceContextSourceMatches(source EnergyDataSource, proof epathSQLRadiantSurfaceContextIdentity) bool {
	if epathSQLValidateRadiantSurfaceContextIdentity(proof) != nil || !epathSQLOriginalSourceMatches(source, epathSQLOriginalRDD(proof.Source), "annual") {
		return false
	}
	method, component := "sum_report_data", "surface.sensible"
	if proof.Source.SourceUnit == "W" {
		method = "integrate_rate_by_time_interval"
	}
	if proof.Kind == "zone_aggregate" {
		component = "surface.reconciliation"
	}
	if source.ID != fmt.Sprintf("sql-rdd-%d", proof.Source.DictionaryIndex) || !strings.EqualFold(source.ZoneName, proof.Owner.Owner.ZoneName) || source.Units != proof.Source.SourceUnit || source.DriverRole != "context" || source.InspectorSection != "Context" || source.DriverCategory != proof.Category || source.DriverComponent != component || source.AggregationMethod != method || source.MultiplierApplication != "requires_zone_multiplier" || source.EffectiveMultiplier != proof.Owner.ZoneMultiplier*proof.Owner.ZoneListMultiplier || len(source.InputSourceIDs) != 0 || source.Formula != "" || source.AllocationApplied {
		return false
	}
	if proof.Kind == "inside_face" {
		want := epathSQLRadiantStableSurfaceID(proof.Owner.Owner.SurfaceName)
		found, seen := false, map[string]bool{}
		for _, related := range source.RelatedEntityIDs {
			if seen[related] || strings.HasPrefix(related, "surface:") && related != want {
				return false
			}
			seen[related] = true
			found = found || related == want
		}
		return found
	}
	return true
}

func epathCheckSQLRadiantSurfaceContextSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	proof := check.RadiantSurfaceContext
	if proof == nil || epathSQLValidateRadiantSurfaceContextIdentity(*proof) != nil {
		return fmt.Errorf("missing independent radiant surface context source proof")
	}
	target := check.Item.Target
	if check.Item.Period != "annual" || check.Item.Group != "drivers" || target.Collection != "sources" || (target.Field != "rawValue" && target.Field != "effectiveValue") || target.SourceName != proof.Source.Name || !strings.EqualFold(target.SourceKey, proof.Source.KeyValue) || target.SourceUnit != proof.Source.SourceUnit || target.Frequency != "Monthly" || target.Unit != "kWh" || check.Quantity == nil || (check.Item.Scope != "building" && check.Item.Scope != "zone") || check.Item.Scope == "building" && check.Item.Zone != "" || check.Item.Scope == "zone" && !strings.EqualFold(check.Item.Zone, proof.Owner.Owner.ZoneName) {
		return fmt.Errorf("radiant context proof is not bound to its exact scalar selector")
	}
	want := epathSQLQuantity{}
	for month := 0; month < 12; month++ {
		q := proof.Raw[month]
		if target.Field == "effectiveValue" {
			q = proof.Effective[month]
		}
		want = want.add(q)
	}
	if !reflect.DeepEqual(want, *check.Quantity) {
		return fmt.Errorf("radiant context scalar quantity differs from original twelve-month source proof")
	}
	count := 0
	for _, source := range bundle.EnergyExplanation.Sources {
		if source.ID != fmt.Sprintf("sql-rdd-%d", proof.Source.DictionaryIndex) && !(strings.EqualFold(source.Name, proof.Source.Name) && strings.EqualFold(source.KeyValue, proof.Source.KeyValue) && source.ReportingFrequency == proof.Source.ReportingFrequency) {
			continue
		}
		count++
		if !epathSQLRadiantSurfaceContextSourceMatches(source, *proof) {
			return fmt.Errorf("active surface source lost original context-only ownership/metadata")
		}
	}
	if count != 1 {
		return fmt.Errorf("radiant context requires exactly one original candidate source")
	}
	return nil
}
