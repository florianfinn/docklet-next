// Source: getarcaneapp/arcane/backend/internal/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations.

package keyformat

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var RecoveryKeyFormat = regexp.MustCompile(`^[A-Z2-7]{6}(-[A-Z2-7]{6}){7}$`)

func ValidateRecoveryKey(key string) error {
	if !RecoveryKeyFormat.MatchString(strings.TrimSpace(key)) {
		return errors.New("enter the generated recovery key (8 groups of 6 characters)")
	}
	return nil
}

func GenerateRecoveryKey() (string, error) {
	raw := make([]byte, 30)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("failed to generate recovery key: %w", err)
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	groups := make([]string, 0, len(encoded)/6)
	for i := 0; i < len(encoded); i += 6 {
		groups = append(groups, encoded[i:i+6])
	}
	return strings.Join(groups, "-"), nil
}
