package simulation

import "github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"

// Requesting a paid-input meter is an observation probe, not evidence of a
// conditioning service. Keep this after ordinary Hourly expansion: the finite
// zero-service boundary needs actual Monthly request-backed absence, not an
// invented zero or an otherwise unnecessary Hourly chart source.
func (builder *purposePlanBuilder) addEnergyPathSimpleVentilationConditioningProbes() {
	selected, scoped := purposeSelectedZoneSet(builder.request.Scope)
	inScope := false
	for _, target := range energyPathSimpleVentilationTargets(builder.doc) {
		if !scoped || selected[normalizePurposeToken(target.ZoneName)] {
			inScope = true
			break
		}
	}
	if !inScope {
		return
	}
	for _, name := range []string{"Heating:Electricity", "Cooling:Electricity", "Heating:NaturalGas"} {
		output := PurposeOutputObject{
			ObjectType:  "Output:Meter",
			Fields:      []idf.OutputFieldValue{{Name: "Key Name", Value: name}, {Name: "Reporting Frequency", Value: "Monthly"}},
			PurposeIDs:  []SimulationPurposeID{SimulationPurposeBasicEnergy},
			Weight:      "medium",
			Reason:      "Basic Energy Path",
			Description: "Native paid-conditioning availability probe for simple ventilation; absence is not measured zero and does not create a Heating or Cooling service.",
		}
		// Normal addObject semantics reuse an exact Output:Meter opener.
		// Keep that actual transport: the independent paid-absence binding
		// deliberately does not borrow MeterFileOnly or another frequency.
		builder.addObject(output)
	}
}
