// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

package proxy

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/modules"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"gorm.io/gorm"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

// Agent serializes admission and receipts; durable source uniqueness also covers
// recovery-required operations and operations on different files/instances.
type Agent struct {
	DB           *gorm.DB
	Gate         *modules.Service
	Runtime      Runtime
	Engines      Engines
	generationID atomic.Value
	mu           sync.Mutex
	active       map[string]bool
	afterCommit  func(dto.Phase) error
	afterEffect  func(dto.Phase) error
}

func NewAgent(db *gorm.DB, r Runtime, e Engines) (*Agent, error) {
	a := &Agent{DB: db, Runtime: r, Engines: e, active: map[string]bool{}}
	err := a.ReloadGeneration(context.Background())
	return a, err
}

// Generation identifies the current journal lifetime, including explicit data reset.
func (a *Agent) Generation() string { return a.generationID.Load().(string) }
func (a *Agent) ReloadGeneration(ctx context.Context) error {
	var g string
	if err := a.DB.WithContext(ctx).Table("proxy_generation").Where("id = 1").Select("generation").Scan(&g).Error; err != nil {
		return err
	}
	if g == "" {
		return dto.Fail(dto.JournalLost, 409)
	}
	a.generationID.Store(g)
	return nil
}
func databaseGeneration(tx *gorm.DB, expected string) (string, error) {
	var g string
	if err := tx.Table("proxy_generation").Where("id = 1").Select("generation").Scan(&g).Error; err != nil {
		return g, err
	}
	if g != expected || g == "" {
		p := dto.Fail(dto.GenerationMismatch, 409)
		p.Generation = g
		return g, p
	}
	return g, nil
}
func (a *Agent) generation(g string) error {
	if g != a.Generation() {
		p := dto.Fail(dto.GenerationMismatch, 409)
		p.Generation = a.Generation()
		return p
	}
	return nil
}
func (a *Agent) Check(ctx context.Context, i dto.Instance) error {
	if a.Runtime == nil {
		return dto.Fail(dto.EngineUnavailable, 503)
	}
	return a.Runtime.Check(ctx, i)
}
func (a *Agent) Apply(ctx context.Context, q dto.ApplyRequest) (dto.Operation, error) {
	if err := validRequest(q); err != nil {
		return dto.Operation{}, err
	}
	digest, err := Digest(q)
	if err != nil {
		return dto.Operation{}, err
	}
	a.mu.Lock()
	if err = a.generation(q.Generation); err != nil {
		a.mu.Unlock()
		return dto.Operation{}, err
	}
	if q.Registration != q.Instance.Registration {
		a.mu.Unlock()
		return dto.Operation{}, dto.Fail(dto.RegistrationChanged, 409)
	}
	j, err := findJournal(ctx, a.DB, q.ID)
	if err == nil {
		a.mu.Unlock()
		_, o, e := decodeJournal(j)
		if e != nil {
			return o, e
		}
		if j.Digest != digest {
			return o, dto.Fail(dto.OperationConflict, 409)
		}
		// A recorded receipt is returned whatever the container's current identity.
		return o, nil
	}
	if !missing(err) {
		a.mu.Unlock()
		return dto.Operation{}, err
	}
	var f fenceRow
	err = a.DB.WithContext(ctx).First(&f, "id = ?", q.ID).Error
	if err == nil {
		a.mu.Unlock()
		return dto.Operation{}, dto.Fail(dto.Fenced, 409)
	}
	if !missing(err) {
		a.mu.Unlock()
		return dto.Operation{}, err
	}
	// Admission rows outlive the pruned journal, so a replayed ID stays refused.
	var admitted admissionRow
	if err = a.DB.WithContext(ctx).First(&admitted, "id = ?", q.ID).Error; err == nil {
		a.mu.Unlock()
		return dto.Operation{}, dto.Fail(dto.OperationConflict, 409)
	} else if !missing(err) {
		a.mu.Unlock()
		return dto.Operation{}, err
	}
	if err = a.Check(ctx, q.Instance); err != nil {
		a.mu.Unlock()
		return dto.Operation{}, err
	}
	if err = Eligibility(q.Instance); err != nil {
		a.mu.Unlock()
		return dto.Operation{}, err
	}
	e, err := a.Engines.For(q.Instance.Kind)
	if err != nil {
		a.mu.Unlock()
		return dto.Operation{}, err
	}
	i := Intent{Request: q, Old: Bytes{FileState: q.Expected}, New: Bytes{FileState: q.Expected}}
	paths(&i)
	o := dto.Operation{ID: q.ID, InstanceID: q.Instance.ID, Generation: a.Generation(), Registration: q.Registration, Digest: digest, State: dto.Pending, Phase: dto.Admitted, Target: q.Target, Old: q.Expected, New: q.Expected, CreatedAt: time.Now().UTC()}
	ib, err := marshal(i)
	if err != nil {
		a.mu.Unlock()
		return o, err
	}
	ob, err := marshal(o)
	if err != nil {
		a.mu.Unlock()
		return o, err
	}
	source := "host\x00" + q.Instance.Source.Canonical
	commit := func(ctx context.Context) error {
		if er := a.generation(q.Generation); er != nil {
			return er
		}
		return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if _, er := databaseGeneration(tx, q.Generation); er != nil {
				return er
			}
			var busy Journal
			er := tx.Where("source = ? AND finished_at IS NULL", source).First(&busy).Error
			if er == nil {
				return &dto.Problem{Status: 409, Code: dto.ModuleBusy, OperationID: busy.ID}
			}
			if !missing(er) {
				return er
			}
			if er = tx.Create(&admissionRow{ID: q.ID, Generation: a.Generation(), Registration: q.Registration}).Error; er != nil {
				return er
			}
			return tx.Create(&Journal{ID: q.ID, Source: source, Generation: a.Generation(), Digest: digest, Registration: q.Registration, State: o.State, Phase: o.Phase, Intent: ib, Result: ob, CreatedAt: o.CreatedAt}).Error
		})
	}
	if a.Gate != nil {
		err = a.Gate.Admit(ctx, features.Proxy, commit)
	} else {
		err = commit(ctx)
	}
	if err == nil {
		a.active[q.ID] = true
	}
	a.mu.Unlock()
	if err != nil {
		return o, err
	}
	defer a.inactive(q.ID)
	// Admitted work belongs to the journal: a cancelled caller never changes its
	// outcome, and a process exit is settled at startup by the recovery table.
	ctx = context.WithoutCancel(ctx)
	if err = a.faultCommit(dto.Admitted); err != nil {
		return o, err
	}
	prepared, err := e.Prepare(ctx, q)
	if Transient(err) {
		return o, err // a failed read is no graph refusal; recovery rejects an unchanged target
	}
	if err != nil {
		return a.finish(ctx, i, withDiagnostic(o, err), dto.Rejected, reason(err, dto.InvalidGraph), "")
	}
	// The request, paths and baseline are protocol-owned, never engine-selected.
	prepared.Request = q
	paths(&prepared)
	i = prepared
	if i.Old.FileState != q.Expected {
		return a.finish(ctx, i, o, dto.Rejected, dto.StaleHash, "")
	}
	if i.Old.FileState != Snapshot(i.Old.Content, i.Old.Present).FileState || i.New.FileState != Snapshot(i.New.Content, i.New.Present).FileState || !equalGraph(i.GraphHashes, q.GraphHashes) {
		return a.finish(ctx, i, o, dto.Rejected, dto.InvalidInput, "")
	}
	if (i.Restart || i.Reload) && q.Instance.Kind != dto.Caddy || i.Restart && i.Reload || q.Instance.Kind == dto.Traefik && !i.New.Present {
		return a.finish(ctx, i, o, dto.Rejected, dto.InvalidInput, "")
	}
	if i.Mode&0044 == 0 || i.Mode&07000 != 0 {
		return a.finish(ctx, i, o, dto.Rejected, dto.UnsupportedMetadata, "")
	}
	if i.Restart && !q.Acknowledgements.Restart {
		return a.finish(ctx, i, o, dto.Rejected, dto.AcknowledgementRequired, "")
	}
	if i.Restart {
		_, i.StartedBefore, err = a.Runtime.Epoch(ctx, q.Instance)
		if Transient(err) {
			return o, err // nothing written yet; recovery rejects an unchanged target
		}
		if err != nil {
			return a.finish(ctx, i, o, dto.Rejected, reason(err, dto.ContainerChanged), "")
		}
	}
	var owned verifiedFile
	ownedErr := a.DB.WithContext(ctx).First(&owned, "source = ? AND path = ?", "host\x00"+q.Instance.Source.Canonical, q.Target).Error
	if ownedErr != nil && !missing(ownedErr) {
		return o, ownedErr
	}
	i.Owned = ownedErr == nil || !i.Old.Present && i.New.Present
	o.Old = i.Old.FileState
	o.New = i.New.FileState
	if err = a.save(ctx, i, o); err != nil {
		return o, err
	}
	if err = a.effect(ctx, i, &o, dto.Staging, func() error {
		if er := a.Runtime.Stage(ctx, i); er != nil {
			return er
		}
		return e.Validate(ctx, i)
	}); err != nil {
		return a.effectFailure(ctx, i, o, err, false)
	}
	i.PublishedAt = time.Now().UTC()
	if err = a.effect(ctx, i, &o, dto.Publishing, func() error {
		if er := a.recheck(ctx, i); er != nil {
			return er
		}
		return a.Runtime.Publish(ctx, i)
	}); err != nil {
		return a.effectFailure(ctx, i, o, err, true)
	}
	return a.activateObserve(ctx, i, o, e, true)
}
func (a *Agent) inactive(id string) { a.mu.Lock(); delete(a.active, id); a.mu.Unlock() }

// MaxDiagnostic bounds stored engine text; longer text is cut on a rune boundary.
const MaxDiagnostic = 4096

// diagnostic keeps the location and bounded text of a typed engine refusal.
func diagnostic(err error) *dto.Diagnostic {
	p, ok := errors.AsType[*dto.Problem](err)
	if !ok || p.Path == "" && p.Line <= 0 && p.Detail == "" {
		return nil
	}
	detail := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, strings.ToValidUTF8(p.Detail, ""))
	if len(detail) > MaxDiagnostic {
		cut := MaxDiagnostic
		for cut > 0 && !utf8.RuneStart(detail[cut]) {
			cut--
		}
		detail = detail[:cut]
	}
	path := p.Path
	if len(path) > 1024 {
		path = ""
	}
	return &dto.Diagnostic{Path: path, Line: max(p.Line, 0), Detail: detail}
}

// withDiagnostic records err's diagnostic on o, keeping an earlier one when err has none.
func withDiagnostic(o dto.Operation, err error) dto.Operation {
	if d := diagnostic(err); d != nil {
		o.Diagnostic = d
	}
	return o
}
func reason(err error, fallback dto.Reason) dto.Reason {
	if p, ok := errors.AsType[*dto.Problem](err); ok {
		return p.Code
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return dto.ExecRunning
	}
	return fallback
}
func (a *Agent) faultCommit(p dto.Phase) error {
	if a.afterCommit != nil {
		return a.afterCommit(p)
	}
	return nil
}
func (a *Agent) effect(ctx context.Context, i Intent, o *dto.Operation, p dto.Phase, fn func() error) error {
	o.Phase = p
	if err := a.save(ctx, i, *o); err != nil {
		return err
	}
	if err := a.faultCommit(p); err != nil {
		return err
	}
	if err := fn(); err != nil {
		return err
	}
	if a.afterEffect != nil {
		return a.afterEffect(p)
	}
	return nil
}
func (a *Agent) save(ctx context.Context, i Intent, o dto.Operation) error {
	ib, err := marshal(i)
	if err != nil {
		return err
	}
	ob, err := marshal(o)
	if err != nil {
		return err
	}
	res := a.DB.WithContext(ctx).Model(&Journal{}).Where("id = ? AND generation = ?", o.ID, o.Generation).Updates(map[string]any{"intent": ib, "result": ob, "state": o.State, "phase": o.Phase, "finished_at": o.FinishedAt})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return dto.Fail(dto.JournalLost, 409)
	}
	return nil
}
func (a *Agent) recheck(ctx context.Context, i Intent) error {
	if err := a.Check(ctx, i.Request.Instance); err != nil {
		return err
	}
	b, err := a.Runtime.Target(ctx, i.Request.Instance, i.Request.Target)
	if err != nil {
		return err
	}
	if b.FileState != i.Old.FileState {
		return stale(i.Request.Target)
	}
	g, err := a.Runtime.Graph(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	if !equalGraph(g, i.GraphHashes) {
		return stale(i.Request.Target)
	}
	return nil
}
func (a *Agent) cleanup(ctx context.Context, i Intent) error {
	var er error
	if e, err := a.Engines.For(i.Request.Instance.Kind); err == nil {
		er = e.Cleanup(ctx, i)
	}
	return errors.Join(er, a.Runtime.Cleanup(ctx, i))
}
func (a *Agent) finish(ctx context.Context, i Intent, o dto.Operation, s dto.State, r dto.Reason, k dto.EvidenceKind) (dto.Operation, error) {
	// Persist settlement even when the request disconnected. An exec whose exit
	// is unknown retains its staging and lock until explicit operator recovery.
	settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	ctx = settlement
	// Cleanup is part of settlement. A failed cleanup must retain the source lock.
	if r != dto.ExecRunning {
		if err := a.cleanup(ctx, i); err != nil {
			s = dto.RecoveryRequired
			r = reason(err, dto.RestoreFailed)
			o = withDiagnostic(o, err)
		}
	}
	o.State = s
	o.Reason = r
	o.Evidence = k
	if s.Final() {
		now := time.Now().UTC()
		o.FinishedAt = &now
	} else {
		o.FinishedAt = nil
	}
	err := a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		aCopy := &Agent{DB: tx}
		if er := aCopy.save(ctx, i, o); er != nil {
			return er
		}
		source := "host\x00" + i.Request.Instance.Source.Canonical
		if s == dto.Verified && i.Owned && i.New.Present {
			if er := tx.Save(&verifiedFile{Source: source, Path: i.Request.Target, Hash: i.New.Hash, Content: i.New.Content}).Error; er != nil {
				return er
			}
		}
		if i.Owned && i.New.Present && s == dto.AppliedUnverified {
			var row verifiedFile
			er := tx.First(&row, "source = ? AND path = ?", source, i.Request.Target).Error
			if missing(er) {
				if er = tx.Create(&verifiedFile{Source: source, Path: i.Request.Target, Hash: "", Content: []byte{}}).Error; er != nil {
					return er
				}
			} else if er != nil {
				return er
			}
		}
		if s == dto.Verified && !i.New.Present {
			return tx.Delete(&verifiedFile{}, "source = ? AND path = ?", source, i.Request.Target).Error
		}
		return nil
	})
	return o, err
}
func (a *Agent) effectFailure(ctx context.Context, i Intent, o dto.Operation, err error, published bool) (dto.Operation, error) {
	if errors.Is(err, errCrash) {
		return o, err
	}
	if Transient(err) {
		// A failed read or write call proves neither a refusal nor a publication: the
		// journal keeps its phase and recovery decides from the actual target bytes.
		return o, err
	}
	o = withDiagnostic(o, err)
	r := reason(err, dto.InvalidGraph)
	if r == dto.ExecRunning {
		return a.finish(ctx, i, o, dto.RecoveryRequired, r, "")
	}
	if !published {
		return a.finish(ctx, i, o, dto.Rejected, r, "")
	}
	current, e := a.Runtime.Target(ctx, i.Request.Instance, i.Request.Target)
	if e == nil && current.FileState == i.Old.FileState {
		return a.finish(ctx, i, o, dto.Rejected, r, "")
	}
	return a.finish(ctx, i, o, dto.RecoveryRequired, r, "")
}

// errCrash is used only by deterministic phase fault tests; it is never returned by production I/O.
var errCrash = errors.New("injected process interruption")
