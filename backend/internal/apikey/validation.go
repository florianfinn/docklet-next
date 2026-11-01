// Source: getarcaneapp/arcane/backend/internal/apikey/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M15 simple-user rights and identity contracts.

package apikey

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"strings"
	"time"
)

func (s *ApiKeyService) ValidateApiKey(ctx context.Context, rawKey string) (*user.User, error) {
	localUser, _, err := s.ValidateApiKeyWithID(ctx, rawKey)
	return localUser, err
}

// ValidateApiKeyWithID is like ValidateApiKey but additionally returns the
// API key record so callers can resolve permissions according to its kind.
func (s *ApiKeyService) ValidateApiKeyWithID(ctx context.Context, rawKey string) (*user.User, *ApiKey, error) {
	apiKey, err := s.validateRawAPIKeyInternal(ctx, rawKey)
	if err != nil {
		return nil, nil, err
	}

	if apiKey.UserID == nil {
		return nil, nil, ErrApiKeyInvalid
	}

	localUser, err := s.userService.GetUserByID(ctx, *apiKey.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user for API key: %w", err)
	}

	if isStaticAPIKeyInternal(*apiKey) {
		return localUser, apiKey, nil
	}
	if localUser.DisabledAt != nil {
		return nil, nil, ErrApiKeyInvalid
	}
	if apiKey.Kind != ApiKeyKindPersonal {
		ps, err := s.userService.ResolveUserPermissions(ctx, localUser.ID)
		if err != nil || ps == nil || !ps.IsGlobalAdmin() {
			return nil, nil, ErrApiKeyInvalid
		}
	}
	return localUser, apiKey, nil
}

func (s *ApiKeyService) GetEnvironmentByApiKey(ctx context.Context, rawKey string) (*string, error) {
	apiKey, err := s.validateRawAPIKeyInternal(ctx, rawKey)
	if err != nil {
		return nil, err
	}
	if apiKey.UserID != nil {
		return nil, ErrApiKeyInvalid
	}
	return apiKey.EnvironmentID, nil
}

// validateRawAPIKeyInternal resolves a raw key to its validated record. A cache
// hit skips the per-candidate Argon2id derivation; expiry is always re-checked
// so a key expiring mid-TTL is rejected immediately.
func (s *ApiKeyService) validateRawAPIKeyInternal(ctx context.Context, rawKey string) (*ApiKey, error) {
	rawKey = strings.TrimSpace(rawKey)
	keyPrefix, err := parseAPIKeyPrefixInternal(rawKey)
	if err != nil {
		return nil, err
	}

	cacheKey := s.hashRawAPIKeyInternal(rawKey)
	if cached, ok, _ := s.validatedKeyCache.Get(cacheKey); ok {
		if cached.ExpiresAt != nil && cached.ExpiresAt.Before(time.Now()) {
			s.validatedKeyCache.Delete(cacheKey)
			return nil, ErrApiKeyExpired
		}
		s.markApiKeyUsedDebouncedInternal(ctx, cached.ID)
		return &cached, nil
	}

	// Snapshot before the read: if a revocation lands between this read and the
	// publish below, the generation will have moved and the publish is dropped.
	gen := s.cacheGen.Load()
	var apiKeys []ApiKey
	if findAPIKeysErr := s.db.WithContext(ctx).Where("key_prefix = ?", keyPrefix).Find(&apiKeys).Error; findAPIKeysErr != nil {
		return nil, fmt.Errorf("failed to find API keys: %w", findAPIKeysErr)
	}

	for _, apiKey := range apiKeys {
		if validateApiKeyHashErr := s.validateApiKeyHash(apiKey.KeyHash, rawKey); validateApiKeyHashErr == nil {
			if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
				return nil, ErrApiKeyExpired
			}

			s.storeValidatedKeyInternal(gen, cacheKey, apiKey)
			s.markApiKeyUsedDebouncedInternal(ctx, apiKey.ID)

			return &apiKey, nil
		}
	}

	return nil, ErrApiKeyInvalid
}

// storeValidatedKeyInternal publishes a validated key into the cache unless an
// invalidation ran since the generation snapshot taken before the caller's DB
// read. The current request still proceeds — it validated against a row that
// existed at read time, indistinguishable from a request racing the revocation
// itself — but the result must not outlive the revocation in the cache.
func (s *ApiKeyService) storeValidatedKeyInternal(gen uint64, cacheKey string, apiKey ApiKey) {
	s.cacheFillMu.Lock()
	defer s.cacheFillMu.Unlock()
	if s.cacheGen.Load() == gen {
		s.validatedKeyCache.Set(cacheKey, apiKey)
	}
}
