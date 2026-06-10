# Simulation Runner

This document tracks the current runner contract and the purpose-driven
simulation flow. The runner is intentionally split into a run-copy workflow:
temporary output requests are applied to the text that EnergyPlus executes, not
to the source IDF, unless the user explicitly applies those outputs through the
Output workflow.

## Current Request

`SimulationRunRequest` contains:

- `runId`: caller-provided or generated run id.
- `text`: input text to run. When present, the runner writes this to the run
  output directory.
- `inputPath`: existing IDF/IMF/JSON/epJSON path used when `text` is empty.
- `filename`: display and run-copy filename.
- `energyPlusExecutablePath`: explicit EnergyPlus executable path.
- `weatherPath`: optional EPW file.
- `outputDirectory`: optional explicit output directory.
- `standardOutput`: legacy compatibility flag for the previous standard output
  preset.
- `standardOutputMode`: legacy merge/replace mode for the standard preset.
- `purposeRequest`: purpose-driven output and result request.
- `purposeRunPlan`: backend-built plan attached after preparation.
- `resultMode`: result strategy, currently `sql_first` or legacy CSV fallback.
- `useReadVarsESO`: controls EnergyPlus `-r`/ReadVarsESO CSV generation when
  `resultMode` is SQL-first.
- `silent`: suppresses UI-facing status messages.
- `auto`: marks auto-runs started by the app.

## Purpose Flow

The purpose flow is:

1. Parse current text or `inputPath`.
2. Normalize `SimulationPurposeRequest`.
3. Build a `PurposeRunPlan`.
4. Apply missing purpose outputs to the run copy only.
5. Run EnergyPlus.
6. Read SQL first, then CSV, then ESO fallbacks.
7. Build `PurposeResultBundle` while preserving legacy `Series` and
   `HeatFlow` fields.

`BuildSimulationRunPlan` exposes the planning step for UI preview. `RunSimulationText`
uses the purpose flow when `purposeRequest` is present. `RunPurposeSimulationText`
is a convenience wrapper that defaults to Basic Energy + Zone Heat Flow.

## Purpose Model

Supported purpose ids:

- `basic_energy`
- `zone_heat_flow`
- `hvac_loop_check`
- `integrity_check`
- `comfort_check`
- `custom_outputs`

`SimulationPurposeScope` can express selected zones, selected loops, selected
components, output signatures, and custom output objects. Phase 1 uses selected
zones for Zone Heat Flow and keeps HVAC loop checks on wildcard node keys until
loop node resolution is wired to the HVAC tab.

`PurposeRunPlan` reports:

- output objects with purpose tags, signatures, state, and estimated weight
- overall estimated series count, frame count, and weight
- SQL and discovery requirements
- warnings for wildcard scope and frequency conflicts

Output states:

- `existing`: already present in the source IDF.
- `temporary`: added to the run copy only.
- `will_be_persisted`: planned for permanent apply.
- `conflict`: same output target exists with a different frequency or field set.

## Result Reading

`readSimulationOutputs` is split into:

- `collectSimulationFiles`
- `parseERR`
- `parseSQLResults`
- `parseCSVResults`
- `parseHeatFlowFallback`

The result source priority is SQL, then CSV, then ESO. SQL parsing already
feeds legacy `Series` and `HeatFlow`, so older viewers continue to work while
purpose result viewers are added.

## Permanent Outputs

`ApplyPurposeOutputsText` converts a purpose plan into the existing Output apply
pipeline. This keeps permanent output edits behind the same preview/apply
contract as manual Output tab changes.

## Output Discovery

`DiscoverAvailableOutputs` builds a searchable output catalog from available run
artifacts. It reads SQL `ReportDataDictionary`, `.rdd`, and `.mdd` files when
present, then merges selected purpose-plan outputs as `fallback` entries. Each
catalog item reports its object type, key, variable or meter name, units, source,
status (`available` or `fallback`), and purpose tags when applicable.
