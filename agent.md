# Agent Notes

- Keep the committed project lightweight: repo-local runtime downloads live in ignored `.runtime/`.
- Use `scripts/setup.ps1` to prepare `.runtime/go`, `.runtime/bin/wails.exe`, and local Go caches per clone.
- Prefer the top-level `dev.bat` wrapper for Windows developer commands; it applies the PowerShell bypass flags.
- Prefer static frontend assets until a build chain becomes clearly valuable.
- Every implementation pass should end with `scripts/verify.ps1`, then commit and push when the work is complete.
- Every commit should use the repo-local runtime and include a successful Wails build; setup installs a local pre-commit hook for this.
- Keep `frontend/dist/guide.html` cumulative. Update it whenever app usage, limitations, or developer commands change.
- Protect user work in the git tree. Do not revert unrelated changes.
- Favor small IDF-domain functions that can be tested without launching the desktop shell.
- Keep EnergyPlus input parsing/conversion in `internal/epinput`; reserve `internal/idf` for low-level IDF parsing and analysis helpers.
- Support EnergyPlus 22+ as the default compatibility range and keep version-specific IDD/schema integration pluggable.
- Input viewing should keep Text, JSON, and Table modes in sync from one parsed/cached EnergyPlus model; Table mode should be organized by IDF object type and support row/column orientation changes.
