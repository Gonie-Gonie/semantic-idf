package simulation

// Two bounded tests only: the missing topology proof, not new SQL arithmetic.
import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Gonie-Gonie/semantic-idf/cmd/semantic-idf/internal/idf"
)

func epathSQLHeatOnlyOriginalFixture(t *testing.T) (string, epathRealSQLHeatOnlyFurnace) {
	t.Helper()
	data, err := os.ReadFile("testdata/energy_path_real_models/models/25.1/Furnace.idf")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "2a221defefdffa3792d69924b439f466094ccb4a9fe14aa750e5ffe817fcd1f8" {
		t.Fatal("Furnace original changed")
	}
	return string(data), epathRealSQLHeatOnlyFurnace{AirLoopName: "Typical Terminal Reheat 1", FurnaceName: "Gas Furnace 1", FanName: "Supply Fan 1", FuelCoilName: "Furnace Coil", ControlZone: "East Zone", ServedZones: []string{"West Zone", "EAST ZONE", "NORTH ZONE"}}
}
func TestEnergyPathSQLHeatOnlyOriginalThreeRecipientsNotControlOnly(t *testing.T) {
	text, d := epathSQLHeatOnlyOriginalFixture(t)
	p, err := epathSQLValidateHeatOnlyFurnaceOriginal(text, d)
	if err != nil || len(p.ServedZones) != 3 || len(p.Owners) != 8 {
		t.Fatalf("actual original route rejected: %#v %v", p, err)
	}
	for _, placement := range []string{"", "DrawThrough"} {
		t.Run("native placement "+placement, func(t *testing.T) {
			doc, err := idf.Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			one := func(typ string) *idf.Object {
				for i := range doc.Objects {
					if doc.Objects[i].Type == typ {
						return &doc.Objects[i]
					}
				}
				t.Fatalf("missing original %s", typ)
				return nil
			}
			furnace := one("AirLoopHVAC:Unitary:Furnace:HeatOnly")
			fan := one("Fan:OnOff")
			coil := one("Coil:Heating:Fuel")
			furnace.Fields[10].Value = placement
			if placement == "DrawThrough" {
				// Rewire the actual inner ports, preserving the wrapper/Branch ports.
				bridge := fan.Fields[8].Value
				coil.Fields[5].Value = furnace.Fields[2].Value
				coil.Fields[6].Value = bridge
				fan.Fields[7].Value = bridge
				fan.Fields[8].Value = furnace.Fields[3].Value
			}
			proof, err := epathSQLValidateHeatOnlyFurnaceOriginal(doc.String(), d)
			if err != nil || len(proof.ServedZones) != 3 || len(proof.Owners) != 8 {
				t.Fatalf("native placement route rejected: %#v %v", proof, err)
			}
		})
	}
	d.ServedZones = []string{"East Zone"}
	if _, err := epathSQLValidateHeatOnlyFurnaceOriginal(text, d); err == nil {
		t.Fatal("control-only recipients accepted")
	}
}
func TestEnergyPathSQLHeatOnlyOriginalDisconnectedRoutesRejected(t *testing.T) {
	text, d := epathSQLHeatOnlyOriginalFixture(t)
	for _, edit := range []struct {
		typ   string
		field int
		value string
	}{
		{"Fan:OnOff", 8, "Disconnected fan outlet"},
		{"AirTerminal:SingleDuct:ConstantVolume:NoReheat", 2, "Disconnected terminal inlet"},
		{"AirLoopHVAC:ZoneMixer", 2, "Foreign Zone return"},
		{"AirLoopHVAC:Unitary:Furnace:HeatOnly", 9, "Missing fan"},
		{"Coil:Heating:Fuel", 2, "Electricity"},
	} {
		t.Run(edit.typ, func(t *testing.T) {
			doc, err := idf.Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			for i := range doc.Objects {
				if doc.Objects[i].Type == edit.typ {
					doc.Objects[i].Fields[edit.field].Value = edit.value
					break
				}
			}
			if _, err := epathSQLValidateHeatOnlyFurnaceOriginal(doc.String(), d); err == nil {
				t.Fatal("disconnected/changed source topology accepted")
			}
		})
	}
	for _, short := range []string{"wrapper", "fan", "coil", "all"} {
		t.Run("connected short circuit "+short, func(t *testing.T) {
			doc, err := idf.Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			one := func(typ string) *idf.Object {
				for i := range doc.Objects {
					if doc.Objects[i].Type == typ {
						return &doc.Objects[i]
					}
				}
				t.Fatalf("missing original %s", typ)
				return nil
			}
			furnace := one("AirLoopHVAC:Unitary:Furnace:HeatOnly")
			fan := one("Fan:OnOff")
			coil := one("Coil:Heating:Fuel")
			branch := one("Branch")
			air := one("AirLoopHVAC")
			// Keep every inter-component equality valid, so a disconnected-route
			// check alone cannot reject these native inlet == outlet mutations.
			switch short {
			case "wrapper":
				furnace.Fields[3].Value = furnace.Fields[2].Value
				coil.Fields[6].Value = furnace.Fields[3].Value
				branch.Fields[5].Value = furnace.Fields[3].Value
				air.Fields[9].Value = furnace.Fields[3].Value
			case "fan":
				fan.Fields[8].Value = fan.Fields[7].Value
				coil.Fields[5].Value = fan.Fields[8].Value
			case "coil":
				coil.Fields[5].Value = coil.Fields[6].Value
				fan.Fields[8].Value = coil.Fields[5].Value
			case "all":
				for _, pair := range []struct {
					object *idf.Object
					slots  []int
				}{{furnace, []int{2, 3}}, {fan, []int{7, 8}}, {coil, []int{5, 6}}, {branch, []int{4, 5}}, {air, []int{6, 9}}} {
					for _, slot := range pair.slots {
						pair.object.Fields[slot].Value = "same native node"
					}
				}
			}
			want := short
			if want == "all" {
				want = "wrapper"
			}
			if _, err := epathSQLValidateHeatOnlyFurnaceOriginal(doc.String(), d); err == nil || !strings.Contains(err.Error(), "HeatOnly "+want+" native inlet and outlet must be distinct") {
				t.Fatalf("connected native short circuit did not fail at distinct-port proof: %v", err)
			}
		})
	}
}
