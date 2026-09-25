package simulation

// Independent test-only supplemental proof. Core PV40 stays unchanged; no
// production inventory, classifier, scalar reader or candidate is authority.
import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLPVCogenerationMembership struct {
	OriginalText, ExecutedText, MTDText       string
	OriginalSHA256, ExecutedSHA256, MTDSHA256 string
	Members                                   map[string][]string // Native full key:name; never split PV key colons.
}

type epathSQLPVCogenerationFrames struct {
	Membership         epathSQLPVCogenerationMembership
	SQLSHA256, MTDPath string
	OutputPlan         PurposeRunPlan
	Parents            map[string]epathSQLPVSourceIdentity // exact Monthly and Hourly only
	AncillaryIDs       map[string]int                      // actual native identities inside the unchanged PV40
}

func epathSQLPVCogenerationParentSpec() epathSQLPVNativeSpec {
	return epathSQLPVNativeSpec{ID: "cogeneration.electricity", Name: "Cogeneration:Electricity", Role: "consumed_parent", Sign: "nonnegative", MetricGroup: "endUses", IsMeter: true}
}

func epathSQLPVProofSHA(text string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(text))) }

func epathSQLPVCogenerationMembershipProof(original, executed, mtd string, proof epathSQLPVOriginalProof) (epathSQLPVCogenerationMembership, error) {
	out := epathSQLPVCogenerationMembership{}
	if _, err := epathSQLBindPVExecuted(original, executed, proof); err != nil {
		return out, err
	}
	// This finite untouched Shop has neither custom meters nor EMS. The narrow
	// census rejects additional native metered outputs and storage overrides;
	// another arbitrary meter's presence is not silently treated as harmless.
	for _, text := range []string{original, executed} {
		doc, err := idf.Parse(text)
		if err != nil {
			return out, err
		}
		for _, object := range doc.Objects {
			typ := strings.ToLower(strings.TrimSpace(object.Type))
			if strings.HasPrefix(typ, "meter:") || strings.HasPrefix(typ, "energymanagementsystem:") {
				return out, fmt.Errorf("finite PV/Cogeneration meter proof rejects additional custom/EMS object %s", object.Type)
			}
		}
	}
	headers := map[string]string{
		"Electricity:Facility":         "Electricity:Facility [J], ResourceType=Electricity, contents are:",
		"ElectricityProduced:Facility": "ElectricityProduced:Facility [J], ResourceType=ElectricityProduced, contents are:",
		"Cogeneration:Electricity":     "Cogeneration:Electricity [J], ResourceType=Electricity, EndUse=Cogeneration, contents are:",
	}
	sections := map[string][]string{}
	current := ""
	for _, raw := range strings.Split(mtd, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			current = ""
			continue
		}
		if strings.HasPrefix(line, "For Meter=") {
			current = ""
			header := strings.TrimPrefix(line, "For Meter=")
			for name, want := range headers {
				if !strings.HasPrefix(strings.ToLower(header), strings.ToLower(name+" [")) {
					continue
				}
				if header != want {
					return out, fmt.Errorf("native MTD parent resource/unit identity differs: %s", name)
				}
				if _, duplicate := sections[name]; duplicate {
					return out, fmt.Errorf("duplicate native MTD parent section %s", name)
				}
				sections[name] = []string{}
				current = name
			}
			continue
		}
		if current != "" {
			member := strings.ToLower(line)
			if !strings.Contains(member, ":") || strings.HasPrefix(member, "onmeter=") {
				return out, fmt.Errorf("invalid native MTD constituent")
			}
			for _, existing := range sections[current] {
				if existing == member {
					return out, fmt.Errorf("duplicate native MTD member %s", member)
				}
			}
			sections[current] = append(sections[current], member)
		}
	}
	d := proof.Declaration
	member := func(key, name string) string { return strings.ToLower(key + ":" + name) }
	ancillary := member(d.InverterName, "Inverter Ancillary AC Electricity Energy")
	produced := []string{member(d.InverterName, "Inverter Conversion Loss Decrement Energy"), member(d.StorageName, "Electric Storage Production Decrement Energy"), member(d.StorageName, "Electric Storage Discharge Energy")}
	for _, pv := range d.Generators {
		produced = append(produced, member(pv.Name, "Generator Produced DC Electricity Energy"))
	}
	for name, want := range map[string][]string{"Cogeneration:Electricity": {ancillary}, "ElectricityProduced:Facility": produced} {
		actual, exists := sections[name]
		if !exists {
			return out, fmt.Errorf("missing native MTD parent %s", name)
		}
		sort.Strings(actual)
		sort.Strings(want)
		if !reflect.DeepEqual(actual, want) {
			return out, fmt.Errorf("native MTD parent %s constituent roster changed", name)
		}
	}
	facility, exists := sections["Electricity:Facility"]
	if !exists || len(facility) == 0 {
		return out, fmt.Errorf("missing native Electricity:Facility MTD membership")
	}
	foundAncillary := false
	for _, actual := range facility {
		foundAncillary = foundAncillary || actual == ancillary
		for _, forbidden := range append(append([]string(nil), produced...), member(d.StorageName, "Electric Storage Charge Energy"), member(d.InverterName, "Inverter DC Input Electricity Energy"), member(d.InverterName, "Inverter AC Output Electricity Energy"), member(d.InverterName, "Inverter Conversion Loss Energy")) {
			if actual == forbidden {
				return out, fmt.Errorf("native transfer/produced constituent appeared in consumed Facility membership")
			}
		}
	}
	if !foundAncillary {
		return out, fmt.Errorf("ancillary consumption is absent from native Facility membership")
	}
	for key := range sections {
		sort.Strings(sections[key])
	}
	return epathSQLPVCogenerationMembership{OriginalText: original, ExecutedText: executed, MTDText: mtd, OriginalSHA256: epathSQLPVProofSHA(original), ExecutedSHA256: epathSQLPVProofSHA(executed), MTDSHA256: epathSQLPVProofSHA(mtd), Members: sections}, nil
}

func epathCompileSQLPVCogenerationFrames(observed epathRealOracleEvidence, core epathSQLPVSourceFrames, meterPath string) (epathSQLPVCogenerationFrames, error) {
	out := epathSQLPVCogenerationFrames{Parents: map[string]epathSQLPVSourceIdentity{}, AncillaryIDs: map[string]int{}}
	if err := epathSQLValidatePVSourceFrames(core); err != nil {
		return out, err
	}
	wantPath, err := filepath.Abs(filepath.Join(filepath.Dir(observed.sqlPath), "eplusout.mtd"))
	if err != nil {
		return out, err
	}
	actualPath, err := filepath.Abs(meterPath)
	if err != nil {
		return out, err
	}
	if !strings.EqualFold(filepath.Clean(actualPath), filepath.Clean(wantPath)) {
		return out, fmt.Errorf("PV native MTD must be the same captured SQL sibling")
	}
	bytes, err := os.ReadFile(actualPath)
	if err != nil {
		return out, err
	}
	out.Membership, err = epathSQLPVCogenerationMembershipProof(observed.originalText, observed.executedText, string(bytes), core.Original)
	if err != nil {
		return out, err
	}
	out.MTDPath = actualPath
	out.SQLSHA256, err = epathSQLPVFileSHA(observed.sqlPath)
	if err != nil {
		return out, err
	}
	if out.SQLSHA256 != core.SQLSHA256 || out.Membership.ExecutedSHA256 != core.ExecutedSHA256 || !reflect.DeepEqual(observed.Weather, core.Weather) {
		return out, fmt.Errorf("PV Cogeneration source borrowed another capture or weather")
	}
	if observed.outputPlan == nil {
		return out, fmt.Errorf("Cogeneration source has no actual output plan")
	}
	planBytes, err := json.Marshal(observed.outputPlan)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(planBytes, &out.OutputPlan); err != nil {
		return out, err
	}
	doc, err := idf.Parse(observed.originalText)
	if err != nil {
		return out, err
	}
	run, err := idf.Parse(observed.executedText)
	if err != nil {
		return out, err
	}
	db, err := epathOpenOracleSQL(observed.sqlPath)
	if err != nil {
		return out, err
	}
	defer db.Close()
	spec := epathSQLPVCogenerationParentSpec()
	for _, frequency := range []string{"Monthly", "Hourly"} {
		dictionary, err := epathSQLPVReadDictionary(db, spec, frequency)
		if err != nil {
			return out, err
		}
		if _, collision := core.Sources[dictionary.Index]; collision {
			return out, fmt.Errorf("Cogeneration parent borrowed a core native source")
		}
		rows, err := epathSQLPVReadRows(db, dictionary)
		if err != nil {
			return out, err
		}
		summary, raw, energy, negative, err := epathSQLPVNativeSummary(spec, dictionary, core.Weather, rows)
		if err != nil {
			return out, err
		}
		count := 0
		for _, source := range observed.Sources {
			if source.DictionaryIndex == dictionary.Index {
				count++
				if !epathSQLPVSourceSummaryEqual(summary, source) {
					return out, fmt.Errorf("Cogeneration original native observation differs from independently collected source")
				}
			}
		}
		if count != 1 {
			return out, fmt.Errorf("Cogeneration native observed identity count=%d", count)
		}
		opener, err := epathSQLPVRequestBinding(doc, run, observed.outputPlan, spec, frequency)
		if err != nil {
			return out, err
		}
		out.Parents[frequency] = epathSQLPVSourceIdentity{Spec: spec, Source: summary, Dictionary: dictionary, Rows: rows, NativeMonthlyJ: raw, NativeMonthlyKWh: energy, NativeAnnualJ: *summary.RawSum, NativeAnnualKWh: *summary.EnergyKWh, HasNegativeValue: negative, OutputObjectIndex: opener, RequestBound: true, AggregationBasis: "model_total", EffectiveMultiplier: 1}
		for id, identity := range core.Sources {
			if identity.Spec.ID == "inverter.ancillary" && identity.Dictionary.Frequency == frequency {
				if out.AncillaryIDs[frequency] != 0 {
					return out, fmt.Errorf("duplicate ancillary companion")
				}
				out.AncillaryIDs[frequency] = id
			}
		}
	}
	// A selected blank-key identity cannot hide a second invalid foreign-key one.
	rows, err := db.Query(`SELECT ReportDataDictionaryIndex,ReportingFrequency FROM ReportDataDictionary WHERE Name=? COLLATE NOCASE`, spec.Name)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id int
		var frequency string
		if err := rows.Scan(&id, &frequency); err != nil {
			rows.Close()
			return out, err
		}
		if strings.EqualFold(frequency, "Monthly") || strings.EqualFold(frequency, "Hourly") {
			p, found := out.Parents[frequency]
			if !found || id != p.Dictionary.Index {
				rows.Close()
				return out, fmt.Errorf("extra/foreign Cogeneration native M/H dictionary")
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if err := epathSQLValidatePVCogenerationFrames(core, out); err != nil {
		return out, err
	}
	return out, nil
}

func epathSQLValidatePVCogenerationFrames(core epathSQLPVSourceFrames, frame epathSQLPVCogenerationFrames) error {
	// Caller validates the core once at each compile/evaluate/registry boundary;
	// this validates this small supplement, not all PV40 for each source scalar.
	if len(frame.Parents) != 2 || len(frame.AncillaryIDs) != 2 || frame.SQLSHA256 != core.SQLSHA256 || frame.MTDPath == "" {
		return fmt.Errorf("Cogeneration parent/capture census incomplete")
	}
	proof, err := epathSQLPVCogenerationMembershipProof(frame.Membership.OriginalText, frame.Membership.ExecutedText, frame.Membership.MTDText, core.Original)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(proof, frame.Membership) || proof.OriginalSHA256 != core.Original.OriginalSHA256 || proof.ExecutedSHA256 != core.ExecutedSHA256 {
		return fmt.Errorf("Cogeneration original/executed/MTD membership proof changed")
	}
	original, err := idf.Parse(proof.OriginalText)
	if err != nil {
		return err
	}
	executed, err := idf.Parse(proof.ExecutedText)
	if err != nil {
		return err
	}
	spec := epathSQLPVCogenerationParentSpec()
	seen := map[int]bool{}
	for _, frequency := range []string{"Monthly", "Hourly"} {
		identity, found := frame.Parents[frequency]
		if !found || identity.Dictionary.Frequency != frequency || !reflect.DeepEqual(identity.Spec, spec) || identity.OriginalOwnerIndex != nil || identity.ExecutedOwnerIndex != nil || identity.EffectiveMultiplier != 1 || identity.AggregationBasis != "model_total" || !identity.RequestBound || seen[identity.Dictionary.Index] {
			return fmt.Errorf("Cogeneration parent identity/owner/knownness changed")
		}
		seen[identity.Dictionary.Index] = true
		opener, err := epathSQLPVRequestBinding(original, executed, &frame.OutputPlan, spec, frequency)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(opener, identity.OutputObjectIndex) {
			return fmt.Errorf("Cogeneration output opener no longer follows original request proof")
		}
		if _, collision := core.Sources[identity.Dictionary.Index]; collision {
			return fmt.Errorf("Cogeneration parent overlaps a core source")
		}
		summary, raw, energy, negative, err := epathSQLPVNativeSummary(spec, identity.Dictionary, core.Weather, identity.Rows)
		if err != nil {
			return err
		}
		if !epathSQLPVSourceSummaryEqual(summary, identity.Source) || negative != identity.HasNegativeValue || !epathSQLPVNativeNear(*summary.RawSum, identity.NativeAnnualJ) || !epathSQLPVNativeNear(*summary.EnergyKWh, identity.NativeAnnualKWh) {
			return fmt.Errorf("Cogeneration parent native annual quantity changed")
		}
		for month := 0; month < 12; month++ {
			if !epathSQLPVNativeNear(raw[month], identity.NativeMonthlyJ[month]) || !epathSQLPVNativeNear(energy[month], identity.NativeMonthlyKWh[month]) {
				return fmt.Errorf("Cogeneration parent monthly quantity changed")
			}
		}
		ancillary, found := core.Sources[frame.AncillaryIDs[frequency]]
		if !found || ancillary.Spec.ID != "inverter.ancillary" || ancillary.Dictionary.Frequency != frequency || ancillary.Spec.Owner == nil || !strings.EqualFold(ancillary.Spec.Owner.ObjectName, core.Original.Declaration.InverterName) {
			return fmt.Errorf("Cogeneration parent borrowed a non-ancillary member")
		}
	}
	m, h := frame.Parents["Monthly"], frame.Parents["Hourly"]
	for month := 0; month < 12; month++ {
		if !epathSQLPVNativeNear(m.NativeMonthlyKWh[month], h.NativeMonthlyKWh[month]) {
			return fmt.Errorf("Cogeneration parent Monthly/Hourly observation differs")
		}
	}
	return nil
}
