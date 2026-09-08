package simulation

import (
	"fmt"
	"math"
	"strings"
)

// This is a narrow independent presentation proof, not another allocation
// algorithm. Exact original Monthly/J sources determine the reviewed 3-decimal
// grid before any candidate is read. It resolves a combustion-kind boundary
// such as original 0.109935/0.110244 -> displayed 0.110/0.110 without widening
// numeric bounds or guessing a kind from the candidate's label or quotient.
func epathSQLDirectHVACDisplayedPair(frames epathSQLFrames, model epathRealSQLModel, zone, service string, month int) (*epathSQLConversionProof, error) {
	zone = strings.ToLower(zone)
	owner, exists := frames.Zones[zone]
	if !exists || month < 1 || month > 12 || model.Precision.DecimalPlaces != 3 || !epathOracleFinite(owner.Multiplier) || owner.Multiplier <= 0 || service != "cooling" && service != "heating" {
		return nil, fmt.Errorf("exact direct presentation requires a reviewed Zone/month/service and 3-decimal grid")
	}
	ids := frames.LoadSourceIDs[epathSQLKey(zone, service, month)]
	if len(ids) != 1 {
		return nil, fmt.Errorf("exact direct presentation needs one original sensible load authority")
	}
	load, exists := frames.SourceIdentities[ids[0]]
	name := "Zone Air System Sensible Heating Energy"
	if service == "cooling" {
		name = "Zone Air System Sensible Cooling Energy"
	}
	if !exists || load.DictionaryIndex != ids[0] || !strings.EqualFold(load.Name, name) || !strings.EqualFold(load.KeyValue, zone) || load.SourceUnit != "J" || load.ReportingFrequency != "Monthly" || load.IsMeter {
		return nil, fmt.Errorf("unreviewed original load reporting class for exact direct presentation")
	}
	loads, err := epathSQLMonthly(load, model.Precision)
	if err != nil {
		return nil, err
	}
	if !epathSQLZoneCarrierQuantityEqual(frames.Loads[epathSQLKey(zone, service, month)], loads[month-1].times(owner.Multiplier)) {
		return nil, fmt.Errorf("exact direct presentation lost the independently observed load multiplier")
	}
	round := func(value float64) float64 { return math.Round(value*1000) / 1000 }
	from := round(round(loads[month-1].Value) * owner.Multiplier)
	carriers := map[string]bool{}
	for _, definition := range model.Services {
		if definition.Service != service {
			continue
		}
		for _, siteID := range definition.SiteIDs {
			for _, site := range model.Site {
				if site.ID == siteID && !site.Facility && site.EndUse == service && site.Tabular == nil {
					carriers[site.Carrier] = true
				}
			}
		}
	}
	if len(carriers) == 0 {
		return nil, fmt.Errorf("exact direct presentation has no declared service carriers")
	}
	to, rawDirect := 0.0, epathSQLQuantity{}
	seen := map[int]bool{}
	for carrier := range carriers {
		observation, present := frames.DirectHVAC[epathSQLDirectHVACKey(zone, service, carrier, month)]
		if !present || !observation.Present || len(observation.SourceIDs) == 0 {
			return nil, fmt.Errorf("an unobserved or allocated carrier cannot use the exact direct presentation proof")
		}
		quantity := epathSQLQuantity{}
		for _, id := range observation.SourceIDs {
			identity, exists := frames.DirectHVACSourceIdentities[id]
			if !exists || seen[id] || identity.Source.DictionaryIndex != id || identity.Service != service || identity.Carrier != carrier || !strings.EqualFold(identity.Owner.ZoneName, zone) {
				return nil, fmt.Errorf("exact direct presentation has a duplicated or foreign component")
			}
			if err := epathSQLValidateDirectHVACSourceIdentity(identity); err != nil {
				return nil, err
			}
			values, _ := epathSQLMonthly(identity.Source, identity.Precision)
			quantity = quantity.add(values[month-1].positive())
			to += round(values[month-1].Value)
			seen[id] = true
		}
		if !epathSQLZoneCarrierQuantityEqual(quantity, observation.Quantity) {
			return nil, fmt.Errorf("exact direct presentation disagrees with its original component sum")
		}
		rawDirect = rawDirect.add(quantity)
	}
	to = round(to)
	if !epathOracleFinite(from) || !epathOracleFinite(to) || from <= 0 || to <= 0 || rawDirect.Value <= 0 {
		return nil, fmt.Errorf("exact conversion kind requires two positive presented endpoints")
	}
	return &epathSQLConversionProof{From: epathSQLQuantity{Value: from}, To: epathSQLQuantity{Value: to}}, nil
}
