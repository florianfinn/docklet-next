// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package middleware

import (
	"github.com/florianfinn/docklet-next/types/volume"
	"reflect"
	"strings"
)

// Validate every path and body identity after decoding, before a handler has effects.
func validateVolumeEntry(route string, input any) error {
	if !strings.Contains(route, "/volumes") && !strings.Contains(route, "/volume-backups") {
		return nil
	}
	return validateVolumeFields(reflect.ValueOf(input), false)
}
func validateVolumeFields(v reflect.Value, body bool) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return validateVolumeFields(v.Elem(), body)
	}
	if v.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			if err := validateVolumeFields(v.Index(i), body); err != nil {
				return err
			}
		}
		return nil
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		field := v.Field(i)
		if field.Kind() == reflect.String && (name == "VolumeName" || name == "NewVolumeName" || name == "Key" || body && name == "Name") {
			if err := volume.ValidateName(field.String()); err != nil {
				return err
			}
		}
		if name == "Body" || name == "Candidates" {
			if err := validateVolumeFields(field, true); err != nil {
				return err
			}
		}
	}
	return nil
}
