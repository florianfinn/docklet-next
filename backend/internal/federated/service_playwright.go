// Source: getarcaneapp/arcane/backend/internal/federated/service_playwright.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

//go:build playwright

package federated

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/user"
)

// CreatePlaywrightCredential creates the service identity, credential, and role
// assignment required by the federated-auth end-to-end flow in one transaction.
func (s *FederatedCredentialService) CreatePlaywrightCredential(ctx context.Context, issuerURL string, audiences []string, subject, roleID string, tokenTTLSeconds int) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("federated credential service is not available")
	}
	if strings.TrimSpace(issuerURL) == "" || strings.TrimSpace(subject) == "" || strings.TrimSpace(roleID) == "" || len(audiences) == 0 {
		return "", errors.New("issuerUrl, subject, roleId, and audiences are required")
	}
	if tokenTTLSeconds <= 0 {
		tokenTTLSeconds = 600
	}

	var credentialID string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		serviceUser := user.User{
			Username:         "svc_federated_e2e_" + strings.ReplaceAll(uuid.New().String(), "-", ""),
			IsServiceAccount: true,
		}
		if err := tx.Create(&serviceUser).Error; err != nil {
			return fmt.Errorf("failed to create federated e2e user: %w", err)
		}

		credential := FederatedCredential{
			Name:            "Playwright Federated Credential",
			Enabled:         true,
			IssuerURL:       strings.TrimRight(strings.TrimSpace(issuerURL), "/"),
			Audiences:       database.StringSlice(audiences),
			SubjectClaim:    "sub",
			SubjectMatch:    strings.TrimSpace(subject),
			MatchType:       FederatedCredentialMatchExact,
			RoleID:          strings.TrimSpace(roleID),
			IdentityUserID:  serviceUser.ID,
			TokenTTLSeconds: tokenTTLSeconds,
		}
		if err := tx.Create(&credential).Error; err != nil {
			return fmt.Errorf("failed to create federated e2e credential: %w", err)
		}

		assignment := role.UserRoleAssignment{
			UserID: serviceUser.ID,
			RoleID: credential.RoleID,
			Source: role.RoleAssignmentSourceManual,
		}
		if err := tx.Create(&assignment).Error; err != nil {
			return fmt.Errorf("failed to create federated e2e role assignment: %w", err)
		}

		credentialID = credential.ID
		return nil
	})
	if err != nil {
		return "", err
	}
	return credentialID, nil
}
