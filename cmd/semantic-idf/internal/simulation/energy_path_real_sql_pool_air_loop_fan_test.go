package simulation

// Independent acceptance support. Pool-specific original qualification, not fan quantity math.
// The caller must compile native Pool frames before it can publish fan binding.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

type epathSQLPoolAirLoopFanBinding struct {
	Original epathSQLPoolOriginalProof
	Fan      epathRealSQLAirLoopFan
}

// Return nil only for the genuinely absent opt-in. A present but unresolved
// Pool must fail, never fall through to the legacy mixer-only route validator.
func epathSQLBindPoolAirLoopFans(original string, declarations []epathRealSQLPoolSystem, sources []epathSQLPoolSourceFrames, fans []epathRealSQLAirLoopFan) (map[string]epathSQLPoolAirLoopFanBinding, error) {
	if len(declarations) == 0 {
		if len(sources) != 0 {
			return nil, fmt.Errorf("Pool fan source frames exist without the original Pool declaration")
		}
		return nil, nil
	}
	if len(declarations) != 1 || len(sources) != 1 || len(fans) != 1 {
		return nil, fmt.Errorf("Pool fan requires one finite original system, compiled source frame and fan declaration")
	}
	proofs, err := epathSQLValidatePoolOriginal(original, declarations)
	if err != nil {
		return nil, err
	}
	if len(proofs) != 1 || !reflect.DeepEqual(proofs[0], sources[0].Original) {
		return nil, fmt.Errorf("Pool fan source frame lost exact original text/hash/topology binding")
	}
	if err = epathSQLValidatePoolSourceFrames(sources[0]); err != nil {
		return nil, fmt.Errorf("Pool fan requires previously compiled native source observations: %w", err)
	}
	if err = epathSQLPoolAirLoopFanIdentity(proofs[0], fans[0]); err != nil {
		return nil, err
	}
	if err = epathSQLPoolFanOriginalOccurrences(original, fans[0]); err != nil {
		return nil, err
	}
	// Detach every nested declaration/owner/recipient slice. Later mutation of
	// current source frames must not also rewrite the original binding stamp.
	var binding epathSQLPoolAirLoopFanBinding
	raw, err := json.Marshal(epathSQLPoolAirLoopFanBinding{Original: proofs[0], Fan: fans[0]})
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &binding); err != nil {
		return nil, err
	}
	return map[string]epathSQLPoolAirLoopFanBinding{fans[0].SiteID: binding}, nil
}

func epathSQLPoolFanOriginalOccurrences(original string, fan epathRealSQLAirLoopFan) error {
	doc, err := idf.Parse(original)
	if err != nil {
		return err
	}
	count := 0
	for _, object := range doc.Objects {
		for at := 0; at+1 < len(object.Fields); at++ {
			if !strings.EqualFold(epathSQLSharedField(object, at), fan.ObjectType) || !epathSQLSharedSame(epathSQLSharedField(object, at+1), fan.ObjectName) {
				continue
			}
			count++
			if !strings.EqualFold(object.Type, "Branch") || at < 2 || (at-2)%4 != 0 || at+4 != len(object.Fields) {
				return fmt.Errorf("Pool fan has a non-native or non-outlet consuming reference")
			}
		}
	}
	if count != 1 {
		return fmt.Errorf("Pool fan has missing or competing original consuming references")
	}
	return nil
}

func epathSQLPoolAirLoopFanIdentity(original epathSQLPoolOriginalProof, fan epathRealSQLAirLoopFan) error {
	d := original.Declaration
	if fan.SiteID != "fans.electricity" || fan.ObjectType != "Fan:VariableVolume" || !epathSQLSharedSame(fan.ObjectName, d.FanName) || !epathSQLSharedSame(fan.AirLoopName, d.AirLoopName) {
		return fmt.Errorf("Pool fan declaration differs from the proved native fan or air loop")
	}
	want, err := epathSQLAirLoopFanZones(original.ServedZones)
	if err != nil {
		return err
	}
	declared, err := epathSQLAirLoopFanZones(d.ServedZones)
	if err != nil {
		return err
	}
	got, err := epathSQLAirLoopFanZones(fan.ServedZones)
	if err != nil {
		return err
	}
	if len(want) != 5 || !reflect.DeepEqual(want, got) || !reflect.DeepEqual(want, declared) {
		return fmt.Errorf("Pool fan does not have the exact original five served recipients")
	}
	for _, zone := range fan.ServedZones {
		if epathSQLSharedSame(zone, d.ReturnPlenumZoneName) {
			return fmt.Errorf("Pool fan cannot serve its return plenum")
		}
	}
	fans := 0
	for _, owner := range original.Owners {
		if !strings.HasPrefix(strings.ToLower(owner.ObjectType), "fan:") {
			continue
		}
		fans++
		if owner.ObjectType != fan.ObjectType || !epathSQLSharedSame(owner.ObjectName, fan.ObjectName) || owner.ObjectIndex < 0 || owner.PlantLoopName != "" || owner.ZoneName != "" || owner.FuelType != "" {
			return fmt.Errorf("Pool fan is not the unique original central air component")
		}
	}
	if fans != 1 {
		return fmt.Errorf("Pool original proof has a missing or duplicate fan owner")
	}
	return nil
}

// Called in addition to, never instead of, existing fan declaration, auxiliary
// roster, policy and numeric validation. Binding is a detached original stamp.
func epathSQLValidatePoolAirLoopFanBinding(binding epathSQLPoolAirLoopFanBinding, declarations []epathRealSQLPoolSystem, sources []epathSQLPoolSourceFrames, fan epathRealSQLAirLoopFan) error {
	if len(declarations) != 1 || len(sources) != 1 || !reflect.DeepEqual(binding.Fan, fan) || !reflect.DeepEqual(binding.Original.Declaration, declarations[0]) || !reflect.DeepEqual(binding.Original, sources[0].Original) {
		return fmt.Errorf("Pool fan original declaration/hash/source binding changed after compilation")
	}
	if err := epathSQLValidatePoolSourceFrames(sources[0]); err != nil {
		return fmt.Errorf("Pool fan source proof changed after binding: %w", err)
	}
	return epathSQLPoolAirLoopFanIdentity(binding.Original, fan)
}
