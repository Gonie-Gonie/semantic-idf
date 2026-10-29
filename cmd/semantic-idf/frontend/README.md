# Frontend Layout

`cmd/semantic-idf/frontend/src` is the canonical static app source served by Wails. Keep pages, styles, and JavaScript modules here until a bundler becomes worthwhile.

- `src/app.js`: tiny browser entrypoint.
- `src/js`: feature modules for state, actions, views, navigation, settings, and analysis.
- `src/js/hvac-operation.js` resolves exact calendar/day-type schedule traces
  for the selected equipment occurrence. `views/hvac-operation-view.js` renders
  Availability timelines. `hvac-control-expressions.js` projects the authored
  control records into compact cards; `views/hvac-control-expression-view.js`
  renders nested On / Off expressions and grouped parameter-control sources
  with explicit I/O and control/state roles.
  These consume `HVACReport.operatingProfiles` and `operatingSchedules` from
  `internal/idf/hvac_operation*.go`, independently of simulation results and the
  Profile tab's schedule approximations. Availability uses
  `configuredAvailabilityLayers` / `configuredAvailability` for minimum static
  conditions, retaining supported schedule/manager rules and plant activation
  membership while excluding runtime decisions. Configuration errors stay
  unknown; runtime unknowns remain in On / Off and the legacy expression API.
  Exact object IDs, typed containment and circuit ports scope each profile;
  raw setpoint/performance schedules do not become binary availability gates.
  `hvac-availability-timeline.js` builds bounded, compressed permission timelines;
  `views/hvac-availability-view.js` owns representative day/week and annual
  binary patches with source owners, related input rows and a final configured
  row, with shared horizontal zoom/pan and a time cursor/status. Its only visible
  control is the time-view selector; calendar APIs remain compatible for programmatic users.
  On / Off consumes the authored `onOffExpression` AST, with AND/OR only for
  explicit Boolean operators and named rules for manager priority/overrides.
  Parameter cards retain exact graph memberships, signed references and native
  Action fields; their compact grouping does not invent control equations.
- `src/js/simulation-comparison-data.js` builds metadata catalogs for related
  Energy Path, Heat Flow and Comfort variables plus shared environment outputs.
  `views/simulation-comparison-view.js` places their line/scatter/histogram controls at the
  bottom of each result. Materialize selected traces through the existing result
  resource owner, preserving source identity, reporting intervals, gaps and zero.
  The canonical `series` API remains available without a standalone Series tab.
  `hvac-inspection-histogram.js` counts complete selected observations for these
  comparisons and HVAC Custom graphs, with common bins per display unit and
  independent unit axes stacked beneath one overlaid plot.
  `time-series-chart.js` owns delegated horizontal wheel zoom and double-click
  reset for temporal plots. Renderers redraw the selected source window and
  retain full-period Y limits; current-frame updates retain the viewport. Mounted
  SVGs weakly own their render callbacks, while abandoned string registrations
  have a bounded pending cache. Result cards share the Simulation chart font,
  width and grid tokens rather than individual window breakpoints.
- `src/js/simulation-progress.js`: measured work, approximate remaining-time
  presentation, HTTP progress polling and response-byte progress. Unknown totals
  remain indeterminate; legacy stage-count percentages are not displayed as time.
- `src/js/simulation-result-parts.js`: named-run resource ownership, bounded
  catalog hydration, deferred observations and view dependency declarations.
  `result-resource-scheduler.js` deduplicates requests and promotes foreground
  work over speculative reads; two reads share an 8 MiB admission window.
  `simulation-result-worker.js` parses bounded JSON resources off the UI thread
  and transfers numeric buffers without changing binary64 values. The canonical
  result is never sent through structured clone: its lazy point getters would
  otherwise expand every series. Compact v1 and complete legacy results remain
  supported. Background admission is limited to 384 MiB per result; this is a
  transfer-byte budget, not a bound on the user's fully opened result or exports.
  Active view data comes first, then nearby scope/period/source data, other tab
  defaults and remaining resources. Other tab defaults share a priority and
  enter the queue round-robin in tab order; selection changes promote existing
  requests. Immutable columns are validated once as their bounded chunks arrive,
  and every canonical binding shares that validation. Add future views through dependency rules,
  not separate fetch loops. Deferred reads suspend rendering without treating
  unloaded values as missing; retries check run and selection identity. Replacing
  a result releases its queue, requests and worker. Exports explicitly load all
  parts before capturing the existing complete-result snapshot.
- `src/js/auxiliary-panel.js`: the in-app Settings/Guide/Tools panel. Main stays
  mounted with its analysis and simulation state. Up to three frames are loaded
  lazily and retained, including drafts, manual routes and running batch tools.
  `auxiliary-context.js` shares Main's Wails callbacks and progress event bus;
  `auxiliary-navigation.js` closes/switches the panel and retains standalone links.
  Tools reads the live Main input and applies changes through a guarded host
  handoff. Opening or closing a panel never restores or reanalyzes Main.
- `src/js/settings.js` and `src/styles/settings.css`: Settings form, dirty state,
  section navigation and generated-run storage controls. `settings-client.js`
  owns defaults, normalization, persistence and cross-window synchronization.
- `src/js/i18n.js`: locale normalization, per-key English fallback, DOM translation
  and language-change events. `src/js/locales/` holds the registry and separate
  English, Korean, Japanese, Hindi, Spanish and French catalogs. English owns
  every interface key; Korean is complete, and the other catalogs are overlays.
- `src/vendor`: vendored browser libraries used directly by static modules.
- `src/samples`: bundled sample inputs used by the app and tests.
- `src/manual`: the single source directory for the English/Korean technical reference manual, ordered by its manifest; see [authoring and catalog refresh](src/manual/README.md).
- `wailsjs`: generated Wails bindings, ignored by git.
- `dist`: reserved for future generated build output, ignored by git.

Use `localizedMessage` for status messages that must follow a later language
change, and `setLocalizedText` for dynamic DOM labels. Raw backend errors,
model names, field IDs and units retain their source text. Keep familiar industry
terms such as HVAC, Energy Path and COP in English; translate general actions,
explanations and warnings. Do not copy the English catalog into partial locales.
Locale redraws use `view-presentation.js` and the `preservePresentation` render
option to retain disclosures, focus, scroll and the existing Topology camera.
Ordinary model changes keep the renderer's normal reset behavior.

The manual follows Settings: Korean uses Korean content; the other five app
languages use English. Its localized metric descriptions are maintained in the
manual directory, with original-field snapshots and per-field English fallback.
Use `dev test -Area i18n` to check catalog coverage, placeholders, runtime locale
changes, settings synchronization and Guide behavior.
