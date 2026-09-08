package simulation

import (
	"fmt"
	"reflect"
	"strings"
)

// A combined VRF service is one allocated carrier contribution. Its separately
// observed local inputs are retained by source checks and the Building ledger,
// not relabeled as a second direct carrier branch beside the same subtotal.
func epathSQLValidateVRFServiceConsumer(service *epathSQLZoneServiceProof) error {
	if service == nil || service.NativeVRF == nil || service.DirectHVAC || service.AnnualTabular || service.Unavailable || service.Basis != "service_path_allocation" {
		return fmt.Errorf("native VRF consumer requires one independent combined service proof")
	}
	if err := epathSQLValidateVRFZoneServiceProof(service); err != nil {
		return err
	}
	proof := service.NativeVRF
	if proof.ZoneName == "" || proof.Service != service.Service || !epathOracleValidPeriod(proof.Period) || len(service.Carriers) != 1 {
		return fmt.Errorf("native VRF consumer lost its exact Zone/service/period/carrier")
	}
	value, ok := service.Carriers["electricity"]
	if !ok || !epathSQLZoneCarrierQuantityEqual(value, proof.Value) || !proof.Direct.valid() || !proof.Allocated.valid() || proof.Direct.Value < 0 || proof.Allocated.Value < 0 ||
		!epathSQLZoneCarrierQuantityEqual(proof.Direct.add(proof.Allocated), proof.Value) {
		return fmt.Errorf("native VRF local/shared partitions contradict its independent subtotal")
	}
	if !proof.Owned {
		if value.Value != 0 || proof.Direct.Value != 0 || proof.Allocated.Value != 0 {
			return fmt.Errorf("unserved Zone acquired native VRF consumption")
		}
		for _, originals := range proof.Originals {
			if len(originals) != 0 {
				return fmt.Errorf("unserved Zone acquired native VRF source ownership")
			}
		}
		return nil
	}
	if len(proof.Originals) != 1 || len(proof.Originals["electricity"]) == 0 {
		return fmt.Errorf("owned native VRF service lacks original constituent identities")
	}
	for carrier, originals := range proof.Originals {
		if carrier != "electricity" {
			return fmt.Errorf("native VRF service acquired a different carrier")
		}
		if err := epathSQLValidateOriginalSources(originals); err != nil {
			return err
		}
		for _, original := range originals {
			identity := original.NativeVRF
			if identity == nil || identity.Observation.Service != proof.Service {
				return fmt.Errorf("native VRF quality contains an unproved or different-service input")
			}
			if !identity.Observation.Shared && !strings.EqualFold(identity.Observation.ZoneName, proof.ZoneName) {
				return fmt.Errorf("native VRF quality contains another Zone's local input")
			}
			owner := false
			for _, terminal := range identity.System.Terminals {
				owner = owner || strings.EqualFold(terminal.ZoneName, proof.ZoneName)
			}
			if !owner {
				return fmt.Errorf("native VRF quality contains an outdoor system that does not serve this Zone")
			}
		}
	}
	return nil
}

func epathSQLVRFQualitySources(quality *epathSQLQualityProof, service *epathSQLZoneServiceProof) error {
	if quality == nil {
		return fmt.Errorf("native VRF quality destination is absent")
	}
	if err := epathSQLValidateVRFServiceConsumer(service); err != nil {
		return err
	}
	originals := map[string]map[string]epathSQLOriginalSource{}
	for carrier, values := range service.NativeVRF.Originals {
		originals[carrier] = map[string]epathSQLOriginalSource{}
		for key, value := range values {
			originals[carrier][key] = value
		}
	}
	if quality.SiteSources == nil {
		quality.SiteSources = map[string]map[string][]int{}
	}
	if quality.SiteOriginals == nil {
		quality.SiteOriginals = map[string]map[string]map[string]epathSQLOriginalSource{}
	}
	quality.SiteSources[service.Service] = map[string][]int{}
	quality.SiteOriginals[service.Service] = originals
	return nil
}

// Denominator observations may explain the conversion allocation, but are not
// additional delivered load at its thermal endpoint or purchased site energy.
func epathSQLVRFQualityWeightSources(check epathSQLModelCheck, service string) (map[string]epathSQLOriginalSource, error) {
	out := map[string]epathSQLOriginalSource{}
	if check.Quality == nil {
		return nil, fmt.Errorf("VRF ratio weights lack a quality proof")
	}
	for _, dependency := range check.Quality.Dependencies {
		proof := dependency.ZoneService
		if proof == nil || proof.NativeVRF == nil || proof.Service != service {
			continue
		}
		if check.Item.Scope != "zone" || dependency.Item.Scope != check.Item.Scope || !strings.EqualFold(dependency.Item.Zone, check.Item.Zone) || dependency.Item.Period != check.Item.Period ||
			!strings.EqualFold(proof.NativeVRF.ZoneName, check.Item.Zone) || proof.NativeVRF.Period != check.Item.Period {
			return nil, fmt.Errorf("VRF denominator proof escaped its exact Zone/service/period")
		}
		if err := epathSQLValidateVRFServiceConsumer(proof); err != nil {
			return nil, err
		}
		for key, original := range proof.NativeVRF.Weights {
			if previous, exists := out[key]; exists && !reflect.DeepEqual(previous, original) {
				return nil, fmt.Errorf("VRF denominator has conflicting original source evidence")
			}
			out[key] = original
		}
	}
	return out, nil
}
