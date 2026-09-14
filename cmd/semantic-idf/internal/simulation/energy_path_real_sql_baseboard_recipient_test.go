package simulation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

// Configuration provenance only. None of these records enters SQL quantity,
// load, surface-pressure, CF-exclusion, or allocation frames.
type epathSQLBaseboardRecipientReference struct {
	ParentName, SurfaceName, ZoneName  string
	ParentIndex, SurfaceIndex          int
	RadiantFraction, RecipientFraction float64
}

type epathSQLBaseboardRecipientSource struct {
	Name, Key, Unit, Frequency string
	References                 []epathSQLBaseboardRecipientReference
}

type epathSQLBaseboardRecipientQualification struct {
	ParentIDs map[string]bool
	Sources   map[int]epathSQLBaseboardRecipientSource
}

func epathSQLBaseboardRecipientOutput(name string) (string, bool) {
	switch name {
	case "Surface Inside Face Convection Heat Gain Energy":
		return "J", false
	case "Surface Inside Face Convection Heat Gain Rate":
		return "W", false
	case "Zone Air Heat Balance Surface Convection Rate":
		return "W", true
	}
	return "", false
}

func epathSQLBaseboardSamePhysicalObject(original, executed idf.Object) bool {
	if !strings.EqualFold(original.Type, executed.Type) || len(original.Fields) != len(executed.Fields) {
		return false
	}
	for index := range original.Fields {
		if strings.TrimSpace(original.Fields[index].Value) != strings.TrimSpace(executed.Fields[index].Value) {
			return false
		}
	}
	return true
}

func epathSQLCompileBaseboardRecipientQualification(original, executed idf.Document, observed []epathRealSQLSource, declarations []epathRealSQLBaseboardContext) (*epathSQLBaseboardRecipientQualification, error) {
	out := &epathSQLBaseboardRecipientQualification{ParentIDs: map[string]bool{}, Sources: map[int]epathSQLBaseboardRecipientSource{}}
	owners, bySurface, byZone := map[string]string{}, map[string][]epathSQLBaseboardRecipientReference{}, map[string][]epathSQLBaseboardRecipientReference{}
	for _, declaration := range declarations {
		key := strings.ToLower(declaration.OwnerName)
		if zone, exists := owners[key]; exists {
			if !strings.EqualFold(zone, declaration.ZoneName) {
				return nil, fmt.Errorf("baseboard recipient declarations disagree on original Zone ownership")
			}
			continue
		}
		owner := epathSQLBaseboardOwner(declaration.ZoneName, declaration.OwnerName)
		if err := epathSQLValidateNativeBaseboardOriginalOwner(original, owner); err != nil {
			return nil, err
		}
		if err := epathSQLValidateNativeBaseboardOriginalOwner(executed, owner); err != nil {
			return nil, fmt.Errorf("executed baseboard ownership differs from original: %w", err)
		}
		parent, _ := epathSQLBaseboardOne(original, epathSQLNativeBaseboardType, declaration.OwnerName)
		runParent, _ := epathSQLBaseboardOne(executed, epathSQLNativeBaseboardType, declaration.OwnerName)
		if !epathSQLBaseboardSamePhysicalObject(parent, runParent) {
			return nil, fmt.Errorf("executed baseboard physical fields differ from original")
		}
		owners[key] = declaration.ZoneName
		out.ParentIDs[fmt.Sprintf("component:%d", runParent.Index)] = true
		radiant, _ := strconv.ParseFloat(epathSQLBaseboardField(parent, 7), 64)
		for position := 9; position < len(parent.Fields); position += 2 {
			fraction, _ := strconv.ParseFloat(epathSQLBaseboardField(parent, position+1), 64)
			if radiant <= 0 || fraction <= 0 {
				continue
			}
			name := epathSQLBaseboardField(parent, position)
			surface, _ := epathSQLBaseboardOne(original, "BuildingSurface:Detailed", name)
			runSurface, err := epathSQLBaseboardOne(executed, "BuildingSurface:Detailed", name)
			if err != nil || !epathSQLBaseboardSamePhysicalObject(surface, runSurface) {
				return nil, fmt.Errorf("executed recipient surface differs from exact original identity")
			}
			ref := epathSQLBaseboardRecipientReference{epathSQLBaseboardField(parent, 0), name, declaration.ZoneName, runParent.Index, runSurface.Index, radiant, fraction}
			bySurface[strings.ToLower(name)] = append(bySurface[strings.ToLower(name)], ref)
			byZone[strings.ToLower(declaration.ZoneName)] = append(byZone[strings.ToLower(declaration.ZoneName)], ref)
		}
	}
	seen := map[string]bool{}
	for _, source := range observed {
		unit, aggregate := epathSQLBaseboardRecipientOutput(source.Name)
		if unit == "" || source.ReportingFrequency != "Monthly" && source.ReportingFrequency != "Hourly" {
			continue
		}
		identity := source.Name + "|" + strings.ToLower(source.KeyValue) + "|" + source.ReportingFrequency
		if source.DictionaryIndex <= 0 || source.IsMeter || source.SourceUnit != unit || seen[identity] {
			return nil, fmt.Errorf("recipient metadata has an ambiguous original SQL identity")
		}
		if _, duplicate := out.Sources[source.DictionaryIndex]; duplicate {
			return nil, fmt.Errorf("recipient metadata reused an original SQL dictionary index")
		}
		seen[identity] = true
		refs := bySurface[strings.ToLower(source.KeyValue)]
		if aggregate {
			refs = byZone[strings.ToLower(source.KeyValue)]
		}
		out.Sources[source.DictionaryIndex] = epathSQLBaseboardRecipientSource{source.Name, source.KeyValue, unit, source.ReportingFrequency, refs}
	}
	// The standalone equipment-source unit fixtures have no surface observations.
	// A surface-reporting model cannot silently lose one configured recipient.
	if len(out.Sources) > 0 {
		for name := range bySurface {
			found := false
			for _, source := range out.Sources {
				_, aggregate := epathSQLBaseboardRecipientOutput(source.Name)
				found = found || !aggregate && strings.EqualFold(source.Key, name)
			}
			if !found {
				return nil, fmt.Errorf("configured recipient lacks any original surface SQL observation")
			}
		}
	}
	return out, nil
}

// Parse individual configuration assertions, not an exact explanation golden.
// Quoted names and numeric fractions must independently resolve to the original
// objects; a fluent but wrong parent/surface sentence is not evidence.
var epathSQLBaseboardRecipientTrace = regexp.MustCompile(`Non-additive recipient configuration: (\S+) ("(?:\\.|[^"\\])*") \[component:([0-9]+), object ([0-9]+)\] -> surface ("(?:\\.|[^"\\])*") \[object ([0-9]+)\], Zone ("(?:\\.|[^"\\])*"); radiant fraction=([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?); recipient share of radiant output=([0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)\.`)

func epathSQLCheckBaseboardRecipientQualification(sources []EnergyDataSource, proof *epathSQLBaseboardRecipientQualification) error {
	if proof == nil || proof.ParentIDs == nil || proof.Sources == nil {
		return fmt.Errorf("missing original baseboard recipient metadata proof")
	}
	seen := map[int]bool{}
	for _, source := range sources {
		unit, aggregate := epathSQLBaseboardRecipientOutput(source.Name)
		if unit == "" || source.ReportingFrequency != "Monthly" && source.ReportingFrequency != "Hourly" {
			continue
		}
		var index int
		if _, err := fmt.Sscanf(source.ID, "sql-rdd-%d", &index); err != nil {
			return fmt.Errorf("recipient source lost its native SQL ID")
		}
		original, exists := proof.Sources[index]
		if !exists || seen[index] || source.ID != fmt.Sprintf("sql-rdd-%d", index) || source.SourceType != "sql_report_data" || source.IsMeter || source.Name != original.Name || !strings.EqualFold(source.KeyValue, original.Key) || source.SourceUnit != original.Unit || source.Units != original.Unit || source.ReportingFrequency != original.Frequency {
			return fmt.Errorf("recipient source metadata differs from the original SQL census")
		}
		seen[index] = true
		traces := epathSQLBaseboardRecipientTrace.FindAllStringSubmatch(source.Explanation, -1)
		if len(traces) != len(original.References) || strings.Count(source.Explanation, "Non-additive recipient configuration:") != len(traces) {
			return fmt.Errorf("recipient lost or invented a non-additive configuration trace")
		}
		expectedParents, matched := map[string]bool{}, map[int]bool{}
		for _, trace := range traces {
			parent, _ := strconv.Unquote(trace[2])
			surface, _ := strconv.Unquote(trace[5])
			zone, _ := strconv.Unquote(trace[7])
			componentIndex, _ := strconv.Atoi(trace[3])
			parentIndex, _ := strconv.Atoi(trace[4])
			surfaceIndex, _ := strconv.Atoi(trace[6])
			radiant, _ := strconv.ParseFloat(trace[8], 64)
			fraction, _ := strconv.ParseFloat(trace[9], 64)
			found := -1
			for i, ref := range original.References {
				if strings.EqualFold(trace[1], epathSQLNativeBaseboardType) && strings.EqualFold(parent, ref.ParentName) && strings.EqualFold(surface, ref.SurfaceName) && strings.EqualFold(zone, ref.ZoneName) && componentIndex == ref.ParentIndex && parentIndex == ref.ParentIndex && surfaceIndex == ref.SurfaceIndex && radiant == ref.RadiantFraction && fraction == ref.RecipientFraction && (source.ZoneName == "" || strings.EqualFold(source.ZoneName, ref.ZoneName)) {
					found = i
				}
			}
			if found < 0 || matched[found] {
				return fmt.Errorf("recipient trace has a foreign parent/surface/Zone/index/fraction")
			}
			matched[found] = true
			expectedParents[fmt.Sprintf("component:%d", parentIndex)] = true
		}
		actualParents := map[string]int{}
		for _, id := range source.RelatedEntityIDs {
			if proof.ParentIDs[id] {
				actualParents[id]++
			}
		}
		if len(actualParents) != len(expectedParents) {
			return fmt.Errorf("recipient source lost its parent ID or labeled a nonrecipient")
		}
		for id := range expectedParents {
			if actualParents[id] != 1 {
				return fmt.Errorf("recipient parent ID is absent or duplicated")
			}
		}
		lower := strings.ToLower(source.Explanation)
		if len(original.References) > 0 {
			passiveQualified := strings.Contains(lower, "not isolated passive") || strings.Contains(lower, "not an isolated passive")
			if !passiveQualified || !strings.Contains(lower, "baseboard-only") || aggregate && (!strings.Contains(lower, "environmental") || !strings.Contains(lower, "hvac")) || !aggregate && (!strings.Contains(lower, "not operation") || !strings.Contains(lower, "no heat is added or subtracted")) {
				return fmt.Errorf("recipient explanation lost the mixed-response/non-additive physical qualification")
			}
		} else if strings.Contains(lower, "baseboard-only") || strings.Contains(lower, "radiant-convective baseboard") {
			return fmt.Errorf("nonrecipient acquired baseboard response qualification")
		}
	}
	if len(seen) != len(proof.Sources) {
		return fmt.Errorf("candidate omitted an independently observed recipient/nonrecipient source")
	}
	return nil
}
