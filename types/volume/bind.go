// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"encoding/hex"
	"github.com/florianfinn/docklet-next/types/backup"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

func IsBindKey(name string) bool   { return strings.HasPrefix(name, "bind:") }
func BindKey(source string) string { return "bind:" + hex.EncodeToString([]byte(source)) }

// BindSource validates the literal daemon-host identity without opening host paths.
func BindSource(key string) (string, error) {
	encoded, ok := strings.CutPrefix(key, "bind:")
	if !ok || encoded != strings.ToLower(encoded) || len(encoded) > 510 {
		return "", backup.NewProblem(400, "volume.bind.invalid-key", nil)
	}
	decoded, err := hex.DecodeString(encoded)
	source := string(decoded)
	if err != nil || !utf8.Valid(decoded) || strings.ContainsRune(source, 0) || !path.IsAbs(source) || source == "/" || path.Clean(source) != source || len(decoded) > 255 {
		return "", backup.NewProblem(400, "volume.bind.invalid-key", nil)
	}
	return source, nil
}
func ValidateName(name string) error {
	if IsBindKey(name) {
		_, err := BindSource(name)
		return err
	}
	return nil
}
func RefuseBind(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if IsBindKey(name) {
		return backup.NewProblem(409, "volume.bind.unsupported", nil)
	}
	return nil
}
func DisplayName(name string) string {
	if IsBindKey(name) {
		if source, err := BindSource(name); err == nil {
			return source
		}
	}
	return name
}

type BindConsumer struct {
	ContainerID string `json:"containerId"`
	Name        string `json:"name"`
	Destination string `json:"destination"`
	ReadOnly    bool   `json:"readOnly"`
	Running     bool   `json:"running"`
}
type Bind struct {
	Key              string         `json:"key"`
	Source           string         `json:"source"`
	Kind             string         `json:"kind" enum:"directory,file,unknown"`
	Eligible         bool           `json:"eligible"`
	Reason           string         `json:"reason,omitempty"`
	Consumers        []BindConsumer `json:"consumers"`
	LastSnapshotSize *int64         `json:"lastSnapshotSize,omitempty"`
	LastSnapshotAt   *time.Time     `json:"lastSnapshotAt,omitempty"`
	MeasuredSize     *int64         `json:"measuredSize,omitempty"`
	MeasuredAt       *time.Time     `json:"measuredAt,omitempty"`
	MeasuredPartial  *bool          `json:"measuredPartial,omitempty"`
}
type BindMeasurement struct {
	Key             string    `json:"key"`
	MeasuredSize    int64     `json:"measuredSize"`
	MeasuredAt      time.Time `json:"measuredAt"`
	MeasuredPartial bool      `json:"measuredPartial"`
}
