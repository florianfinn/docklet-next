// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

// Package guard owns host-local image admission and persisted protection checks.
package guard

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/moby/moby/client"
)

const ReservedHost = "docklet-retention.invalid"

// Protection survives process restart; only the owning point/run releases it.
type Protection struct {
	ImageID string `gorm:"primaryKey;not null"`
	Reason  string `gorm:"primaryKey;not null"`
	OwnerID string `gorm:"primaryKey;not null"`
}

func (Protection) TableName() string { return "image_protections" }

type Service struct {
	DB               *database.DB
	Docker           func(context.Context) (*client.Client, error)
	Admission        *runs.Admission
	SystemReferences func() []string
	Removed          func(context.Context, string)
	Audit            func(context.Context, string, string, string, error)
}

func (s *Service) Acquire(ctx context.Context) (func(), error) {
	if s == nil || s.Admission == nil {
		return nil, imageDTO.Failure("image.busy", 409)
	}
	lease, ok, err := s.Admission.TryAcquire(ctx, scheduler.AdmissionKey{Scope: "image-mutation"})
	if err != nil || !ok {
		return nil, imageDTO.Failure("image.busy", 409)
	}
	return func() { lease.Release(context.WithoutCancel(ctx)) }, nil
}
func ValidateReference(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return imageDTO.Failure("image.invalid-reference", 400)
	}
	// Check the host before parsing too: mixed-case reserved hosts must not escape.
	if strings.EqualFold(strings.SplitN(raw, "/", 2)[0], ReservedHost) {
		return imageDTO.Failure("image.reserved-reference", 400)
	}
	named, err := reference.ParseNormalizedNamed(raw)
	if err != nil {
		return imageDTO.Failure("image.invalid-reference", 400)
	}
	if strings.EqualFold(reference.Domain(named), ReservedHost) {
		return imageDTO.Failure("image.reserved-reference", 400)
	}
	return nil
}
func PublicTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !strings.EqualFold(strings.SplitN(tag, "/", 2)[0], ReservedHost) {
			out = append(out, tag)
		}
	}
	return out
}
func (s *Service) Reasons(ctx context.Context, id string) ([]imageDTO.Protection, error) {
	if s.DB == nil {
		return nil, imageDTO.Failure("image.usage-unavailable", 503)
	}
	var pins []Protection
	if err := s.DB.WithContext(ctx).Where("image_id = ?", id).Find(&pins).Error; err != nil {
		return nil, imageDTO.Failure("image.usage-unavailable", 503)
	}
	out := make([]imageDTO.Protection, 0, len(pins))
	for _, pin := range pins {
		out = append(out, imageDTO.Protection{Reason: pin.Reason, OwnerID: pin.OwnerID})
	}
	return out, nil
}

// Protect/Release require the caller's image lease; they never infer settlement.
func (s *Service) Protect(ctx context.Context, p Protection) error {
	if p.ImageID == "" || p.OwnerID == "" || (p.Reason != "restore-point" && p.Reason != "active-update" && p.Reason != "system") {
		return imageDTO.Failure("image.invalid-reference", 400)
	}
	if s.DB == nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	if err := s.DB.WithContext(ctx).Where(p).FirstOrCreate(&p).Error; err != nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	return nil
}
func (s *Service) Release(ctx context.Context, p Protection) error {
	if s.DB == nil || p.ImageID == "" || p.OwnerID == "" || p.Reason == "" {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	if err := s.DB.WithContext(ctx).Where(&p).Delete(&Protection{}).Error; err != nil {
		return imageDTO.Failure("update.persistence-failed", 500)
	}
	return nil
}
func (s *Service) Check(ctx context.Context, c *client.Client, id string) error {
	reasons, err := s.Reasons(ctx, id)
	if err != nil {
		return err
	}
	for _, p := range reasons {
		code := "image.restore-point-protected"
		if p.Reason == "active-update" {
			code = "image.active-update-protected"
		}
		if p.Reason == "system" {
			code = "image.system-protected"
		}
		return imageDTO.Failure(code, 409)
	}
	containers, err := c.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return imageDTO.Failure("image.usage-unavailable", 503)
	}
	for _, item := range containers.Items {
		if item.ImageID == "" {
			return imageDTO.Failure("image.usage-unavailable", 503)
		}
		if item.ImageID == id {
			code := "image.in-use"
			if item.Labels[libarcane.InternalResourceLabel] == "true" || item.Labels["com.getarcaneapp.arcane"] == "true" {
				code = "image.system-protected"
			}
			return imageDTO.Failure(code, 409)
		}
	}
	if s.SystemReferences != nil {
		for _, ref := range s.SystemReferences() {
			if ref == "" {
				continue
			}
			v, e := c.ImageInspect(ctx, ref)
			if e != nil {
				if errdefs.IsNotFound(e) {
					continue
				}
				return imageDTO.Failure("image.usage-unavailable", 503)
			}
			if v.ID == id {
				return imageDTO.Failure("image.system-protected", 409)
			}
		}
	}
	return nil
}

// RemoveLocked resolves aliases to the full ID and rechecks before non-recursive removal.
func (s *Service) RemoveLocked(ctx context.Context, c *client.Client, ref string, actions ...string) (_ int64, resultErr error) {
	id := ""
	action := "delete"
	if len(actions) > 0 {
		action = actions[0]
	}
	defer func() {
		if s.Audit != nil {
			s.Audit(ctx, action, id, ref, resultErr)
		}
	}()
	inspect, err := c.ImageInspect(ctx, ref)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return 0, imageDTO.Failure("image.not-found", 404)
		}
		return 0, imageDTO.Failure("image.usage-unavailable", 503)
	}
	id = inspect.ID
	if inspect.ID == "" {
		return 0, imageDTO.Failure("image.usage-unavailable", 503)
	}
	if err = s.Check(ctx, c, inspect.ID); err != nil {
		return 0, err
	}
	again, err := c.ImageInspect(ctx, ref)
	if err != nil || again.ID != inspect.ID {
		return 0, imageDTO.Failure("image.changed", 409)
	}
	if _, err = c.ImageRemove(ctx, inspect.ID, client.ImageRemoveOptions{Force: false, PruneChildren: false}); err != nil {
		if errdefs.IsConflict(err) {
			return 0, imageDTO.Failure("image.in-use", 409)
		}
		return 0, imageDTO.Failure("image.remove-failed", 502)
	}
	if s.Removed != nil {
		s.Removed(context.WithoutCancel(ctx), inspect.ID)
	}
	return inspect.Size, nil
}
func (s *Service) Prune(ctx context.Context, options system.PruneImagesOptions) (_ *imageDTO.PruneReport, resultErr error) {
	defer func() {
		if resultErr != nil && s.Audit != nil {
			s.Audit(ctx, "bulk_prune", "", "", resultErr)
		}
	}()
	cutoff := time.Time{}
	switch options.Mode {
	case system.PruneImageModeAll, system.PruneImageModeDangling:
		if options.Until != "" {
			return nil, imageDTO.Failure("image.invalid-prune", 400)
		}
	case system.PruneImageModeOlderThan:
		if d, e := time.ParseDuration(options.Until); e == nil && d > 0 {
			cutoff = time.Now().Add(-d)
		} else if t, e := time.Parse(time.RFC3339, options.Until); e == nil {
			cutoff = t
		} else if seconds, e := strconv.ParseFloat(options.Until, 64); e == nil && seconds >= 0 && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds < 1<<63 {
			whole, fraction := math.Modf(seconds)
			cutoff = time.Unix(int64(whole), int64(fraction*1e9))
		} else {
			return nil, imageDTO.Failure("image.invalid-prune", 400)
		}
	default:
		return nil, imageDTO.Failure("image.invalid-prune", 400)
	}
	release, err := s.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	c, err := s.Docker(ctx)
	if err != nil {
		return nil, imageDTO.Failure("image.prune-failed", 502)
	}
	opts := client.ImageListOptions{All: false}
	if options.Mode == system.PruneImageModeDangling {
		opts.Filters = make(client.Filters).Add("dangling", "true")
	}
	list, err := c.ImageList(ctx, opts)
	if err != nil {
		return nil, imageDTO.Failure("image.prune-failed", 502)
	}
	out := &imageDTO.PruneReport{ImagesDeleted: []string{}, Skipped: []imageDTO.PruneItem{}, Failed: []imageDTO.PruneItem{}, SpaceReclaimedKnown: false}
	seen := map[string]bool{}
	for _, item := range list.Items {
		if seen[item.ID] || (!cutoff.IsZero() && !time.Unix(item.Created, 0).Before(cutoff)) {
			continue
		}
		seen[item.ID] = true
		_, err = s.RemoveLocked(ctx, c, item.ID, "bulk_prune")
		if err != nil {
			p := imageDTO.Neutral(err, "image.remove-failed", 502)
			entry := imageDTO.PruneItem{ID: item.ID, Code: p.Problem.Code}
			if p.Status == 409 || p.Status == 404 {
				out.Skipped = append(out.Skipped, entry)
			} else {
				out.Failed = append(out.Failed, entry)
			}
			continue
		}
		out.ImagesDeleted = append(out.ImagesDeleted, item.ID)
	}
	return out, nil
}

// Search terms have Docker's free-text grammar; only the private host is reserved.
func ValidateSearchTerm(term string) error {
	for _, word := range strings.Fields(term) {
		if strings.EqualFold(strings.SplitN(word, "/", 2)[0], ReservedHost) {
			return imageDTO.Failure("image.reserved-reference", 400)
		}
	}
	return nil
}
