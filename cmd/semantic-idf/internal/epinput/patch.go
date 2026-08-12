package epinput

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

func PatchFieldValue(model *Model, objectIndex int, fieldIndex int, path []string, rawValue string) error {
	if model == nil {
		return fmt.Errorf("model is nil")
	}

	object, err := findObjectBySourceIndex(model, objectIndex)
	if err != nil {
		return err
	}
	if fieldIndex < 0 || fieldIndex >= len(object.Fields) {
		return fmt.Errorf("field index %d out of range", fieldIndex)
	}

	nextValue, err := parseJSONLiteral(rawValue)
	if err != nil {
		return err
	}

	patched, err := patchJSONValue(object.Fields[fieldIndex].Value, path, nextValue)
	if err != nil {
		return err
	}
	object.Fields[fieldIndex].Value = patched
	model.Version = DetectVersion(model.Objects)
	return EnsureSupportedVersion(model)
}

func findObjectBySourceIndex(model *Model, objectIndex int) (*InputObject, error) {
	for i := range model.Objects {
		if model.Objects[i].SourceIndex == objectIndex {
			return &model.Objects[i], nil
		}
	}
	if objectIndex >= 0 && objectIndex < len(model.Objects) {
		return &model.Objects[objectIndex], nil
	}
	return nil, fmt.Errorf("object index %d out of range", objectIndex)
}

func parseJSONLiteral(rawValue string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(rawValue)))
	decoder.UseNumber()
	value, err := parseOrderedJSONValue(decoder)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON value: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("invalid JSON value: %w", err)
	}
	return value, nil
}

func patchJSONValue(current any, path []string, next any) (any, error) {
	if len(path) == 0 {
		return next, nil
	}

	head := path[0]
	tail := path[1:]
	switch value := current.(type) {
	case orderedObject:
		for i := range value {
			if value[i].Key != head {
				continue
			}
			patched, err := patchJSONValue(value[i].Value, tail, next)
			if err != nil {
				return nil, err
			}
			value[i].Value = patched
			return value, nil
		}
		return nil, fmt.Errorf("path key %q not found", head)
	case map[string]any:
		child, ok := value[head]
		if !ok {
			return nil, fmt.Errorf("path key %q not found", head)
		}
		patched, err := patchJSONValue(child, tail, next)
		if err != nil {
			return nil, err
		}
		value[head] = patched
		return value, nil
	case []any:
		index, err := strconv.Atoi(head)
		if err != nil || index < 0 || index >= len(value) {
			return nil, fmt.Errorf("array index %q out of range", head)
		}
		patched, err := patchJSONValue(value[index], tail, next)
		if err != nil {
			return nil, err
		}
		value[index] = patched
		return value, nil
	default:
		return nil, fmt.Errorf("cannot patch path %q inside scalar value", head)
	}
}
