# Technical reference manual sources

This single directory owns the shipped Guide content. manifest.json defines
chapter order, titles and English/Korean source filenames. Guide HTML contains
the application shell; guide-manual.js reads and renders these local sources.
The build embeds this directory through the existing frontend asset filesystem.

## Authoring

- Update the owning chapter rather than creating progress notes or duplicated
  HTML/i18n prose. Keep workflows, algorithms, units, examples and limits current.
- Maintain complete .en.md/.ko.md counterparts in manifest order.
- Use explicit section IDs, e.g. `## Source identity {#source-identity}`.
  Keep their order and IDs equal between languages so switching retains context.
- Use relative manual links such as `./simulation.en.md#run-copy`.
  Do not link to repository Go/developer docs unavailable in the shipped app.
- Supported Markdown: H1–H4, paragraphs, lists, quotes, pipe tables, fenced code,
  inline code, emphasis and links. Raw HTML is escaped; executable/unsafe links
  are disabled. Use plain Unicode mathematical notation or fenced formulas.
- Keep metadata/data authority in code where appropriate. Metric definitions
  are generated from the Go registry rather than manually reauthored in chapters.

## Refresh the offline metric catalog

From the repository root after setting up the repo Go toolchain:

```powershell
Set-ExecutionPolicy -Scope Process Bypass -Force
. .\scripts\toolchain.ps1
$manualTools = Use-RepoToolchain -RequireGo
& $manualTools.GoExe run ./cmd/semantic-idf/internal/manualcatalog
```

metric-guides.json is the bundled fallback for previews without a desktop API.
The renderer prefers live Wails/local HTTP definitions when available. The
asset contract test compares the fallback with the registry and validates source
files, translation section IDs and cross-links. Browser tests validate the actual
Guide, navigation/history/search, safe rendering, cache and offline behavior.

## Verify a manual change

Use `dev test -Area guide` for its frontend/backend contracts or inspect changed
scope using `dev test -Plan`. `dev verify` includes the selected tests, frontend
source validation and a production Wails build. Tests remain in their existing
Go packages; no separate manual testing directory is required.
