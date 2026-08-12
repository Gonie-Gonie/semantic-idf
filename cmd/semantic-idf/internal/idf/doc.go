// Package idf parses EnergyPlus IDF documents and builds the analyzer's
// semantic model.
//
// File groups in this package are intentionally organized by feature prefix:
//
//   - document, parser, edit: core IDF representation and low-level editing.
//   - analyze, summary, diagnostics, cleanup: cross-domain analysis and fixes.
//   - field_catalog: EnergyPlus field metadata used by diagnostics and graphing.
//   - geometry: zone, surface, construction, and plan/3D geometry extraction.
//   - hvac*: HVAC loop, service-coupling, resolver coverage, and output logic.
//   - output*: requested output objects and standard recommendation helpers.
//   - profile*: schedule profile analysis, graph deck data, and apply helpers.
//   - semantic_yaml*: semantic YAML rendering and golden-file support.
//   - simulation_requirements: small simulation input requirement checks.
//
// Keep tightly coupled IDF analysis helpers in this package while they share unexported
// parser/catalog helpers. New UI, CLI, or static-check code should live outside
// this package unless it needs direct access to those internals.
package idf
