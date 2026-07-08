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
- Batch Simulation can export purpose metrics, compact Basic Energy
  explanation summary/source metadata rows, and Sankey edge metadata rows as CSV.

## Changed

- _None._

## Fixed

- _None._
