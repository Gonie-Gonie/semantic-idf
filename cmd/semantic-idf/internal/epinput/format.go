package epinput

import (
	"path/filepath"
	"strings"
)

type Format string

const (
	FormatUnknown Format = "unknown"
	FormatIDF     Format = "idf"
	FormatEPJSON  Format = "epjson"
)

func DetectFormat(filename string, content []byte) Format {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".idf", ".imf":
		return FormatIDF
	case ".epjson", ".json":
		return FormatEPJSON
	}

	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return FormatUnknown
	}
	if strings.HasPrefix(trimmed, "{") {
		return FormatEPJSON
	}
	return FormatIDF
}

func Parse(filename string, content []byte) (*Model, error) {
	format := DetectFormat(filename, content)
	switch format {
	case FormatEPJSON:
		return ParseEPJSON(content)
	case FormatIDF:
		return ParseIDF(string(content))
	default:
		return &Model{Format: FormatUnknown}, nil
	}
}

func Write(model *Model, target Format) (string, error) {
	target = NormalizeFormat(target)
	switch target {
	case FormatEPJSON:
		return WriteEPJSON(model)
	case FormatIDF:
		return WriteIDF(model), nil
	default:
		return "", ErrUnsupportedFormat{Format: target}
	}
}

func NormalizeFormat(format Format) Format {
	switch Format(strings.ToLower(string(format))) {
	case "idf":
		return FormatIDF
	case "epjson", "json":
		return FormatEPJSON
	default:
		return format
	}
}

type ErrUnsupportedFormat struct {
	Format Format
}

func (e ErrUnsupportedFormat) Error() string {
	return "unsupported EnergyPlus input format: " + string(e.Format)
}
