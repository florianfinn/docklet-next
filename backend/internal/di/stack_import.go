// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package di

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/stackimport"
)

func provideStackImportModuleInternal(cfg *config.Config, registries *registry.ContainerRegistryService) (*stackimport.Module, error) {
	resolver, err := stackimport.NewResolver(registries, cfg.StackImportGitHubRawURL)
	if err != nil {
		return nil, err
	}
	return stackimport.NewWithResolver(resolver), nil
}
