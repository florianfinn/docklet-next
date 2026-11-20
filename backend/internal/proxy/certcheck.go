// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package proxy

import (
	"context"
	"github.com/google/uuid"
	"regexp"

	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

var certificateHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validateCertificateCheck(q dto.CertificateCheckRequest) error {
	if !hostio.Relative(q.File) || hostio.Secret(q.File) || len(q.File) > 1024 || q.Router == "" || len(q.Router) > 128 || !certificateHash.MatchString(q.SourceHash) {
		return dto.Fail(dto.InvalidInput, 422)
	}
	if _, _, err := hostio.ProbeArgs(q.Host, "https://127.0.0.1/", ""); err != nil {
		return err
	}
	return nil
}

// CheckCertificate is a read-only observation. It takes no operation reservation,
// writes no journal and retains no result in either agent or hub storage.
func (a *Agent) CheckCertificate(ctx context.Context, q dto.HostCertificateCheckRequest) (dto.CertificateObservation, error) {
	if err := validateCertificateCheck(q.CertificateCheckRequest); err != nil {
		return dto.CertificateObservation{}, err
	}
	operation := uuid.NewString()
	ctx = hostio.WithProbeOperation(ctx, operation)
	defer ForgetProbe(a.Runtime, q.Instance, operation)
	var result dto.CertificateObservation
	check := func(ctx context.Context) error {
		if err := a.Check(ctx, q.Instance); err != nil {
			return err
		}
		engine, err := a.Engines.For(q.Instance.Kind)
		if err != nil {
			return err
		}
		checker, ok := engine.(interface {
			CheckCertificate(context.Context, dto.Instance, dto.CertificateCheckRequest) (dto.CertificateObservation, error)
		})
		if !ok {
			return dto.Fail(dto.CertificateUnavailable, 409)
		}
		result, err = checker.CheckCertificate(ctx, q.Instance, q.CertificateCheckRequest)
		return err
	}
	var err error
	if a.Gate != nil {
		err = a.Gate.Admit(ctx, features.Proxy, check)
	} else {
		err = check(ctx)
	}
	return result, err
}

func (m *Module) CheckCertificate(ctx context.Context, id string, q dto.CertificateCheckRequest) (dto.CertificateObservation, error) {
	if err := validateCertificateCheck(q); err != nil {
		return dto.CertificateObservation{}, err
	}
	instance, err := m.Instance(ctx, id)
	if err != nil {
		return dto.CertificateObservation{}, err
	}
	return m.Transport.CheckCertificate(ctx, dto.HostCertificateCheckRequest{Instance: instance, CertificateCheckRequest: q})
}

// CertificateRoute fences the exact source file and router selected by the UI.
func CertificateRoute(read dto.ReadResult, q dto.CertificateCheckRequest) (dto.Route, error) {
	current := false
	for _, f := range read.Files {
		if f.Path == q.File && f.Hash == q.SourceHash {
			current = true
		}
	}
	if current {
		for _, route := range read.Routes {
			if route.File == q.File && route.Name == q.Router {
				return route, nil
			}
		}
	}
	return dto.Route{}, dto.Fail(dto.RouteChanged, 409)
}

func CertificateResult(response hostio.Response, route, entry string) (dto.CertificateObservation, error) {
	if response.Certificate == nil {
		return dto.CertificateObservation{}, dto.Fail(dto.CertificateUnavailable, 409)
	}
	result := *response.Certificate
	result.Route.Name, result.Route.EntryPoint = route, entry
	return result, nil
}

// PrewarmProbe is outside engine observation windows, including recovery runs.
func PrewarmProbe(ctx context.Context, runtime any, instance dto.Instance) error {
	if runner, ok := runtime.(interface {
		PrewarmProbe(context.Context, dto.Instance) error
	}); ok {
		return runner.PrewarmProbe(ctx, instance)
	}
	return nil
}

func EnsureProbePrepared(ctx context.Context, runtime any, instance dto.Instance) error {
	if runner, ok := runtime.(interface {
		EnsureProbePrepared(context.Context, dto.Instance) error
	}); ok {
		return runner.EnsureProbePrepared(ctx, instance)
	}
	return nil
}

func ForgetProbe(runtime any, instance dto.Instance, operation string) {
	if runner, ok := runtime.(interface{ ForgetProbe(dto.Instance, string) }); ok {
		runner.ForgetProbe(instance, operation)
	}
}
