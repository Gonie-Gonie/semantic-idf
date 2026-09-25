package idf

import (
	"fmt"
	"strings"
	"testing"
)

func TestNativeHeatOnlyWholeCohortDoesNotEraseTrueSiblingPaths(t *testing.T) {
	for missing := 0; missing <= 3; missing++ {
		t.Run(fmt.Sprintf("missing_%d", missing), func(t *testing.T) {
			doc := hvacHeatOnlyOriginal(t, true)
			for _, name := range []string{"Zone1DirectAir", "Zone3DirectAir", "Zone2DirectAir"}[:missing] {
				hvacSmallOfficeObject(t, &doc, "AirTerminal:SingleDuct:ConstantVolume:NoReheat", name).Fields[2].Value = "Disconnected " + name
			}
			want := hvacHeatOnlyAllPaths()
			for _, zone := range []string{"west zone", "north zone", "east zone"}[:missing] {
				want[zone] = 0
			}
			report := hvacHeatOnlyPaths(t, doc, want)
			cohorts := ResolveNativeHeatOnlyFurnaceCohorts(doc, report.ServiceModel.ZoneServices)
			if len(cohorts) != 1 || cohorts[0].Complete != (missing == 0) || cohorts[0].AirLoop == nil || cohorts[0].AirLoop.Name != "Typical Terminal Reheat 1" || cohorts[0].System.ObjectIndex != 92 {
				t.Fatalf("whole original cohort lost exact owner or borrowed surviving subset: %+v", cohorts)
			}
		})
	}
}

func TestNativeHeatOnlyWholeCohortRejectsReturnAndBorrowedOwnership(t *testing.T) {
	for _, mutation := range []string{"return-node", "extra-splitter-outlet", "removed-splitter-outlet", "shared-loop", "unrelated-wrapper-path", "foreign-loop-path"} {
		t.Run(mutation, func(t *testing.T) {
			doc := hvacHeatOnlyOriginal(t, true)
			switch mutation {
			case "return-node":
				hvacSmallOfficeObject(t, &doc, "ZoneHVAC:EquipmentConnections", "West Zone").Fields[5].Value = "Foreign return"
			case "extra-splitter-outlet":
				splitter := hvacSmallOfficeObject(t, &doc, "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter")
				splitter.Fields = append(splitter.Fields, Field{Value: "Unproved fourth recipient"})
			case "removed-splitter-outlet":
				splitter := hvacSmallOfficeObject(t, &doc, "AirLoopHVAC:ZoneSplitter", "Zone Supply Air Splitter")
				splitter.Fields = splitter.Fields[:len(splitter.Fields)-1]
			case "shared-loop":
				loop := *hvacSmallOfficeObject(t, &doc, "AirLoopHVAC", "Typical Terminal Reheat 1")
				loop.Fields = append([]Field(nil), loop.Fields...)
				loop.Index, loop.Fields[0].Value = len(doc.Objects), "Foreign shared loop"
				doc.Objects = append(doc.Objects, loop)
			}
			report := AnalyzeHVAC(doc)
			changed := false
			if strings.HasSuffix(mutation, "-path") {
				for si := range report.ServiceModel.ZoneServices {
					for pi := range report.ServiceModel.ZoneServices[si].Paths {
						path := &report.ServiceModel.ZoneServices[si].Paths[pi]
						if path.ServiceKind != "heating" || !strings.EqualFold(path.ZoneName, "West Zone") {
							continue
						}
						if mutation == "unrelated-wrapper-path" {
							path.Conditioning = nil // Another heater's path is not this Furnace's recipient proof.
						} else {
							copy := *path.AirLoop
							copy.Name, copy.ObjectIndex = "Foreign loop", 99999
							path.AirLoop = &copy
						}
						changed = true
					}
				}
				if !changed {
					t.Fatal("literal ownership mutation missed its path")
				}
			}
			cohorts := ResolveNativeHeatOnlyFurnaceCohorts(doc, report.ServiceModel.ZoneServices)
			if len(cohorts) != 1 || cohorts[0].Complete {
				t.Fatalf("unproved whole cohort accepted: %s %+v", mutation, cohorts)
			}
		})
	}
}
