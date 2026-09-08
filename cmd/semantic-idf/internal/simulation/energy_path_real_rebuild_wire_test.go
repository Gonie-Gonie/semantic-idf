package simulation

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Saved rebuilds must inspect the same actual v2 wire as saved snapshots. Native
// observed-value proof is private; only the production v2 writer decides which
// zero scalars are emitted. Do not teach the oracle to infer missing zeros, or
// use the application's compatibility decoder to repair the resulting payload.
func epathRealRebuiltBundleForOracle(bundle PurposeResultBundle) (PurposeResultBundle, error) {
	if bundle.EnergyExplanation.Schema != energyExplanationSchema {
		return PurposeResultBundle{}, fmt.Errorf("rebuilt oracle input requires original canonical v2; legacy upgrade is not an acceptance boundary")
	}
	wire, err := json.Marshal(bundle)
	if err != nil {
		return PurposeResultBundle{}, fmt.Errorf("marshal rebuilt candidate: %w", err)
	}
	if err := epathValidateOracleCandidateWire(bytes.NewReader(wire)); err != nil {
		return PurposeResultBundle{}, err
	}
	return epathDecodeOriginalOracleCandidate(bytes.NewReader(wire))
}
