// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original demand-driven widget flights, generation fences and stale backoff.

package widget

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitydto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	dto "github.com/florianfinn/docklet-next/types/widget"
)

type cacheKey struct {
	Kind       dto.Kind
	Revision   int64
	Generation uint64
}
type cacheEntry struct {
	Key      cacheKey
	Good     dto.Result
	Error    *dto.Problem
	Failures int
	Next     time.Time
}
type flight struct {
	Done   chan struct{}
	Result dto.Result
}

type Service struct {
	db         *database.DB
	activities activitylib.Service
	mu         sync.Mutex
	generation map[dto.Kind]uint64
	cache      map[dto.Kind]cacheEntry
	flights    map[cacheKey]*flight
	titles     *titleCache
	now        func() time.Time
}

func NewService(db *database.DB, activities activitylib.Service) *Service {
	return &Service{db: db, activities: activities, generation: map[dto.Kind]uint64{}, cache: map[dto.Kind]cacheEntry{}, flights: map[cacheKey]*flight{}, titles: newTitles(), now: time.Now}
}

func (s *Service) Config(ctx context.Context, kind dto.Kind) (dto.Config, error) {
	if !kind.Valid() {
		return dto.Config{}, dto.Failure(dto.InvalidConfig)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := load(ctx, s.db, kind)
	return r.config(), err
}
func (s *Service) List(ctx context.Context) ([]dto.Summary, error) {
	out := make([]dto.Summary, 0, 3)
	for _, kind := range []dto.Kind{dto.Plex, dto.Overseerr, dto.Minecraft} {
		c, err := s.Config(ctx, kind)
		if err != nil {
			return nil, err
		}
		out = append(out, dto.Summary{Kind: kind, Enabled: c.Enabled, Configured: c.Address != "", OpenURL: c.OpenURL, Stack: c.Stack})
	}
	return out, nil
}
func (s *Service) Save(ctx context.Context, kind dto.Kind, in dto.ConfigUpdate) (dto.Config, error) {
	if !kind.Valid() {
		return dto.Config{}, dto.Failure(dto.InvalidConfig)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := load(ctx, s.db, kind)
	if err != nil {
		return dto.Config{}, err
	}
	r, err := updatedRow(kind, old, in, s.now())
	if err != nil {
		return dto.Config{}, err
	}
	if err = save(ctx, s.db, r); err != nil {
		return dto.Config{}, err
	}
	s.invalidate(kind)
	return r.config(), nil
}

// invalidate runs under mu; flights retain their old key until completion.
func (s *Service) invalidate(kind dto.Kind) {
	s.generation[kind]++
	delete(s.cache, kind)
}
func ttl(kind dto.Kind) time.Duration {
	switch kind {
	case dto.Plex:
		return 15 * time.Second
	case dto.Minecraft:
		return 30 * time.Second
	default:
		return 60 * time.Second
	}
}
func clone(r dto.Result) dto.Result {
	b, _ := json.Marshal(r)
	var result dto.Result
	_ = json.Unmarshal(b, &result)
	return result
}
func failure(kind dto.Kind, err error) dto.Result { return dto.Result{Kind: kind, Error: problem(err)} }

func (s *Service) Get(ctx context.Context, kind dto.Kind) dto.Result {
	if !kind.Valid() {
		return failure(kind, dto.Failure(dto.InvalidConfig))
	}
	for {
		if ctx.Err() != nil {
			return failure(kind, transportProblem(ctx, ctx.Err()))
		}
		s.mu.Lock()
		r, err := load(ctx, s.db, kind)
		if err != nil {
			s.mu.Unlock()
			return failure(kind, err)
		}
		if r.Address == "" {
			s.mu.Unlock()
			return failure(kind, dto.Failure(dto.NotConfigured))
		}
		if !r.Enabled {
			s.mu.Unlock()
			return failure(kind, dto.Failure(dto.Disabled))
		}
		credential, err := secret(r)
		if err != nil {
			s.mu.Unlock()
			return failure(kind, err)
		}
		key := cacheKey{Kind: kind, Revision: r.Revision, Generation: s.generation[kind]}
		now := s.now()
		if cached, ok := s.cache[kind]; ok && cached.Key == key && now.Before(cached.Next) {
			result := cached.Good
			result.Kind = kind
			result.Error = cached.Error
			result.Stale = result.FetchedAt != nil && result.Error != nil
			if result.FetchedAt != nil {
				result.AgeSeconds = max(0, int64(now.Sub(*result.FetchedAt).Seconds()))
			}
			s.mu.Unlock()
			return clone(result)
		}
		f, ok := s.flights[key]
		if !ok {
			f = &flight{Done: make(chan struct{})}
			s.flights[key] = f
			go s.runFlight(context.WithoutCancel(ctx), r, credential, key, f)
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return failure(kind, transportProblem(ctx, ctx.Err()))
		case <-f.Done:
		}
		s.mu.Lock()
		current, err := load(ctx, s.db, kind)
		valid := err == nil && current.Revision == key.Revision && s.generation[kind] == key.Generation
		result := clone(f.Result)
		s.mu.Unlock()
		if err != nil {
			return failure(kind, err)
		}
		// A waiter never receives an old flight's data after a save or action.
		if valid {
			return result
		}
	}
}

func (s *Service) runFlight(parent context.Context, r row, credential string, key cacheKey, f *flight) {
	ctx, cancel := context.WithTimeout(parent, fetchBudget)
	defer cancel()
	result := fetch(ctx, r.config(), credential, key.Generation, s.titles, s.now())
	if result.Error != nil {
		result.Error.Certificate = nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.flights, key)
	current, err := load(parent, s.db, key.Kind)
	if err == nil && current.Revision == key.Revision && s.generation[key.Kind] == key.Generation {
		e := s.cache[key.Kind]
		if e.Key != key {
			e = cacheEntry{Key: key}
		}
		now := s.now()
		if result.Error == nil {
			e.Good = clone(result)
			e.Error = nil
			e.Failures = 0
			e.Next = now.Add(ttl(key.Kind))
		} else {
			e.Failures++
			e.Error = result.Error
			delays := []time.Duration{30 * time.Second, 60 * time.Second, 2 * time.Minute, 5 * time.Minute}
			e.Next = now.Add(delays[min(e.Failures-1, len(delays)-1)])
			result = clone(e.Good)
			result.Kind = key.Kind
			result.Error = e.Error
			result.Stale = result.FetchedAt != nil
			if result.FetchedAt != nil {
				result.AgeSeconds = max(0, int64(now.Sub(*result.FetchedAt).Seconds()))
			}
		}
		s.cache[key.Kind] = e
	}
	f.Result = clone(result)
	close(f.Done)
}

func fetch(ctx context.Context, c dto.Config, credential string, generation uint64, titles *titleCache, now time.Time) dto.Result {
	r := dto.Result{Kind: c.Kind}
	var err error
	if c.Kind == dto.Minecraft {
		r.Minecraft, err = fetchMinecraft(ctx, c.Address)
	} else {
		f := newFetchClient(c, credential)
		defer f.close()
		if c.Kind == dto.Plex {
			r.Plex, err = fetchPlex(ctx, f)
		} else {
			r.Overseerr, err = fetchOverseerr(ctx, f, c.Revision, generation, titles, now)
		}
	}
	if err != nil {
		return failure(c.Kind, err)
	}
	r.FetchedAt = &now
	return r
}

func (s *Service) Test(ctx context.Context, kind dto.Kind, in dto.ConfigUpdate) (dto.Result, error) {
	if !kind.Valid() {
		return dto.Result{}, dto.Failure(dto.InvalidConfig)
	}
	s.mu.Lock()
	old, err := load(ctx, s.db, kind)
	s.mu.Unlock()
	if err != nil {
		return dto.Result{}, err
	}
	r, err := updatedRow(kind, old, in, s.now())
	if err != nil {
		return dto.Result{}, err
	}
	credential, err := secret(r)
	if err != nil {
		return failure(kind, err), nil
	}
	if r.Address == "" {
		return failure(kind, dto.Failure(dto.NotConfigured)), nil
	}
	ctx, cancel := context.WithTimeout(ctx, fetchBudget)
	defer cancel()
	return fetch(ctx, r.config(), credential, 0, newTitles(), s.now()), nil
}

func (s *Service) Action(ctx context.Context, id int64, action string, actor *user.Actor) (dto.ActionResult, error) {
	if id <= 0 || (action != "approve" && action != "decline") {
		return dto.ActionResult{}, dto.Failure(dto.InvalidConfig)
	}
	s.mu.Lock()
	r, err := load(ctx, s.db, dto.Overseerr)
	s.mu.Unlock()
	if err != nil {
		return dto.ActionResult{}, err
	}
	if r.Address == "" {
		return dto.ActionResult{}, dto.Failure(dto.NotConfigured)
	}
	if !r.Enabled {
		return dto.ActionResult{}, dto.Failure(dto.Disabled)
	}
	credential, err := secret(r)
	if err != nil {
		return dto.ActionResult{}, err
	}
	if s.activities == nil {
		return dto.ActionResult{}, dto.Failure(dto.PersistenceFailed)
	}
	resource := "service-widget"
	resourceID := strconv.FormatInt(id, 10)
	name := "Overseerr request"
	started, err := s.activities.StartActivity(ctx, activitylib.StartRequest{EnvironmentID: "0", Type: activitydto.TypeResourceAction, ResourceType: &resource, ResourceID: &resourceID, ResourceName: &name, StartedBy: actor, LatestMessage: "Overseerr request " + action, Metadata: database.JSON{"widget": "overseerr", "action": action, "requestId": id}})
	if err != nil {
		return dto.ActionResult{}, dto.Failure(dto.PersistenceFailed)
	}
	actionCtx, cancel := context.WithTimeout(ctx, fetchBudget)
	f := newFetchClient(r.config(), credential)
	err = f.request(actionCtx, "POST", "/api/v1/request/"+resourceID+"/"+action, nil)
	f.close()
	cancel()
	// An attempted action may have reached the producer even if its reply failed.
	s.mu.Lock()
	s.invalidate(dto.Overseerr)
	s.mu.Unlock()
	status := activitydto.StatusSuccess
	message := "Overseerr request " + action
	var errorText *string
	if err != nil {
		status = activitydto.StatusFailed
		p := problem(err)
		p.Certificate = nil
		err = p
		message = string(p.Code)
		errorText = &message
	}
	auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(ctx), fetchBudget)
	defer auditCancel()
	if _, auditErr := s.activities.CompleteActivity(auditCtx, started.ID, status, message, errorText); auditErr != nil {
		return dto.ActionResult{ActivityID: started.ID}, dto.Failure(dto.PersistenceFailed)
	}
	return dto.ActionResult{ActivityID: started.ID}, err
}
