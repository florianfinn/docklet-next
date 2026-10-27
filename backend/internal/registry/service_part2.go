// Source: getarcaneapp/arcane/backend/internal/registry/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package registry

import (
	"context"
	"encoding/base64"

	"errors"
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"

	kit "go.getarcane.app/kit/pkg"
	"log/slog"

	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/cenkalti/backoff/v5"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/moby/moby/client"

	"github.com/samber/mo"

	"go.getarcane.app/sys/crypto"
	"go.getarcane.app/updater/digest"
	"go.getarcane.app/updater/refs"
	"go.getarcane.app/updater/registry"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"

	"github.com/florianfinn/docklet-next/backend/internal/registry/children/browse"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/registryauth"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

// ImageDigest fetches the current digest for an image:tag from the registry
// This is used for digest-based update detection for non-semver tags
func (s *ContainerRegistryService) ImageDigest(ctx context.Context, imageRef string) (string, error) {
	normalizedRef, _, err := normalizeImageReferenceForDistributionInternal(imageRef)
	if err != nil {
		return "", err
	}

	digestValue, found, err := s.cache.GetWithLoaders(normalizedRef, func(_ []string) (map[string]string, error) {
		loadCtx, cancel := context.WithTimeout(ctx, timeouts.DefaultRegistry)
		defer cancel()

		result, loadErr := s.InspectImageDigest(loadCtx, normalizedRef, nil)
		if loadErr != nil {
			return nil, loadErr
		}
		return map[string]string{normalizedRef: result.Digest}, nil
	})
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("registry digest cache loader returned no digest")
	}
	return digestValue, nil
}

func (s *ContainerRegistryService) InspectImageDigest(ctx context.Context, imageRef string, externalCreds []containerregistry.Credential) (*containerregistry.DigestResult, error) {
	parts, err := refs.NormalizeReference(imageRef)
	if err != nil {
		return nil, err
	}

	var lastResult *containerregistry.DigestResult
	var lastErr error
	fallbackWarningLogged := false

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = 10 * time.Second
	bo.RandomizationFactor = 0.3

	_, retryErr := backoff.Retry(ctx, func() (*containerregistry.DigestResult, error) {
		result, inspectImageDigestViaDaemonErr := s.inspectImageDigestViaDaemonInternal(ctx, parts.NormalizedRef, parts.RegistryHost, externalCreds)
		if inspectImageDigestViaDaemonErr == nil {
			lastResult = result
			return result, nil
		}

		if isRateLimitErrorInternal(inspectImageDigestViaDaemonErr) {
			lastResult = result
			lastErr = inspectImageDigestViaDaemonErr
			slog.DebugContext(ctx, "rate limited by registry, will retry",
				"registry", parts.RegistryHost,
				"imageRef", parts.NormalizedRef)
			return nil, inspectImageDigestViaDaemonErr
		}

		if !isDistributionFallbackEligibleInternal(inspectImageDigestViaDaemonErr) {
			lastResult = result
			lastErr = inspectImageDigestViaDaemonErr
			return nil, backoff.Permanent(inspectImageDigestViaDaemonErr)
		}

		if !fallbackWarningLogged {
			fallbackWarningLogged = true
			slog.WarnContext(ctx, "distribution inspect unavailable, falling back to direct registry digest lookup",
				"imageRef", parts.NormalizedRef,
				"registry", parts.RegistryHost,
				"error", inspectImageDigestViaDaemonErr.Error())
		}

		fallbackResult, fallbackErr := s.inspectImageDigestViaRegistryInternal(ctx, parts.RegistryHost, parts.Repository, parts.Tag, externalCreds)
		if fallbackErr == nil {
			lastResult = fallbackResult
			return fallbackResult, nil
		}

		if isRateLimitErrorInternal(fallbackErr) {
			lastResult = fallbackResult
			lastErr = fallbackErr
			slog.DebugContext(ctx, "rate limited by registry on fallback, will retry",
				"registry", parts.RegistryHost,
				"imageRef", parts.NormalizedRef)
			return nil, fallbackErr
		}

		lastResult = fallbackResult
		lastErr = fmt.Errorf("daemon digest lookup failed; registry fallback failed: %w", errors.Join(inspectImageDigestViaDaemonErr, fallbackErr))
		return nil, backoff.Permanent(lastErr)
	}, backoff.WithBackOff(bo), backoff.WithMaxTries(5))

	if retryErr != nil {
		if errors.Is(retryErr, context.Canceled) || errors.Is(retryErr, context.DeadlineExceeded) {
			return lastResult, retryErr
		}
		return lastResult, lastErr
	}

	return lastResult, nil
}

func (
	s *ContainerRegistryService,
) inspectImageDigestViaDaemonInternal(
	ctx context.Context,
	normalizedRef, registryHost string,
	externalCreds []containerregistry.Credential,
) (
	*containerregistry.DigestResult,
	error,
) {
	dockerClient, err := s.getDockerClientInternal(ctx)
	if err != nil {
		return nil, err
	}

	value, result, err := registryOperationWithCredentialsInternal(ctx, s, registryHost, "distribution inspect of "+normalizedRef, externalCreds,
		func(ctx context.Context, credential *resolvedRegistryCredential) (string, error) {
			return s.fetchDigestFromDaemonInternal(ctx, dockerClient, registryHost, normalizedRef, credential)
		})
	if result != nil {
		result.Digest = value
	}
	return result, err
}

func (
	s *ContainerRegistryService,
) inspectImageDigestViaRegistryInternal(
	ctx context.Context,
	registryHost, repository, tag string,
	externalCreds []containerregistry.Credential,
) (
	*containerregistry.DigestResult,
	error,
) {
	value, result, err := registryOperationWithCredentialsInternal(ctx, s, registryHost, "registry manifest inspect of "+registryHost+"/"+repository+":"+tag, externalCreds,
		func(ctx context.Context, credential *resolvedRegistryCredential) (string, error) {
			return s.fetchDigestFromRegistryInternal(ctx, registryHost, repository, tag, credential)
		})
	if result != nil {
		result.Digest = value
	}
	return result, err
}

// Stored credentials go first: anonymous requests share a per-registry quota with every other unauthenticated client.
func registryOperationWithCredentialsInternal[
	T any,
](
	ctx context.Context,
	s *ContainerRegistryService,
	registryHost, operation string,
	externalCreds []containerregistry.Credential,
	fetch func(
		context.Context,
		*resolvedRegistryCredential,
	) (
		T,
		error,
	),
) (
	T,
	*containerregistry.DigestResult,
	error,
) {
	var zero T
	matchedCredentials, credErr := s.getMatchingRegistryCredentialsInternal(ctx, registryHost, externalCreds)

	var lastErr error
	var lastResult *containerregistry.DigestResult
	for _, credential := range matchedCredentials {
		lastResult = &containerregistry.DigestResult{AuthMethod: "credential", AuthUsername: credential.Username, AuthRegistry: registryHost, UsedCredential: true}
		value, err := fetch(ctx, &credential)
		if err == nil {
			return value, lastResult, nil
		}
		lastErr = err
		if !browse.IsUnauthorizedRegistryError(err) {
			return zero, lastResult, fmt.Errorf("%s failed with credentials: %w", operation, err)
		}
	}
	if lastErr != nil {
		// Docker Hub anonymous quotas are too small to be worth a retry; elsewhere a stale credential must not break public images.
		if isDockerHubRegistryInternal(registryHost) {
			return zero, lastResult, fmt.Errorf("%s failed: %w", operation, lastErr)
		}
		slog.DebugContext(ctx, "credentialed registry lookup failed, retrying anonymously",
			"registry", registryHost,
			"operation", operation,
			"error", lastErr.Error())
	}

	result := &containerregistry.DigestResult{AuthMethod: "anonymous", AuthRegistry: registryHost}
	value, err := fetch(ctx, nil)
	if err == nil {
		return value, result, nil
	}
	if credErr != nil && browse.IsUnauthorizedRegistryError(err) {
		return zero, result, fmt.Errorf("%s: anonymous access unauthorized; credential lookup failed: %w", operation, errors.Join(err, credErr))
	}
	return zero, result, fmt.Errorf("%s failed: %w", operation, err)
}

func (s *ContainerRegistryService) getDockerClientInternal(ctx context.Context) (RegistryDaemonClient, error) {
	if s.dockerClient == nil {
		return nil, errors.New("docker client unavailable")
	}

	dockerClient, err := s.dockerClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get docker client: %w", err)
	}
	if dockerClient == nil {
		return nil, errors.New("docker client unavailable")
	}

	return dockerClient, nil
}

func (
	s *ContainerRegistryService,
) getMatchingRegistryCredentialsInternal(
	ctx context.Context,
	registryHost string,
	externalCreds []containerregistry.Credential,
) (
	[]resolvedRegistryCredential,
	error,
) {
	if len(externalCreds) > 0 {
		resolvedCredentials := make([]resolvedRegistryCredential, 0, len(externalCreds))
		for _, cred := range externalCreds {
			if !cred.Enabled || strings.TrimSpace(cred.Username) == "" || strings.TrimSpace(cred.Token) == "" {
				continue
			}
			if !registryauth.IsRegistryMatch(cred.URL, registryHost) {
				continue
			}

			resolvedCredentials = append(resolvedCredentials, resolvedRegistryCredential{
				Username:      strings.TrimSpace(cred.Username),
				Token:         strings.TrimSpace(cred.Token),
				ServerAddress: normalizeRegistryServerAddressInternal(cred.URL),
			})
		}
		return resolvedCredentials, nil
	}

	if s == nil || s.db == nil {
		return nil, nil
	}

	registries, err := s.GetEnabledRegistries(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load enabled registries: %w", err)
	}

	creds := make([]resolvedRegistryCredential, 0, len(registries))
	for i := range registries {
		reg := &registries[i]
		if !registryauth.IsRegistryMatch(reg.URL, registryHost) {
			continue
		}

		credential, credErr := s.credentialForRegistryInternal(ctx, reg)
		if credErr != nil {
			return nil, imageDTO.Neutral(credErr, "registry.credential-unavailable", 503)
		}
		if credential != nil {
			creds = append(creds, *credential)
		}
	}

	return creds, nil
}

// credentialForRegistryInternal resolves the stored credential of one registry.
// It returns nil when the registry is configured for anonymous access.
func (s *ContainerRegistryService) credentialForRegistryInternal(ctx context.Context, reg *ContainerRegistry) (*resolvedRegistryCredential, error) {
	if reg.Token != "" || reg.AWSSecretAccessKey != "" {
		if err := settings.ValidateCredentialEncryption(); err != nil {
			return nil, err
		}
	}
	if reg.RegistryType == RegistryTypeECR {
		if reg.AWSAccessKeyID == "" && reg.AWSSecretAccessKey == "" {
			return nil, nil
		}
		ecrUser, ecrPass, err := s.GetOrRefreshECRToken(ctx, reg)
		if err != nil {
			return nil, fmt.Errorf("failed to get ECR token: %w", err)
		}
		return &resolvedRegistryCredential{
			Username:      ecrUser,
			Token:         ecrPass,
			ServerAddress: normalizeRegistryServerAddressInternal(reg.URL),
		}, nil
	}

	username := strings.TrimSpace(reg.Username)
	if username == "" || reg.Token == "" {
		return nil, nil
	}

	token, err := crypto.Decrypt(reg.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt registry token: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}

	return &resolvedRegistryCredential{
		Username:      username,
		Token:         token,
		ServerAddress: normalizeRegistryServerAddressInternal(reg.URL),
	}, nil
}

// browseRegistryInternal resolves a stored registry and its credential for browsing.
func (s *ContainerRegistryService) browseRegistryInternal(ctx context.Context, id string) (containerregistry.ContainerRegistry, *containerregistry.Credential, error) {
	reg, err := s.GetRegistryByID(ctx, id)
	if err != nil {
		return containerregistry.ContainerRegistry{}, nil, err
	}
	credential, err := s.credentialForRegistryInternal(ctx, reg)
	if err != nil {
		return containerregistry.ContainerRegistry{}, nil, err
	}
	result := containerregistry.ContainerRegistry{ID: reg.ID, URL: reg.URL, Username: reg.Username, Insecure: reg.Insecure, Enabled: reg.Enabled}
	if credential == nil {
		return result, nil, nil
	}
	return result, &containerregistry.Credential{URL: reg.URL, Username: credential.Username, Token: credential.Token, Enabled: true}, nil
}

// SyncRegistries syncs registries from a manager to this agent instance
// It creates, updates, or deletes registries to match the provided list
func (s *ContainerRegistryService) SyncRegistries(ctx context.Context, items []containerregistry.Sync) error {
	if err := settings.ValidateCredentialEncryption(); err != nil {
		return err
	}
	for _, item := range items {
		if err := validateRegistryAddress(item.URL); err != nil {
			return err
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scoped := &ContainerRegistryService{db: &database.DB{DB: tx}}
		existing, err := scoped.getExistingRegistriesMapInternal(ctx)
		if err != nil {
			return imageDTO.Failure("registry.sync-failed", 503)
		}
		seen := map[string]bool{}
		for _, item := range items {
			if item.ID == "" || seen[item.ID] {
				return imageDTO.Failure("registry.invalid-config", 400)
			}
			seen[item.ID] = true
			if err = scoped.processSyncItemInternal(ctx, item, existing); err != nil {
				return imageDTO.Failure("registry.sync-failed", 503)
			}
		}
		if err = scoped.deleteUnsyncedInternal(ctx, existing, seen); err != nil {
			return imageDTO.Failure("registry.sync-failed", 503)
		}
		return nil
	})
}

func (s *ContainerRegistryService) getExistingRegistriesMapInternal(ctx context.Context) (map[string]*ContainerRegistry, error) {
	var existingRegistries []ContainerRegistry
	if err := s.db.WithContext(ctx).Find(&existingRegistries).Error; err != nil {
		return nil, fmt.Errorf("failed to get existing registries: %w", err)
	}

	existingMap := make(map[string]*ContainerRegistry)
	for i := range existingRegistries {
		existingMap[existingRegistries[i].ID] = &existingRegistries[i]
	}

	return existingMap, nil
}

func (s *ContainerRegistryService) processSyncItemInternal(ctx context.Context, item containerregistry.Sync, existingMap map[string]*ContainerRegistry) error {
	existing, exists := existingMap[item.ID]
	if exists {
		return s.updateExistingRegistryInternal(ctx, item, existing)
	}
	return s.createNewRegistryInternal(ctx, item)
}

func (s *ContainerRegistryService) updateExistingRegistryInternal(ctx context.Context, item containerregistry.Sync, existing *ContainerRegistry) error {
	needsUpdate, err := s.checkRegistryNeedsUpdateInternal(item, existing)
	if err != nil {
		return err
	}

	if needsUpdate {
		existing.UpdatedAt = time.Now()
		if syncRegistryErr := s.db.WithContext(ctx).Save(existing).Error; syncRegistryErr != nil {
			return fmt.Errorf("failed to update registry %s: %w", item.ID, syncRegistryErr)
		}
	}

	return nil
}

func (s *ContainerRegistryService) checkRegistryNeedsUpdateInternal(item containerregistry.Sync, existing *ContainerRegistry) (bool, error) {
	newType, err := NormalizeRegistryType(item.RegistryType)
	if err != nil {
		return false, err
	}

	needsUpdate := utils.ApplyChanged(&existing.URL, mo.Some(item.URL))
	needsUpdate = utils.ApplyNullable(&existing.Description, mo.PointerToOption(item.Description)) || needsUpdate
	needsUpdate = utils.ApplyChanged(&existing.Insecure, mo.Some(item.Insecure)) || needsUpdate
	needsUpdate = utils.ApplyChanged(&existing.Enabled, mo.Some(item.Enabled)) || needsUpdate

	// Normalizing first gives the manager and the local copy the same
	// representation, so the comparison below only reports real changes.
	repositoryNames, err := normalizeRepositoryNamesInternal(item.RepositoryNames)
	if err != nil {
		return false, err
	}
	needsUpdate = utils.ApplySliceChanged(&existing.RepositoryNames, mo.Some(repositoryNames)) || needsUpdate

	// Clear stale credentials when registry type changes during sync
	if newType != existing.RegistryType {
		if newType == RegistryTypeECR {
			existing.Username = ""
			existing.Token = ""
		} else {
			existing.AWSAccessKeyID = ""
			existing.AWSSecretAccessKey = ""
			existing.AWSRegion = ""
			existing.ECRToken = ""
			existing.ECRTokenGeneratedAt = nil
		}
		needsUpdate = true
	}

	needsUpdate = utils.ApplyChanged(&existing.RegistryType, mo.Some(newType)) || needsUpdate

	if newType == RegistryTypeGeneric {
		needsUpdate = utils.ApplyChanged(&existing.Username, mo.Some(item.Username)) || needsUpdate

		tokenChanged, applyEncryptedErr := utils.ApplyEncrypted(&existing.Token, item.Token)
		if applyEncryptedErr != nil {
			return false, fmt.Errorf("failed to apply token for registry %s: %w", existing.ID, applyEncryptedErr)
		}
		return tokenChanged || needsUpdate, nil
	}

	credChanged := utils.ApplyChanged(&existing.AWSAccessKeyID, mo.Some(item.AWSAccessKeyID))
	credChanged = utils.ApplyChanged(&existing.AWSRegion, mo.Some(item.AWSRegion)) || credChanged

	// Sync replaces credential material; empty manager values clear saved secrets.
	if item.AWSSecretAccessKey != "" || existing.AWSSecretAccessKey != "" {
		secretChanged, encryptAWSSecretErr := utils.ApplyEncrypted(&existing.AWSSecretAccessKey, item.AWSSecretAccessKey)
		if encryptAWSSecretErr != nil {
			return false, fmt.Errorf("failed to apply AWS secret for registry %s: %w", existing.ID, encryptAWSSecretErr)
		}
		credChanged = secretChanged || credChanged
	}

	// Invalidate cached ECR token when credentials change
	if credChanged {
		existing.ECRToken = ""
		existing.ECRTokenGeneratedAt = nil
	}
	needsUpdate = credChanged || needsUpdate

	return needsUpdate, nil
}

func (s *ContainerRegistryService) createNewRegistryInternal(ctx context.Context, item containerregistry.Sync) error {
	registryType, err := NormalizeRegistryType(item.RegistryType)
	if err != nil {
		return err
	}
	repositoryNames, err := normalizeRepositoryNamesInternal(item.RepositoryNames)
	if err != nil {
		return err
	}

	newRegistry := &ContainerRegistry{
		ID:              item.ID,
		URL:             item.URL,
		Description:     item.Description,
		Insecure:        item.Insecure,
		Enabled:         item.Enabled,
		RegistryType:    registryType,
		RepositoryNames: repositoryNames,
		AWSAccessKeyID:  item.AWSAccessKeyID,
		AWSRegion:       item.AWSRegion,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if registryType == RegistryTypeGeneric {
		newRegistry.Username = item.Username

		encryptedToken, encryptErr := crypto.Encrypt(item.Token)
		if encryptErr != nil {
			return fmt.Errorf("failed to encrypt token for new registry %s: %w", item.ID, encryptErr)
		}
		newRegistry.Token = encryptedToken
	} else if item.AWSSecretAccessKey != "" {
		encryptedSecret, encryptNewAWSSecretErr := crypto.Encrypt(item.AWSSecretAccessKey)
		if encryptNewAWSSecretErr != nil {
			return fmt.Errorf("failed to encrypt AWS secret for new registry %s: %w", item.ID, encryptNewAWSSecretErr)
		}
		newRegistry.AWSSecretAccessKey = encryptedSecret
	}
	if createRegistryErr := s.db.WithContext(ctx).Create(newRegistry).Error; createRegistryErr != nil {
		return fmt.Errorf("failed to create registry %s: %w", item.ID, createRegistryErr)
	}
	return nil
}

func NormalizeRegistryType(value string) (string, error) {
	registryType := strings.ToLower(strings.TrimSpace(value))
	if registryType == "" {
		return RegistryTypeGeneric, nil
	}

	switch registryType {
	case RegistryTypeGeneric, RegistryTypeECR:
		return registryType, nil
	default:
		return "", common.Classify(
			common.ErrValidation,
			&base.FieldError{
				Field: "registryType",
				Err: errors.New(
					"registry type must be one of: generic, ecr",
				),
			},
		)
	}
}

// normalizeRepositoryNamesInternal trims, filters and deduplicates the given
// entries (preserving first-occurrence order) and validates what remains. It
// returns a non-nil slice so that GORM always serializes to a JSON array.
func normalizeRepositoryNamesInternal(raw []string) (database.StringSlice, error) {
	names := kit.Unique(kit.TrimNonEmpty(raw))
	result := make(database.StringSlice, 0, len(names))
	for _, name := range names {
		// A repository name is only a path, so pair it with placeholder domain
		// and tag segments to validate it against the reference grammar.
		if _, err := reference.ParseNormalizedNamed("registry.invalid/" + name + "/placeholder:latest"); err != nil {
			return nil, common.Classify(common.ErrValidation, &base.FieldError{Field: "repositoryNames", Err: fmt.Errorf("invalid repository name %q", name)})
		}
		result = append(result, name)
	}

	return result, nil
}

func (s *ContainerRegistryService) deleteUnsyncedInternal(ctx context.Context, existingMap map[string]*ContainerRegistry, syncedIDs map[string]bool) error {
	for id := range existingMap {
		if !syncedIDs[id] {
			if err := s.db.WithContext(ctx).Where("id = ?", id).Delete(&ContainerRegistry{}).Error; err != nil {
				return fmt.Errorf("failed to delete registry %s: %w", id, err)
			}
		}
	}
	return nil
}

func normalizeImageReferenceForDistributionInternal(imageRef string) (string, string, error) {
	parts, err := refs.NormalizeReference(imageRef)
	if err != nil {
		return "", "", err
	}

	return parts.NormalizedRef, parts.RegistryHost, nil
}

func normalizeRegistryServerAddressInternal(registryURL string) string {
	normalizedHost := strings.TrimSpace(registryauth.NormalizeRegistryForComparison(registryURL))
	if normalizedHost == "" {
		return ""
	}

	if normalizedHost == "docker.io" {
		return registryauth.NormalizeRegistryURL(registryURL)
	}

	return normalizedHost
}

func isRateLimitErrorInternal(err error) bool {
	if err == nil {
		return false
	}
	return IsRateLimitErrorString(err.Error())
}

func IsRateLimitErrorString(msg string) bool {
	msgLower := strings.ToLower(msg)
	indicators := []string{
		"toomanyrequests",
		"rate limit",
		"too many requests",
		"status: 429",
		"status 429",
		"retry-after",
	}
	for _, indicator := range indicators {
		if strings.Contains(msgLower, indicator) {
			return true
		}
	}
	return false
}

func isDockerHubRegistryInternal(registryHost string) bool {
	return registryauth.NormalizeRegistryForComparison(registryHost) == "docker.io"
}

func isDistributionFallbackEligibleInternal(err error) bool {
	if err == nil {
		return false
	}

	if registry.IsFallbackEligibleDaemonError(err) {
		return true
	}

	if browse.IsUnauthorizedRegistryError(err) {
		return false
	}

	errLower := strings.ToLower(err.Error())
	return strings.Contains(errLower, "context deadline exceeded") ||
		strings.Contains(errLower, "client.timeout exceeded") ||
		strings.Contains(errLower, "i/o timeout")
}

func (
	s *ContainerRegistryService,
) fetchDigestFromDaemonInternal(
	ctx context.Context,
	dockerClient RegistryDaemonClient,
	registryHost, normalizedRef string,
	credential *resolvedRegistryCredential,
) (
	string,
	error,
) {
	var inspectOptions client.DistributionInspectOptions
	if credential != nil {
		authHeader, err := registryauth.EncodeAuthHeader(credential.Username, credential.Token, credential.ServerAddress)
		if err != nil {
			return "", fmt.Errorf("encode registry auth header for %s: %w", registryHost, err)
		}
		inspectOptions.EncodedRegistryAuth = authHeader
	}

	inspectResult, err := dockerClient.DistributionInspect(ctx, normalizedRef, inspectOptions)
	if err != nil {
		return "", err
	}
	digestValue, err := digest.Normalize(inspectResult.Descriptor.Digest.String())
	if err != nil {
		return "", fmt.Errorf("distribution inspect returned invalid digest for %s: %w", normalizedRef, err)
	}
	return digestValue, nil
}

func (s *ContainerRegistryService) fetchDigestFromRegistryInternal(ctx context.Context, registryHost, repository, tag string, credential *resolvedRegistryCredential) (string, error) {
	var distributionCredential *authn.AuthConfig
	if credential != nil {
		distributionCredential = &authn.AuthConfig{
			Username: strings.TrimSpace(credential.Username),
			Password: strings.TrimSpace(credential.Token),
		}
	}

	return registry.FetchDigest(
		ctx,
		registryHost,
		repository,
		tag,
		distributionCredential,
		s.distributionHTTPClient,
	)
}

// ListImageTags discovers repository tags with the same credential precedence as
// digest checks. External credentials replace local credentials when supplied.
func (s *ContainerRegistryService) ListImageTags(ctx context.Context, imageRef string, externalCreds []containerregistry.Credential) ([]string, error) {
	if refs.IsDigestPinnedReference(imageRef) || refs.IsImageIDLikeReference(imageRef) {
		return nil, errors.New("cannot discover tags for an immutable image reference")
	}
	parts, err := refs.NormalizeReference(imageRef)
	if err != nil {
		return nil, err
	}
	lookupCtx, cancel := s.tagLookupContextInternal(ctx)
	defer cancel()

	tags, _, err := registryOperationWithCredentialsInternal(lookupCtx, s, parts.RegistryHost, "registry tag listing of "+parts.NormalizedRef, externalCreds,
		func(ctx context.Context, credential *resolvedRegistryCredential) ([]string, error) {
			var auth *authn.AuthConfig
			if credential != nil {
				auth = &authn.AuthConfig{Username: credential.Username, Password: credential.Token}
			}
			return registry.FetchTags(ctx, parts.RegistryHost, parts.Repository, auth, s.distributionHTTPClient)
		})
	return tags, err
}

// tagLookupContextInternal bounds tag and manifest walks by the configured registry tag timeout.
func (s *ContainerRegistryService) tagLookupContextInternal(ctx context.Context) (context.Context, context.CancelFunc) {
	timeoutSeconds := 0
	if s.settingsService != nil {
		timeoutSeconds = s.settingsService.GetSettingsConfig().RegistryTagTimeout.AsInt()
	}
	return context.WithTimeout(ctx, timeouts.GetDuration(timeoutSeconds, timeouts.DefaultRegistryTags))
}

const ecrTokenTTL = 12 * time.Hour

type ecrTokenResult struct {
	username string
	password string
}

// GetOrRefreshECRToken returns a valid ECR auth token (username + password) for the given
// registry. If the cached token (stored encrypted in the DB) is still within its 12-hour
// validity window it is returned directly; otherwise a new token is obtained from the AWS
// ECR API, persisted back to the DB, and returned.
// Concurrent refreshes for the same registry are deduplicated via singleflight.
func (s *ContainerRegistryService) GetOrRefreshECRToken(ctx context.Context, reg *ContainerRegistry) (username, password string, err error) {
	if err := settings.ValidateCredentialEncryption(); err != nil {
		return "", "", err
	}
	// Fast path: return cached token if still valid.
	if reg.ECRTokenGeneratedAt != nil && time.Since(reg.ECRTokenGeneratedAt.UTC()) < ecrTokenTTL {
		if reg.ECRToken != "" {
			decrypted, decErr := crypto.Decrypt(reg.ECRToken)
			if decErr == nil && strings.TrimSpace(decrypted) != "" {
				return "AWS", decrypted, nil
			}
		}
	}

	// Slow path: deduplicate concurrent refreshes for the same registry.
	result, sErr, _ := s.ecrRefreshGroup.Do(reg.ID, func() (any, error) {
		// Detach from the request context so a cancelled caller doesn't
		// abort the shared refresh for all waiting goroutines.
		refreshCtx := context.WithoutCancel(ctx)
		return s.refreshECRTokenInternal(refreshCtx, reg)
	})
	if sErr != nil {
		return "", "", sErr
	}
	r, ok := result.(*ecrTokenResult)
	if !ok {
		return "", "", errors.New("unexpected ECR token result type")
	}
	return r.username, r.password, nil
}

func (s *ContainerRegistryService) refreshECRTokenInternal(ctx context.Context, reg *ContainerRegistry) (*ecrTokenResult, error) {
	// Decrypt the stored AWS secret access key.
	secretKey, decErr := crypto.Decrypt(reg.AWSSecretAccessKey)
	if decErr != nil {
		return nil, fmt.Errorf("failed to decrypt AWS secret key for registry %s: %w", reg.URL, decErr)
	}
	secretKey = strings.TrimSpace(secretKey)
	if secretKey == "" {
		return nil, fmt.Errorf("AWS secret access key is empty for registry %s", reg.URL)
	}

	// Call AWS ECR GetAuthorizationToken.
	cfg, cfgErr := config.LoadDefaultConfig(ctx,
		config.WithRegion(reg.AWSRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			reg.AWSAccessKeyID,
			secretKey,
			"",
		)),
	)
	if cfgErr != nil {
		return nil, fmt.Errorf("failed to load AWS config for registry %s: %w", reg.URL, cfgErr)
	}

	ecrClient := ecr.NewFromConfig(cfg)
	result, ecrErr := ecrClient.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if ecrErr != nil {
		return nil, fmt.Errorf("failed to get ECR authorization token for registry %s: %w", reg.URL, ecrErr)
	}
	if len(result.AuthorizationData) == 0 || result.AuthorizationData[0].AuthorizationToken == nil {
		return nil, fmt.Errorf("ECR returned empty authorization data for registry %s", reg.URL)
	}

	// Decode base64 token → "AWS:<password>".
	decoded, decodeErr := base64.StdEncoding.DecodeString(*result.AuthorizationData[0].AuthorizationToken)
	if decodeErr != nil {
		return nil, fmt.Errorf("failed to decode ECR token for registry %s: %w", reg.URL, decodeErr)
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, fmt.Errorf("unexpected ECR token format for registry %s", reg.URL)
	}
	ecrPassword := parts[1]

	// Persist the new token (encrypted) and generation timestamp.
	encryptedToken, encErr := crypto.Encrypt(ecrPassword)
	if encErr != nil {
		return nil, fmt.Errorf("failed to encrypt ECR token for registry %s: %w", reg.URL, encErr)
	}
	now := time.Now().UTC()
	reg.ECRToken = encryptedToken
	reg.ECRTokenGeneratedAt = &now
	if saveErr := s.db.WithContext(ctx).Model(reg).Updates(map[string]any{
		"ecr_token":              encryptedToken,
		"ecr_token_generated_at": now,
	}).Error; saveErr != nil {
		// Non-fatal: log but continue — the token is still usable for this call.
		slog.WarnContext(ctx, "failed to persist ECR token to database", "registry", reg.URL, "error", saveErr)
	}

	return &ecrTokenResult{username: "AWS", password: ecrPassword}, nil
}

const (
	ociImageVersionLabel = "org.opencontainers.image.version"
	versionLabelCacheTTL = 3 * time.Hour
)

// ErrNoVersionLabel is returned when the remote image config carries no
// org.opencontainers.image.version label.
var ErrNoVersionLabel = errors.New("image config has no version label")
