// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package registry

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"uuid"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestPR1RegistryProjectionNeverContainsCredentials(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	s := NewContainerRegistryService(db, nil, nil, nil)
	sentinel := uuid.New().String()
	record, err := s.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "credential-identity", Token: sentinel})
	require.NoError(t, err)
	for _, value := range []any{registryProjection(record), record} {
		body, e := json.Marshal(value)
		require.NoError(t, e)
		require.NotContains(t, string(body), sentinel)
		require.NotContains(t, string(body), record.Token)
		require.NotContains(t, string(body), "credential-identity")
		require.NotContains(t, string(body), `"token"`)
		require.NotContains(t, string(body), `"username"`)
	}
	handler := NewHandler(s, nil)
	for _, read := range []func() (*containerregistry.ContainerRegistry, error){func() (*containerregistry.ContainerRegistry, error) {
		r, e := handler.GetRegistry(t.Context(), &GetContainerRegistryInput{ID: record.ID})
		if e != nil {
			return nil, e
		}
		return &r.Body.Data, nil
	}} {
		dto, e := read()
		require.NoError(t, e)
		require.True(t, dto.HasCredential)
	}
	replacement := uuid.New().String()
	updated, err := s.UpdateRegistry(t.Context(), record.ID, containerregistry.UpdateContainerRegistryRequest{Token: &replacement})
	require.NoError(t, err)
	plaintext, err := crypto.Decrypt(updated.Token)
	require.NoError(t, err)
	require.Equal(t, replacement, plaintext)
	description := "Updated public description"
	updated, err = s.UpdateRegistry(t.Context(), record.ID, containerregistry.UpdateContainerRegistryRequest{ClearCredential: true, Description: &description})
	require.NoError(t, err)
	require.False(t, registryProjection(updated).HasCredential)
	require.Equal(t, &description, updated.Description)
}
func TestPR1RegistryWeakKeyRefusesWritesAndSync(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	s := NewContainerRegistryService(db, nil, nil, nil)
	field, _ := reflect.TypeFor[config.Config]().FieldByName("EncryptionKey")
	defer crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "test-encryption-key-for-testing-32bytes-min"})
	for _, key := range []string{"", field.Tag.Get("default")} {
		crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: key})
		_, err := s.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "credential-identity", Token: uuid.New().String()})
		assert.Equal(t, "registry.credential-unavailable", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
		var afterCreate int64
		require.NoError(t, db.Model(&ContainerRegistry{}).Count(&afterCreate).Error)
		require.Zero(t, afterCreate, "weak-key credential creation must have zero persisted effects")
		require.Error(t, s.SyncRegistries(t.Context(), nil))
		var count int64
		require.NoError(t, db.Model(&ContainerRegistry{}).Count(&count).Error)
		require.Zero(t, count)
	}
}
func TestPR1RegistrySyncReplacementIsAtomic(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	s := NewContainerRegistryService(db, nil, nil, nil)
	original, err := s.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "original", Token: uuid.New().String()})
	require.NoError(t, err)
	err = s.SyncRegistries(t.Context(), []containerregistry.Sync{{ID: "first", URL: "https://registry.example.com", RegistryType: "generic", Username: "replacement", Token: uuid.New().String(), Enabled: true}, {ID: "second", URL: "https://other.example.com", RegistryType: "unsupported", Enabled: true}})
	require.Error(t, err)
	rows, err := s.GetAllRegistries(t.Context())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, original.ID, rows[0].ID)
}
func TestPR1RegistryRejectsUserinfoAndReservedHost(t *testing.T) {
	for _, address := range []string{"https://credential-user:credential-value@registry.example.com", "https://docklet-retention.invalid", "DOCKLET-RETENTION.INVALID"} {
		require.Error(t, validateRegistryAddress(address))
	}
}
func TestPR1KeyProbeUsesActualEncryptor(t *testing.T) {
	setupContainerRegistryTestDBInternal(t)
	require.NoError(t, settings.ValidateCredentialEncryption())
	value, err := crypto.Encrypt(strings.Repeat("x", 40))
	require.NoError(t, err)
	require.NotEmpty(t, value)

}

func TestPR1RegistryTestResultNeverContainsDaemonDiagnostic(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	sentinel := uuid.New().String()
	s := NewContainerRegistryService(db, func(context.Context) (RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{registryLoginFn: func(context.Context, client.RegistryLoginOptions) (client.RegistryLoginResult, error) {
			return client.RegistryLoginResult{}, errors.New(sentinel)
		}}, nil
	}, nil, nil)
	reg, err := s.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "credential-identity", Token: uuid.New().String()})
	require.NoError(t, err)
	_, err = NewHandler(s, nil).TestRegistry(t.Context(), &TestContainerRegistryInput{ID: reg.ID})
	require.Error(t, err)
	result, ok := err.(*containerregistry.RegistryTestResult)
	require.True(t, ok)
	require.Equal(t, 503, result.GetStatus())
	require.Equal(t, "registry.unavailable", result.Code)
	require.False(t, result.CheckedAt.IsZero())
	body, e := json.Marshal(result)
	require.NoError(t, e)
	require.NotContains(t, string(body), sentinel)
	require.NotContains(t, err.Error(), sentinel)
}

func TestPR1StoredCredentialFailureNeverFallsBackToAnonymous(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	s := NewContainerRegistryService(db, nil, nil, nil)
	reg, err := s.CreateRegistry(t.Context(), containerregistry.CreateContainerRegistryRequest{URL: "https://registry.example.com", Username: "credential-identity", Token: uuid.New().String()})
	require.NoError(t, err)
	require.NoError(t, db.Model(reg).Update("token", uuid.New().String()).Error)
	auth, err := s.GetRegistryAuthForHost(t.Context(), "registry.example.com")
	require.Error(t, err)
	require.Empty(t, auth)
	_, err = s.credentialForRegistryInternal(t.Context(), reg)
	require.Error(t, err)
}

func TestPR1RegistryHTTPAcceptsConditionalCredentialsAndPartialClear(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	h := NewHandler(NewContainerRegistryService(db, nil, nil, nil), nil)
	router := echo.New()
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("test", "test"))
	huma.Register(api, huma.Operation{OperationID: "create-registry", Method: "POST", Path: "/container-registries"}, h.CreateRegistry)
	huma.Register(api, huma.Operation{OperationID: "update-registry", Method: "PUT", Path: "/container-registries/{id}"}, h.UpdateRegistry)
	sentinel := uuid.New().String()
	call := func(method, path string, input any) *httptest.ResponseRecorder {
		payload, e := json.Marshal(input)
		require.NoError(t, e)
		request := httptest.NewRequest(method, path, strings.NewReader(string(payload)))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), sentinel)
		return response
	}
	response := call("POST", "/api/container-registries", map[string]any{"url": "https://registry.example.com", "username": "credential-identity", "token": sentinel})
	var created struct {
		Data containerregistry.ContainerRegistry
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	require.True(t, created.Data.HasCredential)
	response = call("PUT", "/api/container-registries/"+created.Data.ID, map[string]any{"clearCredential": true, "description": "Public metadata", "registryType": "ecr", "awsRegion": "example-region"})
	var cleared struct {
		Data containerregistry.ContainerRegistry
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &cleared))
	require.False(t, cleared.Data.HasCredential)
	require.Equal(t, "Public metadata", *cleared.Data.Description)
	require.Equal(t, "ecr", cleared.Data.RegistryType)
	require.Equal(t, "example-region", cleared.Data.AWSRegion)
}

func TestPR1SyncedClearedCredentialRemainsAnonymous(t *testing.T) {
	for _, kind := range []string{"generic", "ecr"} {
		t.Run(kind, func(t *testing.T) {
			db := setupContainerRegistryTestDBInternal(t)
			s := NewContainerRegistryService(db, nil, nil, nil)
			initial := containerregistry.Sync{ID: "configured", URL: "https://registry.example.com", RegistryType: kind, Enabled: true, AWSRegion: "example-region"}
			if kind == "generic" {
				initial.Username = "credential-identity"
				initial.Token = uuid.New().String()
			} else {
				initial.AWSAccessKeyID = uuid.New().String()
				initial.AWSSecretAccessKey = uuid.New().String()
			}
			require.NoError(t, s.SyncRegistries(t.Context(), []containerregistry.Sync{initial}))
			replacement := containerregistry.Sync{ID: initial.ID, URL: initial.URL, RegistryType: kind, Enabled: true, AWSRegion: initial.AWSRegion}
			require.NoError(t, s.SyncRegistries(t.Context(), []containerregistry.Sync{replacement}))
			for _, id := range []string{"configured", "new-cleared"} {
				if id == "new-cleared" {
					replacement.ID = id
					require.NoError(t, s.SyncRegistries(t.Context(), []containerregistry.Sync{replacement}))
				}
				reg, err := s.GetRegistryByID(t.Context(), id)
				require.NoError(t, err)
				require.Empty(t, reg.Token)
				require.Empty(t, reg.AWSSecretAccessKey)
				require.Empty(t, reg.AWSAccessKeyID)
				require.False(t, registryProjection(reg).HasCredential)
				auth, err := s.GetRegistryAuthForHost(t.Context(), "registry.example.com")
				require.NoError(t, err)
				require.Empty(t, auth)
				cred, err := s.credentialForRegistryInternal(t.Context(), reg)
				require.NoError(t, err)
				require.Nil(t, cred)
			}
		})
	}
}

func TestPR1ClearedCredentialTestCannotClaimAuthenticationSuccess(t *testing.T) {
	db := setupContainerRegistryTestDBInternal(t)
	s := NewContainerRegistryService(db, nil, nil, nil)
	require.NoError(t, db.Create(&ContainerRegistry{ID: "cleared", URL: "https://registry.example.com", RegistryType: "generic", Enabled: true}).Error)
	response, err := NewHandler(s, nil).TestRegistry(t.Context(), &TestContainerRegistryInput{ID: "cleared"})
	require.Nil(t, response)
	require.Error(t, err)
	result, ok := err.(*containerregistry.RegistryTestResult)
	require.True(t, ok)
	require.False(t, result.OK)
	require.Equal(t, "registry.authentication-required", result.Code)
	require.Equal(t, 502, result.GetStatus())
}
