package simulation

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Exact paired Monthly values, scope/basis identities and source-ID sets for
// the three colliding annual branch families in the original real LargeOffice
// capture 20260907T162304. Other graph content is intentionally omitted.
var epathRealAnnualLinkIdentityRows = []EnergyPathLink{
	{Period: "M1", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 94504.149, ToValue: 411809.768, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1930", "sql-rdd-2070", "sql-rdd-2138", "sql-rdd-2206", "sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2410", "sql-rdd-2478", "sql-rdd-2546", "sql-rdd-2614", "sql-rdd-2682", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2886", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M2", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 55730.546, ToValue: 278132.057, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1930", "sql-rdd-2070", "sql-rdd-2138", "sql-rdd-2206", "sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2410", "sql-rdd-2478", "sql-rdd-2546", "sql-rdd-2614", "sql-rdd-2682", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2886", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M3", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 91.759, ToValue: 91.759, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1511", "sql-rdd-1575", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M3", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 17153.303, ToValue: 127620.352, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2138", "sql-rdd-2206", "sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2410", "sql-rdd-2478", "sql-rdd-2546", "sql-rdd-2614", "sql-rdd-2682", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2886", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M4", FromID: "driver.air.mechanical_ventilation.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 0.009, ToValue: 0.009, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"derived-driver-mechanical_ventilation_fallback_sensible-perimeter_bot_zn_1", "derived-driver-mechanical_ventilation_fallback_sensible-perimeter_mid_zn_4", "derived-driver-mechanical_ventilation_fallback_sensible-perimeter_top_zn_2", "sql-rdd-1770", "sql-rdd-1826", "sql-rdd-1842", "sql-rdd-1878", "sql-rdd-1879", "sql-rdd-1906", "sql-rdd-1907", "sql-rdd-1914", "sql-rdd-1915", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M4", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 1116.673, ToValue: 1116.673, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1511", "sql-rdd-1523", "sql-rdd-1541", "sql-rdd-1561", "sql-rdd-1575", "sql-rdd-1587", "sql-rdd-1625", "sql-rdd-1639", "sql-rdd-1651", "sql-rdd-1689", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M4", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 6499.058, ToValue: 67448.603, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2206", "sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2410", "sql-rdd-2478", "sql-rdd-2546", "sql-rdd-2614", "sql-rdd-2682", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2886", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M5", FromID: "driver.air.mechanical_ventilation.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "Perimeter_mid_ZN_2", ServiceKind: "cooling", FromValue: 0.009, ToValue: 0.009, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"derived-driver-mechanical_ventilation_fallback_sensible-perimeter_mid_zn_2", "sql-rdd-1810", "sql-rdd-1898", "sql-rdd-1899", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M5", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 4737.599, ToValue: 4737.599, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1467", "sql-rdd-1469", "sql-rdd-1471", "sql-rdd-1473", "sql-rdd-1487", "sql-rdd-1489", "sql-rdd-1491", "sql-rdd-1493", "sql-rdd-1511", "sql-rdd-1523", "sql-rdd-1541", "sql-rdd-1561", "sql-rdd-1575", "sql-rdd-1587", "sql-rdd-1605", "sql-rdd-1625", "sql-rdd-1639", "sql-rdd-1651", "sql-rdd-1669", "sql-rdd-1689", "sql-rdd-1699", "sql-rdd-1701", "sql-rdd-1703", "sql-rdd-1705", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M5", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 194.414, ToValue: 14183.979, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2206", "sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2410", "sql-rdd-2478", "sql-rdd-2546", "sql-rdd-2614", "sql-rdd-2682", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2886", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M6", FromID: "driver.air.mechanical_ventilation.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "Perimeter_bot_ZN_2", ServiceKind: "cooling", FromValue: 0.001, ToValue: 0.001, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"derived-driver-mechanical_ventilation_fallback_sensible-perimeter_bot_zn_2", "sql-rdd-1778", "sql-rdd-1882", "sql-rdd-1883", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M6", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 8813.045, ToValue: 8813.045, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1467", "sql-rdd-1469", "sql-rdd-1471", "sql-rdd-1473", "sql-rdd-1487", "sql-rdd-1489", "sql-rdd-1491", "sql-rdd-1493", "sql-rdd-1511", "sql-rdd-1523", "sql-rdd-1541", "sql-rdd-1561", "sql-rdd-1575", "sql-rdd-1587", "sql-rdd-1605", "sql-rdd-1625", "sql-rdd-1639", "sql-rdd-1651", "sql-rdd-1669", "sql-rdd-1689", "sql-rdd-1699", "sql-rdd-1701", "sql-rdd-1703", "sql-rdd-1705", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M6", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 0.157, ToValue: 1501.071, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2954", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M7", FromID: "driver.air.mechanical_ventilation.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 0.026, ToValue: 0.026, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"derived-driver-mechanical_ventilation_fallback_sensible-midfloor_plenum", "derived-driver-mechanical_ventilation_fallback_sensible-perimeter_mid_zn_2", "derived-driver-mechanical_ventilation_fallback_sensible-perimeter_mid_zn_3", "sql-rdd-1762", "sql-rdd-1810", "sql-rdd-1818", "sql-rdd-1874", "sql-rdd-1875", "sql-rdd-1898", "sql-rdd-1899", "sql-rdd-1902", "sql-rdd-1903", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M7", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 10912.995, ToValue: 10912.995, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1467", "sql-rdd-1469", "sql-rdd-1471", "sql-rdd-1473", "sql-rdd-1487", "sql-rdd-1489", "sql-rdd-1491", "sql-rdd-1493", "sql-rdd-1511", "sql-rdd-1523", "sql-rdd-1541", "sql-rdd-1561", "sql-rdd-1575", "sql-rdd-1587", "sql-rdd-1605", "sql-rdd-1625", "sql-rdd-1639", "sql-rdd-1651", "sql-rdd-1669", "sql-rdd-1689", "sql-rdd-1699", "sql-rdd-1701", "sql-rdd-1703", "sql-rdd-1705", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M7", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "zone_load_allocation", RuleID: "allocation.by_zone_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 190.895, ToValue: 375.594, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2274", "sql-rdd-2342", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M8", FromID: "driver.air.mechanical_ventilation.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 0.01, ToValue: 0.01, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"derived-driver-mechanical_ventilation_fallback_sensible-perimeter_mid_zn_2", "derived-driver-mechanical_ventilation_fallback_sensible-perimeter_top_zn_3", "sql-rdd-1810", "sql-rdd-1850", "sql-rdd-1898", "sql-rdd-1899", "sql-rdd-1918", "sql-rdd-1919", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M8", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 9214.747, ToValue: 9214.747, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1467", "sql-rdd-1469", "sql-rdd-1471", "sql-rdd-1473", "sql-rdd-1487", "sql-rdd-1489", "sql-rdd-1491", "sql-rdd-1493", "sql-rdd-1511", "sql-rdd-1523", "sql-rdd-1541", "sql-rdd-1561", "sql-rdd-1575", "sql-rdd-1587", "sql-rdd-1605", "sql-rdd-1625", "sql-rdd-1639", "sql-rdd-1651", "sql-rdd-1669", "sql-rdd-1689", "sql-rdd-1699", "sql-rdd-1701", "sql-rdd-1703", "sql-rdd-1705", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M8", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "zone_load_allocation", RuleID: "allocation.by_zone_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 199.126, ToValue: 1393.57, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2274", "sql-rdd-2342", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M9", FromID: "driver.air.mechanical_ventilation.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "Perimeter_top_ZN_3", ServiceKind: "cooling", FromValue: 0.001, ToValue: 0.001, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"derived-driver-mechanical_ventilation_fallback_sensible-perimeter_top_zn_3", "sql-rdd-1850", "sql-rdd-1918", "sql-rdd-1919", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M9", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 6143.342, ToValue: 6143.342, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1467", "sql-rdd-1469", "sql-rdd-1471", "sql-rdd-1473", "sql-rdd-1487", "sql-rdd-1489", "sql-rdd-1491", "sql-rdd-1493", "sql-rdd-1511", "sql-rdd-1523", "sql-rdd-1541", "sql-rdd-1561", "sql-rdd-1575", "sql-rdd-1587", "sql-rdd-1605", "sql-rdd-1625", "sql-rdd-1639", "sql-rdd-1651", "sql-rdd-1669", "sql-rdd-1689", "sql-rdd-1699", "sql-rdd-1701", "sql-rdd-1703", "sql-rdd-1705", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M9", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 44.103, ToValue: 9006.804, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M10", FromID: "driver.air.mechanical_ventilation.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "Perimeter_bot_ZN_3", ServiceKind: "cooling", FromValue: 0.001, ToValue: 0.001, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"derived-driver-mechanical_ventilation_fallback_sensible-perimeter_bot_zn_3", "sql-rdd-1786", "sql-rdd-1886", "sql-rdd-1887", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M10", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "", ServiceKind: "cooling", FromValue: 1473.651, ToValue: 1473.651, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1511", "sql-rdd-1523", "sql-rdd-1541", "sql-rdd-1561", "sql-rdd-1575", "sql-rdd-1587", "sql-rdd-1625", "sql-rdd-1639", "sql-rdd-1651", "sql-rdd-1689", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M10", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 2570.25, ToValue: 45243.632, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2206", "sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2410", "sql-rdd-2478", "sql-rdd-2546", "sql-rdd-2614", "sql-rdd-2682", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2886", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M11", FromID: "driver.surface.exterior_walls.cooling.building", ToID: "load.cooling.building", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ZoneName: "Perimeter_bot_ZN_1", ServiceKind: "cooling", FromValue: 13.319, ToValue: 13.319, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1511", "sql-rdd-2015", "sql-rdd-2107", "sql-rdd-2175", "sql-rdd-2243", "sql-rdd-2311", "sql-rdd-2379", "sql-rdd-2447", "sql-rdd-2515", "sql-rdd-2583", "sql-rdd-2651", "sql-rdd-2719", "sql-rdd-2787", "sql-rdd-2855", "sql-rdd-2923", "sql-rdd-2991", "sql-rdd-3059", "sql-rdd-3127", "sql-rdd-3195", "sql-rdd-3263"}},
	{Period: "M11", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 28296.3, ToValue: 155676.737, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-2070", "sql-rdd-2138", "sql-rdd-2206", "sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2410", "sql-rdd-2478", "sql-rdd-2546", "sql-rdd-2614", "sql-rdd-2682", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2886", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
	{Period: "M12", FromID: "load.heating.building", ToID: "end_use.heating.building", Relation: "load_to_end_use", Basis: "service_path_allocation", RuleID: "allocation.by_service_path_load_share", ZoneName: "", ServiceKind: "heating", FromValue: 77222.545, ToValue: 360800.592, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"sql-rdd-1930", "sql-rdd-2070", "sql-rdd-2138", "sql-rdd-2206", "sql-rdd-2274", "sql-rdd-2342", "sql-rdd-2410", "sql-rdd-2478", "sql-rdd-2546", "sql-rdd-2614", "sql-rdd-2682", "sql-rdd-2750", "sql-rdd-2818", "sql-rdd-2886", "sql-rdd-2954", "sql-rdd-3022", "sql-rdd-3090", "sql-rdd-3158", "sql-rdd-3226", "sql-rdd-4078"}},
}

func TestEnergyPathRealAnnualLinkIdentityPreservesProvenanceBranches(t *testing.T) {
	periods := epathRealLinkIdentityPeriods()
	original, _ := json.Marshal(periods)
	nodes, links, _, _ := aggregateEnergyPathV2MonthlyPeriods(periods)
	epathAssertRealAnnualBranches(t, links)
	after, _ := json.Marshal(periods)
	if string(after) != string(original) {
		t.Fatal("annual identity qualification mutated original Monthly links")
	}

	reversed := epathRealLinkIdentityPeriods()
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	for i := range reversed {
		for a, b := 0, len(reversed[i].Links)-1; a < b; a, b = a+1, b-1 {
			reversed[i].Links[a], reversed[i].Links[b] = reversed[i].Links[b], reversed[i].Links[a]
		}
	}
	_, reverseLinks, _, _ := aggregateEnergyPathV2MonthlyPeriods(reversed)
	if !reflect.DeepEqual(epathRealLinkIdentitySet(links), epathRealLinkIdentitySet(reverseLinks)) {
		t.Fatal("annual semantic IDs, paired values or provenance depend on Monthly/input order")
	}
	once, _ := json.Marshal(links)
	qualifyEnergyPathLinkCollisions(links)
	twice, _ := json.Marshal(links)
	if string(once) != string(twice) {
		t.Fatal("completed-graph identity qualification is not idempotent")
	}

	bundle := PurposeResultBundle{EnergyExplanation: EnergyExplanationResult{
		Schema: energyExplanationSchema, Scope: EnergyExplanationScope{Kind: "building"},
		Nodes: nodes, Links: links,
	}}
	projection, err := ProjectEnergyPath(bundle, EnergyPathSelection{})
	if err != nil {
		t.Fatalf("real-shaped canonical parallel paths remain unselectable: %v", err)
	}
	encoded, err := json.Marshal(projection.PurposeResults)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PurposeResultBundle
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	roundtrip, err := ProjectEnergyPath(decoded, EnergyPathSelection{})
	if err != nil {
		t.Fatalf("JSON roundtrip lost valid parallel identities: %v", err)
	}
	if !reflect.DeepEqual(epathRealLinkIdentitySet(links), epathRealLinkIdentitySet(roundtrip.View.Links)) {
		t.Fatal("JSON roundtrip changed branch identity, paired values or source provenance")
	}
	// External ambiguous canonical input still fails instead of silently picking
	// one branch. The fix is in builder identity, not a weakened public validator.
	broken := projection.PurposeResults
	broken.EnergyExplanation.Links = append(append([]EnergyPathLink(nil), links...), links[0])
	if _, err := ProjectEnergyPath(broken, EnergyPathSelection{}); err == nil {
		t.Fatal("strict projection accepted a duplicated canonical branch")
	}
}

func TestEnergyPathRealLinkQualificationUsesExactSemanticNames(t *testing.T) {
	for _, variant := range []string{"zone punctuation", "endpoint punctuation", "escaped delimiters", "same semantic duplicate"} {
		t.Run(variant, func(t *testing.T) {
			first := EnergyPathLink{FromID: "driver.test", ToID: "load.cooling", Relation: "driver_to_load", Basis: "heat_balance_share", RuleID: "heat.driver_balance", ServiceKind: "cooling", ZoneName: "Office A", FromValue: 1, ToValue: 1, SourceIDs: []string{"source-one"}}
			second := first
			second.FromValue, second.ToValue = 2, 2
			second.SourceIDs = []string{"source-two"}
			switch variant {
			case "zone punctuation":
				second.ZoneName = "Office_A"
			case "endpoint punctuation":
				first.FromID = "driver.A-B"
				second.FromID = "driver.A_B"
			case "escaped delimiters":
				first.ZoneName = "Office;basis=x"
				second.ZoneName = "Office"
				second.Basis = "x;basis=heat_balance_share"
			}
			first.ID, second.ID = energyPathLinkID(first), energyPathLinkID(second)
			links := []EnergyPathLink{first, second}
			qualifyEnergyPathLinkCollisions(links)
			if variant == "same semantic duplicate" {
				if links[0].ID != links[1].ID || links[0].ID != first.ID {
					t.Fatal("ambiguous same-identity input received arbitrary synthetic IDs")
				}
				return
			}
			if links[0].ID == links[1].ID || links[0].ID == first.ID || links[1].ID == second.ID {
				t.Fatalf("every colliding branch needs a distinct semantic ID: %q / %q", links[0].ID, links[1].ID)
			}
			reversed := []EnergyPathLink{second, first}
			qualifyEnergyPathLinkCollisions(reversed)
			if links[0].ID != reversed[1].ID || links[1].ID != reversed[0].ID {
				t.Fatal("semantic qualifier depends on array order")
			}
			for i, want := range []EnergyPathLink{first, second} {
				got := links[i]
				got.ID = want.ID
				if !reflect.DeepEqual(got, want) {
					t.Fatal("ID qualification changed a paired value or source")
				}
			}
		})
	}
}

func epathRealLinkIdentityPeriods() []EnergyPeriod {
	out := []EnergyPeriod{}
	for month := 1; month <= 12; month++ {
		id := "M" + strconv.Itoa(month)
		period := EnergyPeriod{ID: id, Kind: "monthly"}
		nodeIndex := map[string]int{}
		addNode := func(id string, value float64, sourceIDs []string) {
			if index, ok := nodeIndex[id]; ok {
				period.Nodes[index].Value += value
				period.Nodes[index].SourceIDs = appendUniqueStrings(period.Nodes[index].SourceIDs, sourceIDs...)
				return
			}
			level := strings.Split(id, ".")[0]
			domain := "thermal"
			service := "cooling"
			carrier := ""
			endUse := ""
			if strings.Contains(id, "heating") {
				service = "heating"
			}
			if level == "end_use" || level == "carrier" {
				domain = "site"
				carrier = "natural_gas"
				endUse = "heating"
				service = "heating"
			}
			if level == "carrier" {
				service, endUse = "", ""
			}
			nodeIndex[id] = len(period.Nodes)
			period.Nodes = append(period.Nodes, EnergyExplanationNode{ID: id, Level: level, Kind: id, Value: value, Unit: "kWh", ScaleDomain: domain, ServiceKind: service, Carrier: carrier, EndUse: endUse, Period: period.ID, SourceIDs: append([]string(nil), sourceIDs...)})
			if level == "end_use" {
				period.Nodes[len(period.Nodes)-1].endUseCarriers = []string{carrier}
			}
		}
		for _, original := range epathRealAnnualLinkIdentityRows {
			if original.Period != id {
				continue
			}
			link := original
			link.ID = energyPathLinkID(link)
			link.SourceIDs = append([]string(nil), link.SourceIDs...)
			period.Links = append(period.Links, link)
			addNode(link.FromID, link.FromValue, link.SourceIDs)
			addNode(link.ToID, link.ToValue, link.SourceIDs)
		}
		// One independent, noncolliding path proves existing stable IDs are kept.
		if month == 1 {
			link := EnergyPathLink{FromID: "end_use.heating.building", ToID: "carrier.natural_gas.building", Relation: "end_use_to_carrier", Basis: "reported_meter", ServiceKind: "heating", Period: id, FromValue: 1, ToValue: 1, FromUnit: "kWh", ToUnit: "kWh", SourceIDs: []string{"unique-meter-source"}}
			link.ID = energyPathLinkID(link)
			period.Links = append(period.Links, link)
			addNode(link.FromID, link.FromValue, link.SourceIDs)
			addNode(link.ToID, link.ToValue, link.SourceIDs)
		}
		out = append(out, period)
	}
	return out
}

func epathAssertRealAnnualBranches(t *testing.T, links []EnergyPathLink) {
	t.Helper()
	expected := map[string]EnergyPathLink{}
	for _, original := range epathRealAnnualLinkIdentityRows {
		key := energyPathV2LinkAggregationKey(original)
		row, ok := expected[key]
		if !ok {
			row = original
			row.FromValue = 0
			row.ToValue = 0
			row.SourceIDs = nil
		}
		row.FromValue = roundedEnergyNumber(row.FromValue + original.FromValue)
		row.ToValue = roundedEnergyNumber(row.ToValue + original.ToValue)
		row.SourceIDs = appendUniqueStrings(row.SourceIDs, original.SourceIDs...)
		expected[key] = row
	}
	if len(expected) != 9 || len(links) != 10 {
		t.Fatalf("real branch count expected9+unique1, got %d/%d", len(expected), len(links))
	}
	seenIDs := map[string]bool{}
	baseCounts := map[string]int{}
	for _, link := range links {
		if link.ID == "" || seenIDs[link.ID] {
			t.Fatalf("annual link ID remains ambiguous: %q", link.ID)
		}
		seenIDs[link.ID] = true
		base := energyPathLinkID(link)
		baseCounts[base]++
		want, ok := expected[energyPathV2LinkAggregationKey(link)]
		if !ok {
			if link.ID != base || link.Relation != "end_use_to_carrier" {
				t.Fatalf("unrelated unique link ID changed: %+v", link)
			}
			continue
		}
		if link.ID == base || !strings.Contains(link.ID, ".branch;rule=") {
			t.Fatalf("colliding branch has no explicit semantic qualifier: %q", link.ID)
		}
		if link.FromValue != want.FromValue || link.ToValue != want.ToValue || link.Period != "annual" {
			t.Fatalf("paired annual amount changed: %+v / %+v", link, want)
		}
		left, right := append([]string(nil), link.SourceIDs...), append([]string(nil), want.SourceIDs...)
		sort.Strings(left)
		sort.Strings(right)
		if !reflect.DeepEqual(left, right) {
			t.Fatalf("annual branch lost source IDs: %s", link.ID)
		}
	}
	counts := []int{}
	for _, count := range baseCounts {
		counts = append(counts, count)
	}
	sort.Ints(counts)
	if !reflect.DeepEqual(counts, []int{1, 2, 2, 5}) {
		t.Fatalf("distinct provenance branches were merged/dropped: %v", counts)
	}
}

func epathRealLinkIdentitySet(links []EnergyPathLink) map[string]EnergyPathLink {
	out := map[string]EnergyPathLink{}
	for _, link := range links {
		link.SourceIDs = append([]string(nil), link.SourceIDs...)
		sort.Strings(link.SourceIDs)
		link.RelatedPathIDs = append([]string(nil), link.RelatedPathIDs...)
		sort.Strings(link.RelatedPathIDs)
		out[link.ID] = link
	}
	return out
}
