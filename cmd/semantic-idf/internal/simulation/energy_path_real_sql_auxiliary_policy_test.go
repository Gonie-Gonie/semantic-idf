package simulation

import (
	"fmt"
	"strings"
)

// These are independently reviewed broad-meter policies, not a fallback for
// arbitrary auxiliaries. Fan pools and unassigned evidence keep their separate
// contracts. In particular, a condenser load is not a heating-plant load.
type epathSQLAuxiliaryPolicy struct {
	MeterName string
	Services  []string
}

func epathSQLAllocatedAuxiliaryPolicy(endUse, carrier, weight, method string) (epathSQLAuxiliaryPolicy, error) {
	if carrier == "electricity" {
		if endUse == "pumps" && weight == "cooling_plus_heating" && method == "plant_loop_load_share" {
			return epathSQLAuxiliaryPolicy{MeterName: "Pumps:Electricity", Services: []string{"cooling", "heating"}}, nil
		}
		if endUse == "heat_rejection" && weight == "cooling" && method == "condenser_loop_load_share" {
			return epathSQLAuxiliaryPolicy{MeterName: "HeatRejection:Electricity", Services: []string{"cooling"}}, nil
		}
	}
	return epathSQLAuxiliaryPolicy{}, fmt.Errorf("allocated auxiliary lacks an explicit supported site/carrier/weight/method proof")
}

func epathSQLAuxiliaryWeight(frames epathSQLFrames, model epathRealSQLModel, policy epathSQLAuxiliaryPolicy, zone string, month int) (epathSQLQuantity, error) {
	weight := epathSQLQuantity{}
	for _, service := range policy.Services {
		key := epathSQLKey(zone, service, month)
		load, exists := frames.Loads[key]
		if !exists || !load.valid() || load.Value < 0 {
			return epathSQLQuantity{}, fmt.Errorf("auxiliary weight lacks a known %s/%s/M%d load", zone, service, month)
		}
		if epathSQLNativeRadiantService(model, service) {
			ids := frames.LoadSourceIDs[key]
			if len(ids) != 1 {
				return epathSQLQuantity{}, fmt.Errorf("native radiant auxiliary weight needs one exact original equipment observation")
			}
			if _, err := epathSQLRadiantFrameLoadSource(frames, ids[0], zone, service, month); err != nil {
				return epathSQLQuantity{}, err
			}
		}
		weight = weight.add(load)
	}
	return weight, nil
}

func epathSQLAuxiliaryWeightSource(frames epathSQLFrames, model epathRealSQLModel, index int, zone, service string, month int) (epathSQLOriginalSource, error) {
	if epathSQLNativeRadiantService(model, service) {
		ids := frames.LoadSourceIDs[epathSQLKey(zone, service, month)]
		identity, err := epathSQLRadiantFrameLoadSource(frames, index, zone, service, month)
		if err != nil || len(ids) != 1 || ids[0] != index {
			return epathSQLOriginalSource{}, fmt.Errorf("auxiliary weight lost its exact native radiant source: %v", err)
		}
		return epathSQLOriginalRadiant(identity)
	}
	source, exists := frames.SourceIdentities[index]
	_, native := frames.RadiantLoadSourceIdentities[index]
	if native || !exists || source.DictionaryIndex != index || index <= 0 || source.IsMeter || !strings.EqualFold(source.KeyValue, zone) || source.ReportingFrequency != "Monthly" || source.SourceUnit != "J" {
		return epathSQLOriginalSource{}, fmt.Errorf("auxiliary weight source escapes original Zone/Monthly/J ownership")
	}
	return epathSQLOriginalRDD(source), nil
}
