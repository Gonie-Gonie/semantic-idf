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
- Added an annual `energyExplanationSummary` result payload for carrier,
  end-use, delivered-load, heat-driver, residual, and top-zone rollups.
- Basic Energy explanation plans and result payloads now expose the
  `direct_only` allocation policy so exports and UI panels distinguish measured
  or derived links from future allocated views.
- Basic Energy purpose runs now request monthly delivered-load and heat-driver
  explanation outputs while reusing hourly Zone Heat Flow outputs when that
  purpose is selected.
- Batch purpose simulation metrics now include compact annual Energy Use,
  Delivered Load, Heat Driver, residual, mapped-percent, and top heat-driver
  values from the Basic Energy explanation summary, plus two-row delta tables
  for end-use energy, delivered loads, and heat drivers. The batch explanation
  comparison also ranks the largest annual changes and labels missing baseline
  or comparison rows.

## Changed

- _None._

## Fixed

- _None._
