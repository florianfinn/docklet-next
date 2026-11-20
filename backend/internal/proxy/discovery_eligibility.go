// Source: docklet-next/issues/528
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 setup source eligibility and its server revalidation.

package proxy

import (
	"context"
	"path"

	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// SourceEligibilityReader is implemented by engines that can tell, from the
// container's own static configuration and without a registration, whether a
// discovered source may back a new instance.
type SourceEligibilityReader interface {
	SourceEligibility(context.Context, dto.Instance) (dto.SourceEligibility, error)
}

// DiscoverHost is the agent-side discovery: Docker sources plus their setup eligibility.
func (m *Module) DiscoverHost(ctx context.Context) ([]dto.Discovery, error) {
	rows, err := m.Runtime.Discover(ctx)
	if err != nil {
		return nil, err
	}
	for n := range rows {
		for k := range rows[n].Sources {
			e, err := m.sourceEligibility(ctx, rows[n], rows[n].Sources[k])
			if err != nil {
				return nil, err
			}
			rows[n].Sources[k].Eligibility = &e
		}
	}
	return rows, nil
}

func (m *Module) sourceEligibility(ctx context.Context, d dto.Discovery, s dto.Source) (dto.SourceEligibility, error) {
	if s.ReadOnly {
		reason := s.Reason
		if reason == "" {
			reason = dto.ReadOnly
		}
		return dto.SourceEligibility{Reason: reason}, nil
	}
	e, err := m.Agent.Engines.For(d.Kind)
	if err != nil {
		return dto.SourceEligibility{}, err
	}
	reader, ok := e.(SourceEligibilityReader)
	if !ok {
		return dto.SourceEligibility{Reason: dto.IncompleteInventory}, nil
	}
	return reader.SourceEligibility(ctx, dto.Instance{Kind: d.Kind, EnvironmentID: "0", Identity: d.Identity, Source: s})
}

// revalidateSource applies the setup rules again on the server: a confirmed
// ineligible source and a route directory other than a fixed one are refused.
// Sources without eligibility (older stored data) keep the previous rules.
func revalidateSource(s dto.Source, routeDirectory string) error {
	e := s.Eligibility
	if e == nil {
		return nil
	}
	if !e.Selectable {
		return dto.Fail(dto.SourceIneligible, 409)
	}
	if e.RouteDirectory != nil && e.RouteDirectory.Fixed && underMount(s.MountTarget, routeDirectory) != underMount(s.MountTarget, e.RouteDirectory.Value) {
		return dto.Fail(dto.RouteDirectoryMismatch, 422)
	}
	return nil
}

// underMount resolves a route directory the way the engines do: absolute as given, otherwise below the mount.
func underMount(mount, p string) string {
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Join(mount, p)
}

// revalidateSettings checks a settings update against the source as currently
// discovered for the registered container and mount target.
func (m *Module) revalidateSettings(ctx context.Context, i dto.Instance) error {
	rows, err := m.Transport.Discover(ctx, i.EnvironmentID)
	if err != nil {
		return err
	}
	for _, d := range rows {
		if d.Identity.ContainerID != i.Identity.ContainerID {
			continue
		}
		for _, s := range d.Sources {
			if s.MountTarget == i.Source.MountTarget {
				return revalidateSource(s, i.RouteDirectory)
			}
		}
	}
	return dto.Fail(dto.ContainerChanged, 409)
}
