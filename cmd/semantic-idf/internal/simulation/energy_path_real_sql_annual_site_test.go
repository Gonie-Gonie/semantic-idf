package simulation

import "fmt"

// One original-source union supports both real dictionary leaves and exact
// annual cells; a Tabular source is never assigned a fabricated RDD index.
func epathSQLSiteOriginals(frames epathSQLFrames, site epathRealSQLSite, period string, precision epathRealSQLPrecision) (map[string]epathSQLOriginalSource, map[string]bool, error) {
	out, required := map[string]epathSQLOriginalSource{}, map[string]bool{}
	annual, err := epathSQLSiteIsAnnual(frames, site.ID)
	if err != nil {
		return nil, nil, err
	}
	q, err := epathSQLSitePeriod(frames, site.ID, period)
	if err != nil {
		return nil, nil, err
	}
	if annual {
		if site.Tabular == nil || len(frames.SiteSources[site.ID]) != 0 {
			return nil, nil, fmt.Errorf("annual site source contradicts its reviewed reporting class")
		}
		original, err := epathSQLOriginalTabular(site, frames.SiteAnnual[site.ID])
		if err != nil {
			return nil, nil, err
		}
		key, err := epathSQLOriginalKey(original)
		if err != nil {
			return nil, nil, err
		}
		out[key] = original
		if q != nil && !q.includesZero() {
			required[key] = true
		}
		return out, required, nil
	}
	if site.Tabular != nil || len(frames.SiteSources[site.ID]) == 0 {
		return nil, nil, fmt.Errorf("monthly site lacks its original dictionary leaves")
	}
	for _, id := range frames.SiteSources[site.ID] {
		source, ok := frames.SourceIdentities[id]
		if !ok || id <= 0 || source.DictionaryIndex != id || !source.IsMeter {
			return nil, nil, fmt.Errorf("unbound original site meter %d", id)
		}
		original := epathSQLOriginalRDD(source)
		key, err := epathSQLOriginalKey(original)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := out[key]; exists {
			return nil, nil, fmt.Errorf("duplicate original site source")
		}
		out[key] = original
		values, err := epathSQLMonthly(source, precision)
		if err != nil {
			return nil, nil, err
		}
		for _, month := range epathSQLPeriodMonths(period) {
			if !values[month-1].includesZero() {
				required[key] = true
			}
		}
	}
	return out, required, nil
}

// Original Tabular numeric values are annual normalized kWh. Their exact
// cell and reported source precision remain independent of candidate values.
func epathCheckSQLModelOriginalSource(bundle PurposeResultBundle, check epathSQLModelCheck) error {
	p := check.OriginalSource
	target := check.Item.Target
	if p == nil || p.Tabular == nil || p.RDD != nil || check.Item.Scope != "building" || check.Item.Zone != "" || check.Item.Period != "annual" || check.Item.Unit != "kWh" || target.Collection != "sources" || target.Field != "rawValue" && target.Field != "effectiveValue" || target.SourceName != p.Name || target.SourceKey != p.Key || target.SourceUnit != p.Tabular.Selector.Unit || target.Frequency != "Annual" || target.Unit != "kWh" {
		return fmt.Errorf("annual original source target contradicts its independently reviewed cell")
	}
	if check.Quantity == nil || !check.Quantity.valid() || !epathSQLZoneCarrierQuantityEqual(*check.Quantity, p.Tabular.Quantity) {
		return fmt.Errorf("annual source quantity lost its original cell precision")
	}
	key, err := epathSQLOriginalKey(*p)
	if err != nil {
		return err
	}
	sources := map[string]EnergyDataSource{}
	var ids []string
	for _, source := range bundle.EnergyExplanation.Sources {
		if _, exists := sources[source.ID]; exists {
			return fmt.Errorf("duplicate original source candidate identity")
		}
		sources[source.ID] = source
		if source.Name == p.Name && source.KeyValue == p.Key {
			ids = append(ids, source.ID)
		}
	}
	if len(ids) != 1 {
		return fmt.Errorf("annual source needs exactly one bound original observation")
	}
	if err := epathSQLVerifyOriginalSources(ids, sources, map[string]epathSQLOriginalSource{key: *p}, map[string]bool{key: true}, "annual"); err != nil {
		return err
	}
	actual, err := epathReadOracleCandidate(bundle, check.Item, check.Want)
	if err != nil {
		return err
	}
	return epathCheckSQLModelQuantity(actual, check.Quantity)
}
