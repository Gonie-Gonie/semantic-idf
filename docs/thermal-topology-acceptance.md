# Thermal topology acceptance record

The final acceptance scenarios execute real parser/analyzer code and real
frontend ES modules. The browser harness serves the production frontend source
to an installed Chromium-family browser; it does not reimplement topology or
renderer logic in the test.

| Checklist | Automated flow | Evidence |
| --- | --- | --- |
| TOPO-280 Exterior zone | Select a spatial exterior wall, project it to the compact Outdoors edge, and inspect Gross area, Multiplier, openings, physical UA, and the exact OBC source. Confirm the Network remains zone-level with no boundary drill-down or area-basis controls. | `TestTOPO280ExteriorZoneIntegratedFlow`; browser signal `topo280` |
| TOPO-281 Interzone pair | Select a reciprocal zone edge, retain both source boundaries and the canonical opening, verify reversed construction layers, and inspect Gross area and Multiplier without creating boundary graph nodes. | `TestTOPO281InterzonePairIntegratedFlow`; browser signal `topo281` |
| TOPO-282 Modeling decision/QA | Select a geometrically adjacent but adiabatic observation as a two-surface QA target, prove no thermal relation is generated, and preserve stable boundary identity after the source fix. Confirm the Topology inspector has no Diagnostics or Actions section. | `TestTOPO282ModelingDecisionAndQAIntegratedFlow`; browser signal `topo282` |
| TOPO-283 Air coupling | Keep ZoneMixing and AFN paths separate from conductive edges and expose direction, schedule, design flow, base surface, component, and source evidence. | `TestTOPO283AirCouplingIntegratedFlow`; browser signal `topo283` |
| TOPO-284 Simulation separation | Build and validate the signed simulation-overlay result as a separate backend contract, while confirming the Topology tab exposes no simulated-heat metric, period selector, or heat-flow ledger and continues to show static UA. | `TestTOPO284SimulationOverlayIntegratedFlow`; browser signal `topo284` |
| TOPO-285 Context/Batch | Capture and restore Network metric, selection, and pan/zoom without backend calls; compare the fixed multiplier-adjusted Batch interzone area 8→12 m² as Δ4 m² / 50%. | `TestTOPO285SettingsAndBatchRoundTripContract`; browser signal `topo285` |

Run the integrated flows with:

```powershell
go test ./cmd/semantic-idf/internal/idf ./cmd/semantic-idf/internal/simulation ./cmd/semantic-idf/internal/frontendchecks -run 'TestTOPO28|TestTOPO285' -count=1
```

The repository-wide gate remains `scripts/verify.ps1`; it includes static
frontend checks, every Go package, headless-browser harnesses, and a production
Wails build.
