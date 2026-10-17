// Source: getarcaneapp/arcane/backend/internal/environment/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories; functional cut: removed S3 backup destinations.

package environment

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	"github.com/florianfinn/docklet-next/types/environment"
	"go.getarcane.app/kit/pkg"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DownloadEdgeMTLSCA downloads the Arcane-managed edge mTLS CA certificate.
func (h *EnvironmentHandler) DownloadEdgeMTLSCA(ctx context.Context, _ *DownloadEdgeMTLSCAInput) (*huma.StreamResponse, error) {
	var edgeCfg *edge.Config
	if h.cfg != nil {
		edgeCfg = &edge.Config{
			EdgeMTLSMode:      h.cfg.EdgeMTLSMode,
			EdgeMTLSCAFile:    h.cfg.EdgeMTLSCAFile,
			EdgeMTLSAssetsDir: h.cfg.EdgeMTLSAssetsDir,
		}
	}
	caPath, err := edge.AvailableManagerMTLSCAPath(edgeCfg)
	if err != nil {
		return nil, huma.Error404NotFound("Arcane-managed edge mTLS CA is not available")
	}

	// os.* rather than acfs: the CA path may resolve to a user-configured
	// location anywhere on the host, so no confinement root exists for it.
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to read generated edge mTLS CA", "path", caPath, "error", err.Error())
		return nil, huma.Error500InternalServerError("Failed to read Arcane-generated edge mTLS CA")
	}

	fileName := filepath.Base(caPath)
	if strings.TrimSpace(fileName) == "" {
		fileName = "ca.crt"
	}

	return &huma.StreamResponse{
		Body: func(humaCtx huma.Context) { //nolint:contextcheck // context is obtained from humaCtx.Context()
			humaCtx.SetHeader("Content-Type", "application/x-pem-file")
			humaCtx.SetHeader("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
			humaCtx.SetHeader("Content-Length", strconv.Itoa(len(caPEM)))

			if written, writeErr := humaCtx.BodyWriter().Write(bytes.Clone(caPEM)); writeErr != nil || written != len(caPEM) {
				slog.WarnContext(humaCtx.Context(), "Failed to stream edge mTLS CA download", "fileName", fileName, "bytesWritten", written, "bytesExpected", len(caPEM), "error", writeErr)
				return
			}
			h.logMTLSAuditEventInternal(humaCtx.Context(), nil, event.EventTypeEnvironmentMTLSDownload,
				"mTLS CA downloaded",
				fmt.Sprintf("Administrator downloaded edge mTLS CA %q", fileName),
				database.JSON{
					"fileName": fileName,
					"kind":     "ca",
				})
		},
	}, nil
}

func (h *EnvironmentHandler) DownloadEnvironmentMTLSBundle(ctx context.Context, input *DownloadEnvironmentMTLSBundleInput) (*huma.StreamResponse, error) {
	env, files, err := h.loadEnvironmentMTLSFilesInternal(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)

	for _, file := range files {
		downloadName := environmentMTLSAssetDownloadNameInternal(env, file.Name)
		header := &zip.FileHeader{
			Name:   downloadName,
			Method: zip.Deflate,
		}
		header.SetMode(environmentMTLSAssetFileModeInternal(file))

		entry, createErr := zipWriter.CreateHeader(header)
		if createErr != nil {
			slog.ErrorContext(ctx, "Failed to create mTLS bundle entry", "environmentId", input.ID, "fileName", downloadName, "error", createErr.Error())
			return nil, huma.Error500InternalServerError("Failed to build mTLS bundle")
		}

		if _, writeErr := entry.Write([]byte(file.Content)); writeErr != nil {
			slog.ErrorContext(ctx, "Failed to write mTLS bundle entry", "environmentId", input.ID, "fileName", downloadName, "error", writeErr.Error())
			return nil, huma.Error500InternalServerError("Failed to build mTLS bundle")
		}
	}

	if closeErr := zipWriter.Close(); closeErr != nil {
		slog.ErrorContext(ctx, "Failed to finalize mTLS bundle", "environmentId", input.ID, "error", closeErr.Error())
		return nil, huma.Error500InternalServerError("Failed to build mTLS bundle")
	}

	fileName := environmentMTLSDownloadBaseNameInternal(env) + "-mtls.zip"

	return &huma.StreamResponse{
		Body: func(humaCtx huma.Context) { //nolint:contextcheck // context is obtained from humaCtx.Context()
			humaCtx.SetHeader("Content-Type", "application/zip")
			humaCtx.SetHeader("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
			humaCtx.SetHeader("Content-Length", strconv.Itoa(archive.Len()))

			if written, writeErr := humaCtx.BodyWriter().Write(archive.Bytes()); writeErr != nil || written != archive.Len() {
				slog.WarnContext(
					humaCtx.Context(),
					"Failed to stream edge mTLS bundle download",
					"environmentId",
					input.ID,
					"fileName",
					fileName,
					"bytesWritten",
					written,
					"bytesExpected",
					archive.Len(),
					"error",
					writeErr,
				)
				return
			}
			h.logMTLSAuditEventInternal(humaCtx.Context(), env, event.EventTypeEnvironmentMTLSDownload,
				"mTLS bundle downloaded",
				fmt.Sprintf("Administrator downloaded edge mTLS bundle %q (%d files)", fileName, len(files)),
				database.JSON{
					"fileName":  fileName,
					"kind":      "bundle",
					"fileCount": len(files),
				})
		},
	}, nil
}

func (h *EnvironmentHandler) DownloadEnvironmentMTLSFile(ctx context.Context, input *DownloadEnvironmentMTLSFileInput) (*huma.StreamResponse, error) {
	env, file, err := h.loadEnvironmentMTLSFileInternal(ctx, input.ID, input.FileName)
	if err != nil {
		return nil, err
	}

	fileContent := []byte(file.Content)
	downloadName := environmentMTLSAssetDownloadNameInternal(env, file.Name)

	return &huma.StreamResponse{
		Body: func(humaCtx huma.Context) { //nolint:contextcheck // context is obtained from humaCtx.Context()
			humaCtx.SetHeader("Content-Type", "application/x-pem-file")
			humaCtx.SetHeader("Content-Disposition", fmt.Sprintf("attachment; filename=%q", downloadName))
			humaCtx.SetHeader("Content-Length", strconv.Itoa(len(fileContent)))

			if written, writeErr := humaCtx.BodyWriter().Write(fileContent); writeErr != nil || written != len(fileContent) {
				slog.WarnContext(
					humaCtx.Context(),
					"Failed to stream edge mTLS asset download",
					"environmentId",
					input.ID,
					"fileName",
					file.Name,
					"bytesWritten",
					written,
					"bytesExpected",
					len(
						fileContent,
					),
					"error",
					writeErr,
				)
				return
			}
			h.logMTLSAuditEventInternal(humaCtx.Context(), env, event.EventTypeEnvironmentMTLSDownload,
				"mTLS asset downloaded",
				fmt.Sprintf("Administrator downloaded edge mTLS asset %q", file.Name),
				database.JSON{
					"fileName":  file.Name,
					"kind":      "file",
					"sensitive": isSensitiveMTLSAssetNameInternal(file.Name),
				})
		},
	}, nil
}

func (h *EnvironmentHandler) loadEnvironmentMTLSEnvironmentInternal(ctx context.Context, environmentID string) (*Environment, error) {
	env, err := h.environmentService.GetEnvironmentByID(ctx, environmentID)
	if err != nil {
		return nil, huma.Error404NotFound("Environment not found")
	}

	if !env.IsEdge {
		return nil, huma.Error400BadRequest("Environment is not an edge agent")
	}

	if env.ApiKeyID == nil {
		return nil, huma.Error400BadRequest("Environment does not have an API key configured")
	}

	if env.AccessToken == nil || *env.AccessToken == "" {
		return nil, huma.Error400BadRequest("Environment is missing access token")
	}

	return env, nil
}

func (h *EnvironmentHandler) loadEnvironmentMTLSFilesInternal(ctx context.Context, environmentID string) (*Environment, []DeploymentSnippetFile, error) {
	env, err := h.loadEnvironmentMTLSEnvironmentInternal(ctx, environmentID)
	if err != nil {
		return nil, nil, err
	}

	snippets, err := h.environmentService.GenerateEdgeDeploymentSnippets(ctx, env.ID, h.cfg.GetAppURL(), *env.AccessToken, &edge.Config{
		EdgeMTLSMode:      h.cfg.EdgeMTLSMode,
		EdgeMTLSCAFile:    h.cfg.EdgeMTLSCAFile,
		EdgeMTLSAssetsDir: h.cfg.EdgeMTLSAssetsDir,
		AppURL:            h.cfg.GetAppURL(),
	})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to generate environment mTLS assets", "environmentId", environmentID, "error", err.Error())
		return nil, nil, huma.Error500InternalServerError("Failed to generate environment mTLS assets")
	}

	if snippets.MTLS == nil || len(snippets.MTLS.Files) == 0 {
		return nil, nil, huma.Error404NotFound("mTLS assets are not available for this environment")
	}

	return env, snippets.MTLS.Files, nil
}

func (h *EnvironmentHandler) loadEnvironmentMTLSFileInternal(ctx context.Context, environmentID, fileName string) (*Environment, DeploymentSnippetFile, error) {
	env, files, err := h.loadEnvironmentMTLSFilesInternal(ctx, environmentID)
	if err != nil {
		return nil, DeploymentSnippetFile{}, err
	}

	for _, file := range files {
		if file.Name == fileName {
			return env, file, nil
		}
	}

	return nil, DeploymentSnippetFile{}, huma.Error404NotFound("Requested mTLS asset was not found")
}

// isSensitiveMTLSAssetNameInternal reports whether the given generated asset
// filename contains secret material (currently just the agent private key).
// Sensitive asset contents must not be returned inline in JSON responses; the
// client should fetch them via the admin-only download endpoint instead.
func isSensitiveMTLSAssetNameInternal(fileName string) bool {
	name := strings.ToLower(strings.TrimSpace(fileName))
	return strings.HasSuffix(name, ".key") || strings.HasSuffix(name, "-key.pem") || strings.HasSuffix(name, "_key.pem")
}

func environmentMTLSDownloadBaseNameInternal(env *Environment) string {
	baseName := cmp.Or(strings.TrimSpace(env.Name), "environment")

	baseName = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, baseName)

	baseName = cmp.Or(strings.Trim(baseName, "-"), "environment")

	return baseName + "-" + env.ID
}

func environmentMTLSAssetDownloadNameInternal(env *Environment, fileName string) string {
	baseName := environmentMTLSDownloadBaseNameInternal(env)

	switch fileName {
	case "agent.crt":
		return baseName + ".pem"
	case "agent.key":
		return baseName + ".key"
	default:
		return fileName
	}
}

func environmentMTLSAssetFileModeInternal(file DeploymentSnippetFile) os.FileMode {
	if parsed, err := strconv.ParseUint(strings.TrimSpace(file.Permissions), 8, 32); err == nil && parsed != 0 {
		return os.FileMode(parsed)
	}
	return kit.Ternary[os.FileMode](isSensitiveMTLSAssetNameInternal(file.Name), 0o600, 0o644)
}

// logMTLSAuditEventInternal records an audit event for administrator-triggered
// edge mTLS actions (downloads, bundle exports). Must never include raw
// certificate content or private key material.
func (h *EnvironmentHandler) logMTLSAuditEventInternal(ctx context.Context, env *Environment, eventType event.EventType, title, description string, extra database.JSON) {
	if h == nil || h.eventService == nil {
		return
	}

	user, _ := userctx.CurrentUserFromContext(ctx)
	var userID, username *string
	if user != nil {
		userID = new(user.ID)
		username = new(user.Username)
	}

	if extra == nil {
		extra = database.JSON{}
	}
	if remoteAddr := strings.TrimSpace(middleware.GetRemoteAddrFromContext(ctx)); remoteAddr != "" {
		extra["remoteAddr"] = remoteAddr
	}

	req := event.CreateEventRequest{
		Type:        eventType,
		Severity:    event.EventSeverityInfo,
		Title:       title,
		Description: description,
		UserID:      userID,
		Username:    username,
		Metadata:    extra,
	}
	if env != nil {
		envID := env.ID
		req.ResourceType = new("environment")
		req.ResourceID = &envID
		req.ResourceName = new(env.Name)
		req.EnvironmentID = &envID
	}

	if _, err := h.eventService.CreateEvent(ctx, req); err != nil {
		slog.WarnContext(ctx, "Failed to record mTLS audit event", "type", string(eventType), "error", err)
	}
}

const edgeMTLSCertificateExpiryWarningWindow = 30 * 24 * time.Hour

func generatedEdgeMTLSClientCertPathInternal(cfg *config.Config, envID string) (string, error) {
	if cfg == nil {
		return "", errors.New("config not available")
	}
	if edge.NormalizeEdgeMTLSMode(cfg.EdgeMTLSMode) == edge.EdgeMTLSModeDisabled {
		return "", errors.New("edge mTLS is disabled")
	}

	edgeCfg := &edge.Config{
		EdgeMTLSAssetsDir: cfg.EdgeMTLSAssetsDir,
	}

	certPath, err := edge.GeneratedManagerClientMTLSCertPath(edgeCfg, envID)
	if err != nil {
		return "", fmt.Errorf("resolve generated edge mTLS client certificate path: %w", err)
	}
	// os.* rather than acfs: the assets dir may be user-configured to anywhere
	// on the host, so no confinement root exists for this path.
	if _, statErr := os.Stat(certPath); statErr != nil {
		return "", fmt.Errorf("stat generated edge mTLS client certificate: %w", statErr)
	}

	return certPath, nil
}

func readGeneratedEdgeMTLSCertificateInfoInternal(cfg *config.Config, envID string) (*environment.EdgeMTLSCertificate, error) {
	certPath, err := generatedEdgeMTLSClientCertPathInternal(cfg, envID)
	if err != nil {
		return nil, err
	}

	// os.* rather than acfs: the assets dir may be user-configured to anywhere
	// on the host, so no confinement root exists for this path.
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read generated edge mTLS client certificate: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, errors.New("decode generated edge mTLS client certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse generated edge mTLS client certificate: %w", err)
	}

	expiresAt := cert.NotAfter.UTC()
	now := time.Now().UTC()
	remaining := expiresAt.Sub(now)
	info := &environment.EdgeMTLSCertificate{
		ExpiresAt:     &expiresAt,
		DaysRemaining: new(edgeMTLSCertificateDaysRemainingInternal(now, expiresAt)),
		Expired:       now.After(expiresAt),
		ExpiringSoon:  now.Before(expiresAt) && remaining <= edgeMTLSCertificateExpiryWarningWindow,
	}

	if commonName := strings.TrimSpace(cert.Subject.CommonName); commonName != "" {
		info.CommonName = &commonName
	}

	return info, nil
}

func edgeMTLSCertificateDaysRemainingInternal(now, expiresAt time.Time) int {
	remaining := expiresAt.Sub(now)
	if remaining <= 0 {
		return 0
	}
	return int(math.Ceil(remaining.Hours() / 24))
}
