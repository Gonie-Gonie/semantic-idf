# Getting started

SemanticIDF reads an EnergyPlus model, presents its editable input, derives static analysis, and inspects simulation results. This manual explains both the workflow and how to interpret the numbers. Static model analysis and EnergyPlus execution are separate operations.

## Read this manual {#manual}

Settings, Guide and Tools open in a panel inside the app. Main remains behind it with the current input, analysis results, simulation results, selection and view position. Close the panel with its × button or Escape to return to Main. Each panel keeps its working state while the app is open, including unsaved Settings changes and the Guide chapter. Opening help or settings does not repeat analysis or rerun EnergyPlus. Applying a Diagnose fix or selecting another input in Tools changes Main's input and updates its analysis.

The chapter list takes you from basic use to technical interpretation. The section list opens a specific topic in the current chapter. Search covers chapter text and the metric catalog; a result opens its chapter and section. You can bookmark a chapter/section URL and use browser Back/Forward.

The manual follows the application language selected in Settings: Korean uses the Korean reference, and all other languages use the English reference. Shared section identifiers keep the same topic when the app language changes. Established feature and industry names such as Metrics, Profile, Energy Path and HVAC remain in English where they make the reference clearer.

| Task | Start here |
| --- | --- |
| Open, edit, save or locate an input object | [Input and source navigation](./input.en.md) |
| Understand area, envelope, loads or schedule values | [Metrics and Profile](./metrics.en.md) |
| Understand the 3D/Plan/Network model | [Topology](./topology.en.md) |
| Trace air/water systems in the input | [HVAC model interpretation](./hvac.en.md) |
| Run EnergyPlus or inspect time-dependent outputs | [Simulation](./simulation.en.md) |
| Explain drivers, delivered loads and consumed energy | [Energy Path](./energy-path.en.md) |
| Compare files, clean input or configure the app | [Tools and settings](./tools.en.md) |
| Automate exports and read existing SQL | [CLI, API and exports](./automation.en.md) |
| Resolve confusing units, missing values or failed runs | [Reference](./reference.en.md) |

The manual and its offline metric catalog are bundled with the executable. Reading help does not need an internet connection, an EnergyPlus installation or a running simulation. A standalone browser preview can use the bundled catalog when the desktop API is unavailable.

## First session {#first-session}

1. Start the executable without a CLI command.
2. The reference Large Office model loads at startup. Its values belong to that sample, not your project.
3. Select Open and choose your own input.
4. Wait for the relevant analysis stage. Metrics and input views become available before later stages finish.
5. Inspect Metrics and any warnings, then check the geometry and system connections that matter for your task.
6. Save intentional edits. Configure an engine and weather only when you need to execute a simulation.

Supported picker extensions include .idf, .imf, .epjson, .json and .txt. A filename extension is not proof that the contents form a supported EnergyPlus model. The input's Version and parsing results determine interpretation.

Opening a model starts analysis automatically. Changing a structured field refreshes the shared document and analysis; there is no separate Analyze button in the normal workflow. A busy or unfinished result is not a calculated zero.

## Workspace map {#workspace}

The left pane contains synchronized Text, JSON and Table views of one editable document. The right pane contains Metrics, Topology, Profile, HVAC and Simulation. Resize the divider to suit the task; each pane scrolls independently.

Tools, Guide and Settings are separate auxiliary pages. Returning to the app preserves the workspace through the existing navigation mechanism. Entering an auxiliary page is not a request to rerun EnergyPlus.

| Result | Question answered | What it does not establish |
| --- | --- | --- |
| Metrics | What is explicitly specified or derivable from this input? | Measured building performance or completed engine execution |
| Topology | How are spatial objects and declared thermal boundaries connected? | A dynamic heat-flow result inferred from proximity |
| Profile | How do declared load and schedule patterns differ by source/Zone? | A weather-validated annual simulation |
| HVAC | Which objects, nodes and services form the model's systems? | Actual equipment consumption from nominal ratings |
| Simulation | What did this execution/reporting source observe? | That an edited input still matches a previous result |

Selection is shared across participating Metrics, Profile, Topology and input views. HVAC and Simulation maintain their own selection context. A selected Zone in an analysis panel is not permission to replace the scope of a restored simulation result.

## A reliable model review {#review-workflow}

Use this sequence when checking a new model:

1. Confirm the model identity and Version.
2. In Metrics, inspect inventory, area and statuses. Check unexpected zero or missing values before interpreting ratios.
3. In Topology, examine a representative exterior surface, opening and interzone pair.
4. In Profile, compare load intensities and schedules for representative Zones.
5. In HVAC, trace representative supply/return and Zone service paths.
6. Use Diagnose to review unresolved references and cleanup candidates.
7. Run the necessary simulation purposes with a verified weather file.
8. Inspect ERR status, source availability, time coverage and Energy Path quality before comparing annual values.

For example, an unexpectedly small floor-area-normalized energy value may arise from a changed denominator, a Zone multiplier, or missing consumption. It is not automatically evidence of improved efficiency. Compare absolute quantities, area basis and coverage together.

## Save, revert and result identity {#save-revert}

Save writes current editor content to the opened path, or asks for a destination if there is no original path. Revert restores the snapshot from the most recent Open operation; it is not a multi-step undo history.

Navigation Back/Forward restores view state rather than editing the document. Normal text editing undo/redo and view history are distinct. See [Input](./input.en.md) for object-level edits and [Simulation](./simulation.en.md) for retained run inputs.

Simulation output requests are normally added to an execution copy. A saved result belongs to its executed input, engine, weather and run directory. Changing the editor later does not change that result's underlying measurements. Exporting an old result does not silently include current editor edits.

## Keyboard and source exploration {#keyboard}

These are default shortcuts; Settings can customize them. Shortcuts are context-sensitive, and ordinary typing in an editor takes precedence where appropriate.

| Action | Default |
| --- | --- |
| Open / Save | Ctrl+O / Ctrl+S |
| View history Back / Forward | Alt+Left / Alt+Right |
| Command palette | Ctrl+K |
| Focus the other pane | F6 |
| Reveal selected source | Ctrl+Shift+S |
| Definition / references | F12 / Shift+F12 |
| Current search | / |
| Primary destination / available views | Enter / Alt+Enter |
| Clear selection | Escape |
| Text / JSON / Table | Ctrl+2 / Ctrl+3 / Ctrl+4 |
| Metrics / Profile / HVAC / Simulation / Topology | Ctrl+Alt+1 / 2 / 3 / 4 / 6 |
| Topology 3D / Plan / Network | 1 / 2 / 3 within Topology |
| Topology Fit / Connectivity / Area / UA / QA | F / T / A / U / Q within Topology |

The internal Semantic projection supports identity and navigation; it is not a visible input tab. A configured shortcut for that internal projection does not expose a hidden editor.

## Choose the right evidence {#evidence}

Three questions require different evidence:

- **Model specification:** an input object, its references and supported calculation rules.
- **Engine behavior:** the actual executed input, reporting source, calendar and ERR completion.
- **Allocation explanation:** observed consumption, demonstrated ownership and the selected accounting policy.

A green result, plausible diagram or successful export answers only the corresponding check. Unknown ownership, unavailable output and observed zero remain distinct throughout the manual.

Continue with [Input and source navigation](./input.en.md) for editing, or [Simulation](./simulation.en.md) to run and interpret results.
