# PTHP direct-energy acceptance evidence

This is checklist section 22, following the approved PTAC checkpoint `fab1650`.
An engine completion, source recipe, candidate diagnostic and approved expected
manifest are different gates. None can silently replace another.

## Original ownership and observations

The original `DOAToPTHP.idf` has SHA-256
`a73420b559c1f90061f65f6f1b6ccd6845a817d9110a7fbd5d63c266142435c6`.
Five literal SPACE Zones each own one packaged terminal heat pump, DX cooling
coil, DX heating coil, supplemental NaturalGas coil and local supply fan.
EquipmentConnections, EquipmentList, typed component references and DOAS mixer
connections are checked against the whole original input before scope filtering.
PLENUM-1 has no HVAC equipment; its observed sensible loads are zero. All six
Zone multipliers are one, with no ZoneList or ZoneGroup. Separate regression
fixtures prove that already-model-total component energy is not multiplied again.

The old `real-pthp-25-1-20260907T163131.692144900` capture had none of the 35
required direct-consumption identities at any frequency. It is not a zero-valued
direct-energy fixture and cannot approve a fallback-only result. The separate
normal capture `real-pthp-25-1-20260908T095959.079056400` adds the standard purpose
requests without changing original equipment, geometry, schedules or sizing.
Its 35 exact Monthly/J/non-meter identities contain 420 finite nonnegative
observations, with complete 2017 Weather months and no NULL or duplicate identity.

| Physical consumption | Owners | Annual original kWh |
| --- | ---: | ---: |
| Cooling DX electricity | 5 | 7565.922327728445 |
| Heating DX electricity | 5 | 129.0817607060335 |
| Heating defrost electricity | 5 | 44.544796591612524 |
| Heating crankcase electricity | 5 | 3963.256209764137 |
| Supplemental NaturalGas | 5 | 1718.862453752996 |
| Supplemental ancillary NaturalGas | 5 | 0 |
| Supplemental fuel-coil electricity | 5 | 0 |

Original meter-detail membership contains five cooling-electricity, twenty
heating-electricity and ten heating-gas constituents. Maximum monthly
component-to-meter differences are 2.05e-12, 4.55e-13 and 2.84e-13 kWh respectively.
The large crankcase contribution is measured, not a residual or an assumed
efficiency loss. It must not disappear from the heating-energy denominator.
Unlike this PTHP, the PTAC has a cooling-crankcase observation; its five-role
cohort remains separate and unchanged.

Both DX and Fuel coils report `Heating Coil Electricity Energy`. Output name
recognition therefore cannot establish physical ownership. Production and the
independent original-input validator separately resolve the exact name/key/type,
parent and Zone. Cross-type duplicate reporting identities, disconnected foreign
coils, shared references and stale selected-scope requests cannot legitimize an
ambiguous component. Missing, NULL, duplicate or negative members reject a whole
carrier cohort; complete observed-zero owners cannot receive its remainder.

Cooling uses COP. Mixed electricity/NaturalGas heating uses **Load / site energy**,
not an assumed COP or combustion efficiency. Its combined conversion denominator
and two carrier-qualified allocation ledgers are checked separately. Individual
fan energy is not reported: the broad Fans meter combines the DOAS and five local
fans. Although 32 original hourly node-flow series have 8,760 valid observations
each, they do not isolate an independently measured fan-energy pool. All
1010.3597051453659 kWh of broad fan energy remains explicitly unassigned.

## Verification-boundary findings

The new engine succeeds in 10.10 seconds with 24,523 warnings and no Severe/Fatal.
Its initial capture test nevertheless fails with 103 independent checks. Original
SQL, normal run result and manifests are retained; that test is not called a pass.
The first mismatch is observed-zero PLENUM interzone reconciliation provenance:
the serialized original contains both numeric zeros, but the test passed a native
bundle to an oracle that intentionally reads strict original v2 field presence.
A separate test-only verification copy now traverses the existing writer,
preflight and plain reader. Native RunResult, captured JSON and provenance hashes
remain unchanged; absent/NULL values and unknown Zone scope remain unknown.

The distinct SHA-bound saved candidate checks 17,113 metrics in all eight groups
with no coverage gaps. Diagnostic 01 retains three failures, all July natural-gas
Building energy reconciliation fields. Both the original facility and mapped
heating meter are exactly zero that month. Production correctly removes the zero
carrier node and its orphan accounting row, an existing EPATH-111 contract.
The independent SiteChecks now bind expected, explained and signed residual to one
exact whole-row proof. Whole-row absence is allowed only when all three valid
independent quantities admit zero. Positive expected/explained with zero residual,
present missing/NULL fields, contradictory context and unknown Monthly sources
still fail. Metric keys, numeric centers, precision and production pruning do not
change.

The focused capture/rebuild wire regressions pass in 0.497 seconds. The Site,
Reconciliation and AnnualSite tests pass in 13.615 seconds, including missing/null,
balanced-positive, tiny-positive and invented-Monthly-zero counterexamples. The
existing AnnualSite hand fixture now includes its truthful annual wrapper and
`residual` basis; its derived mixed-temporal quality fixture uses annual 240 kWh
versus monthly 20 kWh without changing its original 98.825%/100% expectations.
The quality regression and new saved diagnostic pass together in 13.640 seconds.

Diagnostic 02 passes all 17,113 metrics with zero numeric/contract failures and
zero coverage gaps in 12.55 seconds. It produces the separate, still-unapproved
pending-review artifact 01 with SHA-256
`1dbbf71ae4d346c57c582999ad4ae58dad55460b4584303c863fa012e1d2c38a`.
The required-key registry has SHA-256
`5142434aa3513763c71b956199a60a015b2f5b41471168cffae9b78cde40113c`.
It retains 5,819 known zeros, 806 explicit nulls, 455 count pairs and 4,927
coverage records across 91 scope-period contexts. The nulls remain 221 partial,
572 unavailable and 13 not-applicable values, not invented measurements.

An independent read-only arithmetic audit of original SQL against the serialized
original result separately checks 937 Building/Zone scalar and conversion-endpoint
values. Maximum displayed differences are 0.004666234 kWh for Zone and
0.003858526 kWh for Building, within the separately bounded per-source/month
decimal-three presentation stages. All 156 conversion pairs retain their exact
displayed endpoints and ratio kind; no scalar or tolerance was fitted to the
candidate. Literal recipe membership covers 81 driver-term bindings, 112 unique
Monthly load/direct/site/driver identities and all 46 heat-transfer surfaces.
The previous 340 Monthly identities and all 4,080 existing observations are
exactly unchanged by the added 420 direct-consumption observations.

All four earlier approvals pass current-code saved-original rebuilding without
changing their expected headers or companions: PTAC 17,021 metrics in 23.473
seconds, Small Office 17,675 in 19.073 seconds, Ideal Loads 17,121 in 30.827 seconds,
and Large Office 46,224 in 82.972 seconds. This also exercises the stricter
whole-row proofs against their original annual wrappers and period metadata.

## Provenance and limits

| Artifact | SHA-256 |
| --- | --- |
| New original SQL | `5470d4b338f0ad78c6b050ae4c196a78d856fa03a1c511b3db8aa98bf87b3b3e` |
| Executed input | `99a65916fa9895a3cc5d823696fbf202f4d620fa76b076fc8167be832a28dfd1` |
| Original simulation-result JSON | `2d17e2d005635cc3302472904176f9950973fed815360327b27ae26a1f1c08c1` |
| Candidate 01 | `49640515df820690493a5ed1b8ebf5d363bb38fb242b490c52dffaea0a0f0cd5` |
| Production source digest | `30025e59cf96fa4ef5c63e46824fb7b4e0bad636c724c65def3f4c79666fc12b` |

The original example describes Miami; the catalog deliberately retains controlled
Chicago TMY3 weather. Original warnings are not suppressed. This exercise validates
result transformation for the specified fixture, not climate suitability, design
adequacy or all heat-pump configurations.

## Approved expected manifest and saved acceptance

Root explicitly approves the separately reviewed pending artifact
`1dbbf71ae4d346c57c582999ad4ae58dad55460b4584303c863fa012e1d2c38a`.
The independent pending review recomputes 2,870 scalar assertions directly from
original SQL, including 140 component-source values, 584 exact-zero assertions,
208 unavailable assertions and 182 annual-versus-monthly sums. Maximum arithmetic
difference is 7.2759576e-12. The exact required-field selector bindings number
12,361; all belong to their declared source/scope/period. The 78 referenced
accounting records match their original Building records, not a blanket scope
exception. All 108 production-source files reproduce the bound digest.

The hand-authored expected header and generated 141,378-byte lossless companion
retain every metric. Companion SHA-256 is
`03928d1682f75f465290ccc7cba98b9f97305b98da62500e5a9837c441fb5724`;
uncompressed metric SHA-256 is
`a0de98770c1f3eefd635d320b73d0b1b69d3fb8369c37e5a0a5efc39a8ce9c5f`.
Separate saved-original-wire acceptance verifies all 17,113 expected values,
identities, counts and statuses in 22.21 seconds; the offline integrity guard
requires this fifth approved fixture. Together both tests pass in 23.915 seconds.
The four earlier headers/companions and all historical capture/diagnostic files
are unchanged.

Section 22 now has 5 of 19 approved catalog fixtures. Fan coil with plant loops
is next in order; fourteen catalog entries and later checklist sections remain
unfinished. Normal full repository verification, Wails build, commit and push
are still required for this implementation checkpoint.

The normal `scripts/verify.ps1` gate now passes without exclusions: app tests
21.138 seconds, CLI 6.247 seconds, frontend/browser checks 166.044 seconds and
simulation 288.174 seconds; all other packages pass. The Windows production
Wails executable builds successfully in 7.474 seconds. Normal commit-hook
verification and push follow; this does not mark the remaining checklist done.
