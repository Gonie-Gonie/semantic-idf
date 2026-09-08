package simulation

import "strings"

const (
	energyPathThermalBoundaryActiveSurfaceSource = "active_surface_source"
	energyPathThermalBoundaryMixed               = "mixed_thermal_boundaries"
)

// Empty is the unchanged legacy boundary, not an identity element: adding an
// active surface source to an unmarked air/other load produces a mixed boundary.
// Call this only when two actual selected contributors are being combined,
// never for an empty accumulator or inspector-only alternative provenance.
func mergeEnergyPathThermalBoundary(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == next {
		return current
	}
	return energyPathThermalBoundaryMixed
}

func energyPathThermalBoundaryRatioLabel(boundary string) string {
	switch strings.TrimSpace(boundary) {
	case energyPathThermalBoundaryActiveSurfaceSource:
		return "Active surface source / site energy"
	case energyPathThermalBoundaryMixed:
		return "Mixed thermal boundaries / site energy"
	default:
		return ""
	}
}
