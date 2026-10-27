// Source: getarcaneapp/arcane/backend/internal/registry/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package registry

import (
	"context"

	"encoding/json/v2"
	"errors"
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"

	kit "go.getarcane.app/kit/pkg"
	"log/slog"
	"net/http"

	"strings"
	"time"

	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	"github.com/google/go-containerregistry/pkg/authn"

	dockerregistry "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/samber/hot"
	"github.com/samber/mo"
	"go.getarcane.app/kit/normalization"

	"go.getarcane.app/sys/crypto"

	"go.getarcane.app/updater/registry"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/registry/children/browse"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/registryauth"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
)

const (
	registryCacheTTL                    = 30 * time.Minute
	RegistryTypeGeneric          string = "generic"
	RegistryTypeECR              string = "ecr"
	registryPullCountKeyPrefix          = "container_registry:pulls:"
	registryRateLimitKeyPrefix          = "container_registry:rate_limits:"
	dockerHubRateLimitRepository        = "ratelimitpreview/test"
	dockerHubRateLimitTag               = "latest"
)

type RegistryDaemonClient interface {
	RegistryLogin(ctx context.Context, options client.RegistryLoginOptions) (client.RegistryLoginResult, error)
	DistributionInspect(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error)
}

type registryDaemonGetter func(context.Context) (RegistryDaemonClient, error)

type resolvedRegistryCredential struct {
	Username      string
	Token         string
	ServerAddress string
}

type registryRateLimitCacheEntryInternal struct {
	RateLimit registry.RateLimitInfo `json:"rateLimit"`
	CheckedAt time.Time              `json:"checkedAt"`
}

type rateLimitRoundTripFuncInternal func(*http.Request) (*http.Response, error)

func (f rateLimitRoundTripFuncInternal) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type ContainerRegistryService struct {
	db                     *database.DB
	dockerClient           registryDaemonGetter
	cache                  *hot.HotCache[string, string]
	labelCache             *hot.HotCache[string, string]
	ecrRefreshGroup        singleflight.Group
	distributionHTTPClient *http.Client
	kvService              *kv.KVService
	settingsService        *settings.SettingsService
	browse                 *browse.Service
}

// NewContainerRegistryService creates a registry service. kvService may be nil
// in tests that do not need pull tracking or rate-limit caching.
func NewContainerRegistryService(
	db *database.DB,
	dockerClient registryDaemonGetter,
	kvService *kv.KVService,
	settingsService *settings.SettingsService,
	distributionHTTPClients ...*http.Client,
) *ContainerRegistryService {
	distributionHTTPClient := &http.Client{Timeout: 30 * time.Second}
	if len(distributionHTTPClients) > 0 && distributionHTTPClients[0] != nil {
		distributionHTTPClient = distributionHTTPClients[0]
	}
	service := &ContainerRegistryService{
		db:                     db,
		dockerClient:           dockerClient,
		distributionHTTPClient: distributionHTTPClient,
		kvService:              kvService,
		settingsService:        settingsService,
	}
	service.browse = browse.NewService(service.browseRegistryInternal, service.tagLookupContextInternal, distributionHTTPClient.Transport)
	backgroundLoader := func(imageRefs []string) (map[string]string, error) {
		digests := make(map[string]string, len(imageRefs))
		var firstErr error
		for _, imageRef := range imageRefs {
			// Per-ref timeout: a single deadline shared across the whole
			// sequential batch left later refs with whatever the earlier ones
			// had not already spent, so one slow registry starved the tail.
			ctx, cancel := context.WithTimeout(context.Background(), timeouts.DefaultRegistry) //nolint:forbidigo // Cache revalidation runs independently of request cancellation.
			result, err := service.InspectImageDigest(ctx, imageRef, nil)
			cancel()
			if err != nil {
				slog.DebugContext(ctx, "registry revalidation failed for image", "imageRef", imageRef, "error", err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			digests[imageRef] = result.Digest
		}

		// Returning an error alongside results makes hot discard the whole map,
		// so report success whenever anything resolved: one unreachable registry
		// used to throw away every digest already fetched, forcing the entire
		// batch to be refetched on the next tick. Refs that failed are simply
		// left uncached (this cache has no missing-key cache) and retried on
		// next access; only a fully failed batch surfaces the error, which
		// KeepOnError turns into "retain the previous digests".
		if len(digests) > 0 {
			return digests, nil
		}
		return nil, firstErr
	}
	service.cache = hot.NewHotCache[string, string](hot.LRU, 4096).
		WithTTL(registryCacheTTL).
		WithRevalidation(registryCacheTTL, backgroundLoader).
		WithRevalidationErrorPolicy(hot.KeepOnError).
		WithJanitor().
		Build()
	// No static loader: ImageVersionLabel uses GetWithLoaders so failed
	// resolutions are not cached and retry on the next request.
	service.labelCache = hot.NewHotCache[string, string](hot.LRU, 64).
		WithTTL(versionLabelCacheTTL).
		Build()
	return service
}

func (s *ContainerRegistryService) GetAllRegistries(ctx context.Context) ([]ContainerRegistry, error) {
	var registries []ContainerRegistry
	if err := s.db.WithContext(ctx).Find(&registries).Error; err != nil {
		return nil, fmt.Errorf("failed to get container registries: %w", err)
	}
	return registries, nil
}

func (s *ContainerRegistryService) GetRegistriesPaginated(ctx context.Context, params pagination.QueryParams) ([]containerregistry.ContainerRegistry, pagination.Response, error) {
	var registries []ContainerRegistry
	q := s.db.WithContext(ctx).Model(&ContainerRegistry{})

	q = pagination.ApplyLikeSearch(q, params.Search, "url LIKE ? OR username LIKE ? OR COALESCE(description, '') LIKE ?")

	q = pagination.ApplyBooleanFilter(q, "enabled", params.Filters["enabled"])
	q = pagination.ApplyBooleanFilter(q, "insecure", params.Filters["insecure"])

	out, paginationResp, err := params.PaginateSortAndMapDB[ContainerRegistry, containerregistry.ContainerRegistry](q, &registries)
	if err != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to list container registries: %w", err)
	}

	for i := range out {
		out[i] = registryProjection(&registries[i])
	}
	return out, paginationResp, nil
}

func (s *ContainerRegistryService) GetRegistryByID(ctx context.Context, id string) (*ContainerRegistry, error) {
	var registryRecord ContainerRegistry
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&registryRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrContainerRegistryNotFound
		}
		return nil, fmt.Errorf("failed to get container registry: %w", err)
	}
	return &registryRecord, nil
}

func (s *ContainerRegistryService) CreateRegistry(ctx context.Context, req containerregistry.CreateContainerRegistryRequest) (*ContainerRegistry, error) {
	if err := validateRegistryAddress(req.URL); err != nil {
		return nil, err
	}
	if err := settings.ValidateCredentialEncryption(); err != nil {
		return nil, err
	}
	if err := normalization.Normalize(&req); err != nil {
		return nil, err
	}
	registryType, err := NormalizeRegistryType(req.RegistryType)
	if err != nil {
		return nil, err
	}
	repositoryNames, err := normalizeRepositoryNamesInternal(req.RepositoryNames)
	if err != nil {
		return nil, err
	}

	registryRecord := &ContainerRegistry{
		URL:             req.URL,
		Description:     req.Description,
		Insecure:        req.Insecure != nil && *req.Insecure,
		Enabled:         req.Enabled == nil || *req.Enabled,
		RegistryType:    registryType,
		RepositoryNames: repositoryNames,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if registryType == RegistryTypeECR {
		if strings.TrimSpace(req.AWSAccessKeyID) == "" {
			return nil, common.Classify(common.ErrValidation, &base.FieldError{Field: "awsAccessKeyId", Err: errors.New("AWS Access Key ID is required")})
		}
		if strings.TrimSpace(req.AWSRegion) == "" {
			return nil, common.Classify(common.ErrValidation, &base.FieldError{Field: "awsRegion", Err: errors.New("AWS Region is required")})
		}
		if strings.TrimSpace(req.AWSSecretAccessKey) == "" {
			return nil, common.Classify(common.ErrValidation, &base.FieldError{Field: "awsSecretAccessKey", Err: errors.New("AWS Secret Access Key is required")})
		}
		encryptedSecret, encryptErr := crypto.Encrypt(req.AWSSecretAccessKey)
		if encryptErr != nil {
			return nil, fmt.Errorf("failed to encrypt AWS secret access key: %w", encryptErr)
		}
		registryRecord.AWSAccessKeyID = req.AWSAccessKeyID
		registryRecord.AWSSecretAccessKey = encryptedSecret
		registryRecord.AWSRegion = req.AWSRegion
	} else {
		if strings.TrimSpace(req.Username) == "" {
			return nil, common.Classify(common.ErrValidation, &base.FieldError{Field: "username", Err: errors.New("username is required")})
		}
		if strings.TrimSpace(req.Token) == "" {
			return nil, common.Classify(common.ErrValidation, &base.FieldError{Field: "token", Err: errors.New("token is required")})
		}
		encryptedToken, encryptTokenErr := crypto.Encrypt(req.Token)
		if encryptTokenErr != nil {
			return nil, fmt.Errorf("failed to encrypt token: %w", encryptTokenErr)
		}
		registryRecord.Username = req.Username
		registryRecord.Token = encryptedToken
	}
	if createRegistryErr := s.db.WithContext(ctx).Create(registryRecord).Error; createRegistryErr != nil {
		return nil, fmt.Errorf("failed to create registry: %w", createRegistryErr)
	}
	return registryRecord, nil
}

func (s *ContainerRegistryService) UpdateRegistry(ctx context.Context, id string, req containerregistry.UpdateContainerRegistryRequest) (*ContainerRegistry, error) {
	if req.URL != nil {
		if err := validateRegistryAddress(*req.URL); err != nil {
			return nil, err
		}
	}
	if err := normalization.Normalize(&req); err != nil {
		return nil, err
	}
	if req.ClearCredential {
		if req.Username != nil || req.Token != nil || req.AWSAccessKeyID != nil || req.AWSSecretAccessKey != nil {
			return nil, imageDTO.Failure("registry.invalid-config", 400)
		}
		return s.clearRegistryCredentials(ctx, id, req)
	}
	if req.Username != nil || req.Token != nil || req.AWSAccessKeyID != nil || req.AWSSecretAccessKey != nil {
		if err := settings.ValidateCredentialEncryption(); err != nil {
			return nil, err
		}
	}

	registryRecord, err := s.GetRegistryByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if applyRegistryTypeUpdateErr := s.applyRegistryTypeUpdateInternal(registryRecord, req.RegistryType); applyRegistryTypeUpdateErr != nil {
		return nil, applyRegistryTypeUpdateErr
	}

	storedCredentials := map[string]bool{"token": registryRecord.Token != ""}
	updatedCredentials := map[string]bool{"token": req.Token != nil && *req.Token != ""}
	if registryRecord.RegistryType == RegistryTypeECR {
		storedCredentials = map[string]bool{
			"awsAccessKeyId":     registryRecord.AWSAccessKeyID != "",
			"awsSecretAccessKey": registryRecord.AWSSecretAccessKey != "",
		}
		updatedCredentials = map[string]bool{
			"awsAccessKeyId":     req.AWSAccessKeyID != nil && *req.AWSAccessKeyID != "",
			"awsSecretAccessKey": req.AWSSecretAccessKey != nil && *req.AWSSecretAccessKey != "",
		}
	}
	if validateCredentialTargetChangeErr := validation.ValidateCredentialTargetChange(
		"registry URL",
		registryRecord.URL,
		req.URL,
		normalizeRegistryServerAddressInternal,
		storedCredentials,
		updatedCredentials,
	); validateCredentialTargetChangeErr != nil {
		return nil, validateCredentialTargetChangeErr
	}

	// Update common fields
	utils.ApplyChanged(&registryRecord.URL, mo.PointerToOption(req.URL))
	utils.ApplyNullable(&registryRecord.Description, mo.PointerToOption(req.Description))
	utils.ApplyChanged(&registryRecord.Insecure, mo.PointerToOption(req.Insecure))
	utils.ApplyChanged(&registryRecord.Enabled, mo.PointerToOption(req.Enabled))

	// RepositoryNames: nil pointer means "don't touch"; empty slice means "clear".
	if req.RepositoryNames != nil {
		repositoryNames, normalizeRepositoryNamesErr := normalizeRepositoryNamesInternal(*req.RepositoryNames)
		if normalizeRepositoryNamesErr != nil {
			return nil, normalizeRepositoryNamesErr
		}
		registryRecord.RepositoryNames = repositoryNames
	}

	if registryRecord.RegistryType == RegistryTypeECR {
		if updateECRRegistryFieldsErr := s.updateECRRegistryFieldsInternal(registryRecord, req); updateECRRegistryFieldsErr != nil {
			return nil, updateECRRegistryFieldsErr
		}
	} else if updateGenericRegistryFieldsErr := s.updateGenericRegistryFieldsInternal(registryRecord, req); updateGenericRegistryFieldsErr != nil {
		return nil, updateGenericRegistryFieldsErr
	}

	registryRecord.UpdatedAt = time.Now()
	if updateRegistryErr := s.db.WithContext(ctx).Save(registryRecord).Error; updateRegistryErr != nil {
		return nil, fmt.Errorf("failed to update registry: %w", updateRegistryErr)
	}
	return registryRecord,
		nil
}

func (s *ContainerRegistryService) applyRegistryTypeUpdateInternal(localRegistry *ContainerRegistry, registryType *string) error {
	if registryType == nil {
		return nil
	}

	nextType, err := NormalizeRegistryType(*registryType)
	if err != nil {
		return err
	}

	if nextType != localRegistry.RegistryType {
		return common.Classify(
			common.ErrValidation,
			&base.FieldError{
				Field: "registryType",
				Err: errors.New(
					"registry type cannot be changed after creation",
				),
			},
		)
	}

	return nil
}

func (s *ContainerRegistryService) updateECRRegistryFieldsInternal(localRegistry *ContainerRegistry, req containerregistry.UpdateContainerRegistryRequest) error {
	utils.ApplyChanged(&localRegistry.AWSAccessKeyID, mo.PointerToOption(req.AWSAccessKeyID))
	utils.ApplyChanged(&localRegistry.AWSRegion, mo.PointerToOption(req.AWSRegion))

	if req.AWSSecretAccessKey != nil && *req.AWSSecretAccessKey != "" {
		encryptedSecret, err := crypto.Encrypt(*req.AWSSecretAccessKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt AWS secret access key: %w", err)
		}
		utils.ApplyChanged(&localRegistry.AWSSecretAccessKey, mo.Some(encryptedSecret))
	}

	if strings.TrimSpace(localRegistry.AWSAccessKeyID) == "" {
		return common.Classify(common.ErrValidation, &base.FieldError{Field: "awsAccessKeyId", Err: errors.New("AWS Access Key ID is required")})
	}
	if strings.TrimSpace(localRegistry.AWSRegion) == "" {
		return common.Classify(common.ErrValidation, &base.FieldError{Field: "awsRegion", Err: errors.New("AWS Region is required")})
	}
	if strings.TrimSpace(localRegistry.AWSSecretAccessKey) == "" {
		return common.Classify(common.ErrValidation, &base.FieldError{Field: "awsSecretAccessKey", Err: errors.New("AWS Secret Access Key is required")})
	}

	if req.AWSAccessKeyID != nil || req.AWSSecretAccessKey != nil || req.AWSRegion != nil {
		localRegistry.ECRToken = ""
		localRegistry.ECRTokenGeneratedAt = nil
	}

	return nil
}

func (s *ContainerRegistryService) updateGenericRegistryFieldsInternal(localRegistry *ContainerRegistry, req containerregistry.UpdateContainerRegistryRequest) error {
	utils.ApplyChanged(&localRegistry.Username, mo.PointerToOption(req.Username))

	if req.Token != nil && *req.Token != "" {
		encryptedToken, err := crypto.Encrypt(*req.Token)
		if err != nil {
			return fmt.Errorf("failed to encrypt token: %w", err)
		}
		utils.ApplyChanged(&localRegistry.Token, mo.Some(encryptedToken))
	}

	if strings.TrimSpace(localRegistry.Username) == "" {
		return common.Classify(common.ErrValidation, &base.FieldError{Field: "username", Err: errors.New("username is required")})
	}

	return nil
}

func (s *ContainerRegistryService) DeleteRegistry(ctx context.Context, id string) error {
	if err := s.db.WithContext(ctx).Where("id = ?", id).Delete(&ContainerRegistry{}).Error; err != nil {
		return fmt.Errorf("failed to delete container registry: %w", err)
	}
	return nil
}

// GetDecryptedToken returns the decrypted token for a registry
func (s *ContainerRegistryService) GetDecryptedToken(ctx context.Context, id string) (string, error) {
	registryRecord, err := s.GetRegistryByID(ctx, id)
	if err != nil {
		return "", err
	}

	decryptedToken, err := crypto.Decrypt(registryRecord.Token)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt token: %w", err)
	}

	return decryptedToken, nil
}

// GetEnabledRegistries returns all enabled registries
func (s *ContainerRegistryService) GetEnabledRegistries(ctx context.Context) ([]ContainerRegistry, error) {
	var registries []ContainerRegistry
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&registries).Error; err != nil {
		return nil, fmt.Errorf("failed to get enabled container registries: %w", err)
	}
	return registries, nil
}

// GetRegistryAuthForImage returns X-Registry-Auth for the image's registry host.
//
// The registry-auth methods tolerate a nil receiver: callers such as build.BuildService may
// hold no registry service, and are wired in as a buildtypes.RegistryAuthProvider where a
// typed-nil pointer would otherwise satisfy the interface's nil checks and panic on use.
func (s *ContainerRegistryService) GetRegistryAuthForImage(ctx context.Context, imageRef string) (string, error) {
	if s == nil {
		return "", nil
	}
	registryHost, err := registryauth.GetRegistryAddress(imageRef)
	if err != nil {
		return "", err
	}
	return s.GetRegistryAuthForHost(ctx, registryHost)
}

// GetRegistryAuthForHost returns X-Registry-Auth for a configured and enabled registry.
func (s *ContainerRegistryService) GetRegistryAuthForHost(ctx context.Context, registryHost string) (string, error) {
	if s == nil {
		return "", nil
	}
	normalizedRegistryHost := registryauth.NormalizeRegistryForComparison(registryHost)
	if normalizedRegistryHost == "" {
		return "", nil
	}

	authConfigs, err := s.GetAllRegistryAuthConfigs(ctx)
	if err != nil {
		return "", err
	}
	if len(authConfigs) == 0 {
		return "", nil
	}

	cfg, ok := authConfigs[normalizedRegistryHost]
	if !ok {
		return "", nil
	}

	return registryauth.EncodeAuthHeader(cfg.Username, cfg.Password, cfg.ServerAddress)
}

func (s *ContainerRegistryService) GetAllRegistryAuthConfigs(ctx context.Context) (map[string]dockerregistry.AuthConfig, error) {
	if s == nil {
		return nil, nil
	}
	registries, err := s.GetEnabledRegistries(ctx)
	if err != nil {
		return nil, err
	}

	authConfigs := make(map[string]dockerregistry.AuthConfig, len(registries))
	for i := range registries {
		reg := &registries[i]
		if !reg.Enabled {
			continue
		}

		normalizedHost := strings.TrimSpace(registryauth.NormalizeRegistryForComparison(reg.URL))
		if normalizedHost == "" {
			continue
		}

		serverAddress := normalizedHost
		if normalizedHost == "docker.io" {
			serverAddress = registryauth.NormalizeRegistryURL(reg.URL)
		}
		if serverAddress == "" {
			continue
		}

		if reg.Token != "" || reg.AWSSecretAccessKey != "" {
			if err := settings.ValidateCredentialEncryption(); err != nil {
				return nil, err
			}
		}
		var username, token string

		if reg.RegistryType == "ecr" {
			if reg.AWSAccessKeyID == "" && reg.AWSSecretAccessKey == "" {
				continue
			}
			ecrUser, ecrPass, ecrErr := s.GetOrRefreshECRToken(ctx, reg)
			if ecrErr != nil {
				return nil, imageDTO.Neutral(ecrErr, "registry.credential-unavailable", 503)
			}
			username = ecrUser
			token = ecrPass
		} else {
			username = strings.TrimSpace(reg.Username)
			if username == "" || reg.Token == "" {
				continue
			}
			decryptedToken, decryptErr := crypto.Decrypt(reg.Token)
			if decryptErr != nil {
				return nil, imageDTO.Failure("registry.credential-unavailable", 503)
			}
			token = strings.TrimSpace(decryptedToken)
			if token == "" {
				continue
			}
		}

		authConfig := dockerregistry.AuthConfig{
			Username:      username,
			Password:      token,
			ServerAddress: serverAddress,
		}
		for _, key := range registryauth.LookupKeys(normalizedHost) {
			authConfigs[key] = authConfig
		}
	}

	return kit.Ternary(len(authConfigs) == 0, nil, authConfigs), nil
}

// RecordImagePull increments Arcane's observed successful pull counter for an image registry.
func (s *ContainerRegistryService) RecordImagePull(ctx context.Context, imageRef string) error {
	if s.kvService == nil {
		return nil
	}

	registryHost, err := normalizePullRegistryHostInternal(imageRef)
	if err != nil {
		return err
	}
	if registryHost == "" {
		return nil
	}

	if _, incrementInt64Err := s.kvService.IncrementInt64(ctx, registryPullCountKeyInternal(registryHost), 1); incrementInt64Err != nil {
		return incrementInt64Err
	}

	return nil
}

// GetRegistryPullUsage returns pull usage visibility for configured registries.
func (s *ContainerRegistryService) GetRegistryPullUsage(ctx context.Context) (containerregistry.PullUsageResponse, error) {
	registries, err := s.GetAllRegistries(ctx)
	if err != nil {
		return containerregistry.PullUsageResponse{}, err
	}

	results := make([]containerregistry.PullUsage, 0, len(registries))
	for i := range registries {
		results = append(results, s.buildRegistryPullUsageInternal(ctx, registries[i]))
	}

	return containerregistry.PullUsageResponse{Registries: results}, nil
}

func (s *ContainerRegistryService) buildRegistryPullUsageInternal(ctx context.Context, reg ContainerRegistry) containerregistry.PullUsage {
	registryHost := registryauth.NormalizeRegistryForComparison(reg.URL)
	usage := containerregistry.PullUsage{
		RegistryID:    reg.ID,
		Provider:      registryProviderInternal(registryHost, reg.RegistryType),
		Registry:      registryHost,
		DisplayName:   registryDisplayNameInternal(registryHost, reg.RegistryType),
		ObservedPulls: s.getObservedPullsInternal(ctx, registryHost),
		AuthMethod:    "unknown",
		CheckedAt:     time.Now().UTC(),
	}

	if registryHost != "docker.io" || !reg.Enabled {
		return usage
	}

	credential, authMethod, authUsername, err := s.dockerHubCredentialForRegistryInternal(reg)
	usage.AuthMethod = authMethod
	usage.AuthUsername = authUsername
	usage.Repository = "ratelimitpreview/test"
	if err != nil {
		usage.Code = "registry.unavailable"
		usage.Error = imageDTO.Failure(usage.Code, 503).Error()
		return usage
	}

	if cachedRateLimit, checkedAt, ok := s.getCachedRateLimitInternal(ctx, reg.ID); ok {
		ensureRateLimitUsedInternal(cachedRateLimit)
		usage.Limit = cachedRateLimit.Limit
		usage.Remaining = cachedRateLimit.Remaining
		usage.Used = cachedRateLimit.Used
		usage.WindowSeconds = cachedRateLimit.WindowSeconds
		usage.Source = cachedRateLimit.Source
		usage.CheckedAt = checkedAt
		return usage
	}

	rateLimit, err := s.fetchDockerHubRateLimitInternal(ctx, credential)
	usage.CheckedAt = time.Now().UTC()
	if err != nil {
		usage.Code = "registry.unavailable"
		usage.Error = imageDTO.Failure(usage.Code, 503).Error()
		return usage
	}
	ensureRateLimitUsedInternal(rateLimit)

	usage.Limit = rateLimit.Limit
	usage.Remaining = rateLimit.Remaining
	usage.Used = rateLimit.Used
	usage.WindowSeconds = rateLimit.WindowSeconds
	usage.Source = rateLimit.Source
	s.setCachedRateLimitInternal(ctx, reg.ID, rateLimit, usage.CheckedAt)

	return usage
}

func ensureRateLimitUsedInternal(rateLimit *registry.RateLimitInfo) {
	if rateLimit == nil || rateLimit.Used != nil || rateLimit.Limit == nil || rateLimit.Remaining == nil {
		return
	}
	rateLimit.Used = new(max(*rateLimit.Limit-*rateLimit.Remaining, 0))
}

func (s *ContainerRegistryService) getObservedPullsInternal(ctx context.Context, registryHost string) int64 {
	if s.kvService == nil || registryHost == "" {
		return 0
	}

	value, err := s.kvService.GetInt64(ctx, registryPullCountKeyInternal(registryHost), 0)
	if err != nil {
		slog.WarnContext(ctx, "failed to read registry pull count", "registry", registryHost, "error", err)
		return 0
	}

	return value
}

func (s *ContainerRegistryService) dockerHubCredentialForRegistryInternal(reg ContainerRegistry) (*authn.AuthConfig, string, string, error) {
	if reg.RegistryType != RegistryTypeGeneric {
		return nil, "anonymous", "", nil
	}

	username := strings.TrimSpace(reg.Username)
	if username == "" || strings.TrimSpace(reg.Token) == "" {
		return nil, "anonymous", "", nil
	}

	token, err := crypto.Decrypt(reg.Token)
	if err != nil {
		return nil, "credential", username, fmt.Errorf("failed to decrypt Docker Hub credential: %w", err)
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, "anonymous", "", nil
	}

	return &authn.AuthConfig{
		Username: username,
		Password: token,
	}, "credential", username, nil
}

func (s *ContainerRegistryService) fetchDockerHubRateLimitInternal(ctx context.Context, credential *authn.AuthConfig) (*registry.RateLimitInfo, error) {
	return registry.FetchRegistryRateLimit(ctx, "docker.io", dockerHubRateLimitRepository, dockerHubRateLimitTag, credential, dockerHubRateLimitHTTPClientInternal(s.distributionHTTPClient))
}

func dockerHubRateLimitHTTPClientInternal(httpClient *http.Client) *http.Client {
	if httpClient == nil {
		return nil
	}

	cloned := *httpClient
	baseTransport := cloned.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	cloned.Transport = rateLimitRoundTripFuncInternal(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet &&
			req.URL.Host == "registry-1.docker.io" &&
			req.URL.Path == "/v2/"+dockerHubRateLimitRepository+"/manifests/"+dockerHubRateLimitTag {
			rewritten := req.Clone(req.Context())
			rewritten.Method = http.MethodHead
			req = rewritten
		}
		return baseTransport.RoundTrip(req)
	})

	return &cloned
}

func (s *ContainerRegistryService) getCachedRateLimitInternal(ctx context.Context, registryID string) (*registry.RateLimitInfo, time.Time, bool) {
	if s.kvService == nil || registryID == "" {
		return nil, time.Time{}, false
	}

	raw, ok, err := s.kvService.Get(ctx, registryRateLimitKeyInternal(registryID))
	if err != nil {
		slog.WarnContext(ctx, "failed to read registry rate limit cache", "registryId", registryID, "error", err)
		return nil, time.Time{}, false
	}
	if !ok {
		return nil, time.Time{}, false
	}

	var entry registryRateLimitCacheEntryInternal
	if unmarshalErr := json.Unmarshal([]byte(raw), &entry); unmarshalErr != nil {
		slog.WarnContext(ctx, "failed to parse registry rate limit cache", "registryId", registryID, "error", unmarshalErr)
		return nil, time.Time{}, false
	}
	if time.Since(entry.CheckedAt) > registryCacheTTL {
		return nil, time.Time{}, false
	}

	return &entry.RateLimit, entry.CheckedAt, true
}

func (s *ContainerRegistryService) setCachedRateLimitInternal(ctx context.Context, registryID string, rateLimit *registry.RateLimitInfo, checkedAt time.Time) {
	if s.kvService == nil || registryID == "" || rateLimit == nil {
		return
	}

	payload, err := json.Marshal(registryRateLimitCacheEntryInternal{
		RateLimit: *rateLimit,
		CheckedAt: checkedAt,
	})
	if err != nil {
		slog.WarnContext(ctx, "failed to encode registry rate limit cache", "registryId", registryID, "error", err)
		return
	}

	if setErr := s.kvService.Set(ctx, registryRateLimitKeyInternal(registryID), string(payload)); setErr != nil {
		slog.WarnContext(ctx, "failed to save registry rate limit cache", "registryId", registryID, "error", setErr)
	}
}

func normalizePullRegistryHostInternal(imageRef string) (string, error) {
	registryHost, err := registryauth.GetRegistryAddress(imageRef)
	if err != nil {
		return "", fmt.Errorf("parse image registry for %q: %w", imageRef, err)
	}

	return registryauth.NormalizeRegistryForComparison(registryHost), nil
}

func registryPullCountKeyInternal(registryHost string) string {
	return registryPullCountKeyPrefix + registryauth.NormalizeRegistryForComparison(registryHost)
}

func registryRateLimitKeyInternal(registryID string) string {
	return registryRateLimitKeyPrefix + strings.TrimSpace(registryID)
}

func registryProviderInternal(registryHost, registryType string) string {
	if registryType == RegistryTypeECR {
		return "ecr"
	}
	return kit.Ternary(registryHost == "docker.io", "dockerhub", "generic")
}

func registryDisplayNameInternal(registryHost, registryType string) string {
	if registryType == RegistryTypeECR {
		return "Amazon ECR"
	}
	switch {
	case registryHost == "" || registryHost == "docker.io":
		return "Docker Hub"
	case strings.Contains(registryHost, "ghcr.io"):
		return "GitHub Container Registry"
	case strings.Contains(registryHost, "gcr.io"):
		return "Google Container Registry"
	case strings.Contains(registryHost, "quay.io"):
		return "Quay.io Registry"
	default:
		return registryHost
	}
}

func (s *ContainerRegistryService) TestRegistry(ctx context.Context, registryURL, username, token string) error {
	if strings.TrimSpace(username) == "" && strings.TrimSpace(token) == "" {
		// No credentials configured — skip the credential test.
		return nil
	}

	dockerClient, err := s.getDockerClientInternal(ctx)
	if err != nil {
		return err
	}

	_, err = dockerClient.RegistryLogin(ctx, client.RegistryLoginOptions{
		Username:      strings.TrimSpace(username),
		Password:      strings.TrimSpace(token),
		ServerAddress: normalizeRegistryServerAddressInternal(registryURL),
	})
	if err != nil {
		return fmt.Errorf("registry login failed: %w", err)
	}

	return nil
}

// TestECRRegistry tests connectivity for an ECR registry by generating an auth token
// and attempting a Docker login.
func (s *ContainerRegistryService) TestECRRegistry(ctx context.Context, reg *ContainerRegistry) error {
	ecrUser, ecrPass, err := s.GetOrRefreshECRToken(ctx, reg)
	if err != nil {
		return fmt.Errorf("failed to obtain ECR token: %w", err)
	}

	dockerClient, err := s.getDockerClientInternal(ctx)
	if err != nil {
		return err
	}

	_, err = dockerClient.RegistryLogin(ctx, client.RegistryLoginOptions{
		Username:      ecrUser,
		Password:      ecrPass,
		ServerAddress: normalizeRegistryServerAddressInternal(reg.URL),
	})
	if err != nil {
		return fmt.Errorf("ECR registry login failed: %w", err)
	}

	return nil
}
