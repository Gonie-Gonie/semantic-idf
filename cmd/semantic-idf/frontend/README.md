# Frontend Layout

`cmd/semantic-idf/frontend/src` is the canonical static app source served by Wails. Keep pages, styles, and JavaScript modules here until a bundler becomes worthwhile.

- `src/app.js`: tiny browser entrypoint.
- `src/js`: feature modules for state, actions, views, navigation, settings, and analysis.
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
