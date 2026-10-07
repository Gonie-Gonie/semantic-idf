# Tools and settings

Tools provides separate workspaces for Batch Metrics, Batch Simulation and Diagnose. Settings controls appearance, interaction, Profile interpretation and engine execution defaults. Select a tool in the left selector; do not confuse a tool's selected files with the model open in the main editor.

## Batch Metrics workflow {#batch-metrics}

1. Select Batch Metrics and choose several input files.
2. Review filenames and labels so identically named files are distinguishable.
3. Start analysis and wait for the completed progress count.
4. Filter the combined table to the metric families of interest.
5. Switch orientation between metrics as rows and files as rows.
6. Select two file headers to set baseline A and comparison B.
7. Export the currently selected comparison/table representation.

This performs static model analysis, not EnergyPlus simulation. Each file is parsed independently. Worker concurrency is bounded by the file count, detected CPU count and an upper limit of eight. Failed inputs remain visible rather than acquiring another file's values.

Batch Metrics uses the effective area basis. Optional full Topology adds additional detail; it increases work and result size, so enable it when that detail is needed.

## Delta interpretation {#deltas}

For numeric values with compatible units:

```text
absolute delta = B - A
relative delta (%) = 100 × (B - A) / A, provided A ≠ 0
```

If A is 100 m² and B is 125 m², the delta is +25 m² and +25%. Reversing the baseline gives -25 m² and -20%; percentage change depends on the denominator.

If A is zero, the absolute difference may be valid but the percentage is unavailable. A dash is not 0%. A change in a text value is marked changed/unchanged rather than given a fabricated numeric percentage.

Compare metric status and interpretation along with the delta. In particular, U-value coverage must match: differing coverage or one-sided coverage metadata is marked not comparable. A lower U-value over a smaller known portion of the envelope is not a proven whole-envelope improvement.

Displayed differences are calculated from the comparison's displayed numeric values. Use detailed exports and their precision when tiny changes matter; do not infer additional precision from the percentage column.

## Batch Simulation {#batch-simulation}

Batch Simulation executes separate model runs. Choose inputs, weather and the required purposes, then review the worker setting before starting. Each row has its own status, executed input and output directory.

A row's completed engine status, output availability and Energy Path detail status describe different stages. Some successful runs do not contain the observations required for a particular chart. Only suitable rows can be selected for the corresponding comparison.

Use the row's Energy Path action to inspect that run's retained details. Batch comparison and row details use Building/Annual/all-services; single-run Energy Path additionally supports selectable scope and period. Do not merge unlike weather, input versions, reporting periods or floor-area bases into a single meaningful performance comparison without documenting those differences.

CSV is suited to table analysis. XLSX and HTML Energy Path reports can include selected summaries and trace detail. JSON preserves the structured result/export representation. See [Energy Path](./energy-path.en.md) for scope, source and export semantics.

## Worker and storage settings {#workers}

The default simulation worker count is calculated from detected logical CPUs:

```text
workers = max(1, ceil(CPU count × workerFraction))
if maxWorkers > 0: workers = min(workers, maxWorkers)
```

The default fraction is 0.5. Settings normalization limits the fraction to 0.1–1.0; nonpositive fractions restore the default. A maxWorkers value of zero means no additional configured cap. The batch's explicit worker choice and available work also determine actual execution.

For 12 detected CPUs and fraction 0.5, the initial count is six; maxWorkers=3 reduces it to three. This describes concurrency, not a guaranteed sixfold speedup. Several EnergyPlus processes can contend for memory, disk and CPU.

Use an output location with enough space for SQL and other requested observations. More requested frequencies, surfaces and system nodes can produce large results even when the visible chart is small. Keep a run's executed model, plan, ERR and SQL together when retaining reproducible evidence.

## Diagnose workflow {#diagnose}

Diagnose can use the current app input snapshot or a separately selected input. The diagnostic issues and cleanup candidate list are related views, but a diagnostic warning is not automatically permission to remove an object.

1. Select the input and inspect reported issues.
2. Choose cleanup rules on the left.
3. Review the candidate checkboxes and their object identities on the right.
4. Filter candidates to inspect a family without losing the distinction between rule selection and individual exclusions.
5. Preview the proposed cleaned text and removal count.
6. Use Apply selected to update the working document and refresh analysis, or Save cleaned copy for a new file.
7. Compare important references and model quantities after cleanup.

Save cleaned copy preserves the original input file. Apply updates the working document; saving that document later is a separate operation.

## Cleanup rules and limits {#cleanup-rules}

| Rule ID | Behavior | Review point |
| --- | --- | --- |
| remove_unused_schedules | Remove Schedule objects not referenced by other fields | Confirm the model's intended references and external workflow |
| remove_unused_envelope_resources | Remove unreferenced materials, window materials and constructions | Optional review rule; inactive resources may be intentionally retained |
| remove_unused_curves_tables | Remove unreferenced performance curves/tables | Optional review rule; check custom or future system variants |
| remove_duplicate_output_variables | Keep the first Output or OutputControl:Table:Style object with an identical field signature | Same key, variable and frequency semantics must be preserved |
| compact_formatting | Rewrite spacing with the app formatter | Formatting is different from object removal |
| remove_comments_only | Reserved future rule | Unavailable; do not expect a comment-only cleanup pass |

Available unused-schedule and duplicate-output rules can be selected by default. Other resource removals are review choices. Unused means the supported reference scan found no use; it is not a physical irrelevance proof for every extension or external process.

Semantic duplicate-name repair in CLI Clean is a separate opt-in operation. It is not equivalent to removing duplicate Output requests or merging two physical equipment objects.

## Appearance and interaction {#appearance}

Appearance settings include system/light/dark theme, app language, default Input View, graph font size, analysis-tab priority, and Topology colors for background, Zones, walls, roofs, openings and selected objects. Colors change presentation, not material properties or boundary classification.

Default Input View applies when starting a new workspace. A restored workspace and an input view you have already selected take precedence. Changing this preference does not switch the current editor. Profile also has a default time view: representative day/week, monthly average, annual heatmap or load-duration curve.

Keyboard settings replace the default accelerators. If a shortcut seems inactive, check focus and context before changing it: single-key Topology actions apply within that feature, and normal editing should retain ordinary typing. See [Getting started](./getting-started.en.md#keyboard).

The input/result splitter persists layout separately from analysis values. Changing theme, page or splitter does not alter a saved model or regenerate a simulation.

## Profile settings {#profile-settings}

Profile settings select enabled dimensions, display/grouping metrics, numeric tolerance, schedule comparison mode, time view and scale mode. These affect which patterns are compared and how they are shown; a matching tolerance is not numerical equality of complete annual simulations.

Profile Apply settings control clone/edit behavior, whether ZoneList editing or creation is allowed, the copy-name suffix and replacement policy. Review the preview and targeted source objects before applying changes: an original resource may serve several Zones.

Read [Metrics and Profile](./metrics.en.md) for load weights, schedule resolution, unsupported cases and the distinction between a display normalization and the underlying input.

## Generated files and disk usage {#generated-storage}

Open **Settings → Storage** to inspect simulation-run file sizes, refresh the inventory and clean eligible runs. The cards distinguish the total measured size, the amount available to reclaim and protected files. Run counts and storage paths help identify which location is growing. Sizes are logical file lengths, so they can differ from Windows' allocated size on disk. Warnings indicate unreadable paths or an incomplete scan; unmeasured files are not treated as empty.

By default, generated runs live under `%LOCALAPPDATA%/SemanticIDF/simulations` on Windows. Configured run directories are recorded in a bounded management index, so files remain visible after the output location changes. Each bundle contains the executed input copy and EnergyPlus results, including SQL, ERR and run metadata. Cleaning removes the whole eligible bundle; retain important evidence outside the managed run directories before cleaning.

1. Refresh the inventory after long simulations or external file changes.
2. Choose all eligible completed runs, or runs completed more than 7, 30 or 90 days ago.
3. Review the cleanup confirmation, then confirm the action.
4. Inspect the removed run count, reclaimed size and any failures. The inventory refreshes after cleanup.

Automatic cleanup is optional. Enable **Always clean** and save Settings to remove unused completed runs at startup and after results are replaced. Current results remain available while the app is open and are cleaned when the app closes if no simulation or result operation is still running. Disable this option to keep runs until you clean them manually. Cleanup is deferred while another app instance or filesystem-reading CLI command is active. Running simulations, results currently retained by the app, another live app instance's runs, explicit output directories, unknown directories, linked paths and bundles with added user files are protected. Newly generated runs carry ownership metadata. Older runs are eligible only when their metadata verifies ownership under the default app directory; incomplete or unverifiable runs stay protected. A cleanup failure leaves remaining files visible for review rather than reporting them as removed.

Browser/runtime data is measured separately. It includes browser caches, settings and workspace restoration data; this cleanup does not delete it while the app is using it. Settings files, user models, saved exports and engine/weather installations are retained. A run copy explicitly opened, saved or selected as an input for Simulation, Batch Metrics or a CLI command is retained as user data, including after the app closes.

## Persistent settings and recovery {#settings-storage}

The backend saves normalized settings in the local application configuration area, replacing the complete configuration after a successful write. Browser storage also caches UI state. Saving settings does not package engines or weather into the model. The save bar indicates whether the current form has changes; disk cleanup is an independent action and does not save or discard form edits.

If engine discovery fails, inspect installation and additional weather paths in Simulation settings. If a preference behaves unexpectedly, compare the displayed normalized value with the supplied value; invalid or outdated fields can be repaired during loading.

Do not use settings reset as a substitute for diagnosing missing observations. [Reference](./reference.en.md) gives a symptom-based checklist.
