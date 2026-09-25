# PV/storage native acceptance

`pv-storage-25-1` is approved for 16,523 independent semantic metrics. The
closed official `ShopWithPVandBattery.idf` annual capture has zero Severe
errors and 59 warnings. Full original-wire diagnostic and separately approved
saved acceptance both pass. This is source/accounting validation, not HVAC
design certification or completion of the remaining engineering goal.

## Production corrections

- Basic Energy Path requests exact Hourly AirLoop fan pools when the original
  input does not already supply an equivalent unfiltered key or wildcard.
  Requests require actual loop/branch/component/fan ownership and terminal
  recipients, with the complete original fan inventory covered. Mixed local,
  nested, exhaust or unowned fans do not trigger heavy partial observations.
  Native blank-key wildcards are reused without rewriting their literal input.
  This fixture adds exactly five series; no physical input changes.
- Consumed `Cogeneration:<resource>` participates in end-use availability,
  rather than being dropped by a generator/supply name match. Electricity
  ancillary consumption remains canonical Other, not onsite production.
- Driver allocation uses bounded largest-remainder apportionment at the
  existing three-decimal precision. Previously the final source accumulated
  other contributors' rounding errors; four real Zone/month/annual checks
  failed. Stable source ordering now keeps each positive branch within its own
  floor/ceiling quota and preserves exact monthly load closure. Raw signed
  pressures, source identities, eligibility and zero-pressure fallback stay
  unchanged. Existing out-of-bound compatibility is not a new precision promise.

The remote UI is unchanged.

## Independent engineering boundaries

The original 466 physical objects, original output declarations, annual
controls, engine and weather are preserved. Native comparison finds all 733
prior dictionary identities and all 5,864,508 common observations exactly
unchanged, including 2,317,248 reported zeros. All 61,332 Time and 4,068 extended
rows remain exact. Five new fan series add 43,800 observations. Their native
monthly sums close the existing Fans meter without rescaling.

Twenty electrical identities at Monthly/Hourly plus two consumed-Cogeneration
observations retain native metadata, raw/effective values, signs, full calendars
and chart axes. Nine native electrical equations are checked monthly, hourly
and annually. Native Facility closure uses only the EPSM native deadband.
The source registry cannot be bypassed by deleting graph nodes or local checks.
The exact SQL-sibling MTD hash is required through capture, pending and approval.

PV DC production (54,522.196715 kWh), inverter AC output (49,224.732528 kWh),
inverter loss (4,037.580186 kWh), storage charge (10,576.973327 kWh) and discharge
(9,317.089326 kWh) remain distinct native quantities. Storage thermal output
(3,479.368355 kWh) is not assumed equal to charge minus discharge; unobserved
state-of-charge closure is not invented. Inverter ancillary electricity and
its 446.8 kWh consumed Cogeneration parent represent one budget, not two.

The exact five-Zone HVAC topology, terminal delivery and separate unassigned
SHW pump boundary are bound to original and executed inputs. Mandatory seals
cover 751 existing fan/service selectors. Monthly Facility provenance may
remain on independently checked Zone subtotals, never as duplicated Facility
Zone flow or a substituted Building amount. Cached displayed/allocated fields
are checked against proved nodes; cached raw-value consistency is not described
as an additional independent native magnitude proof.

All 114 additional original-owned Hourly companions are nonadditive context of
Monthly authorities: 998,640 Hourly cells and 1,368 Monthly cells, with complete
native month closure. Positive gas equipment consumption is 2,564.370621 kWh;
it is not inferred zero from the native zero latent-gain series. Gas convective
gain retains its actual original fraction and native values. Absent heating,
water-system gas and Zone latent-load outputs remain absent, not known zero.

Actual requested-stage availability remains 12/18 drivers, 9/14 loads, 10/12
end uses and 2/2 carriers. Annual driver closure is 100%; end-use/carrier closure
remains 94.122% and partial. Zero oracle gaps do not mean all native outputs exist.

## Separate review and acceptance

Rejected diagnostic 02 is preserved with 526 numeric/contract failures and 830
coverage gaps, including repeated prerequisite failures. Its four genuine
allocation precision errors were fixed; canonical Other and Hourly source
declarations were corrected in the independent recipe. No tolerance was raised.

Diagnostic 03 passes in 220.861 seconds with 16,523 metrics across eight groups:
5,824 drivers, 4,380 loads, 610 end uses, 358 carriers, 390 ratios, 468
completeness, 2,964 residuals and 1,529 Zone-allocation checks. All 5,275 coverage
records and 11,335 required fields pass. There are 4,916 numeric zeros and 525
typed nulls in the lossless metrics, not a selected summary.

Root separately reran native preservation, fan and Hourly audits, then a
native-only arithmetic script fixed before reading the pending artifact. All
159 representative obligations match. Arithmetic uses native SQL and original
ownership, without production calculations or candidate-derived expectations.
The script author had previously seen candidate metadata while diagnosing bugs;
the independence claim is about arithmetic authority, not never seeing a
candidate. This corroboration supplements, not replaces, the full eight-group
oracle. Pending SHA `e707f6edc6bb6e97cf381d40f5bdce67c2668661f51e69b35cc002d36e68d6d7`
remains a preserved unapproved export; a separate reviewed header authorizes
the installed expected companion.

Expected prepare and installation pass in 8.929 and 9.089 seconds. The subsequent
read-only saved-original-wire acceptance passes in **431.379 seconds**, checking
all eight groups against the approved artifact without rebuilding or writing
the capture. Snapshot provenance binds that production revision, original/executed
inputs, capture, SQL, MTD, engine and weather before comparison.

After tightening the output-request census, a separate current-code rebuild
and saved acceptance passes in **470.510 seconds**, again covering all 16,523
metrics against the same approved header/companion. No old snapshot is reused
and no native capture or approved value is changed.

## Immutable evidence

Capture directory under ignored `.runtime/energy-path-acceptance/25.1/`:
`pv-storage-25-1/real-pv-storage-25-1-20260925T031519.297356600`.

| Artifact | SHA256 |
| --- | --- |
| Original IDF | `9c3f9b637b1e04c2e4b8911854c36ffd6442ea86cfe8e165fccaaeb8b2be9cd0` |
| Executed IDF | `429fa82fca43d276ce68327e81730716a8809885efa0292e8acee88864752be0` |
| SQL | `5f4dde5c868f419b2c169defd38ce7d3ec597fef9e1e970f28fbf8aa3548f2d8` |
| MTD | `87bf8f501d8bb3eacf32d80c29401004c9ff42a2af9d51fb39ae793e11686438` |
| Reviewed recipe | `475f66f87ca840cb3a06ab4ecd0c5e7e89812e7c2dc5d16febd06d368a2f6ee2` |
| Approval-time original-wire candidate 02 | `8206883cc0a58dd267d92517640543ff923af1faae4512708d372c7b61ec2fcb` |
| Diagnostic 03 | `9f96018a601270c9ee9feb5ea83ccd0ec69f2799fd479e67eb733f8e588cf599` |
| Native 159-obligation audit | `af919278e2aad778e1968d3db9df30a310ffd918ba4782dd74ca27a6fc49499f` |
| Separate pending comparison | `fa3c63cdbb6a772ee043f86469d243e43a8b7881c7ec93a419ff8019c4b034c1` |
| Approved header | `d8924d81186d88394e0e0c38a9990417e0900ae491c69d67a68b28bc7d7dd984` |
| Compressed metric companion | `43348780a7c080fd9b031177e26d5330cb2722b766452c9fb230d796d3e270e1` |
| Uncompressed metrics | `566a4b89bb842fca23f2e0c6536b78e888a0b30142ff4f3becf4ffd21cda3ec3` |

## Current checkpoint gates

Graph/Zone-lineage focus passes in 0.979 seconds; Cogeneration canonical Other
in 0.659; allocation/fan/interzone in 7.285; combined PV/source/fan/quality/MTD/
pending/gas-Hourly focus in 55.789. Existing EPATH-210 capacity and external-proof
obligation regressions pass in 7.276 seconds. Actual GUI/App/HTTP/CLI/Python
parity passed before the final allocation change; the full normal gate must
rerun it on the final tree.

All twelve previously approved captures pass allocation-revision rebuilding
and saved acceptance, without an engine run or old snapshot. The frozen replay
script is `oracle-cache/approved-twelve-current-code-replay-20260925.ps1`, SHA
`470f449117e13457df440bbcc2de3214f01baedc3e8b7d8dce2a63233218007d`.
It pins the exact native captures and verifies all 24 prior approval header/
companion hashes both before and after the run. None changed. This replay
precedes the request-only census/wildcard follow-up; the saved SQL parser and
allocator have not changed since it.

| Prior approved fixture (25.1) | Metrics | Current-code saved test, seconds |
| --- | ---: | ---: |
| Large Office | 46,224 | 80.165 |
| Small Office | 17,675 | 19.037 |
| Ideal Loads | 17,121 | 30.259 |
| PTAC | 17,021 | 23.862 |
| PTHP | 17,113 | 24.999 |
| Fan Coil | 8,966 | 9.212 |
| VRF | 16,360 | 74.760 |
| Radiant | 9,615 | 79.273 |
| District Energy | 17,001 | 21.227 |
| Mixed Heating Fuels | 17,275 | 221.182 |
| ZoneGroup | 21,413 | 537.466 |
| Zone multiplier/Pool | 16,976 | 311.746 |

The initial full repository attempt failed in the simulation package after
539.998 seconds: the new request hook exposed the existing PTAC mixed-fan
lightweight constraint and chart-pair assertions. The request gate now demands
the whole original fan census; unresolved mixed inventory stays unassigned.
The pair assertion distinguishes only the exact unfiltered Building-wide
Hourly fan allocation observation from chart pairs, retaining duplicate checks.
Unchanged PTAC plus baseboard/District/VRF and fan-request focused tests pass
in 11.245 seconds. These failures are retained, not treated as a full gate pass.

The final request focus, including literal blank-key preservation, passes in
11.378 seconds. Standalone `scripts/verify.ps1` then passes: simulation
572.184 seconds, frontend/browser 180.000 seconds, App 19.703 seconds, the
remaining packages passing or cached, and the production Wails build in
8.58 seconds. The default test timeout is unchanged. The normal commit hook
is still required before push. The fixture approval count is 13/19, not a
claim that the remaining six cases or the goal are complete.
