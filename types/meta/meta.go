// Source: getarcaneapp/arcane/types/meta/meta.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package meta

// Metadata represents metadata about a configuration or setting.
type Metadata struct {
	// Key is the identifier for the metadata.
	//
	// Required: true
	Key string `json:"key"`

	// Label is the human-readable label for the metadata.
	//
	// Required: true
	Label string `json:"label"`

	// Type is the data type of the metadata value.
	//
	// Required: true
	Type string `json:"type"`

	// Keywords is a list of keywords associated with the metadata.
	//
	// Required: false
	Keywords []string `json:"keywords,omitempty"`

	// Description provides additional information about the metadata.
	//
	// Required: false
	Description string `json:"description,omitempty"`
}
