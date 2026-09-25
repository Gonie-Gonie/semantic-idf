package simulation

import "fmt"

// With no conditioning service or allocated consumer in any period, the finite
// original SimpleVent cohort cannot inherit a legacy HVAC carrier row. Fresh
// direct rows, including their month-first annual total, have one plain ID.
// This is native/original qualification, never selection from candidate IDs.
func epathSQLSimpleVentilationPlainCarrierIDs(frames epathSQLFrames, model epathRealSQLModel, parts map[string]map[string]epathSQLZoneCarrierPart, observed ...epathRealOracleEvidence) (bool, error) {
	active := false
	for _, component := range model.DirectHVACComponents {
		active = active || component.ID == epathSQLSimpleVentilationFanFamily
	}
	if !active {
		return false, nil
	}
	if len(observed) != 1 {
		return false, fmt.Errorf("finite direct-only carrier identity requires one actual original/native evidence")
	}
	if err := epathSQLSimpleVentilationZeroServices(frames, model, observed[0]); err != nil {
		return false, err
	}
	if len(parts) != len(frames.Zones)*13 {
		return false, fmt.Errorf("finite direct-only carrier lost exact Zone/period registry")
	}
	for zone := range frames.Zones {
		for _, period := range epathSQLZoneCarrierPeriods() {
			context, ok := parts[epathSQLZoneCarrierContext(zone, period)]
			fan, fanOK := context["direct/fans"]
			if !ok || !fanOK || !fan.Direct || len(fan.ByCarrier) != 1 {
				return false, fmt.Errorf("finite carrier lost its native direct fan, including known-zero owner")
			}
			if _, ok := fan.ByCarrier["electricity"]; !ok {
				return false, fmt.Errorf("finite fan carrier is not electricity")
			}
			for family, part := range context {
				if family != "direct/fans" && family != "direct/lighting" && family != "direct/equipment" {
					return false, fmt.Errorf("finite direct-only carrier contains an unreviewed family")
				}
				if !part.Direct || part.Unavailable || part.AnnualTabular || len(part.ByCarrier) != 1 || len(part.DirectByCarrier) != 0 {
					return false, fmt.Errorf("finite carrier contains allocated/unknown/annual-only consumption")
				}
				q, ok := part.ByCarrier["electricity"]
				if !ok || !q.valid() || q.Value < 0 {
					return false, fmt.Errorf("finite direct-only carrier lost known nonnegative electricity")
				}
			}
		}
	}
	return true, nil
}
