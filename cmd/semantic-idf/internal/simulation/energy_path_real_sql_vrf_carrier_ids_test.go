package simulation

import (
	"fmt"
	"reflect"
	"strings"
)

// Native VRF reserves the complete owned service pools before the legacy Zone
// projection. With no other allocated path, no Building reconciliation is
// inherited: the Zone carrier pass creates a fresh plain Monthly ID, whose
// Annual aggregate is also plain. This is a source/ownership proof, not a
// fallback search for whichever ID happens to occur in the candidate.
func epathSQLZoneCarrierNativeVRFMonthlyIDs(frames epathSQLFrames, model epathRealSQLModel, zone string) (bool, error) {
	if len(model.NativeVRFSystems) == 0 {
		if len(frames.NativeVRFSystems) != 0 || len(frames.NativeVRFAllocations) != 0 {
			return false, fmt.Errorf("native carrier IDs require declared original systems")
		}
		return false, nil // The six previously reviewed models keep their route.
	}
	key := strings.ToLower(zone)
	originalZone, exists := frames.Zones[key]
	if !exists || originalZone.Name == "" || strings.ToLower(originalZone.Name) != key {
		return false, fmt.Errorf("native carrier ID requested for a foreign Zone")
	}
	if len(model.Services) != 2 || len(model.DirectHVACComponents) != 0 || len(frames.DirectHVAC) != 0 || len(frames.DirectHVACSourceIdentities) != 0 || len(model.FanPools) != 0 {
		return false, fmt.Errorf("native carrier IDs cannot infer a mixed or incomplete allocation route")
	}
	auxSites := map[string]bool{}
	for _, auxiliary := range model.Auxiliaries {
		if auxiliary.SiteID == "" || auxSites[auxiliary.SiteID] || auxiliary.Weight != "unassigned" || auxiliary.AllocationMethod != "unassigned" || len(auxiliary.ServedZones) != 0 || auxiliary.WeightSource != nil {
			return false, fmt.Errorf("native carrier IDs require exclusively unassigned auxiliary pools")
		}
		auxSites[auxiliary.SiteID] = true
	}
	services, serviceSites := map[string]bool{}, map[string]bool{}
	owned := false
	for _, service := range model.Services {
		if service.Service != "cooling" && service.Service != "heating" || services[service.Service] {
			return false, fmt.Errorf("native carrier IDs require exactly both original services")
		}
		services[service.Service] = true
		for _, id := range service.SiteIDs {
			if serviceSites[id] || auxSites[id] {
				return false, fmt.Errorf("native carrier pool has duplicate or auxiliary ownership")
			}
			serviceSites[id] = true
			// Retain the original broad meter binding as well as native closure;
			// coordinated changes to two cached quantities are not evidence.
			ids := frames.SiteSources[id]
			annual, err := epathSQLSiteIsAnnual(frames, id)
			if err != nil || annual || len(ids) != 1 || len(frames.Site[id]) != 12 {
				return false, fmt.Errorf("native carrier pool lacks a unique Monthly meter")
			}
			meter, exists := frames.SourceIdentities[ids[0]]
			name := "Cooling:Electricity"
			if service.Service == "heating" {
				name = "Heating:Electricity"
			}
			if !exists || ids[0] <= 0 || meter.DictionaryIndex != ids[0] || meter.Name != name || meter.KeyValue != "" || !meter.IsMeter || meter.SourceUnit != "J" || meter.ReportingFrequency != "Monthly" || meter.MissingRows != 0 {
				return false, fmt.Errorf("native carrier pool changed its original meter identity")
			}
			quantities, err := epathSQLMonthly(meter, model.Precision)
			if err != nil || !reflect.DeepEqual(quantities, frames.SourceRaw[ids[0]]) {
				return false, fmt.Errorf("native carrier pool changed its original Monthly quantities")
			}
			for month, q := range quantities {
				value := frames.Site[id][month]
				if meter.Months[month].MissingRows != 0 || value == nil || !q.valid() || q.Value < 0 || !value.valid() {
					return false, fmt.Errorf("native carrier pool has an unknown or altered month")
				}
				low, high := q.bounds()
				actualLow, actualHigh := value.bounds()
				if value.Value != q.Value || actualLow != low || actualHigh != high {
					return false, fmt.Errorf("native carrier pool differs from its exact original quantity")
				}
			}
		}
		// This independently revalidates source roster, original owners, all
		// twelve load weights, saved integer allocation and raw 128-ULP closure.
		native, err := epathSQLCompileVRFService(frames, model, service)
		if err != nil || native == nil {
			return false, fmt.Errorf("native carrier IDs lack complete service proof: %v", err)
		}
		for _, period := range epathSQLZoneCarrierPeriods() {
			proof := native.Zones[key][period]
			if proof == nil || proof.NativeVRF == nil || proof.NativeVRF.Service != service.Service || proof.NativeVRF.Period != period || !strings.EqualFold(proof.NativeVRF.ZoneName, originalZone.Name) {
				return false, fmt.Errorf("native carrier IDs lost exact service/Zone/period ownership")
			}
			if service.Service == model.Services[0].Service && period == "annual" {
				owned = proof.NativeVRF.Owned
			} else if proof.NativeVRF.Owned != owned {
				return false, fmt.Errorf("native carrier IDs have inconsistent service ownership")
			}
		}
	}
	for _, site := range model.Site {
		if !site.Facility && (site.EndUse == "cooling" || site.EndUse == "heating") && !serviceSites[site.ID] {
			return false, fmt.Errorf("native carrier IDs cannot discard a foreign HVAC site pool")
		}
	}
	return owned, nil // An observed but unserved Plenum is not a native owner.
}
