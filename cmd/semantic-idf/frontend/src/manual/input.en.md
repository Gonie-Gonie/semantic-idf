# Input, editing and source navigation

Text, JSON and Table show one working EnergyPlus model. Editing a value in one
view changes the document used by all analysis tabs. These views are projections
of parsed objects, rather than three independently saved documents.

## Load and identify a model {#load-model}

Use **Open** to load an IDF or epJSON file. The title and status identify the
loaded file. Analysis starts after the document is installed: Metrics and the
input projections become available first, followed by the more expensive
Profile, HVAC and Topology stages. A panel marked pending has not yet received
the current model's report.

The model's **Version** object supplies EnergyPlus version information. Reading
a version string does not itself prove that an installed engine can run the
model. IDF uses an ordered object/field representation; epJSON uses named object
maps and schema-defined fields. Their layouts differ even when their engineering
meaning is equivalent.

An object name is taken from a real name field when available. Objects without
one retain a type/index identity. A number displayed for an unnamed object is
an analyzer source identity, rather than a newly invented EnergyPlus name.
Duplicate names can make reference resolution ambiguous; check Diagnose before
assuming a click has found the unique intended definition.

Opening another document replaces the working document. Save edits you wish to
keep before changing files. The bundled Large Office sample is a starting
example, not evidence that your own model has equivalent completeness.

## Choose the input view {#input-views}

| View | Best use | Interpretation |
| --- | --- | --- |
| Text | Read familiar IDF object/field sequences and edit individual values | Formatted object display; it is not a free-form code editor |
| JSON | Inspect schema-shaped data, arrays and typed values | Keys, punctuation and structural tokens are read-only; editable value tokens patch the shared model |
| Table | Compare repeated objects of one type | Objects can be rows or columns; headings stay associated with source objects and fields |

Text and Table start with their object groups expanded. Collapse groups to
reduce visual noise. In Table, use the global orientation buttons or the
orientation control for an individual object type. Transposition changes the
presentation, not the model or field order.

Table renders at most 500 matching objects at once. Its hidden-object notice
means that additional matches exist; it does not mean they were deleted or
excluded from analysis. Narrow the filter to inspect them. Analysis and exports
operate on the model, independently of this rendering limit.

The JSON view is an epJSON projection even when the loaded file is IDF. Seeing
JSON does not convert the on-disk file. Detailed surface coordinates use
schema-shaped `vertices` arrays; a vertex's x/y/z components are separate values.
Preserve their units and order when making a geometric edit.

## Filter objects and locate values {#filter-objects}

The shared filter searches object type, real name or source index, field label
and value. Terms are case-insensitive and separated by whitespace; every term
must occur somewhere in the object's searchable content. Terms need not occur
in the same field.

For example, `Lights Office` finds objects whose searchable type/name/fields
contain both words. `Watts 12` may match a calculation method and its value.
This is text matching, not a numeric comparison or regular-expression query.
Filter results therefore do not establish that a method resolves successfully.

Changing the input view keeps the shared filter. If a selection temporarily
reveals an otherwise hidden object, the original filter remains in place.
Clear the temporary reveal or selection to return to the unchanged filter.
When a table reports no matching objects, clear the filter before concluding
that the input contains no objects of the expected type.

## Edit and validate a field {#edit-values}

In Text or Table, focus the value control and type the replacement. **Enter**
or leaving the control commits it. **Escape** restores the value that was
present when the control was edited. Available suggestions help locate valid
reference names; a suggestion does not guarantee the complete model is valid.

In JSON, activate an editable value token. Enter a valid JSON value: a number
such as `0.5`, a quoted string such as `"Always On"`, or the appropriate
schema-shaped value. **Enter** or leaving the editor commits; **Escape** cancels
the current token edit. Malformed JSON is rejected before applying the patch.

The edit is applied to the source object/field or nested JSON path. The document
is reparsed and analysis refreshes. Until that refresh completes, a derived
value may still be pending. Avoid treating a previously displayed result as
verification of the replacement value.

Blank, `0`, `Autosize` and `Autocalculate` are different inputs. A blank can
invoke an EnergyPlus default or omit a required reference. Zero is a numeric
value. Autosize/Autocalculate request engine-derived quantities and are not
measured design values that the static analyzer can always resolve.

Object references deserve particular care. Changing a definition name is not
a promise that every referring field will be renamed automatically. Use
Definition/References and Diagnose to check the affected relationships.

### Example: change lighting density {#lighting-edit-example}

1. Filter for the intended `Lights` object and its Zone or Space target.
2. Check that its calculation method is `Watts/Area` before editing that value.
3. Change 10 to 12 W/m² and commit the field.
4. Wait for current Metrics/Profile results. A 100 m² single-instance target
   changes its design power from 1000 to 1200 W, provided area resolves.
5. Inspect the target, schedule and multiplier. An annual lighting-energy result
   requires a simulation; the changed static power is not annual kWh.
6. Save the model when the resulting source and analyses are satisfactory.

## Save, revert and format identity {#save-revert}

**Save** writes the current working text to the opened file. If no path is
associated with the document, the application asks for a destination. Canceling
the save dialog leaves the document in memory. Save errors leave the working
document available for correction or saving elsewhere.

**Revert** restores the text that was originally loaded. It is not an undo stack
and it is not necessarily the most recently saved text. Saving an edited model
does not redefine that load baseline. Revert then runs analysis for the restored
document; save again only if you intend to write that restored content to disk.

Normal field/profile/HVAC edits are written in the input's original format.
Switching to JSON is a view operation. Serialization can normalize formatting,
field representation or generated labels; it is not a byte-preserving editor.
If comments and exact original formatting matter, retain the original file as
a reference and review the saved document.

Conversion between IDF and epJSON depends on the versioned object schema and
field mapping. The JSON projection helps inspect those mappings; the main
input tabs do not provide a standalone format-conversion command. Changing a
filename extension alone does not convert the content.

Cleanup and automatic fixes are explicit operations in **Tools / Diagnose**.
Review their preview and candidate exclusions before applying them. Removing
unused objects or resolving duplicate names can change reference structure;
do not infer that an apparently redundant object is safe merely because the
current view hides it.

## Select, reveal and follow references {#source-navigation}

Metrics, Profile and Topology share model selection with Text, JSON and Table.
A click commits selection without switching result tabs. Hover only highlights
the target and related occurrences. Double-click or **Enter** on a navigable
target opens its preferred occurrence/view; entering a value editor is a
separate, explicit operation.

| Action or default key | Effect |
| --- | --- |
| Definition / F12 | Reveal the referenced definition in an input view |
| References / Shift+F12 | Cycle known referring occurrences |
| Alt+Enter | Choose among available destinations when more than one applies |
| Alt+Left / Alt+Right | Restore previous/next navigation context |
| Escape | Close a transient chooser first, otherwise clear selection |

Keys are configurable and do not override normal editor input. Use the visible
controls when a keyboard action would conflict with value editing.

A Zone may have several occurrences, such as its geometry and load profiles.
The same object is not duplicated in the input because it appears in several
contexts. A chooser is used where the intended destination is ambiguous.
Following selection preserves filters, view mode and local scope, and does not
switch result tabs automatically. HVAC and Simulation keep their own local
selection; their clicks do not promise source reveal or global follow.

Navigation uses the analyzed snapshot. If the document has changed, a target
may wait for current analysis before it is revealed. Back/Forward restore a
compact view/selection context; they do not undo edits. Scroll, hover and
pan/zoom do not create individual navigation-history entries.

## Interpret problems before proceeding {#input-problems}

| Symptom | Check next |
| --- | --- |
| JSON/Table is pending | Wait for current parsing; inspect the status for a parse error |
| Object cannot be located | Clear filters; verify exact type/name and duplicate-name issues |
| A reference has several possible definitions | Review type-specific target rules and duplicate names in Diagnose |
| A geometric object has no polygon | Check supported object type, numeric coordinates, number of vertices and referenced base surface |
| A value is missing after a successful parse | Read its metric method: parsing success does not guarantee enough data for that calculation |
| An edit is rejected | Check JSON syntax, the target field/path and the backend error before retrying |
| A changed value seems to have no effect | Verify calculation method, actual Zone/Space target, referenced schedule, current analysis and multiplier |

Continue with [Metrics and Profile](./metrics.en.md#reading-metrics),
[Topology](./topology.en.md#topology-workflow) or
[static HVAC analysis](./hvac.en.md#hvac-workflow) to interpret the derived views.
