package idf

import (
	"strings"
	"unicode"
)

// Store source-ordered positions rather than copies of the geometry records.
// Owned-node centroids, enclosure checks and zone signatures share this index.
type thermalGeometryOwnershipIndex struct {
	surfacesByZone  map[string][]int
	surfacesBySpace map[string][]int
	spacesByZone    map[string][]int
}

func newThermalGeometryOwnershipIndex(geometry GeometryReport) thermalGeometryOwnershipIndex {
	index := thermalGeometryOwnershipIndex{
		surfacesByZone:  map[string][]int{},
		surfacesBySpace: map[string][]int{},
		spacesByZone:    map[string][]int{},
	}
	for position, surface := range geometry.Surfaces {
		if surface.IsShading {
			continue
		}
		zoneKey := thermalOwnerNameKey(surface.ZoneName)
		spaceKey := thermalOwnerNameKey(surface.SpaceName)
		index.surfacesByZone[zoneKey] = append(index.surfacesByZone[zoneKey], position)
		index.surfacesBySpace[spaceKey] = append(index.surfacesBySpace[spaceKey], position)
	}
	for position, space := range geometry.Spaces {
		key := thermalOwnerNameKey(space.ZoneName)
		index.spacesByZone[key] = append(index.spacesByZone[key], position)
	}
	return index
}

// Match strings.EqualFold exactly, including Unicode simple-fold cycles and
// significant whitespace. normalizeName trims whitespace and has different
// behavior for names such as the Kelvin sign and long s.
func thermalOwnerNameKey(name string) string {
	return strings.Map(func(character rune) rune {
		if character < unicode.MaxASCII {
			if character >= 'a' && character <= 'z' {
				return character - ('a' - 'A')
			}
			return character
		}
		canonical := character
		for folded := unicode.SimpleFold(character); folded != character; folded = unicode.SimpleFold(folded) {
			if folded < canonical {
				canonical = folded
			}
		}
		return canonical
	}, name)
}

func thermalIndexedCentroid(surfaces []GeometrySurface, positions []int) GeometryPoint {
	var sum GeometryPoint
	count := 0
	for _, position := range positions {
		for _, point := range surfaces[position].WorldVertices {
			sum.X += point.X
			sum.Y += point.Y
			sum.Z += point.Z
			count++
		}
	}
	return thermalRoundedCentroid(sum, count)
}
