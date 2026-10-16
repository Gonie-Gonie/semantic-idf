package idf

import (
	"strings"
	"testing"
)

func hvacConfiguredTestWalk(expression HVACOperatingExpression, visit func(HVACOperatingExpression)) {
	visit(expression)
	for _, child := range expression.Children {
		hvacConfiguredTestWalk(child, visit)
	}
}

func hvacConfiguredTestHasReason(expression HVACOperatingExpression, reason string) bool {
	found := false
	hvacConfiguredTestWalk(expression, func(item HVACOperatingExpression) { found = found || item.Reason == reason })
	return found
}

func hvacConfiguredTestNightCycleProfile(t *testing.T, manager Object, extras []Object, loopType string) HVACOperatingProfile {
	t.Helper()
	fan := hvacOperationTestObject(0, "Fan:OnOff", "Fan", "Availability Schedule Name", "Fan Hours")
	owner := hvacOperationTestObject(1, loopType, "Loop", "Availability Manager List Name", "Managers")
	objects := []Object{fan, owner,
		hvacOperationTestRaw(2, "AvailabilityManagerAssignmentList", "Managers", manager.Type, objectName(manager)), manager,
		hvacOperationTestRaw(4, "Schedule:Constant", "App", "", "1"),
		hvacOperationTestRaw(5, "Schedule:Constant", "Fan Hours", "", "0"),
	}
	objects = append(objects, extras...)
	loop := HVACLoop{ID: "loop:l", ObjectIndex: 1, Type: loopType, SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(fan, "In", "Out")}}}}}
	return hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(Document{Objects: objects}), []HVACLoop{loop}, nil), 0, loop.ID)
}

func TestHVACOperatingConfiguredAvailabilityExcludesOnlyValidRuntimeManager(t *testing.T) {
	manager := hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "App", "Fan Hours", "CycleOnAny", "1", "FixedRunTime", "1800")
	profile := hvacConfiguredTestNightCycleProfile(t, manager, nil, "AirLoopHVAC")
	if !hvacConfiguredTestHasReason(profile.FinalAvailability, "runtime_manager") || !hvacConfiguredTestHasReason(profile.ConfiguredAvailability, "runtime_manager_excluded") || hvacConfiguredTestHasReason(profile.ConfiguredAvailability, "runtime_manager") {
		t.Fatal("legacy runtime execution gate was changed, or runtime outcome leaked into configured permission")
	}
	foundOwn := false
	hvacConfiguredTestWalk(profile.ConfiguredAvailability, func(item HVACOperatingExpression) {
		if item.ScheduleName == "App" {
			t.Fatal("NightCycle applicability became a minimum equipment availability condition")
		}
		if item.ScheduleName == "Fan Hours" {
			foundOwn = true
			if item.Source == nil || item.Source.ObjectIndex != 0 {
				t.Fatalf("own fan availability was attributed to inherited manager: %#v", item)
			}
		}
		if item.Reason == "runtime_manager_excluded" && (item.Op != "always" || item.Value == nil || *item.Value != 0) {
			t.Fatalf("runtime manager projection must be NoAction status, not boolean Off/On: %#v", item)
		}
	})
	if !foundOwn {
		t.Fatal("actual own availability schedule disappeared with runtime manager")
	}
}

func TestHVACOperatingConfiguredAvailabilityRetainsDefinitionErrors(t *testing.T) {
	for _, test := range []struct {
		name, reason string
		manager      Object
		extras       []Object
		loopType     string
	}{
		{"missing_schedule", "missing_or_ambiguous_runtime_manager_schedule", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "Missing", "Fan Hours", "CycleOnAny"), nil, "AirLoopHVAC"},
		{"duplicate_schedule", "missing_or_ambiguous_runtime_manager_schedule", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "App", "Fan Hours", "CycleOnAny"), []Object{hvacOperationTestRaw(6, "Schedule:Constant", "App", "", "1")}, "AirLoopHVAC"},
		{"malformed_schedule", "invalid_runtime_manager_schedule_definition", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "Bad", "Fan Hours", "CycleOnAny"), []Object{hvacOperationTestRaw(6, "Schedule:Constant", "Bad", "", "NaN")}, "AirLoopHVAC"},
		{"bad_interpolation", "invalid_runtime_manager_schedule_definition", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "Bad", "Fan Hours", "CycleOnAny"), []Object{hvacOperationTestRaw(6, "Schedule:Day:Interval", "Day", "", "Bogus", "24:00", "1"), hvacOperationTestRaw(7, "Schedule:Week:Compact", "Week", "AllDays", "Day"), hvacOperationTestRaw(8, "Schedule:Year", "Bad", "", "Week", "1", "1", "12", "31")}, "AirLoopHVAC"},
		{"bad_interpolated_body", "invalid_runtime_manager_schedule_definition", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "Bad", "Fan Hours", "CycleOnAny"), []Object{hvacOperationTestRaw(6, "Schedule:Day:Interval", "Day", "", "Linear", "24:00", "NaN"), hvacOperationTestRaw(7, "Schedule:Week:Compact", "Week", "AllDays", "Day"), hvacOperationTestRaw(8, "Schedule:Year", "Bad", "", "Week", "1", "1", "12", "31")}, "AirLoopHVAC"},
		{"unknown_schedule_type", "invalid_runtime_manager_schedule_definition", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "Bad", "Fan Hours", "CycleOnAny"), []Object{hvacOperationTestRaw(6, "Schedule:NotARealType", "Bad", "", "1")}, "AirLoopHVAC"},
		{"bad_control_type", "invalid_runtime_manager_definition", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "App", "Fan Hours", "NotARealControl"), nil, "AirLoopHVAC"},
		{"bad_run_type", "invalid_runtime_manager_definition", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "App", "Fan Hours", "CycleOnAny", "1", "NotARunType"), nil, "AirLoopHVAC"},
		{"missing_control_zone", "missing_runtime_manager_zone", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "App", "Fan Hours", "CycleOnControlZone", "1", "FixedRunTime", "1800", "Missing Zone"), nil, "AirLoopHVAC"},
		{"unknown_manager_type", "unsupported_manager_type", hvacOperationTestRaw(3, "AvailabilityManager:Unknown", "Night", "App"), nil, "AirLoopHVAC"},
		{"invalid_hybrid_list", "invalid_manager_assignment", hvacOperationTestRaw(3, "AvailabilityManager:HybridVentilation", "Night"), nil, "AirLoopHVAC"},
		{"invalid_plant_nightcycle", "invalid_manager_assignment", hvacOperationTestRaw(3, "AvailabilityManager:NightCycle", "Night", "App", "Fan Hours", "CycleOnAny"), nil, "PlantLoop"},
		{"bad_optimum_algorithm", "invalid_runtime_manager_definition", hvacOperationTestRaw(3, "AvailabilityManager:OptimumStart", "Night", "App", "Fan Hours", "StayOff", "", "", "6", "NotAnAlgorithm"), nil, "AirLoopHVAC"},
		{"missing_default_optimum_zone", "missing_runtime_manager_zone", hvacOperationTestRaw(3, "AvailabilityManager:OptimumStart", "Night", "App", "Fan Hours"), nil, "AirLoopHVAC"},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := hvacConfiguredTestNightCycleProfile(t, test.manager, test.extras, test.loopType)
			if !hvacConfiguredTestHasReason(profile.ConfiguredAvailability, test.reason) || hvacConfiguredTestHasReason(profile.ConfiguredAvailability, "runtime_manager_excluded") {
				t.Fatalf("definition failure was neutralized: %#v", profile.ConfiguredAvailability)
			}
		})
	}
}

func TestHVACOperatingConfiguredPlantActivationKeepsChillerOwnDefaultAndSources(t *testing.T) {
	chiller := hvacOperationTestObject(0, "Chiller:Electric:ReformulatedEIR", "CoolSys1 Chiller 1")
	other := hvacOperationTestObject(6, "Chiller:Electric:ReformulatedEIR", "Other Chiller")
	loopObject := hvacOperationTestObject(1, "PlantLoop", "CoolSys1")
	doc := Document{Objects: []Object{chiller, loopObject,
		hvacOperationTestRaw(2, "PlantEquipmentOperationSchemes", "CoolSys1 Loop Operation Scheme List", "PlantEquipmentOperation:CoolingLoad", "CoolSys1 Operation Scheme", "ALWAYS_ON"),
		hvacOperationTestRaw(3, "PlantEquipmentOperation:CoolingLoad", "CoolSys1 Operation Scheme", "0", "100000000000000", "CoolSys1 Equipment List"),
		hvacOperationTestRaw(4, "PlantEquipmentList", "CoolSys1 Equipment List", chiller.Type, objectName(chiller)),
		hvacOperationTestRaw(5, "Schedule:Constant", "ALWAYS_ON", "", "1"), other,
	}}
	loop := HVACLoop{ID: "plant:cool", ObjectIndex: 1, Type: "PlantLoop", OperationScheme: "CoolSys1 Loop Operation Scheme List", SupplySide: HVACLoopSide{Branches: []HVACBranch{{Components: []HVACComponent{hvacOperationTestComponent(chiller, "In", "Out"), hvacOperationTestComponent(other, "Other In", "Other Out")}}}}}
	profiles := buildHVACOperatingProfiles(newHVACContext(doc), []HVACLoop{loop}, nil)
	profile := hvacOperationTestProfile(t, profiles, 0, loop.ID)
	if !hvacConfiguredTestHasReason(profile.FinalAvailability, "runtime_dispatch") || hvacConfiguredTestHasReason(profile.ConfiguredAvailability, "runtime_dispatch") {
		t.Fatal("load-range dispatch was not separated from authored scheme activation")
	}
	foundActivation := false
	for _, layer := range profile.ConfiguredAvailabilityLayers {
		if layer.Role == "equipment" && (layer.Expression.Op != "always" || layer.Expression.Source == nil || layer.Expression.Source.ObjectIndex != 0) {
			t.Fatalf("ReformulatedEIR got an invented own availability schedule: %#v", layer)
		}
		hvacConfiguredTestWalk(layer.Expression, func(item HVACOperatingExpression) {
			if item.ScheduleName == "ALWAYS_ON" {
				foundActivation = true
				if item.Source == nil || item.Source.ObjectIndex != 2 {
					t.Fatalf("plant selector schedule was attributed to chiller or load-range object: %#v", item)
				}
			}
		})
	}
	if !foundActivation || !hvacGraphTestHasEdge(profile.AvailabilityDefinitions, "definition:2", "schedule:5", "declares_schedule", "", "") {
		t.Fatal("actual plant activation declaration is missing")
	}
	hvacGraphTestNode(t, profile.AvailabilityDefinitions, "definition:0:schedule")
	if !hvacConfiguredTestHasReason(hvacOperationTestProfile(t, profiles, 6, loop.ID).ConfiguredAvailability, "not_in_operation_scheme") {
		t.Fatal("known static nonmembership was discarded as a runtime outcome")
	}
	// Required missing operation scheme remains a real configuration error;
	// the chiller's own default row stays unrestricted independently.
	loop.OperationScheme = ""
	profile = hvacOperationTestProfile(t, buildHVACOperatingProfiles(newHVACContext(doc), []HVACLoop{loop}, nil), 0, loop.ID)
	if !hvacConfiguredTestHasReason(profile.ConfiguredAvailability, "missing_operation_schemes") {
		t.Fatal("missing required plant operation declaration became unrestricted")
	}
	if hvacConfiguredTestHasReason(profile.FinalAvailability, "missing_operation_schemes") {
		t.Fatal("additive configured validation unexpectedly rewrote the legacy wire")
	}
	// Exact raw schedule field evidence is kept on the declaring owner.
	owner := hvacGraphTestNode(t, profiles[0].AvailabilityDefinitions, "definition:2")
	if !strings.Contains(owner.Parameters[2].Name, "Schedule") || owner.Parameters[2].FieldIndex != 3 {
		t.Fatalf("plant selector field identity lost: %#v", owner)
	}
}
