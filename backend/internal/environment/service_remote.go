// Source: getarcaneapp/arcane/backend/internal/environment/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed S3 backup destinations.

package environment

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/environment/children/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/environment/children/snippets"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"strings"
)

func (s *EnvironmentService) resolveRemoteEnvironmentTargetInternal(ctx context.Context, envID string) (*proxy.Target, error) {
	envRecord, err := s.GetEnvironmentByID(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	return proxy.NewTarget(envRecord.ID, envRecord.Name, envRecord.ApiUrl, envRecord.IsEdge, envRecord.AccessToken)
}

func (s *EnvironmentService) ExecuteRemoteRequest(ctx context.Context, envID, method, path string, body []byte) (*remenv.Response, error) {
	target, err := s.resolveRemoteEnvironmentTargetInternal(ctx, envID)
	if err != nil {
		return nil, err
	}

	return s.proxy.Execute(ctx, target, method, path, body)
}

func (s *EnvironmentService) ProxyJSONRequest(ctx context.Context, envID, method, path string, body []byte, out any) error {
	proxyCtx, cancel := s.proxy.Context(ctx)
	defer cancel()

	target, err := s.resolveRemoteEnvironmentTargetInternal(proxyCtx, envID)
	if err != nil {
		return err
	}

	return s.proxy.JSON(proxyCtx, target, method, path, body, out)
}

// ProxyJSONRequestForEnvironment sends a JSON request using an already-loaded
// environment row, avoiding an extra environment lookup on hot stream paths.
func (s *EnvironmentService) ProxyJSONRequestForEnvironment(ctx context.Context, env Environment, method, path string, body []byte, out any) error {
	proxyCtx, cancel := s.proxy.Context(ctx)
	defer cancel()

	target, err := proxy.NewTarget(env.ID, env.Name, env.ApiUrl, env.IsEdge, env.AccessToken)
	if err != nil {
		return err
	}

	return s.proxy.JSON(proxyCtx, target, method, path, body, out)
}

// ProxyRequest sends a request to a remote environment's API.
func (s *EnvironmentService) ProxyRequest(ctx context.Context, envID, method, path string, body []byte) ([]byte, int, error) {
	proxyCtx, cancel := s.proxy.Context(ctx)
	defer cancel()

	resp, err := s.ExecuteRemoteRequest(proxyCtx, envID, method, path, body)
	if err != nil {
		return nil, 0, err
	}

	return resp.Body, resp.StatusCode, nil
}

// DeploymentSnippets contains deployment configuration snippets for an environment.
type DeploymentSnippets struct {
	DockerRun     string
	DockerCompose string
	MTLS          *DeploymentSnippetMTLS
}

type DeploymentSnippetFile struct {
	Name          string `json:"name" doc:"Suggested filename"`
	Content       string `json:"content,omitempty" doc:"PEM file contents. Omitted for sensitive files such as private keys; use downloadUrl instead."`
	DownloadURL   string `json:"downloadUrl,omitempty" doc:"Pairing-permission endpoint to download this file when content is withheld"`
	Sensitive     bool   `json:"sensitive,omitempty" doc:"True when this file is sensitive and must be fetched via downloadUrl"`
	ContainerPath string `json:"containerPath" doc:"Container mount path expected by the mTLS snippet"`
	Permissions   string `json:"permissions" doc:"Suggested file mode"`
}

type DeploymentSnippetMTLS struct {
	DockerRun     string                  `json:"dockerRun" doc:"Docker run snippet using Arcane-generated mTLS assets"`
	DockerCompose string                  `json:"dockerCompose" doc:"Docker compose snippet using Arcane-generated mTLS assets"`
	Files         []DeploymentSnippetFile `json:"files" doc:"Generated PEM files to place on the edge host"`
	HostDirHint   string                  `json:"hostDirHint" doc:"Suggested host directory containing the generated PEM files"`
}

// GenerateDeploymentSnippets generates Docker deployment snippets for an environment.
func (s *EnvironmentService) GenerateDeploymentSnippets(ctx context.Context, envID, managerURL, agentURL, apiKey string) (*DeploymentSnippets, error) {
	dockerRun, dockerCompose := snippets.Direct(strings.TrimRight(managerURL, "/"), agentURL, apiKey)
	return &DeploymentSnippets{
		DockerRun:     dockerRun,
		DockerCompose: dockerCompose,
	}, nil
}

// GenerateEdgeDeploymentSnippets generates Docker deployment snippets for an edge agent.
// Edge agents connect outbound to the manager and don't require exposed ports.
func (s *EnvironmentService) GenerateEdgeDeploymentSnippets(ctx context.Context, envID, managerURL, apiKey string, edgeCfg *edge.Config) (*DeploymentSnippets, error) {
	managerURL = strings.TrimRight(managerURL, "/")
	dockerRun, dockerCompose := snippets.Edge(managerURL, apiKey)
	result := &DeploymentSnippets{
		DockerRun:     dockerRun,
		DockerCompose: dockerCompose,
	}

	envName := ""
	if s != nil && s.db != nil {
		if env, getErr := s.GetEnvironmentByID(ctx, envID); getErr == nil && env != nil {
			envName = env.Name
		}
	}
	if edgeCfg != nil && strings.TrimSpace(edgeCfg.AppURL) == "" {
		edgeCfg.AppURL = managerURL
	}

	generatedAssets, mtlsDockerRun, mtlsDockerCompose := s.snippets.GenerateMTLS(ctx, edgeCfg, envID, envName, managerURL, apiKey)
	if generatedAssets == nil {
		return result, nil
	}

	files := make([]DeploymentSnippetFile, 0, len(generatedAssets.Files))
	for _, file := range generatedAssets.Files {
		files = append(files, DeploymentSnippetFile{
			Name:          file.Name,
			Content:       file.Content,
			ContainerPath: file.ContainerPath,
			Permissions:   file.Permissions,
		})
	}
	result.MTLS = &DeploymentSnippetMTLS{
		DockerRun:     mtlsDockerRun,
		DockerCompose: mtlsDockerCompose,
		Files:         files,
		HostDirHint:   strings.TrimSpace(generatedAssets.HostDirHint),
	}
	return result, nil
}
