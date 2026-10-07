# Frontend Layout

`cmd/semantic-idf/frontend/src` is the canonical static app source served by Wails. Keep pages, styles, and JavaScript modules here until a bundler becomes worthwhile.

- `src/app.js`: tiny browser entrypoint.
- `src/js`: feature modules for state, actions, views, navigation, settings, and analysis.
- `src/vendor`: vendored browser libraries used directly by static modules.
- `src/samples`: bundled sample inputs used by the app and tests.
- `src/manual`: the single source directory for the English/Korean technical reference manual, ordered by its manifest; see [authoring and catalog refresh](src/manual/README.md).
- `wailsjs`: generated Wails bindings, ignored by git.
- `dist`: reserved for future generated build output, ignored by git.
