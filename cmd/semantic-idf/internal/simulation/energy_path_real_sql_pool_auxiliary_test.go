package simulation

// Independent source-local CW consumer. This does not widen the ordinary
// whole-meter auxiliary policy or replace the observed Pumps meter by CW.
import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

type epathSQLPoolPumpConsumer struct {
	Native    epathSQLPoolSourceFrames
	Frames    epathSQLFrames // Only independently bound Zone/primary-load frames.
	Model     epathRealSQLModel
	Auxiliary epathRealSQLAuxiliary
}

// Nil is only a genuinely unrelated legacy auxiliary. A declared Pool pump
// cannot fall through to the older broad-meter allocation on a proof failure.
func epathSQLPoolPumpConsumerFor(frames epathSQLFrames, model epathRealSQLModel, auxiliary epathRealSQLAuxiliary) (*epathSQLPoolPumpConsumer, error) {
	if len(model.PoolSystems) == 0 {
		if len(frames.PoolSystems) != 0 {
			return nil, fmt.Errorf("undeclared native Pool frames")
		}
		return nil, nil
	}
	if len(model.PoolSystems) != 1 || len(frames.PoolSystems) != 1 || !reflect.DeepEqual(model.PoolSystems[0], frames.PoolSystems[0].Original.Declaration) {
		return nil, fmt.Errorf("Pool auxiliary needs one exact original declaration/native binding")
	}
	var site *epathRealSQLSite
	for i := range model.Site {
		if model.Site[i].ID == auxiliary.SiteID {
			if site != nil {
				return nil, fmt.Errorf("duplicate Pool auxiliary site identity")
			}
			site = &model.Site[i]
		}
	}
	if site == nil {
		return nil, fmt.Errorf("Pool auxiliary has no reviewed site")
	}
	if site.EndUse != "pumps" {
		return nil, nil
	}
	native := frames.PoolSystems[0]
	selector := epathRealSQLSelector{Alternatives: []epathRealSQLAlternative{{Name: "Pumps:Electricity", Unit: "J"}}, Keys: []string{""}, IsMeter: true}
	if site.Carrier != "electricity" || site.Facility || site.Tabular != nil || !reflect.DeepEqual(site.Source, selector) || auxiliary.Weight != "cooling" || auxiliary.AllocationMethod != "service_load_share" || auxiliary.WeightSource != nil {
		return nil, fmt.Errorf("Pool pump policy requires its exact Monthly broad meter and source-local cooling declaration")
	}
	if _, err := epathSQLAllocationID(auxiliary.ReconciliationID, "annual"); err != nil {
		return nil, err
	}
	served, err := epathSQLDeclaredZones(frames, auxiliary.ServedZones)
	if err != nil || len(served) != len(native.Original.ServedZones) {
		return nil, fmt.Errorf("Pool pump declared recipients differ from original: %v", err)
	}
	for _, name := range native.Original.ServedZones {
		if !served[strings.ToLower(name)] {
			return nil, fmt.Errorf("Pool pump declared recipient is not original")
		}
	}
	parent := native.Parents["Pumps:Electricity"]
	ids, values := frames.SiteSources[site.ID], frames.Site[site.ID]
	if len(ids) != 1 || ids[0] != parent.Source.DictionaryIndex || len(values) != 12 || !reflect.DeepEqual(frames.SourceIdentities[ids[0]], parent.Source) {
		return nil, fmt.Errorf("Pool pump broad site no longer binds its native parent identity")
	}
	for month, q := range values {
		if q == nil || len(parent.Monthly) != 12 || !epathSQLZoneCarrierQuantityEqual(*q, parent.Monthly[month]) {
			return nil, fmt.Errorf("Pool pump broad site quantity was replaced or multiplied")
		}
	}
	if _, err := epathSQLPoolPumpMathFromSources(native, frames, model); err != nil {
		return nil, err
	}
	// Avoid retaining unrelated driver cells in every scalar/flow proof.
	minimal := epathSQLFrames{Zones: frames.Zones, Loads: map[string]epathSQLQuantity{}, LoadSourceIDs: map[string][]int{}, SourceIdentities: map[int]epathRealSQLSource{}}
	for zone := range frames.Zones {
		for month := 1; month <= 12; month++ {
			key := epathSQLKey(zone, "cooling", month)
			minimal.Loads[key], minimal.LoadSourceIDs[key] = frames.Loads[key], frames.LoadSourceIDs[key]
			for _, id := range frames.LoadSourceIDs[key] {
				minimal.SourceIdentities[id] = frames.SourceIdentities[id]
			}
		}
	}
	boundModel := epathRealSQLModel{OriginalZoneMultiplierProof: model.OriginalZoneMultiplierProof, Precision: model.Precision, PoolSystems: model.PoolSystems}
	return &epathSQLPoolPumpConsumer{Native: native, Frames: minimal, Model: boundModel, Auxiliary: auxiliary}, nil
}

func epathSQLPoolPumpProjection(consumer *epathSQLPoolPumpConsumer, zone, period string) (*epathSQLAuxiliaryZoneProof, error) {
	if consumer == nil || !epathOracleValidPeriod(period) || len(consumer.Model.PoolSystems) != 1 || !reflect.DeepEqual(consumer.Model.PoolSystems[0], consumer.Native.Original.Declaration) {
		return nil, fmt.Errorf("missing bound original Pool pump projection")
	}
	aux := consumer.Auxiliary
	if aux.SiteID == "" || aux.Weight != "cooling" || aux.AllocationMethod != "service_load_share" || aux.WeightSource != nil {
		return nil, fmt.Errorf("Pool projection changed its source-local policy")
	}
	served, err := epathSQLDeclaredZones(consumer.Frames, aux.ServedZones)
	if err != nil || len(served) != len(consumer.Native.Original.ServedZones) {
		return nil, fmt.Errorf("Pool projection changed its original recipients")
	}
	for _, name := range consumer.Native.Original.ServedZones {
		if !served[strings.ToLower(name)] {
			return nil, fmt.Errorf("Pool projection borrowed an unserved recipient")
		}
	}
	key := strings.ToLower(zone)
	if consumer.Frames.Zones[key].Name != zone {
		return nil, fmt.Errorf("Pool projection has no exact original Zone identity")
	}
	calculation, err := epathSQLPoolPumpMathFromSources(consumer.Native, consumer.Frames, consumer.Model)
	if err != nil {
		return nil, err
	}
	family := consumer.Native.Families["pump.cw.electricity"]
	source := consumer.Native.Sources[family.CanonicalID].Source
	sourceID := fmt.Sprintf("sql-rdd-%d", source.DictionaryIndex)
	original := epathSQLOriginalRDD(source)
	parent := consumer.Native.Parents["Pumps:Electricity"].Source
	parentID := fmt.Sprintf("sql-rdd-%d", parent.DictionaryIndex)
	parentOriginal := epathSQLOriginalRDD(parent)
	// The existing site/carrier ribbon preserves the broad meter's provenance.
	// That reference is not the allocation budget: the node must additionally
	// prove the native CW constituent and this Zone's primary cooling weight.
	p := &epathSQLAuxiliaryZoneProof{SiteID: aux.SiteID, EndUse: "pumps", Carrier: "electricity", ZoneName: zone, Period: period, Basis: "service_path_allocation", Weight: aux.Weight, AllocationMethod: aux.AllocationMethod, Owned: served[key], Sources: map[string]epathSQLOriginalSource{parentID: parentOriginal}, NodeSources: map[string]epathSQLOriginalSource{parentID: parentOriginal, sourceID: original}, Required: map[string]bool{}, NodeRequired: map[string]bool{}}
	for _, month := range epathSQLPeriodMonths(period) {
		part := calculation.Months[month-1].Shares[key]
		p.Value = p.Value.add(part)
		_, high := part.bounds()
		if high <= 0 {
			continue
		}
		p.Required[parentID], p.NodeRequired[parentID], p.NodeRequired[sourceID] = true, true, true
		loadKey := epathSQLKey(zone, "cooling", month)
		for _, id := range consumer.Frames.LoadSourceIDs[loadKey] {
			load := consumer.Frames.SourceIdentities[id]
			loadID := fmt.Sprintf("sql-rdd-%d", id)
			p.NodeSources[loadID] = epathSQLOriginalRDD(load)
			p.NodeRequired[loadID] = true
		}
	}
	p.NativePool = consumer
	return p, nil
}

func epathSQLPoolPumpAuxiliaryProofs(consumer *epathSQLPoolPumpConsumer) (map[string]*epathSQLAuxiliaryZoneProof, error) {
	if consumer == nil {
		return nil, fmt.Errorf("nil Pool auxiliary proof")
	}
	out := map[string]*epathSQLAuxiliaryZoneProof{}
	zones := []string{}
	for _, zone := range consumer.Frames.Zones {
		zones = append(zones, zone.Name)
	}
	sort.Strings(zones)
	for _, zone := range zones {
		for _, period := range epathSQLZoneCarrierPeriods() {
			p, err := epathSQLPoolPumpProjection(consumer, zone, period)
			if err != nil {
				return nil, err
			}
			out[epathSQLAuxiliaryZoneKey(p.SiteID, zone, period)] = p
		}
	}
	return out, nil
}

func epathSQLValidatePoolPumpAuxiliaryProof(p *epathSQLAuxiliaryZoneProof) error {
	if p == nil || p.NativePool == nil {
		return fmt.Errorf("missing native CW auxiliary proof")
	}
	want, err := epathSQLPoolPumpProjection(p.NativePool, p.ZoneName, p.Period)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(p, want) {
		return fmt.Errorf("CW auxiliary scalar/source roster differs from independently completed Monthly shares")
	}
	return nil
}

type epathSQLPoolPumpLedgerProof struct {
	Consumer   *epathSQLPoolPumpConsumer
	Period, ID string
}

func epathSQLPoolPumpAllocationMethod(calculation epathSQLPoolPumpMath, period string) (string, error) {
	if !epathOracleValidPeriod(period) {
		return "", fmt.Errorf("invalid Pool pump allocation method period")
	}
	quanta := 0.0
	for _, month := range epathSQLPeriodMonths(period) {
		row := calculation.Months[month-1]
		if !row.Denominator.valid() || !row.ChilledWaterObserved.valid() {
			return "", fmt.Errorf("unproved Pool pump method quantities")
		}
		if row.Denominator.Value <= 0 {
			continue
		}
		// Quantize each native monthly budget, not the annual sum of positive
		// subquantum observations. This is independent of displayed candidate
		// allocations and of the wider per-recipient interval envelopes.
		units := math.Round(row.ChilledWaterObserved.Value * 1000)
		if !epathOracleFinite(units) || units < 0 || units > 1<<53 {
			return "", fmt.Errorf("Pool pump budget is outside exact integer apportionment")
		}
		quanta += units
		if !epathOracleFinite(quanta) || quanta > 1<<53 {
			return "", fmt.Errorf("Pool pump annual quantized budget overflow")
		}
	}
	if quanta == 0 {
		return "unassigned", nil
	}
	return "service_load_share", nil
}

func epathSQLPoolPumpLedgerFields(p *epathSQLPoolPumpLedgerProof) (map[string]epathSQLQuantity, string, error) {
	if p == nil || p.Consumer == nil || !epathOracleValidPeriod(p.Period) {
		return nil, "", fmt.Errorf("missing native Pool pump ledger")
	}
	id, err := epathSQLAllocationID(p.Consumer.Auxiliary.ReconciliationID, p.Period)
	if err != nil || id != p.ID {
		return nil, "", fmt.Errorf("native Pool pump ledger identity changed")
	}
	calculation, err := epathSQLPoolPumpMathFromSources(p.Consumer.Native, p.Consumer.Frames, p.Consumer.Model)
	if err != nil {
		return nil, "", err
	}
	fields := map[string]epathSQLQuantity{"directValue": {}, "overmappedValue": {}}
	for _, month := range epathSQLPeriodMonths(p.Period) {
		row := calculation.Months[month-1]
		fields["expectedValue"] = fields["expectedValue"].add(row.Expected)
		fields["allocatedValue"] = fields["allocatedValue"].add(row.Allocated)
		fields["unassignedValue"] = fields["unassignedValue"].add(row.Unassigned)
		fields["residualValue"] = fields["residualValue"].add(row.Expected.add(row.Allocated.times(-1)))
	}
	method, err := epathSQLPoolPumpAllocationMethod(calculation, p.Period)
	return fields, method, err
}

func epathSQLPoolPumpBuildingLedgerChecks(consumer *epathSQLPoolPumpConsumer, checks *epathSQLModelChecks) error {
	for _, period := range epathSQLZoneCarrierPeriods() {
		id, err := epathSQLAllocationID(consumer.Auxiliary.ReconciliationID, period)
		if err != nil {
			return err
		}
		p := &epathSQLPoolPumpLedgerProof{Consumer: consumer, Period: period, ID: id}
		fields, method, err := epathSQLPoolPumpLedgerFields(p)
		if err != nil {
			return err
		}
		start := len(checks.Rows)
		for _, field := range []string{"expectedValue", "directValue", "allocatedValue", "unassignedValue"} {
			q := fields[field]
			target := epathRealOracleTarget{Collection: "reconciliation", ID: id, Level: "allocation", Field: field, Unit: "kWh", AllocationMethod: method}
			if err := checks.add("zoneAllocation", "building", "", period, "pool_native_pump/"+consumer.Auxiliary.SiteID+"/"+field, "kWh", &q, target, "", nil, nil); err != nil {
				return err
			}
		}
		if err := checks.bindAllocationProof(start); err != nil {
			return err
		}
		checks.Rows[start].Allocation.NativePoolPump = p
	}
	return nil
}

func epathCheckSQLPoolPumpAllocation(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	if check.Allocation == nil || check.Allocation.NativePoolPump == nil {
		return fmt.Errorf("missing bound native Pool pump allocation")
	}
	p := check.Allocation.NativePoolPump
	fields, method, err := epathSQLPoolPumpLedgerFields(p)
	if err != nil {
		return err
	}
	if check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Period != p.Period || check.Item.Group != "zoneAllocation" || check.Item.Target.Collection != "reconciliation" || check.Item.Target.Level != "allocation" || check.Item.Target.ID != p.ID || check.Item.Target.Unit != "kWh" || check.Item.Target.AllocationMethod != method {
		return fmt.Errorf("native Pool pump ledger escaped exact selector")
	}
	for field, q := range check.Allocation.fields() {
		if q == nil || !epathSQLZoneCarrierQuantityEqual(*q, fields[field].positive()) {
			return fmt.Errorf("native Pool pump ledger quantity changed")
		}
	}
	q, exists := fields[check.Item.Target.Field]
	if !exists || check.Quantity == nil || !epathSQLZoneCarrierQuantityEqual(*check.Quantity, q.positive()) {
		return fmt.Errorf("native Pool selected allocation quantity changed")
	}
	_, _, rows, _, err := epathOracleGraph(bundle, "building", "", p.Period)
	if err != nil {
		return err
	}
	var row *EnergyReconciliation
	for i := range rows {
		if rows[i].ID == p.ID {
			if row != nil {
				return fmt.Errorf("duplicate native Pool pump ledger")
			}
			row = &rows[i]
		}
	}
	if row == nil {
		for _, q := range fields {
			if !q.includesZero() {
				return fmt.Errorf("missing non-prunable native Pool pump ledger")
			}
		}
		return nil
	}
	if row.Level != "allocation" || row.Period != p.Period || row.ZoneName != "" || row.Unit != "kWh" || row.AllocationMethod != method {
		return fmt.Errorf("native Pool pump ledger metadata changed")
	}
	for field, value := range map[string]float64{"expectedValue": row.ExpectedValue, "directValue": row.DirectValue, "allocatedValue": row.AllocatedValue, "unassignedValue": row.UnassignedValue, "overmappedValue": row.OvermappedValue, "residualValue": row.ResidualValue} {
		q := fields[field]
		if err := epathCheckSQLModelQuantity(&value, &q); err != nil {
			return fmt.Errorf("native Pool pump %s: %w", field, err)
		}
	}
	return nil
}
