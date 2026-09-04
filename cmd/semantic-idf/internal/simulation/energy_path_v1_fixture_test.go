package simulation

import (
	"os"
	"testing"
)

func writeEnergyPathV1ServiceFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(`
Version, 24.1;

Zone,
  ZONE ONE;

ZoneHVAC:EquipmentConnections,
  ZONE ONE,
  Zone One Equipment,
  Zone One Supply Inlet,
  ,
  Zone One Air Node,
  ;

ZoneHVAC:EquipmentList,
  Zone One Equipment,
  SequentialLoad,
  ZoneHVAC:IdealLoadsAirSystem,
  Zone One Ideal Loads,
  1,
  1,
  ,
  ;

ZoneHVAC:IdealLoadsAirSystem,
  Zone One Ideal Loads,
  Always On,
  Zone One Supply Inlet,
  ,
  ,
  50,
  13,
  0.015,
  0.009,
  NoLimit,
  Autosize,
  Autosize,
  NoLimit,
  Autosize,
  Autosize,
  ,
  ,
  None,
  0.7;
`), 0o644); err != nil {
		t.Fatal(err)
	}
}
