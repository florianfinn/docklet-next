// Source: getarcaneapp/arcane/backend/internal/system/children/upgrade/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; frozen Compose self-update recovery contract and hub-owned dispatch

package upgrade

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/system"
)

type Handler struct{ service *Service }

func NewHandler(service *Service, environments *environment.EnvironmentService, _ *config.Config, _ context.Context) *Handler {
	if service != nil && environments != nil {
		service.SetEnvironments(environments)
	}
	return &Handler{service: service}
}

type CheckUpgradeInput struct {
	EnvironmentID string `path:"id"`
}
type CheckUpgradeOutput struct{ Body dto.SelfUpdateCheck }
type TriggerUpgradeInput struct {
	EnvironmentID string                  `path:"id"`
	Body          *dto.SelfUpdateEnvelope `doc:"Hub requests may select targetVersion only; agents require the authenticated frozen envelope"`
}
type TriggerUpgradeOutput struct{ Body dto.SelfUpdateResponse }

func (h *Handler) CheckUpgradeAvailable(ctx context.Context, input *CheckUpgradeInput) (*CheckUpgradeOutput, error) {
	if h.service == nil {
		return nil, huma.Error503ServiceUnavailable("update.check-unavailable")
	}
	out, err := h.service.Check(ctx, input.EnvironmentID)
	if err != nil {
		return nil, wireError(err)
	}
	return &CheckUpgradeOutput{Body: out}, nil
}
func (h *Handler) TriggerUpgrade(ctx context.Context, input *TriggerUpgradeInput) (*TriggerUpgradeOutput, error) {
	user, err := handlerutil.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if h.service == nil {
		return nil, huma.Error503ServiceUnavailable("update.check-unavailable")
	}
	request := dto.SelfUpdateEnvelope{}
	if input.Body != nil {
		request = *input.Body
	}
	out, err := h.service.StartUpdate(ctx, input.EnvironmentID, request, user.ID, user.Username)
	if err != nil {
		return nil, wireError(err)
	}
	return &TriggerUpgradeOutput{Body: out}, nil
}
func wireError(err error) error {
	var problem *core.Problem
	if errors.As(err, &problem) {
		return huma.NewError(problem.Status, problem.Code)
	}
	var status interface{ GetStatus() int }
	if errors.As(err, &status) {
		return huma.NewError(status.GetStatus(), err.Error())
	}
	return huma.Error503ServiceUnavailable("update.check-unavailable")
}
