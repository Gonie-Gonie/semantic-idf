package simulation

// Independent finite required-source registry. Native evidence is validated once
// at a boundary; the 80 row consumers never rescan native Hourly calendars.
import (
	"encoding/hex"
	"fmt"
	"math"
	"reflect"
	"strings"
)

type epathSQLPVSourceProof struct {
	SystemID, SpecID, Field string
	DictionaryIndex         int
}

type epathSQLPVSourceRegistry struct{ Native []epathSQLPVSourceFrames }

// Only small, detached source attributes reach the consumer. No row slice or
// mutable Source/owner pointer aliases the retained native evidence here.
type epathSQLPVValidatedIdentity struct {
	Chart                            *epathSQLPVSourceChart
	DictionaryIndex                  int
	Name, Key, Frequency, IndexGroup string
	IsMeter                          bool
	OutputObjectIndex                *int
	Quantity                         epathSQLQuantity
}

type epathSQLPVValidatedSources struct {
	canonical  map[string]epathSQLModelCheck
	identities map[int]epathSQLPVValidatedIdentity
}

func epathSQLPVHasSourceCheck(check epathSQLModelCheck) bool {
	return check.PVSource != nil || strings.Contains(check.Item.Key, "|pv_native_source/") || strings.Contains(check.Want.Key, "|pv_native_source/")
}

func epathSQLPVCloneDeclarations(rows []epathRealSQLPVSystem) []epathRealSQLPVSystem {
	if len(rows) == 0 {
		return nil
	}
	out := append([]epathRealSQLPVSystem(nil), rows...)
	for i := range out {
		out[i].Generators = append([]epathRealSQLPVGenerator(nil), rows[i].Generators...)
	}
	return out
}

func epathSQLPVDeclarationsEqual(a, b []epathRealSQLPVSystem) bool {
	return len(a) == 0 && len(b) == 0 || reflect.DeepEqual(a, b)
}

func epathSQLPVSerializedSourceQuantity(identity epathSQLPVSourceIdentity) (epathSQLQuantity, error) {
	v := identity.NativeAnnualKWh
	if !epathOracleFinite(v) || identity.Source.SourceUnit != "J" || identity.Source.RawSum == nil || identity.Source.EnergyKWh == nil ||
		!epathSQLPVNativeNear(v, *identity.Source.EnergyKWh) || !epathSQLPVNativeNear(identity.NativeAnnualJ, *identity.Source.RawSum) ||
		identity.AggregationBasis != "model_total" || identity.EffectiveMultiplier != 1 || !epathSQLPVSignValid(identity.Spec.Sign, v) {
		return epathSQLQuantity{}, fmt.Errorf("PV source scalar lost its independently observed native signed model-total quantity")
	}
	// Native floating summation tolerance, followed by the established ONE
	// public 3dp annual serialization. No positivity clamp or monthly rounding.
	errBound := 0.0
	if v != 0 {
		errBound = 1e-10 * math.Max(1, math.Abs(v))
	}
	round := func(x float64) float64 { return math.Round(x*1000) / 1000 }
	q := epathSQLBounded(round(v), round(v-errBound), round(v+errBound))
	if !q.valid() {
		return epathSQLQuantity{}, fmt.Errorf("PV native serialization interval is invalid")
	}
	return q, nil
}

func epathSQLPVCanonicalSourceCheck(systemID string, identity epathSQLPVSourceIdentity, field string) (epathSQLModelCheck, error) {
	if systemID == "" || field != "rawValue" && field != "effectiveValue" {
		return epathSQLModelCheck{}, fmt.Errorf("invalid PV source requirement")
	}
	q, err := epathSQLPVSerializedSourceQuantity(identity)
	if err != nil {
		return epathSQLModelCheck{}, err
	}
	s := identity.Source
	key := fmt.Sprintf("pv_native_source/%s/%s/%s/%s", identity.Spec.ID, s.Name, s.ReportingFrequency, field)
	target := epathRealOracleTarget{Collection: "sources", Field: field, SourceName: s.Name, SourceKey: s.KeyValue, SourceUnit: "J", Frequency: s.ReportingFrequency, Unit: "kWh"}
	one := epathSQLModelChecks{}
	if err := one.add(identity.Spec.MetricGroup, "building", "", "annual", key, "kWh", &q, target, "", nil, nil); err != nil {
		return epathSQLModelCheck{}, err
	}
	check := one.Rows[0]
	check.PVSource = &epathSQLPVSourceProof{SystemID: systemID, SpecID: identity.Spec.ID, Field: field, DictionaryIndex: s.DictionaryIndex}
	return check, nil
}

// Cheap registry construction. The only compile caller is immediately after
// epathCompileSQLPVSourceFrames has validated the full frame. Public boundary
// preparation below validates it first. This function never blesses a row scan.
func epathSQLPVRegistryLookup(required []epathRealSQLPVSystem, registry *epathSQLPVSourceRegistry) (epathSQLPVValidatedSources, error) {
	out := epathSQLPVValidatedSources{canonical: map[string]epathSQLModelCheck{}, identities: map[int]epathSQLPVValidatedIdentity{}}
	if len(required) == 0 {
		if registry != nil {
			return out, fmt.Errorf("PV registry survived without its independent required declaration")
		}
		return out, nil
	}
	if len(required) != 1 || registry == nil || len(registry.Native) != 1 {
		return out, fmt.Errorf("PV required declaration lost its finite native source registry")
	}
	frame := registry.Native[0]
	if !reflect.DeepEqual(required[0], frame.Original.Declaration) || !frame.DictionaryCensusComplete || len(frame.Sources) != 40 {
		return out, fmt.Errorf("PV native registry changed its original declaration or complete40 census")
	}
	for _, hash := range []string{frame.Original.OriginalSHA256, frame.ExecutedSHA256, frame.SQLSHA256} {
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != 32 {
			return out, fmt.Errorf("PV native registry has invalid provenance hash format")
		}
	}
	specs, err := epathSQLPVSourceSpecs(frame.Original)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	groups := map[string]int{}
	for _, id := range epathSQLPVSourceIDs(frame) {
		identity := frame.Sources[id]
		var spec *epathSQLPVNativeSpec
		for i := range specs {
			if specs[i].ID == identity.Spec.ID {
				spec = &specs[i]
			}
		}
		if spec == nil || !reflect.DeepEqual(*spec, identity.Spec) || id <= 0 || id != identity.Source.DictionaryIndex || id != identity.Dictionary.Index ||
			identity.Source.ReportingFrequency != "Monthly" && identity.Source.ReportingFrequency != "Hourly" || !identity.RequestBound {
			return out, fmt.Errorf("PV required source substituted exact spec/frequency/dictionary/request identity")
		}
		pair := spec.ID + "/" + identity.Source.ReportingFrequency
		if seen[pair] {
			return out, fmt.Errorf("PV required source repeats a native family/frequency")
		}
		seen[pair] = true
		q, err := epathSQLPVSerializedSourceQuantity(identity)
		if err != nil {
			return out, err
		}
		snapshot := epathSQLPVValidatedIdentity{DictionaryIndex: id, Name: identity.Source.Name, Key: identity.Source.KeyValue, Frequency: identity.Source.ReportingFrequency, IndexGroup: identity.Source.IndexGroup, IsMeter: identity.Source.IsMeter, Quantity: q}
		snapshot.Chart, err = epathSQLPVSourceChartExpectation(identity, frame.Weather)
		if err != nil {
			return out, err
		}
		if identity.OutputObjectIndex != nil {
			index := *identity.OutputObjectIndex
			snapshot.OutputObjectIndex = &index
		}
		out.identities[id] = snapshot
		for _, field := range []string{"rawValue", "effectiveValue"} {
			check, err := epathSQLPVCanonicalSourceCheck(required[0].ID, identity, field)
			if err != nil {
				return out, err
			}
			if _, found := out.canonical[check.Want.Key]; found {
				return out, fmt.Errorf("PV required scalar key repeats")
			}
			out.canonical[check.Want.Key] = check
			groups[check.Want.Group]++
		}
	}
	for _, spec := range specs {
		if !seen[spec.ID+"/Monthly"] || !seen[spec.ID+"/Hourly"] {
			return out, fmt.Errorf("PV required source pair missing")
		}
	}
	if len(out.identities) != 40 || len(out.canonical) != 80 || groups["carriers"] != 68 || groups["endUses"] != 4 || groups["loads"] != 8 {
		return out, fmt.Errorf("PV required80 source group/field registry changed")
	}
	return out, nil
}

func epathSQLPVCheckRegistryCensus(checks epathSQLModelChecks, prepared epathSQLPVValidatedSources) error {
	seen := map[string]bool{}
	for _, check := range checks.Rows {
		if !epathSQLPVHasSourceCheck(check) {
			continue
		}
		canonical, found := prepared.canonical[check.Want.Key]
		if !found || seen[check.Want.Key] || !checks.Keys[check.Want.Key] || !reflect.DeepEqual(check, canonical) {
			return fmt.Errorf("PV required native source row lost or changed exact proof/selector/context/quantity: %s", check.Want.Key)
		}
		seen[check.Want.Key] = true
	}
	for key := range prepared.canonical {
		if !seen[key] || !checks.Keys[key] {
			return fmt.Errorf("PV required source row/key removed together: %s", key)
		}
	}
	for key, required := range checks.Keys {
		if strings.Contains(key, "|pv_native_source/") && (!required || !seen[key]) {
			return fmt.Errorf("PV unexpected or orphan source registry key: %s", key)
		}
	}
	return nil
}

func epathSQLPreparePVSourceChecks(checks epathSQLModelChecks) (epathSQLPVValidatedSources, error) {
	// Heavy retained-row/calendar validation exactly once per native frame here,
	// never from the individual raw/effective consumer. No persistent valid flag.
	if checks.PVSourceRegistry != nil {
		for _, native := range checks.PVSourceRegistry.Native {
			if err := epathSQLValidatePVSourceFrames(native); err != nil {
				return epathSQLPVValidatedSources{}, err
			}
		}
	}
	out, err := epathSQLPVRegistryLookup(checks.RequiredPVSystems, checks.PVSourceRegistry)
	if err != nil {
		return epathSQLPVValidatedSources{}, err
	}
	if err := epathSQLPVCheckRegistryCensus(checks, out); err != nil {
		return epathSQLPVValidatedSources{}, err
	}
	return out, nil
}

// Pending has the external recipe, so deleting the check-level anchor is also
// detectable. A no-model standalone checker cannot reconstruct erased evidence.
func epathSQLValidatePVSourceChecksForModel(checks epathSQLModelChecks, model *epathRealSQLModel) error {
	var required []epathRealSQLPVSystem
	if model != nil {
		required = model.PVSystems
	}
	if !epathSQLPVDeclarationsEqual(required, checks.RequiredPVSystems) {
		return fmt.Errorf("PV check registry lost the recipe's independently required declaration")
	}
	_, err := epathSQLPreparePVSourceChecks(checks)
	return err
}

// Compile-final census only. The same compile call already validated native
// rows in epathSQLBindPVSources. Never use this cheap variant for evaluating a
// retained/mutated frame or exporting pending evidence; those use Prepare.
func epathSQLValidatePVCompiledSourceChecks(checks epathSQLModelChecks, model epathRealSQLModel) error {
	if !epathSQLPVDeclarationsEqual(model.PVSystems, checks.RequiredPVSystems) {
		return fmt.Errorf("PV compiled registry lost the model's required declaration")
	}
	prepared, err := epathSQLPVRegistryLookup(checks.RequiredPVSystems, checks.PVSourceRegistry)
	if err != nil {
		return err
	}
	return epathSQLPVCheckRegistryCensus(checks, prepared)
}
