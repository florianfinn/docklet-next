// Source: docklet-next/docs/design/update-policies.md
// Revision: 8907719aba6cd2d3037d287729f689d5ad9e3ec9
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Reject undeclared durable reason codes at the persistence boundary.

package policies

import (
	"github.com/florianfinn/docklet-next/types/image"
	"gorm.io/gorm"
)

// Gorm invokes these hooks for struct saves and map updates alike. Validating
// here also covers status writes that bypass the service's normal helpers.
func (p *Policy) BeforeSave(tx *gorm.DB) error {
	return validateReasons(tx, p.Code, p.ApplyCode, p.BookkeepingCode)
}
func (c *Child) BeforeSave(tx *gorm.DB) error { return validateReasons(tx, c.Code) }

func validateReasons(tx *gorm.DB, codes ...string) error {
	// Updates can use a zero-valued Model and a distinct struct destination.
	switch value := tx.Statement.Dest.(type) {
	case Policy:
		codes = append(codes, value.Code, value.ApplyCode, value.BookkeepingCode)
	case *Policy:
		codes = append(codes, value.Code, value.ApplyCode, value.BookkeepingCode)
	case Child:
		codes = append(codes, value.Code)
	case *Child:
		codes = append(codes, value.Code)
	}

	for _, code := range codes {
		if err := image.ValidateReason(code); err != nil {
			return err
		}
	}
	if updates, ok := tx.Statement.Dest.(map[string]interface{}); ok {
		for name, value := range updates {
			field := tx.Statement.Schema.LookUpField(name)
			if field == nil || (field.DBName != "code" && field.DBName != "apply_code" && field.DBName != "bookkeeping_code") {
				continue
			}
			code, ok := value.(string)
			if !ok {
				return image.ErrUndeclaredReason
			}
			if err := image.ValidateReason(code); err != nil {
				return err
			}
		}
	}
	return nil
}
