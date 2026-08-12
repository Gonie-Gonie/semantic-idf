package epinput

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const MinSupportedMajor = 22

var versionPattern = regexp.MustCompile(`^(\d+)(?:\.(\d+))?`)

type Model struct {
	Format  Format        `json:"format"`
	Version VersionInfo   `json:"version"`
	Objects []InputObject `json:"objects"`
}

type VersionInfo struct {
	Raw       string `json:"raw,omitempty"`
	Major     int    `json:"major,omitempty"`
	Minor     int    `json:"minor,omitempty"`
	Known     bool   `json:"known"`
	Supported bool   `json:"supported"`
}

type InputObject struct {
	Type        string         `json:"type"`
	Name        string         `json:"name,omitempty"`
	NameSource  string         `json:"nameSource,omitempty"`
	Fields      []Field        `json:"fields"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	SourceIndex int            `json:"sourceIndex"`
}

type Field struct {
	Key     string `json:"key"`
	Value   any    `json:"value"`
	Comment string `json:"comment,omitempty"`
}

func ParseVersion(raw string) VersionInfo {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return VersionInfo{}
	}

	info := VersionInfo{Raw: raw}
	matches := versionPattern.FindStringSubmatch(raw)
	if matches == nil {
		return info
	}

	major, _ := strconv.Atoi(matches[1])
	minor := 0
	if matches[2] != "" {
		minor, _ = strconv.Atoi(matches[2])
	}

	info.Major = major
	info.Minor = minor
	info.Known = true
	info.Supported = major >= MinSupportedMajor
	return info
}

func DetectVersion(objects []InputObject) VersionInfo {
	for _, object := range objects {
		if !strings.EqualFold(object.Type, "Version") {
			continue
		}
		for _, field := range object.Fields {
			if field.Key == "version_identifier" || strings.EqualFold(field.Comment, "Version Identifier") || field.Key == "field_1" {
				return ParseVersion(valueToString(field.Value))
			}
		}
	}
	return VersionInfo{}
}

func EnsureSupportedVersion(model *Model) error {
	if model == nil || !model.Version.Known {
		return nil
	}
	if !model.Version.Supported {
		return fmt.Errorf("EnergyPlus version %s is below the supported range; version 22 or newer is required", model.Version.Raw)
	}
	return nil
}

func valueToString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

const (
	NameSourceExplicitField    = "explicit_name_field"
	NameSourceSyntheticDisplay = "synthetic_display_name"
	NameSourceEPJSONInstance   = "epjson_instance_name"
)

func objectInstanceName(objectType string, fields []Field, fallbackIndex int) (name string, remaining []Field, source string) {
	if isNamelessObjectType(objectType) || len(fields) == 0 {
		return fmt.Sprintf("%s %d", objectType, fallbackIndex+1), fields, NameSourceSyntheticDisplay
	}

	first := fields[0]
	if first.Comment != "" && !strings.EqualFold(first.Comment, "Name") {
		return fmt.Sprintf("%s %d", objectType, fallbackIndex+1), fields, NameSourceSyntheticDisplay
	}
	if first.Key != "" && first.Key != "name" && first.Key != "field_1" {
		return fmt.Sprintf("%s %d", objectType, fallbackIndex+1), fields, NameSourceSyntheticDisplay
	}

	name = strings.TrimSpace(valueToString(first.Value))
	return name, fields[1:], NameSourceExplicitField
}

func isNamelessObjectType(objectType string) bool {
	switch strings.ToLower(objectType) {
	case "version",
		"simulationcontrol",
		"timestep",
		"runperiod",
		"globalgeometryrules",
		"shadowcalculation",
		"heatbalancealgorithm",
		"surfaceconvectionalgorithm:inside",
		"surfaceconvectionalgorithm:outside",
		"zoneairheatbalancealgorithm",
		"zoneaircontaminantbalance":
		return true
	default:
		return strings.HasPrefix(strings.ToLower(objectType), "output:") ||
			strings.HasPrefix(strings.ToLower(objectType), "outputcontrol:") ||
			strings.HasPrefix(strings.ToLower(objectType), "meter:")
	}
}

func fieldKey(comment string, index int, used map[string]int) string {
	key := normalizeFieldKey(comment)
	if key == "" {
		key = fmt.Sprintf("field_%d", index+1)
	}

	used[key]++
	if used[key] == 1 {
		return key
	}
	return fmt.Sprintf("%s_%d", key, used[key])
}

func normalizeFieldKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if cut := strings.Index(value, "{"); cut >= 0 {
		value = strings.TrimSpace(value[:cut])
	}

	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func keyToComment(key string) string {
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		switch strings.ToLower(part) {
		case "idf":
			parts[i] = "IDF"
		case "hvac":
			parts[i] = "HVAC"
		default:
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}
