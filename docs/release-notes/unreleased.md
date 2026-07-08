# Unreleased Release Notes

<!--
Add release-note entries under the section that best describes the change.
The release script infers bump size from these sections:
- Breaking Changes: major
- Added or Features: minor
- Fixed, Changed, Performance, Security, Documentation, or internal-only notes: patch
-->

## Breaking Changes

- _None._

## Added

- Added a Basic Energy `energyExplanation` result payload and Energy tab
  subviews for Sankey-style accounting, monthly/zoned explanation ledgers, SQL
  source metadata, HVAC service-path cross-jumps, and residual reconciliation,
  including heat-driver links from delivered loads when heat-balance outputs are
  available.
- Energy explanation Sankey and Systems views can now focus by all results, a
  selected zone, or a selected HVAC service path.
- Energy explanation Sankey can now switch heat drivers between display
  magnitude, signed balance, cooling-pressure, and heating-pressure views.
- Energy explanation Sankey can cap visible heat-driver nodes and group omitted
  drivers as `Other heat drivers` for large models.
- Energy explanation completeness now surfaces missing categories and source
  availability rows in the UI.
- Energy explanation source availability now uses `found`/`missing` status rows
  and populates missing categories from missing source requests.
- Simulation source output cells now link back to matching existing output
  request objects when available.
- Energy explanation payloads now include relationship rule catalogs, and edges
  expose `ruleId` values shown in the Sankey inspector.
- Added an annual `energyExplanationSummary` result payload for carrier,
  end-use, delivered-load, heat-driver, residual, and top-zone rollups.
- Basic Energy explanation plans and result payloads now support
  `direct_only` and `by_zone_load_share` allocation policies. The zone-load
  share mode emits `basis=allocated` Energy Use -> Delivered Load edges, and
  Simulation/Batch Simulation controls can select the policy.
- Basic Energy purpose runs now request monthly delivered-load and heat-driver
  explanation outputs while reusing hourly Zone Heat Flow outputs when that
  purpose is selected.
- Basic Energy end-use meter coverage now includes heat recovery, exterior
  lighting, refrigeration, and natural-gas interior equipment where applicable.
- Basic Energy and standard output plans now cover additional Level 1 energy
  meters for fuel oil, propane, other fuels, steam, district cooling/heating
  end uses, and onsite electricity production when those model features are
  detected.
- Basic Energy explanation summaries now keep energy end-use carrier IDs
  distinct, and onsite electricity production no longer counts as mapped
  facility consumption in residual and mapped-percent calculations.
- Basic Energy Sankey payloads now link onsite electricity production to the
  carrier total with an `onsite_production` support relation instead of treating
  it as a facility consumption end-use.
- Basic Energy explanation parsing now prefers reported energy variables over
  rate fallbacks for the same delivered-load or heat-driver target, avoiding
  duplicate Sankey accounting when both are present; completeness now counts
  those fallback aliases by canonical target instead of raw output-name count.
- Basic Energy heat-driver parsing now preserves explicit sensible heat
  gain/loss signs, keeping positive and negative air-exchange drivers as
  separate Sankey nodes when EnergyPlus reports both as positive energy values.
- Energy explanation summaries now keep explicitly signed heat-driver gain and
  loss nodes separate, so batch summaries and exports do not merge opposite
  air-exchange directions.
- Energy explanation periods now carry their own reconciliation and warning
  rows, and the Reconciliation view can switch between annual and monthly
  accounting gaps.
- Batch Simulation purpose-result CSV exports now include annual/monthly energy
  explanation reconciliation rows with residual, basis, formula, period, and
  source IDs.
- Batch Simulation energy explanation summary, edge, and reconciliation CSV
  rows now fill `source_ids` and `source_object_index` when their source IDs can
  be matched to output request objects.
- Basic Energy source metadata now includes `aggregationMethod` for SQL/report
  sources, and the Sources view, inspector tables, and batch CSV export show it.
- Basic Energy source metadata now preserves both source units and normalized
  graph units, and the Sources view, inspector tables, and batch CSV export show
  both values.
- Basic Energy energy nodes now carry `meterHierarchyLevel` metadata such as
  `facility_total` and `broad_end_use`, and the Sankey inspector shows it.
- The Basic Energy Reconciliation subview now expands each accounting row's
  source IDs into compact source/output links, so residual checks can jump back
  to the matching output request when one is known.
- The Basic Energy Sankey inspector now shows the selected edge `relation`
  alongside basis, rule, formula, and source metadata.
- Heat-driver reconciliation now includes zone/service rows when zone delivered
  load and zone heat-driver sources are available, including monthly period
  rows.
- The Basic Energy Reconciliation subview now ranks the largest zone/service
  heat residuals for the selected annual or monthly period.
- Energy reconciliation source IDs now include both the expected facility total
  source and the mapped consumption end-use sources used in the residual
  formula.
- Basic Energy explanation plans now request the delivered-load alias catalog
  across zone air system, ideal loads, radiant HVAC, coil, and plant demand
  outputs at monthly frequency.
- Basic Energy delivered-load nodes now expose `pathType` metadata (`zone`,
  `system`, or `plant`) alongside `serviceKind`, and batch CSV exports include
  the path type.
- Basic Energy now reports cooling/heating COP as derived KPIs when matching
  electric end-use energy and delivered load are available, without adding
  synthetic COP conversion edges to the Sankey graph. Batch Simulation exposes
  those KPIs as selectable purpose metrics.
- Basic Energy explanation plans now request detailed monthly heat-driver
  outputs for people, lights, equipment, infiltration, ventilation, and mixing
  in addition to heat-balance and fan heat drivers.
- Delivered-load explanation nodes now scope zone loads to `zoneName`, plant
  demand loads to `loopName`, and keep coil/system loads as aggregate system
  nodes so heat-driver reconciliation does not double-count system and plant
  layers when zone loads are available.
- Basic Energy explanation nodes and edges now include related HVAC service path
  IDs when the source model can be analyzed, so Sankey inspectors and Systems
  views can jump to the matching zone service paths directly.
- Basic Energy SQL source metadata now carries matching output request object
  indexes when the run plan references existing outputs, including object index
  `0`, and batch source CSV exports preserve those indexes.
- Basic Energy SQL explanations now preserve custom purpose period scopes and
  emit a `selected_range` period with row-level values, edges, and
  reconciliation for the selected dates.
- Basic Energy SQL explanations now emit daily `D<n>` periods for Daily,
  Hourly, Timestep, or Detailed sources while leaving Monthly/RunPeriod sources
  annual/monthly only.
- Batch Simulation energy explanation CSV exports now keep annual, monthly, and
  selected-range edge/reconciliation rows by default, leaving daily periods in
  the embedded JSON payload to avoid oversized high-resolution CSV exports.
- Batch Simulation can now export the full purpose run result as
  `semantic-idf.batch-simulation/v1` JSON, preserving embedded Basic Energy
  explanation payloads that are too detailed for the default CSV.
- Basic Energy explanations now fall back to annual SQL tabular end-use rows
  when detailed `ReportData` energy rows are unavailable, while preserving
  `sql_tabular` source metadata.
- Energy explanation source tables and Sankey inspectors now prefer exact
  source `objectIndex` links before falling back to output-name matching.
- Basic Energy heat-driver extraction now recognizes object-level fan heat-to-air
  outputs separately from fan electricity use.
- Basic Energy output plan rows now label monthly delivered-load/zone-energy
  requests as `Basic Energy Explain` and monthly heat-balance/fan heat requests
  as `Basic Energy Heat Drivers`.
- Batch purpose simulation metrics now include compact annual Energy Use,
  Delivered Load, Heat Driver, residual, mapped-percent, and top heat-driver
  values from the Basic Energy explanation summary, plus two-row delta tables
  for end-use energy, delivered loads, and heat drivers. The batch explanation
  comparison also ranks the largest annual summary and Sankey edge changes, and
  labels missing baseline or comparison rows.
- Batch Simulation now flags Basic Energy explanation completeness differences
  between two selected cases, including mapped-percent and missing-category
  changes.
- Batch Simulation can export purpose metrics, compact Basic Energy
  explanation summary/source metadata rows, and Sankey edge metadata rows as CSV.
  Edge export rows include related HVAC service path IDs when available.

## Changed

- _None._

## Fixed

- _None._
