# Thermal topology acceptance record

The final acceptance scenarios execute real parser/analyzer code and real
frontend ES modules. The browser harness serves the production frontend source
to an installed Chromium-family browser; it does not reimplement topology or
renderer logic in the test.

| Checklist | Automated flow | Evidence |
| --- | --- | --- |
| TOPO-280 Exterior zone | Select a spatial exterior wall, project it to the compact Outdoors edge, inspect Area/UA/openings and exact OBC anchors, expand, then restore the edge with Back. | `TestTOPO280ExteriorZoneIntegratedFlow`; browser signal `topo280` |
| TOPO-281 Interzone pair | Select and expand a reciprocal zone edge, retain both surfaces and the canonical opening, verify reversed construction layers, reveal both faces, and compare Graph/Matrix Area and UA. | `TestTOPO281InterzonePairIntegratedFlow`; browser signal `topo281` |
| TOPO-282 Modeling decision/QA | Select a geometrically adjacent but adiabatic observation as a two-surface QA target, prove no thermal relation is generated, open the exact invalid-counterpart issue, edit the field, and restore by stable boundary entity. | `TestTOPO282ModelingDecisionAndQAIntegratedFlow`; browser signal `topo282` |
| TOPO-283 Air coupling | Keep ZoneMixing and AFN paths separate from conductive edges and expose direction, schedule, design flow, base surface, component, and source object. | `TestTOPO283AirCouplingIntegratedFlow`; browser signal `topo283` |
| TOPO-284 Simulation overlay | Build a surface-detail run plan, map signed EnergyPlus series, choose a monthly frame, inspect provenance/sign/direction, jump to the output plan, and return to static UA. | `TestTOPO284SimulationOverlayIntegratedFlow`; browser signal `topo284` |
| TOPO-285 Settings/Batch | Capture and restore graph mode, metric, selection, pan/zoom without backend calls; compare model-total interzone area 8→12 m² as Δ4 m² / 50% while physical area remains unchanged. | `TestTOPO285SettingsAndBatchRoundTripContract`; browser signal `topo285` |

Run the integrated flows with:

```powershell
go test ./internal/idf ./internal/simulation ./internal/frontendchecks -run 'TestTOPO28|TestTOPO285' -count=1
```

The repository-wide gate remains `scripts/verify.ps1`; it includes static
frontend checks, every Go package, headless-browser harnesses, and a production
Wails build.
