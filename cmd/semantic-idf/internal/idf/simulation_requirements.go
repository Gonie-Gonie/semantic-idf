package idf

import "strings"

func InputRequiresWeatherFile(doc Document) bool {
	hasRunPeriod := false
	weatherRunPeriods := true
	for _, obj := range doc.Objects {
		switch strings.ToLower(strings.TrimSpace(obj.Type)) {
		case "sizingperiod:weatherfiledays", "sizingperiod:weatherfileconditiontype", "sizingperiod:weatherfiledesignday":
			return true
		case "runperiod":
			hasRunPeriod = true
		case "simulationcontrol":
			if value, ok := simulationControlWeatherRunPeriodValue(obj); ok {
				weatherRunPeriods = yesLike(value)
			}
		}
	}
	return hasRunPeriod && weatherRunPeriods
}

func simulationControlWeatherRunPeriodValue(obj Object) (string, bool) {
	if value := findFieldByCommentWords(obj, "weather", "file", "run", "period"); value != "" {
		return value, true
	}
	if len(obj.Fields) >= 5 {
		return strings.TrimSpace(obj.Fields[4].Value), true
	}
	return "", false
}

func yesLike(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "" || normalized == "yes" || normalized == "true" || normalized == "1"
}
