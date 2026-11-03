// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original cancellation-bound, revision-fenced hub history sampler.

package history

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/history"
	"gorm.io/gorm"
)

type hostState struct {
	revision string
	minute   int64
	previous *dto.Sample
	restarts map[string][]restartPoint
	episodes map[string]episode
}
type Service struct {
	db            *database.DB
	producer      *Producer
	environments  *environment.EnvironmentService
	settings      *settings.SettingsService
	events        *event.EventService
	fetch         func(context.Context, environment.Environment) (dto.Sample, error)
	budget        time.Duration
	mu            sync.Mutex
	inFlight      map[string]bool
	states        map[string]hostState
	snapshots     snapshots
	skipped       atomic.Uint64
	beforePublish func()
}

func NewService(db *database.DB, p *Producer, e *environment.EnvironmentService, s *settings.SettingsService, events *event.EventService) *Service {
	h := &Service{db: db, producer: p, environments: e, settings: s, events: events, budget: 30 * time.Second, inFlight: map[string]bool{}, states: map[string]hostState{}}
	h.fetch = h.fetchSample
	return h
}
func hostRevision(h environment.Environment) string {
	// Credentials can be changed only with the revision; include endpoint and mode explicitly.
	return h.ApiUrl + "\x00" + time.Unix(h.CredentialRevision, 0).Format(time.RFC3339Nano) + "\x00" + boolString(h.IsEdge)
}
func boolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
func (s *Service) fetchSample(ctx context.Context, host environment.Environment) (dto.Sample, error) {
	if host.ID == "0" {
		return s.producer.Sample(ctx)
	}
	var response base.ApiResponse[dto.Sample]
	err := s.environments.ProxyJSONRequestForEnvironment(ctx, host, "GET", dto.SamplePath, nil, &response)
	if err != nil {
		return dto.Sample{}, err
	}
	if !response.Success {
		return dto.Sample{}, dto.Fail("history.sample-invalid", 502)
	}
	return response.Data, Validate(response.Data)
}
func (s *Service) SampleAll(ctx context.Context, now time.Time) error {
	hosts := []environment.Environment{}
	if err := s.db.WithContext(ctx).Find(&hosts).Error; err != nil {
		return err
	}
	active := map[string]bool{}
	var wg sync.WaitGroup
	for _, host := range hosts {
		if !host.Enabled {
			s.Forget(host.ID)
			continue
		}
		active[host.ID] = true
		wg.Go(func() {
			if err := s.SampleHost(ctx, host, now); err != nil && ctx.Err() == nil {
				slog.WarnContext(ctx, "History sample failed", "code", "history.unavailable", "hostId", host.ID)
			}
		})
	}
	s.mu.Lock()
	for id := range s.states {
		if !active[id] {
			delete(s.states, id)
			s.snapshots.forget(id)
		}
	}
	s.mu.Unlock()
	wg.Wait()
	return ctx.Err()
}
func (s *Service) SampleHost(ctx context.Context, host environment.Environment, now time.Time) error {
	s.mu.Lock()
	if s.inFlight[host.ID] {
		s.skipped.Add(1)
		s.mu.Unlock()
		return nil
	}
	s.inFlight[host.ID] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.inFlight, host.ID); s.mu.Unlock() }()
	sampleCtx, cancel := context.WithTimeout(ctx, s.budget)
	defer cancel()
	sample, fetchErr := s.fetch(sampleCtx, host)
	if fetchErr == nil {
		fetchErr = sampleCtx.Err()
	}
	if fetchErr == nil {
		fetchErr = Validate(sample)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Timeout is a down minute, but commit uses the parent context after the fetch joins.
	return s.commit(ctx, host, now.Truncate(time.Minute), sample, fetchErr)
}
func (s *Service) commit(ctx context.Context, host environment.Environment, now time.Time, sample dto.Sample, fetchErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	revision := hostRevision(host)
	old := s.states[host.ID]
	if old.revision != revision {
		old = hostState{revision: revision, restarts: map[string][]restartPoint{}, episodes: map[string]episode{}}
		s.snapshots.forget(host.ID)
	}
	next := hostState{revision: revision, minute: now.Unix(), restarts: map[string][]restartPoint{}, episodes: map[string]episode{}}
	for k, v := range old.episodes {
		next.episodes[k] = v
	}
	conditions := dto.Conditions{HostID: host.ID, SampledAt: now, Items: []dto.Condition{}}
	observations := []observation{{kind: "host", key: host.ID, state: dto.Down}}
	if fetchErr == nil {
		observations, conditions.Items = s.observations(host.ID, now, sample, old, &next)
		next.previous = cloneSample(sample)
	}
	committed := false
	invalidated := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current environment.Environment
		if err := tx.Where("id=?", host.ID).Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				invalidated = true
				return nil
			}
			return err
		}
		if !current.Enabled || hostRevision(current) != revision {
			invalidated = true
			return nil
		}
		var latest int64
		if err := tx.Model(&bucketRow{}).Where("kind='host' AND host_id=?", host.ID).Select("COALESCE(MAX(last_minute),0)").Scan(&latest).Error; err != nil {
			return err
		}
		if now.Unix() <= latest || now.Unix() <= old.minute {
			return nil
		}
		for _, o := range observations {
			if err := upsertMinute(tx, host.ID, now.Unix(), o); err != nil {
				return err
			}
		}
		if err := s.projectTransitions(tx, host.ID, now, conditions.Items, s.snapshots.read(host.ID)); err != nil {
			return err
		}
		if err := s.projectLoad(tx, host, now, observations, &next); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if err != nil {
		return err
	}
	if !committed {
		if invalidated {
			delete(s.states, host.ID)
			s.snapshots.forget(host.ID)
		}
		return nil
	}
	if s.beforePublish != nil {
		s.beforePublish()
	}
	var current environment.Environment
	if err := s.db.WithContext(ctx).Where("id=?", host.ID).Take(&current).Error; err != nil || !current.Enabled || hostRevision(current) != revision {
		delete(s.states, host.ID)
		s.snapshots.forget(host.ID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return nil
	}
	s.states[host.ID] = next
	s.snapshots.publish(host.ID, conditions)
	if s.events != nil {
		s.events.NotifyCommitted()
	}
	return nil
}
func (s *Service) Forget(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, id)
	s.snapshots.forget(id)
}
func (s *Service) SkippedSamples() uint64                          { return s.skipped.Load() }
func (s *Service) Retain(ctx context.Context, now time.Time) error { return retain(ctx, s.db, now) }

func (s *Service) Conditions(ctx context.Context, id string) (dto.Conditions, error) {
	var host environment.Environment
	if err := s.db.WithContext(ctx).Where("id=?", id).Take(&host).Error; err != nil {
		s.Forget(id)
		return dto.Conditions{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[id]
	if !host.Enabled || state.revision != hostRevision(host) {
		delete(s.states, id)
		s.snapshots.forget(id)
	}
	result := s.snapshots.read(id)
	result.HostID = id
	return result, nil
}
