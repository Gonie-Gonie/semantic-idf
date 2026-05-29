# Codex Working Notes

- Primary stack: Go plus Wails v2 with static HTML/CSS/JS.
- Runtime policy: install Go and Wails into ignored `.runtime/` via `scripts/setup.ps1`; do not rely on global Go/Wails for project verification.
- Developer command entrypoint: use `dev.bat` from the repository root for setup, test, build, run, verify, and guide access on Windows.
- Core behavior should live under `internal/idf` so parser, analyzer, and editor logic stays easy to test.
- UI should stay dense and work-focused: editor, object tables, schedules, zones, and HVAC connection views first.
- Avoid adding npm unless the frontend genuinely needs bundling or a component framework.
- Before committing, run `scripts/verify.ps1` so tests and `wails build` pass with the repo-local runtime.
- User guide policy: update `frontend/dist/guide.html` cumulatively, including its update log, when tool usage changes.
- Treat unused-object deletion conservatively and keep parser round trips covered by tests.
- EnergyPlus input policy: IDF and epJSON share `internal/epinput` for detection, version metadata, common object structure, and conversion; IDD/schema validation should be added behind that module rather than scattered through UI code.
- Input view policy: Text leads with formatted object summaries and keeps raw editing below; JSON leads with the structured model tree and keeps editable epJSON below; Table uses object-type-specific spreadsheet tables with global and per-table orientation controls.
- Readability policy: JSON tree labels should use EnergyPlus object type/name and field labels instead of generic Object/Array labels; keep indentation compact. Text and Table groups should default open, and table row headers should stay visually consistent.
