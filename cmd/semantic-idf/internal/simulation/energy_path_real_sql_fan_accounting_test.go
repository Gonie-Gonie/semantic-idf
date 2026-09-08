package simulation

import (
	"fmt"
	"reflect"
	"strings"
)

// A fully audited partition has one independent rounding interval per reported
// pool. The broad meter remains a separate observation, not the allocation's
// numerical source. Pool shares conserve each pool; adding Zone-share errors
// again would double-count uncertainty in the allocated total.
type epathSQLFanPoolTotal struct {
	Pools  []epathRealSQLFanPool
	Months [12]epathSQLQuantity
}

func epathSQLFanAllocatedMonth(checks epathSQLModelChecks, model epathRealSQLModel, auxiliary epathRealSQLAuxiliary, month int, meter epathSQLQuantity) (*epathSQLQuantity, error) {
	declared := []epathRealSQLFanPool{}
	for _, pool := range model.FanPools {
		if pool.SiteID == auxiliary.SiteID {
			declared = append(declared, pool)
		}
	}
	if len(declared) == 0 {
		if _, exists := checks.FanPoolTotals[auxiliary.SiteID]; exists {
			return nil, fmt.Errorf("audited fan pools were removed from the declaration")
		}
		return nil, nil // This auxiliary has no reviewed reported-pool policy.
	}
	proof, ok := checks.FanPoolTotals[auxiliary.SiteID]
	if !ok || !reflect.DeepEqual(declared, proof.Pools) || month < 1 || month > 12 || !meter.valid() || auxiliary.Weight != "cooling_plus_heating" || auxiliary.AllocationMethod != "air_loop_load_share" {
		return nil, fmt.Errorf("fan allocation requires the completed exact independent pool proof")
	}
	owners := map[string]bool{}
	for _, pool := range declared {
		for _, zone := range pool.ServedZones {
			key := strings.ToLower(zone)
			if key == "" || owners[key] {
				return nil, fmt.Errorf("fan accounting has ambiguous pool ownership")
			}
			owners[key] = true
		}
	}
	for _, zone := range auxiliary.ServedZones {
		key := strings.ToLower(zone)
		if !owners[key] {
			return nil, fmt.Errorf("fan auxiliary membership contradicts its audited pools")
		}
		delete(owners, key)
	}
	if len(owners) != 0 {
		return nil, fmt.Errorf("fan auxiliary omits an audited pool owner")
	}
	value := proof.Months[month-1]
	if !value.valid() || value.Value < 0 || !epathSQLFanPoolNear(value.Value, meter.Value) {
		return nil, fmt.Errorf("independent allocated fan pools do not close the original meter")
	}
	return &value, nil
}
