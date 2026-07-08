package idf

import "strings"

const (
	outputPresetStandard      = "standard"
	outputPresetModeMerge     = "merge"
	outputPresetModeReplace   = "replace"
	standardOutputTag         = "standard"
	standardOutputFrequency   = "Monthly"
	standardHeatFlowFrequency = "Hourly"
)

type standardOutputFeatures struct {
	hasElectricity       bool
	hasNaturalGas        bool
	hasDistrictCooling   bool
	hasDistrictHeating   bool
	hasFuelOilNo1        bool
	hasFuelOilNo2        bool
	hasPropane           bool
	hasOtherFuel1        bool
	hasOtherFuel2        bool
	hasSteam             bool
	hasLights            bool
	hasElectricEquipment bool
	hasGasEquipment      bool
	hasOnsiteGeneration  bool
	hasCooling           bool
	hasHeating           bool
	hasFans              bool
	hasPumps             bool
	hasHeatRejection     bool
	hasHeatRecovery      bool
	hasWaterSystems      bool
	hasExteriorLights    bool
	hasRefrigeration     bool
}

// StandardOutputApplyRequest builds the preset used by simulation runs. Replace mode
// keeps matching standard outputs and removes non-standard output requests.
func StandardOutputApplyRequest(doc Document, mode string) OutputApplyRequest {
	return OutputApplyRequest{
		Preset:     outputPresetStandard,
		PresetMode: mode,
	}
}

func StandardOutputRecommendationIDs(doc Document) []string {
	report := AnalyzeOutput(doc)
	ids := make([]string, 0, len(report.Recommendations))
	for _, item := range report.Recommendations {
		if outputRecommendationHasTag(item, standardOutputTag) {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func expandOutputPresetRequest(doc Document, request OutputApplyRequest) OutputApplyRequest {
	if !strings.EqualFold(strings.TrimSpace(request.Preset), outputPresetStandard) {
		return request
	}

	selectedIDs := normalizeOutputRecommendationIDs(request.PresetRecommendationIDs)
	if request.PresetRecommendationIDs == nil {
		selectedIDs = StandardOutputRecommendationIDs(doc)
	}
	request.AddRecommendations = append(selectedIDs, request.AddRecommendations...)
	if strings.EqualFold(strings.TrimSpace(request.PresetMode), outputPresetModeReplace) {
		request.RemoveObjectIndexes = append(request.RemoveObjectIndexes, standardOutputRemoveIndexes(doc, selectedIDs)...)
	}
	request.Preset = ""
	request.PresetMode = ""
	request.PresetRecommendationIDs = nil
	return request
}

func standardOutputRemoveIndexes(doc Document, selectedIDs []string) []int {
	if len(selectedIDs) == 0 {
		return nil
	}
	report := AnalyzeOutput(doc)
	recommendations := map[string]OutputRecommendation{}
	for _, item := range report.Recommendations {
		recommendations[item.ID] = item
	}
	standardSignatures := map[string]bool{}
	for _, id := range selectedIDs {
		if item, ok := recommendations[id]; ok {
			standardSignatures[outputObjectSignature(item.ObjectType, item.Fields)] = true
		}
	}
	keptStandardSignatures := map[string]bool{}
	var indexes []int
	for _, obj := range doc.Objects {
		if !isOutputManagementType(obj.Type) {
			continue
		}
		signature := outputObjectSignature(obj.Type, outputFieldValues(obj))
		if !standardSignatures[signature] {
			indexes = append(indexes, obj.Index)
			continue
		}
		if keptStandardSignatures[signature] {
			indexes = append(indexes, obj.Index)
			continue
		}
		keptStandardSignatures[signature] = true
	}
	return indexes
}

func normalizeOutputRecommendationIDs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func outputRecommendationHasTag(item OutputRecommendation, tag string) bool {
	for _, candidate := range item.Tags {
		if strings.EqualFold(strings.TrimSpace(candidate), tag) {
			return true
		}
	}
	return false
}

func detectOutputFeatures(doc Document) standardOutputFeatures {
	var features standardOutputFeatures
	for _, obj := range doc.Objects {
		objectType := strings.ToLower(strings.TrimSpace(obj.Type))
		switch {
		case strings.EqualFold(obj.Type, "Lights"):
			features.hasLights = true
			features.hasElectricity = true
		case strings.EqualFold(obj.Type, "ElectricEquipment"):
			features.hasElectricEquipment = true
			features.hasElectricity = true
		case strings.EqualFold(obj.Type, "GasEquipment"):
			features.hasGasEquipment = true
			features.hasNaturalGas = true
		case strings.EqualFold(obj.Type, "Exterior:Lights"):
			features.hasExteriorLights = true
			features.hasElectricity = true
		case strings.HasPrefix(objectType, "fan:"):
			features.hasFans = true
			features.hasElectricity = true
		case strings.HasPrefix(objectType, "pump:"):
			features.hasPumps = true
			features.hasElectricity = true
		case strings.Contains(objectType, "districtcooling"):
			features.hasDistrictCooling = true
		case strings.Contains(objectType, "districtheating"):
			features.hasDistrictHeating = true
		case strings.Contains(objectType, "cooling"):
			features.hasCooling = true
			features.hasElectricity = true
		case strings.Contains(objectType, "heating"):
			features.hasHeating = true
		case strings.Contains(objectType, "heatrejection") || strings.Contains(objectType, "coolingtower"):
			features.hasHeatRejection = true
			features.hasElectricity = true
		case strings.Contains(objectType, "heatrecovery") || strings.Contains(objectType, "heatexchanger"):
			features.hasHeatRecovery = true
			features.hasElectricity = true
		case strings.Contains(objectType, "refrigeration"):
			features.hasRefrigeration = true
			features.hasElectricity = true
		case strings.HasPrefix(objectType, "generator:") || strings.HasPrefix(objectType, "electricloadcenter:"):
			features.hasOnsiteGeneration = true
			features.hasElectricity = true
		case strings.HasPrefix(objectType, "wateruse:"):
			features.hasWaterSystems = true
		}
		for _, field := range obj.Fields {
			value := strings.ToLower(strings.TrimSpace(field.Value))
			switch value {
			case "electricity":
				features.hasElectricity = true
			case "naturalgas":
				features.hasNaturalGas = true
			case "districtcooling":
				features.hasDistrictCooling = true
			case "districtheating":
				features.hasDistrictHeating = true
			case "fueloilno1":
				features.hasFuelOilNo1 = true
			case "fueloilno2":
				features.hasFuelOilNo2 = true
			case "propane":
				features.hasPropane = true
			case "otherfuel1":
				features.hasOtherFuel1 = true
			case "otherfuel2":
				features.hasOtherFuel2 = true
			case "steam":
				features.hasSteam = true
			}
		}
	}
	return features
}

func standardOutputRecommendationApplies(item OutputRecommendation, features standardOutputFeatures) bool {
	key := strings.ToLower(outputFieldValue(item.Fields, "Key Name", "Key Value"))
	variable := strings.ToLower(outputFieldValue(item.Fields, "Variable Name"))
	switch {
	case strings.Contains(key, "electricity:facility"):
		return features.hasElectricity
	case strings.Contains(key, "naturalgas:facility"):
		return features.hasNaturalGas
	case strings.Contains(key, "districtcooling:facility"):
		return features.hasDistrictCooling
	case strings.Contains(key, "districtheating:facility"):
		return features.hasDistrictHeating
	case strings.Contains(key, "fueloilno1:facility"):
		return features.hasFuelOilNo1
	case strings.Contains(key, "fueloilno2:facility"):
		return features.hasFuelOilNo2
	case strings.Contains(key, "propane:facility"):
		return features.hasPropane
	case strings.Contains(key, "otherfuel1:facility"):
		return features.hasOtherFuel1
	case strings.Contains(key, "otherfuel2:facility"):
		return features.hasOtherFuel2
	case strings.Contains(key, "steam:facility"):
		return features.hasSteam
	case strings.Contains(key, "water:facility"):
		return features.hasWaterSystems
	case strings.Contains(key, "electricity:cooling"):
		return features.hasCooling
	case strings.Contains(key, "electricity:heating"):
		return features.hasHeating && features.hasElectricity
	case strings.Contains(key, "electricity:interiorlights"):
		return features.hasLights
	case strings.Contains(key, "electricity:interiorequipment"):
		return features.hasElectricEquipment
	case strings.Contains(key, "electricity:fans"):
		return features.hasFans
	case strings.Contains(key, "electricity:pumps"):
		return features.hasPumps
	case strings.Contains(key, "electricity:heatrejection"):
		return features.hasHeatRejection
	case strings.Contains(key, "electricity:heatrecovery"):
		return features.hasHeatRecovery
	case strings.Contains(key, "electricity:watersystems"):
		return features.hasWaterSystems && features.hasElectricity
	case strings.Contains(key, "electricity:exteriorlights"):
		return features.hasExteriorLights
	case strings.Contains(key, "electricity:refrigeration"):
		return features.hasRefrigeration
	case strings.Contains(key, "electricityproduced:facility"):
		return features.hasOnsiteGeneration
	case strings.Contains(key, "districtcooling:cooling"):
		return features.hasDistrictCooling
	case strings.Contains(key, "districtheating:heating"):
		return features.hasDistrictHeating
	case strings.Contains(key, "naturalgas:heating"):
		return features.hasHeating && features.hasNaturalGas
	case strings.Contains(key, "naturalgas:watersystems"):
		return features.hasWaterSystems && features.hasNaturalGas
	case strings.Contains(key, "naturalgas:interiorequipment"):
		return features.hasGasEquipment
	case strings.Contains(variable, "zone lights electricity"):
		return features.hasLights
	case strings.Contains(variable, "zone electric equipment"):
		return features.hasElectricEquipment
	case strings.Contains(variable, "zone gas equipment"):
		return features.hasGasEquipment
	case strings.Contains(variable, "zone air system sensible heating"):
		return false
	case strings.Contains(variable, "zone air system sensible cooling"):
		return false
	default:
		return true
	}
}
