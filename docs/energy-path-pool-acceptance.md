# Pool / Zone multiplier engineering verification

Status: implementation is pushed in `6149d5e`; independent expected values and
fresh saved acceptance now pass. Pool is the twelfth of nineteen approved
numerical fixtures. Full repository/build verification also passes; normal
commit-hook verification and push finish this checkpoint. The user's remote
UI is unchanged.

## Physical and native evidence

The untouched official 25.1 `5ZoneSwimmingPoolZoneMultipliers.idf` has SHA-256
`e8fe7fdc91c4127f55f645ac059323771fa4a92f16e86900953fa8d2ceff9301`.
All six original Zones have multiplier 3 and ListMultiplier 1. Five are supply
recipients; the return plenum retains its measured thermal loads but is not
promoted into a supplied Zone. Original, independently parsed executed and SQL
object indices are different identities and are never inferred by an offset.

The preserved successful capture is
`real-zone-multiplier-25-1-20260914T185259.656652100`:

| Evidence | SHA-256 |
| --- | --- |
| Native SQL | `95d3e0a145d0f39fd0f2518c47a4163f583fa900cee62c1bd904a290a94d20d1` |
| Executed IDF | `9aa607a2372c64ea82f81253912e4abfa596d0dc4fb4e5939131b3046f907f70` |
| Run evidence | `9f6cde70381661c477ef2379af312106af5c926a6d42f29e78e1a73b3a229b24` |

Native execution succeeds with zero Severe/Fatal errors and seven warnings:
four unsupported legacy gas-alias requests and three preserved original
MeterFileOnly duplicate requests. Optional unobserved outputs remain missing,
not measured zero. The independent recapture audit confirms all 309 physical
objects and all 932,530 common native observations are unchanged. All 52 files
in the preservation baseline, including prior approved expectations, remain
unchanged after materializing the new candidate.

## Accounting boundaries

The complete hot-water plant contains five reheat coils, outdoor-air and main
heating coils, and the indoor pool. Pool-water heat and boiler output are
nonadditive thermal context, not extra Zone-air load or purchased consumption.
The process demand prevents a defensible allocation of the whole Heating meter
to space Heating. Building and Zone conversion ratios, including cached legacy
summary collections, therefore retain an explicit unquantified-service boundary.
Observed consumption stays in Building accounting and is unassigned to Zones.

Native chilled-water pump electricity has its own separately proved cooling
plant and five-Zone recipient roster. Only this budget can be allocated by
primary sensible cooling load. Hot-water pump electricity remains unassigned.
Native components already report model totals; their observations receive no
additional Zone multiplier. Representative Zone loads receive factor 3 once.

Individual proportional rounding intervals are insufficient to prove a joint
budget. The independent checker additionally requires exact milli-kWh equality
between all actual Zone allocations, the Building allocation ledger and the
canonical CW source's allocated-only Zone details. Each annual allocation must
be the sum of its twelve completed monthly allocations. A single 0.001-kWh
budget cannot become three 0.001-kWh recipient amounts. Native subquantum
monthly energy can round to zero allocation while its independently integrated
annual source remains nonzero.

All 28 Pool/boiler/pump Energy/Rate Monthly/Hourly identities have mandatory
raw/effective source checks. Annual source scalars use native integration and
one three-decimal transport step, independently of the 8,760 separately rounded
Hourly chart samples. Missing/null source and chart data cannot become observed
zero during the original-wire decoder. Each graph node's mandatory `value` is
also checked before Go's typed decoder can replace a JSON null with zero.

The F1-1 Ground floor remains in all twelve ordinary signed surface cells.
Its measured surface-to-air transfer is neither removed nor adjusted by Pool
water heating. The source must retain its exact executed Pool/floor references
and explain the coupling; it is not an isolated passive envelope quantity.

## Verification history

The native oracle is separate from production builders and uses original IDF,
actual output requests, native SQL dictionaries/calendar and hand-SQL regressions.
The initial complete oracle focus passes in 102.041 seconds. Original-capture
materialization passes in 28.59 seconds and creates the separate immutable
`pool-native-candidate-20260915-01.json` (27,381,669 bytes), with candidate SHA-256
`c735ab79e5f409e31cd6cc99f53e31c8d25dd07e98e6b14a323fd69ba89d9f0a`.
Production SHA-256 is
`6431f82263c80411512404157a56221182b55d9371229688aa14ff7532a32c7f`.

Diagnostic 01 passes 16,975 independent checks across all eight groups with
zero numerical/contract failures and zero coverage gaps in 252.81 seconds.
The additional mandatory Pool-floor interpretation check brings the count to
16,976. Its focused suite passes in 111.620 seconds. Diagnostic 02 preserves
all previous numeric successes but rejects one overly strict oracle assumption:
ordinary native surface sources retain an existing allocation explanation in
`Formula`; that is not a transformation of their measured raw/effective energy.
The failed diagnostic is retained, and no pending or expected artifact is
created from it. A bounded distinction between declared allocation explanation
and source transformation is required before the final retry.

The bounded correction allows only the two existing literal driver-allocation
explanations when `AllocationApplied` is true and `Formula` exactly matches
`AllocationFormula`; `InputSourceIDs` and arbitrary source transformations remain
forbidden. Hand regressions accept both ordinary explanations on Pool and peer
surfaces and reject fabricated subtraction, mismatched or extended formulas.
The surface focus passes in 13.403 seconds. Diagnostic 03 then passes all
16,976 checks, with no numerical/contract failures or coverage gaps, in
253.38 seconds against the same immutable candidate. Diagnostic 02 is preserved.

## Independent pending review

The separately generated, still-unapproved pending artifact has SHA-256
`c6e21b9b835790a0a5dcadad2dc531b77ac2cef7424586a9d9c98602337fd4fe`.
Diagnostic 03 has SHA-256
`4ff60f4f1e9f4d3b1743c4cabf18d74c9cc5a7528c208fab631fc5d147b08a55`.
The complete required-key registry has SHA-256
`b7b7bde80dd225e106750e59ad9e05f8f32c37eefd78ef9768cbdea8d7885824`.

Two separate read-only reviews pass, and root independently reruns both. Native
arithmetic is computed before opening pending evidence; its predeclared basis
SHA-256 stays `96862cfa940822093fd5bb72375c92c73b92f48ffbbd7d46ead453c11e5f7908`.
It verifies 1,318 selected arithmetic obligations, including 56 native annual
source scalars, 13 pump budgets, 475 exact-known-zero checks and 91 explicit
Heating semantic nulls. The maximum native center difference is
2.9103830456733704e-11 kWh. Ordinary driver quantities and actual graph/summary
absence remain covered by the full original-wire diagnostic, not claimed as
independently recomputed by this additional bounded review.

The registry review independently reconstructs all 671 source-only metrics:
574 generic, 40 direct-use, 56 native Pool/plant and one Pool-floor qualifier.
Its 186 native identities include 28 Pool/plant identities, eight measured-zero
ones. Another 115 Hourly companions are nonadditive links to selected Monthly
authorities, not additional scalar budgets. All 91 Building/Zone Annual/Monthly
contexts retain 4,844 coverage records and 10,471 required fields with 12,743
selector bindings. The 156 copied Building ledgers are referenced accounting,
not invented local Zone allocations. All 6,338 explicit zero metrics and 611
typed nulls are preserved. No candidate scalar is copied into expected values;
all 52 preservation-baseline files and current production provenance match.

Unchanged-expectation replay of all eleven earlier fixtures passes. The final
ZoneGroup rebuild passes 21,413 checks in 576.94 seconds; all twenty-two earlier
approval headers and companions retain their original hashes.

On 2026-09-15 KST, root explicitly approves the immutable independent pending
SHA-256 `c6e21b9b835790a0a5dcadad2dc531b77ac2cef7424586a9d9c98602337fd4fe`
after the native physical review, complete diagnostic, two independently rerun
pending audits and eleven-fixture replay above. Approval applies to all 16,976
independently derived metrics, including zeros and semantic nulls; it does not
claim physical HVAC design certification or completion of the overall goal.
The explicit packaging step passes in 8.90 seconds and companion installation
passes in 8.75 seconds. The separately authored approval header is not generated
or rewritten by either step. The 146,471-byte lossless companion has SHA-256
`d0dbef31ca8ccaefbb9da69967bc2c3786736ba039f3bfaac47d025f0040ba2b`;
its uncompressed payload has SHA-256
`a8915b4e59f53499b1816cec887ba4274c13e1e061a1975355ef9fa3d73c7dbc`.
The manually authored header has SHA-256
`a4e87b8787569823ab63fefa85e350fd1bc3d6fb62646d7fabf637ed2565352b`.
The twelve-fixture approved-catalog integrity guard passes in 3.03 seconds.
All 52 preservation-baseline artifacts are rechecked unchanged after packaging.
Fresh saved acceptance rebuilds the current shared projection from the actual
executed input and closed native SQL, not the candidate snapshot. It passes all
16,976 semantic metrics across eight groups in 512.32 seconds (combined catalog
and saved-test package 515.508 seconds). Original captured payloads and files
remain unchanged. Section 22 is now 12/19 approved; PV/storage, no cooling, no
heating, simultaneous operation and Large Office 22.1/23.2/24.2 remain. Full
repository/Wails verification and normal commit/push are the next checkpoint
gates, not substitutes for any remaining actual-model acceptance.

The full `scripts/verify.ps1` gate passes without exclusions or timeout changes:
app 17.931 seconds, frontend/browser checks 176.922 seconds and simulation
566.299 seconds; CLI, input, IDF and tabular pass from cache. The Windows
production executable builds in 6.485 seconds. Both newly approved artifact
hashes remain unchanged after the full gate. Normal commit-hook verification
and push follow; no overall goal completion is claimed.
