// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original bounded owned edit and canonical semantic encoding.

package activity

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"

	dto "github.com/florianfinn/docklet-next/types/container"
)

func runtimeRecreateOperation(operation RuntimeOperation) bool {
	return operation == RuntimeRecreateContainer || operation == RuntimeRedeployContainer
}
func runtimeOwnedEdit(edit *dto.Edit) (*dto.Edit, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(edit)
	body := bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
	if err != nil || len(body) > 64<<10 {
		return nil, errRuntimeInvalidInternal
	}
	var owned dto.Edit
	if err := json.Unmarshal(body, &owned); err != nil {
		return nil, errRuntimeInvalidInternal
	}
	return &owned, nil
}

// Pointer sections are explicit null, present-empty values stay empty and
// array order survives. encoding/json sorts map keys when hashing this tree.
func runtimeRecreateSemantic(edit dto.Edit) any { return runtimeSemanticValue(reflect.ValueOf(edit)) }
func runtimeSemanticValue(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return runtimeSemanticValue(v.Elem())
	case reflect.Struct:
		result := map[string]any{}
		typ := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			result[name] = runtimeSemanticValue(v.Field(i))
		}
		return result
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		result := map[string]any{}
		iter := v.MapRange()
		for iter.Next() {
			result[iter.Key().String()] = runtimeSemanticValue(iter.Value())
		}
		return result
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		result := make([]any, v.Len())
		for i := range result {
			result[i] = runtimeSemanticValue(v.Index(i))
		}
		return result
	default:
		return v.Interface()
	}
}

func runtimeRecreateFields(options RuntimeOptions) []string {
	fields := []string{}
	if options.Recreate == nil {
		return fields
	}
	value := reflect.ValueOf(*options.Recreate)
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if field.IsZero() {
			continue
		}
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name != "-" && name != "" {
			fields = append(fields, name)
		}
	}
	return fields
}

func runtimeWantsAsync(prefer string) bool {
	for _, value := range strings.Split(prefer, ",") {
		if strings.EqualFold(strings.TrimSpace(value), "respond-async") {
			return true
		}
	}
	return false
}

func runtimeRecreateWireBound(body []byte) bool {
	var raw struct {
		Options struct {
			Recreate json.RawMessage `json:"recreate"`
		} `json:"options"`
	}
	return json.Unmarshal(body, &raw) == nil && len(raw.Options.Recreate) <= 64<<10
}
