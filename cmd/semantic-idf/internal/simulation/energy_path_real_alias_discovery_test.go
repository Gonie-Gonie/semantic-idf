package simulation

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Closed, narrow metadata/value snapshot from the actual official 25.1
// large-office-25-1 run real-large-office-25-1-20260907T161337.433232500.
// The engine reported 158 Monthly surface Gain Energy keys and 16 People
// Energy keys. Its RDD advertises generic Zone names, not specific Zone keys.
// ERR rejects TOPFLOOR_PLENUM People and surface Heat Transfer Energy.
// This is a discovery regression, not a locked numerical acceptance manifest.
func TestEnergyPathRealAliasDiscoveryPreservesRequestedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "eplusout.sql")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"CREATE TABLE ReportDataDictionary (ReportDataDictionaryIndex INTEGER PRIMARY KEY, IsMeter INTEGER, Type TEXT, IndexGroup TEXT, TimestepType TEXT, KeyValue TEXT, Name TEXT, ReportingFrequency TEXT, ScheduleName TEXT, Units TEXT)",
		"CREATE TABLE ReportData (ReportDataIndex INTEGER PRIMARY KEY, TimeIndex INTEGER, ReportDataDictionaryIndex INTEGER, Value REAL)",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	const surfaceName = "Surface Inside Face Convection Heat Gain Energy"
	const peopleName = "Zone People Convective Heating Energy"
	if len(epathRealAliasSurfaceRows) != 158 || len(epathRealAliasPeopleRows) != 16 {
		t.Fatal("incomplete closed actual-key snapshot")
	}
	for _, group := range []struct {
		name, indexGroup string
		rows             []epathRealAliasDictionaryRow
	}{
		{surfaceName, "Surface", epathRealAliasSurfaceRows},
		{peopleName, "Zone", epathRealAliasPeopleRows},
	} {
		for _, row := range group.rows {
			if _, err := db.Exec("INSERT INTO ReportDataDictionary VALUES (?, 0, 'Sum', ?, 'Zone', ?, ?, 'Monthly', '', 'J')", row.id, group.indexGroup, row.key, group.name); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, point := range epathRealAliasReportedPoints {
		if _, err := db.Exec("INSERT INTO ReportData VALUES (?, ?, ?, ?)", point.id, point.time, point.dictionary, point.value); err != nil {
			t.Fatal(err)
		}
	}
	// Actual RDD lines: its first column is the reporting timestep class,
	// not a key value proving that TOPFLOOR_PLENUM has this People output.
	rdd := "Zone,Sum,Zone People Convective Heating Energy [J]\nZone,Sum,Surface Inside Face Convection Heat Gain Energy [J]\n"
	if err := os.WriteFile(filepath.Join(dir, "eplusout.rdd"), []byte(rdd), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "eplusout.mdd"), []byte("Zone,Meter,NaturalGas:Facility [J]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, catalogDirectory := epathRealDirectories(t)
	var original []byte
	for _, fixture := range epathLoadRealCatalog(t, catalogDirectory).Fixtures {
		if fixture.ID == "large-office-25-1" {
			original = epathRequireRealFile(t, epathRealCatalogPath(t, catalogDirectory, fixture.ModelPath))
		}
	}
	if len(original) == 0 {
		t.Fatal("official original model missing")
	}
	result, err := DiscoverAvailableOutputs(OutputDiscoveryRequest{
		Text: string(original), OutputDirectory: dir,
		PurposeRequest: &SimulationPurposeRequest{Purposes: []SimulationPurposeID{SimulationPurposeBasicEnergy}, BasicEnergyDetail: PurposeBasicEnergyDetailEnergyPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("absent-plenum-is-not-basement", func(t *testing.T) {
		for _, key := range []string{"GroundFloor_Plenum", "MidFloor_Plenum", "TopFloor_Plenum"} {
			resolution, ok := epathRealAliasResolution(result.PurposeOutputResolutions, "Output:Variable", key, peopleName)
			if !ok || resolution.Status != "fallback" || resolution.ResolvedName != "" || resolution.ResolutionBasis != "" {
				t.Fatalf("absent specific Zone acquired another Zone's output: %+v", resolution)
			}
			if !discoveryHas(result.Items, "Output:Variable", key, peopleName, "fallback") {
				t.Fatalf("absent key %s was marked available/alias", key)
			}
		}
		for _, row := range epathRealAliasPeopleRows {
			resolution, ok := epathRealAliasResolution(result.PurposeOutputResolutions, "Output:Variable", row.key, peopleName)
			if !ok || resolution.Status != "available" || resolution.ResolvedName != peopleName || resolution.ResolutionBasis != "exact" {
				t.Fatalf("real reported People key lost exact resolution: %+v", resolution)
			}
		}
	})
	t.Run("all-158-surface-aliases-stay-on-their-actual-key", func(t *testing.T) {
		for _, row := range epathRealAliasSurfaceRows {
			resolution, ok := epathRealAliasResolution(result.PurposeOutputResolutions, "Output:Variable", row.key, "Surface Inside Face Convection Heat Transfer Energy")
			if !ok || resolution.Status != "alias" || resolution.ResolvedName != surfaceName || resolution.ResolutionBasis != "version_alias" || resolution.Units != "J" || resolution.ReportingFrequency != "Monthly" || !strings.EqualFold(resolution.KeyValue, row.key) || resolution.Source != "sql" {
				t.Fatalf("actual surface alias lost exact-key/source evidence: key=%s resolution=%+v", row.key, resolution)
			}
			item, ok := discoveryFind(result.Items, "Output:Variable", row.key, surfaceName, "available")
			if !ok || item.KeyValue != row.key || item.Name != surfaceName || item.Units != "J" || !strings.Contains(item.Source, "sql") {
				t.Fatalf("reported source original name/key was relabeled: %+v", item)
			}
		}
		var originalNames, inventedNames, selectedPoints int
		if err := db.QueryRow("SELECT count(*) FROM ReportDataDictionary WHERE Name=?", surfaceName).Scan(&originalNames); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT count(*) FROM ReportDataDictionary WHERE Name='Surface Inside Face Convection Heat Transfer Energy'").Scan(&inventedNames); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SELECT count(*) FROM ReportData WHERE ReportDataDictionaryIndex=1403 AND Value<>0").Scan(&selectedPoints); err != nil {
			t.Fatal(err)
		}
		if originalNames != 158 || inventedNames != 0 || selectedPoints != 12 {
			t.Fatalf("closed reported snapshot mutated or empty: %d/%d/%d", originalNames, inventedNames, selectedPoints)
		}
	})
	t.Run("wildcards-case-and-meter-identity-remain-valid", func(t *testing.T) {
		collector := outputDiscoveryCollector{items: map[string]OutputDiscoveryItem{}}
		items, err := discoverOutputsFromSQL(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			collector.add(item)
		}
		for _, key := range []string{"*", ""} {
			item, ok := collector.find("Output:Variable", key, peopleName)
			if !ok || item.Name != peopleName {
				t.Fatalf("general request %q lost available output", key)
			}
		}
		item, ok := collector.find(" output:variable ", " basement ", " zone people convective heating energy ")
		if !ok || item.KeyValue != "BASEMENT" || item.Name != peopleName {
			t.Fatalf("case/whitespace exact-key normalization lost: %+v", item)
		}
		if _, ok := collector.find("Output:Variable", "TOPFLOOR_PLENUM", peopleName); ok {
			t.Fatal("SQL-only explicit key fell back to another Zone")
		}
		collector.add(OutputDiscoveryItem{ObjectType: "Output:Variable", KeyValue: "*", Name: peopleName, Units: "J", Source: "rdd", Status: "available"})
		if _, ok := collector.find("Output:Variable", "TOPFLOOR_PLENUM", peopleName); ok {
			t.Fatal("general dictionary entry proved an absent specific Zone")
		}
		meter, ok := epathRealAliasResolution(result.PurposeOutputResolutions, "Output:Meter", "NaturalGas:Facility", "NaturalGas:Facility")
		if !ok || meter.Status != "available" || meter.ResolvedName != "NaturalGas:Facility" {
			t.Fatalf("meter's name-only identity was mistaken for a Zone key: %+v", meter)
		}
	})
}

func epathRealAliasResolution(items []PurposeOutputResolution, objectType, key, name string) (PurposeOutputResolution, bool) {
	for _, item := range items {
		if strings.EqualFold(item.ObjectType, objectType) && strings.EqualFold(item.KeyValue, key) && strings.EqualFold(item.CanonicalName, name) {
			return item, true
		}
	}
	return PurposeOutputResolution{}, false
}

type epathRealAliasDictionaryRow struct {
	id  int
	key string
}
type epathRealAliasPoint struct {
	id, time, dictionary int
	value                float64
}

var epathRealAliasSurfaceRows = []epathRealAliasDictionaryRow{
	{1403, "BASEMENT_WALL_EAST"},
	{1405, "BASEMENT_WALL_NORTH"},
	{1407, "BASEMENT_WALL_SOUTH"},
	{1409, "BASEMENT_WALL_WEST"},
	{1411, "BASEMENT_FLOOR"},
	{1413, "BASEMENT_CEILING_1"},
	{1415, "BASEMENT_CEILING_2"},
	{1417, "BASEMENT_CEILING_3"},
	{1419, "BASEMENT_CEILING_4"},
	{1421, "BASEMENT_CEILING_5"},
	{1423, "BASEMENT INTERNAL MASS"},
	{1425, "CORE_BOT_ZN_5_WALL_EAST"},
	{1427, "CORE_BOT_ZN_5_WALL_NORTH"},
	{1429, "CORE_BOT_ZN_5_WALL_SOUTH"},
	{1431, "CORE_BOT_ZN_5_WALL_WEST"},
	{1433, "CORE_BOT_ZN_5_FLOOR"},
	{1435, "CORE_BOT_ZN_5_CEILING"},
	{1437, "CORE_BOTTOM INTERNAL MASS"},
	{1439, "CORE_MID_ZN_5_WALL_EAST"},
	{1441, "CORE_MID_ZN_5_WALL_NORTH"},
	{1443, "CORE_MID_ZN_5_WALL_SOUTH"},
	{1445, "CORE_MID_ZN_5_WALL_WEST"},
	{1447, "CORE_MID_ZN_5_FLOOR"},
	{1449, "CORE_MID_ZN_5_CEILING"},
	{1451, "CORE_MID INTERNAL MASS"},
	{1453, "CORE_TOP_ZN_5_WALL_EAST"},
	{1455, "CORE_TOP_ZN_5_WALL_NORTH"},
	{1457, "CORE_TOP_ZN_5_WALL_SOUTH"},
	{1459, "CORE_TOP_ZN_5_WALL_WEST"},
	{1461, "CORE_TOP_ZN_5_FLOOR"},
	{1463, "CORE_TOP_ZN_5_CEILING"},
	{1465, "CORE_TOP INTERNAL MASS"},
	{1467, "GROUNDFLOOR_PLENUM_WALL_EAST"},
	{1469, "GROUNDFLOOR_PLENUM_WALL_NORTH"},
	{1471, "GROUNDFLOOR_PLENUM_WALL_SOUTH"},
	{1473, "GROUNDFLOOR_PLENUM_WEST"},
	{1475, "GROUNDFLOOR_PLENUM_FLOOR_1"},
	{1477, "GROUNDFLOOR_PLENUM_FLOOR_2"},
	{1479, "GROUNDFLOOR_PLENUM_FLOOR_3"},
	{1481, "GROUNDFLOOR_PLENUM_FLOOR_4"},
	{1483, "GROUNDFLOOR_PLENUM_FLOOR_5"},
	{1485, "GROUNDFLOOR_PLENUM_CEILING"},
	{1487, "MIDFLOOR_PLENUM_WALL_EAST"},
	{1489, "MIDFLOOR_PLENUM_WALL_NORTH"},
	{1491, "MIDFLOOR_PLENUM_WALL_SOUTH"},
	{1493, "MIDFLOOR_PLENUM_WALL_WEST"},
	{1495, "MIDFLOOR_PLENUM_FLOOR_1"},
	{1497, "MIDFLOOR_PLENUM_FLOOR_2"},
	{1499, "MIDFLOOR_PLENUM_FLOOR_3"},
	{1501, "MIDFLOOR_PLENUM_FLOOR_4"},
	{1503, "MIDFLOOR_PLENUM_FLOOR_5"},
	{1505, "MIDFLOOR_PLENUM_CEILING"},
	{1507, "PERIMETER_BOT_ZN_1_WALL_EAST"},
	{1509, "PERIMETER_BOT_ZN_1_WALL_NORTH"},
	{1511, "PERIMETER_BOT_ZN_1_WALL_SOUTH"},
	{1513, "PERIMETER_BOT_ZN_1_WALL_WEST"},
	{1515, "PERIMETER_BOT_ZN_1_FLOOR"},
	{1517, "PERIMETER_BOT_ZN_1_CEILING"},
	{1519, "PERIMETER_BOT_ZN_1 INTERNAL MASS"},
	{1521, "PERIMETER_BOT_ZN_1_WALL_SOUTH_WINDOW"},
	{1523, "PERIMETER_BOT_ZN_2_WALL_EAST"},
	{1525, "PERIMETER_BOT_ZN_2_WALL_NORTH"},
	{1527, "PERIMETER_BOT_ZN_2_WALL_SOUTH"},
	{1529, "PERIMETER_BOT_ZN_2_WALL_WEST"},
	{1531, "PERIMETER_BOT_ZN_2_FLOOR"},
	{1533, "PERIMETER_BOT_ZN_2_CEILING"},
	{1535, "PERIMETER_BOT_ZN_2 INTERNAL MASS"},
	{1537, "PERIMETER_BOT_ZN_2_WALL_EAST_WINDOW"},
	{1539, "PERIMETER_BOT_ZN_3_WALL_EAST"},
	{1541, "PERIMETER_BOT_ZN_3_WALL_NORTH"},
	{1543, "PERIMETER_BOT_ZN_3_WALL_SOUTH"},
	{1545, "PERIMETER_BOT_ZN_3_WALL_WEST"},
	{1547, "PERIMETER_BOT_ZN_3_FLOOR"},
	{1549, "PERIMETER_BOT_ZN_3_CEILING"},
	{1551, "PERIMETER_BOT_ZN_3 INTERNAL MASS"},
	{1553, "PERIMETER_BOT_ZN_3_WALL_NORTH_WINDOW"},
	{1555, "PERIMETER_BOT_ZN_4_WALL_EAST"},
	{1557, "PERIMETER_BOT_ZN_4_WALL_NORTH"},
	{1559, "PERIMETER_BOT_ZN_4_WALL_SOUTH"},
	{1561, "PERIMETER_BOT_ZN_4_WALL_WEST"},
	{1563, "PERIMETER_BOT_ZN_4_FLOOR"},
	{1565, "PERIMETER_BOT_ZN_4_CEILING"},
	{1567, "PERIMETER_BOT_ZN_4 INTERNAL MASS"},
	{1569, "PERIMETER_BOT_ZN_4_WALL_WEST_WINDOW"},
	{1571, "PERIMETER_MID_ZN_1_WALL_EAST"},
	{1573, "PERIMETER_MID_ZN_1_WALL_NORTH"},
	{1575, "PERIMETER_MID_ZN_1_WALL_SOUTH"},
	{1577, "PERIMETER_MID_ZN_1_WALL_WEST"},
	{1579, "PERIMETER_MID_ZN_1_FLOOR"},
	{1581, "PERIMETER_MID_ZN_1_CEILING"},
	{1583, "PERIMETER_MID_ZN_1 INTERNAL MASS"},
	{1585, "PERIMETER_MID_ZN_1_WALL_SOUTH_WINDOW"},
	{1587, "PERIMETER_MID_ZN_2_WALL_EAST"},
	{1589, "PERIMETER_MID_ZN_2_WALL_NORTH"},
	{1591, "PERIMETER_MID_ZN_2_WALL_SOUTH"},
	{1593, "PERIMETER_MID_ZN_2_WALL_WEST"},
	{1595, "PERIMETER_MID_ZN_2_FLOOR"},
	{1597, "PERIMETER_MID_ZN_2_CEILING"},
	{1599, "PERIMETER_MID_ZN_2 INTERNAL MASS"},
	{1601, "PERIMETER_MID_ZN_2_WALL_EAST_WINDOW"},
	{1603, "PERIMETER_MID_ZN_3_WALL_EAST"},
	{1605, "PERIMETER_MID_ZN_3_WALL_NORTH"},
	{1607, "PERIMETER_MID_ZN_3_WALL_SOUTH"},
	{1609, "PERIMETER_MID_ZN_3_WALL_WEST"},
	{1611, "PERIMETER_MID_ZN_3_FLOOR"},
	{1613, "PERIMETER_MID_ZN_3_CEILING"},
	{1615, "PERIMETER_MID_ZN_3 INTERNAL MASS"},
	{1617, "PERIMETER_MID_ZN_3_WALL_NORTH_WINDOW"},
	{1619, "PERIMETER_MID_ZN_4_WALL_EAST"},
	{1621, "PERIMETER_MID_ZN_4_WALL_NORTH"},
	{1623, "PERIMETER_MID_ZN_4_WALL_SOUTH"},
	{1625, "PERIMETER_MID_ZN_4_WALL_WEST"},
	{1627, "PERIMETER_MID_ZN_4_FLOOR"},
	{1629, "PERIMETER_MID_ZN_4_CEILING"},
	{1631, "PERIMETER_MID_ZN_4 INTERNAL MASS"},
	{1633, "PERIMETER_MID_ZN_4_WALL_WEST_WINDOW"},
	{1635, "PERIMETER_TOP_ZN_1_WALL_EAST"},
	{1637, "PERIMETER_TOP_ZN_1_WALL_NORTH"},
	{1639, "PERIMETER_TOP_ZN_1_WALL_SOUTH"},
	{1641, "PERIMETER_TOP_ZN_1_WALL_WEST"},
	{1643, "PERIMETER_TOP_ZN_1_FLOOR"},
	{1645, "PERIMETER_TOP_ZN_1_CEILING"},
	{1647, "PERIMETER_TOP_ZN_1 INTERNAL MASS"},
	{1649, "PERIMETER_TOP_ZN_1_WALL_SOUTH_WINDOW"},
	{1651, "PERIMETER_TOP_ZN_2_WALL_EAST"},
	{1653, "PERIMETER_TOP_ZN_2_WALL_NORTH"},
	{1655, "PERIMETER_TOP_ZN_2_WALL_SOUTH"},
	{1657, "PERIMETER_TOP_ZN_2_WALL_WEST"},
	{1659, "PERIMETER_TOP_ZN_2_FLOOR"},
	{1661, "PERIMETER_TOP_ZN_2_CEILING"},
	{1663, "PERIMETER_TOP_ZN_2 INTERNAL MASS"},
	{1665, "PERIMETER_TOP_ZN_2_WALL_EAST_WINDOW"},
	{1667, "PERIMETER_TOP_ZN_3_WALL_EAST"},
	{1669, "PERIMETER_TOP_ZN_3_WALL_NORTH"},
	{1671, "PERIMETER_TOP_ZN_3_WALL_SOUTH"},
	{1673, "PERIMETER_TOP_ZN_3_WALL_WEST"},
	{1675, "PERIMETER_TOP_ZN_3_FLOOR"},
	{1677, "PERIMETER_TOP_ZN_3_CEILING"},
	{1679, "PERIMETER_TOP_ZN_3 INTERNAL MASS"},
	{1681, "PERIMETER_TOP_ZN_3_WALL_NORTH_WINDOW"},
	{1683, "PERIMETER_TOP_ZN_4_WALL_EAST"},
	{1685, "PERIMETER_TOP_ZN_4_WALL_NORTH"},
	{1687, "PERIMETER_TOP_ZN_4_WALL_SOUTH"},
	{1689, "PERIMETER_TOP_ZN_4_WALL_WEST"},
	{1691, "PERIMETER_TOP_ZN_4_FLOOR"},
	{1693, "PERIMETER_TOP_ZN_4_CEILING"},
	{1695, "PERIMETER_TOP_ZN_4 INTERNAL MASS"},
	{1697, "PERIMETER_TOP_ZN_4_WALL_WEST_WINDOW"},
	{1699, "TOPFLOOR_PLENUM_WALL_EAST"},
	{1701, "TOPFLOOR_PLENUM_WALL_NORTH"},
	{1703, "TOPFLOOR_PLENUM_WALL_SOUTH"},
	{1705, "TOPFLOOR_PLENUM_WALL_WEST"},
	{1707, "TOPFLOOR_PLENUM_FLOOR_1"},
	{1709, "TOPFLOOR_PLENUM_FLOOR_2"},
	{1711, "TOPFLOOR_PLENUM_FLOOR_3"},
	{1713, "TOPFLOOR_PLENUM_FLOOR_4"},
	{1715, "TOPFLOOR_PLENUM_FLOOR_5"},
	{1717, "BUILDING_ROOF"},
}
var epathRealAliasPeopleRows = []epathRealAliasDictionaryRow{
	{86, "BASEMENT"},
	{90, "CORE_BOTTOM"},
	{94, "CORE_MID"},
	{98, "CORE_TOP"},
	{102, "PERIMETER_BOT_ZN_1"},
	{106, "PERIMETER_BOT_ZN_2"},
	{110, "PERIMETER_BOT_ZN_3"},
	{114, "PERIMETER_BOT_ZN_4"},
	{118, "PERIMETER_MID_ZN_1"},
	{122, "PERIMETER_MID_ZN_2"},
	{126, "PERIMETER_MID_ZN_3"},
	{130, "PERIMETER_MID_ZN_4"},
	{134, "PERIMETER_TOP_ZN_1"},
	{138, "PERIMETER_TOP_ZN_2"},
	{142, "PERIMETER_TOP_ZN_3"},
	{146, "PERIMETER_TOP_ZN_4"},
}

// The selected BASEMENT People (#86) and BASEMENT_WALL_EAST surface (#1403)
// retain all twelve original nonzero Monthly ReportData rows, not invented zero
// placeholders. Other snapshot rows are metadata-only for key resolution.
var epathRealAliasReportedPoints = []epathRealAliasPoint{
	{85693, 845, 86, 4377656329.786456},
	{85886, 845, 1403, 375081214.046584},
	{151911, 1518, 86, 3965155327.5556946},
	{152104, 1518, 1403, 380647411.596058},
	{225113, 2263, 86, 4668103615.755254},
	{225306, 2263, 1403, 483537442.1413767},
	{295987, 2984, 86, 4115521807.528192},
	{296180, 2984, 1403, 500121118.10806304},
	{369189, 3729, 86, 4329168610.64351},
	{369382, 3729, 1403, 530911561.7490052},
	{440063, 4450, 86, 4062016258.091725},
	{440256, 4450, 1403, 375314701.44755876},
	{513265, 5195, 86, 3746992548.753155},
	{513458, 5195, 1403, 323111524.38105816},
	{586467, 5940, 86, 4197610679.4402466},
	{586660, 5940, 1403, 281845571.3404055},
	{657341, 6661, 86, 3805794750.6283736},
	{657534, 6661, 1403, 384984023.5559756},
	{730543, 7406, 86, 4163046262.131262},
	{730736, 7406, 1403, 475424784.9683618},
	{801417, 8127, 86, 4196489602.7682858},
	{801610, 8127, 1403, 435976914.70401603},
	{874619, 8872, 86, 4220559112.9560747},
	{874812, 8872, 1403, 387421646.3322993},
}
