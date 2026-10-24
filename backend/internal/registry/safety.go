// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package registry

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"net/url"
	"strings"
)

func validateRegistryAddress(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return imageDTO.Failure("registry.invalid-config", 400)
	}
	host := parsed.Hostname()
	if host == "" {
		parsed, err = url.Parse("https://" + raw)
		if err != nil {
			return imageDTO.Failure("registry.invalid-config", 400)
		}
		host = parsed.Hostname()
	}
	if host == "" || parsed.User != nil || strings.EqualFold(host, guard.ReservedHost) {
		return imageDTO.Failure("registry.invalid-config", 400)
	}
	return nil
}
func registryProjection(reg *ContainerRegistry) containerregistry.ContainerRegistry {
	return containerregistry.ContainerRegistry{ID: reg.ID, URL: reg.URL, Description: reg.Description, Insecure: reg.Insecure, Enabled: reg.Enabled, RegistryType: reg.RegistryType, RepositoryNames: reg.RepositoryNames, AWSRegion: reg.AWSRegion, CreatedAt: reg.CreatedAt, UpdatedAt: reg.UpdatedAt, HasCredential: reg.Token != "" || reg.AWSSecretAccessKey != ""}
}

// clearRegistryCredentials changes only credential fields, preserving nonsecret config.
func (s *ContainerRegistryService) clearRegistryCredentials(ctx context.Context, id string, req containerregistry.UpdateContainerRegistryRequest) (*ContainerRegistry, error) {
	reg, err := s.GetRegistryByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.RegistryType != nil {
		kind, e := NormalizeRegistryType(*req.RegistryType)
		if e != nil {
			return nil, e
		}
		reg.RegistryType = kind
	}
	if req.AWSRegion != nil {
		reg.AWSRegion = *req.AWSRegion
	}
	if req.RepositoryNames != nil {
		names, e := normalizeRepositoryNamesInternal(*req.RepositoryNames)
		if e != nil {
			return nil, e
		}
		reg.RepositoryNames = names
	}
	reg.Username = ""
	reg.Token = ""
	reg.AWSAccessKeyID = ""
	reg.AWSSecretAccessKey = ""
	reg.ECRToken = ""
	reg.ECRTokenGeneratedAt = nil
	if req.URL != nil {
		reg.URL = *req.URL
	}
	if req.Description != nil {
		reg.Description = req.Description
	}
	if req.Insecure != nil {
		reg.Insecure = *req.Insecure
	}
	if req.Enabled != nil {
		reg.Enabled = *req.Enabled
	}

	if err = s.db.WithContext(ctx).Save(reg).Error; err != nil {
		return nil, imageDTO.Failure("registry.persistence-failed", 500)
	}
	return reg, nil
}

func registryHTTPError(err error) error {
	var problem *imageDTO.OperationError
	if errors.As(err, &problem) {
		return problem
	}
	if errors.Is(err, common.ErrContainerRegistryNotFound) {
		return imageDTO.Failure("registry.not-found", 404)
	}
	status := common.ToAPIError(err).HTTPStatus()
	if status == 400 {
		return imageDTO.Failure("registry.invalid-config", 400)
	}
	if status == 409 {
		return imageDTO.Failure("registry.exists", 409)
	}
	return imageDTO.Failure("registry.unavailable", 503)
}
