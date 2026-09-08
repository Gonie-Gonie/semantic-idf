package simulation

// RunSimulation returns a native bundle, whereas the independent oracle checks
// original v2 scalar presence. Use the same actual writer/preflight/plain-reader
// boundary as a saved rebuild, only in a separate verification copy. The caller
// must first retain its untouched capture, run result and provenance hashes.
func epathRealCaptureOracleEvidence(evidence epathRealRunEvidence) (epathRealRunEvidence, error) {
	bundle, err := epathRealRebuiltBundleForOracle(evidence.Bundle)
	if err != nil {
		return epathRealRunEvidence{}, err
	}
	oracleEvidence := evidence
	oracleEvidence.Bundle = bundle
	return oracleEvidence, nil
}
