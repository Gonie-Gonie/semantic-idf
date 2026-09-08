package simulation

import (
	"reflect"
	"strings"
	"testing"
)

func TestEnergyPathAllocationOverlapMessageDoesNotClaimPhysicalExcess(t *testing.T) {
	for _, period := range []string{"M1", "annual"} {
		for _, overlap := range []float64{.002, 25} {
			message := energyPathAllocationOverlapMessage("Cooling", "Electricity", overlap, "kWh", period)
			if !strings.Contains(message, "Rounded reported allocation totals") || !strings.Contains(message, "original source precision/ownership") || !strings.Contains(message, "no negative remainder") || strings.Contains(message, "Exact direct zone") {
				t.Fatalf("overlap=%g period=%s lost accounting/source context: %s", overlap, period, message)
			}
			if strings.Contains(message, "without cancelling gaps") != (period == "annual") {
				t.Fatalf("annual period aggregation explanation has wrong scope: %s", message)
			}
			if strings.Contains(message, "only rounding") || strings.Contains(message, "ignore") {
				t.Fatal("unproved small size was used to dismiss a real source mismatch")
			}
		}
	}
}

func TestEnergyPathAllocationOverlapWordingPreservesLedger(t *testing.T) {
	for _, tc := range []struct {
		name, period, status                     string
		expected, direct, gap, overlap, residual float64
	}{
		{"tiny monthly overlap", "M1", "overmapped", 10, 10.001, 0, .001, -.001},
		{"annual gaps cannot cancel overlaps", "annual", "partial", 10, 9.999, .003, .002, .001},
		{"material mismatch remains warning", "annual", "overmapped", 10, 35, 0, 25, -25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := energyPathZoneHVACAllocationRecord{Period: tc.period, ServiceKind: "cooling", Carrier: "electricity", Unit: "kWh", ExpectedValue: tc.expected, DirectValue: tc.direct, UnassignedValue: tc.gap, OvermappedValue: tc.overlap, SourceIDs: []string{"original-meter", "original-coil"}}
			before := record
			rows, warnings := appendEnergyPathZoneHVACAllocationRecords(nil, nil, []energyPathZoneHVACAllocationRecord{record}, tc.period)
			if !reflect.DeepEqual(record, before) || len(rows) != 1 {
				t.Fatal("wording changed original ledger inputs")
			}
			row := rows[0]
			if row.ExpectedValue != tc.expected || row.DirectValue != tc.direct || row.AllocatedValue != 0 || row.UnassignedValue != tc.gap || row.OvermappedValue != tc.overlap || row.ResidualValue != tc.residual || row.Status != tc.status {
				t.Fatalf("wording changed accounting or threshold: %+v", row)
			}
			found := false
			for _, warning := range warnings {
				if warning.Code == "direct_zone_hvac_energy_exceeds_building" {
					found = true
					if warning.Severity != "warning" || !strings.Contains(warning.Message, "Rounded reported") {
						t.Fatal("physical diagnostic was silently suppressed")
					}
				}
			}
			if !found {
				t.Fatal("overlap evidence was removed")
			}
		})
	}
}
