// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package access

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/user"
	"net/url"
	"strings"
)

type Principal struct {
	ID            string
	Administrator bool
	Assignments   []user.AppAssignment
}

func FromPermissions(ps *authz.PermissionSet) Principal {
	if ps == nil {
		return Principal{}
	}
	return Principal{ps.UserID, ps.IsGlobalAdmin(), ps.Assignments}
}
func (p Principal) IsUser() bool { return p.ID != "" && !p.Administrator }
func (p Principal) ProjectIDs(env string) []string {
	out := []string{}
	for _, a := range p.Assignments {
		if a.EnvironmentID == env {
			out = append(out, a.ProjectID)
		}
	}
	return out
}

type Params struct {
	ProjectID, ContainerID, RunID string
	Body                          []byte
}
type Container struct {
	ID    string
	Names []string
}
type Controller struct {
	Projects func(context.Context, string, url.Values) ([]projectDTO.Details, error)
	Runtime  func(context.Context, string, string) ([]Container, error)
	RunOwner func(context.Context, string, string) (string, error)
}
type controllerKey struct{}

func WithController(ctx context.Context, c *Controller) context.Context {
	return context.WithValue(ctx, controllerKey{}, c)
}

type Decision string

const (
	Allow         Decision = ""
	NotFound      Decision = "access.not-found"
	AdminRequired Decision = "access.admin-required"
	FullRequired  Decision = "access.full-required"
	Unavailable   Decision = "access.unavailable"
)

func (d Decision) Status() int {
	switch d {
	case NotFound:
		return 404
	case Unavailable:
		return 503
	default:
		return 403
	}
}
func (d Decision) Error() string  { return string(d) }
func (d Decision) GetStatus() int { return d.Status() }

// Decide orders class, assignment, then level and options. It never changes
// authority and reads container membership into request-owned values.
func Decide(ctx context.Context, p Principal, operation, env string, params Params) Decision {
	if !p.IsUser() {
		return Allow
	}
	r, ok := Rules[operation]
	if !ok {
		return AdminRequired
	}
	switch r.Kind {
	case Public, Self, AppList:
		return Allow
	case Admin, Machine:
		return AdminRequired
	case OwnRun:
		c, _ := ctx.Value(controllerKey{}).(*Controller)
		if c == nil || c.RunOwner == nil {
			return Unavailable
		}
		owner, err := c.RunOwner(ctx, env, params.RunID)
		if err != nil || owner != p.ID {
			return NotFound
		}
		return Allow
	case App:
		level := Level("")
		if r.Ref == ProjectPath {
			for _, a := range p.Assignments {
				if a.EnvironmentID == env && a.ProjectID == params.ProjectID {
					level = Level(a.Level)
					break
				}
			}
		} else {
			if len(p.ProjectIDs(env)) == 0 {
				return NotFound
			}
			c, _ := ctx.Value(controllerKey{}).(*Controller)
			if c == nil || c.Runtime == nil {
				return Unavailable
			}
			for _, a := range p.Assignments {
				if a.EnvironmentID != env {
					continue
				}
				containers, err := c.Runtime(ctx, env, a.ProjectID)
				if err != nil {
					return Unavailable
				}
				for _, v := range containers {
					match := params.ContainerID == v.ID
					for _, name := range v.Names {
						match = match || strings.TrimPrefix(name, "/") == strings.TrimPrefix(params.ContainerID, "/")
					}
					if match && (level == "" || a.Level == string(Full)) {
						level = Level(a.Level)
					}
				}
			}
		}
		if level == "" {
			return NotFound
		}
		if r.Level == Full && level != Full {
			return FullRequired
		}
		if operation == "deploy-project" && level != Full {
			var body map[string]json.RawMessage
			if len(params.Body) > 0 && json.Unmarshal(params.Body, &body) != nil {
				return FullRequired
			}
			for _, key := range []string{"pullPolicy", "forceRecreate", "removeOrphans", "recreateVolumes"} {
				if _, ok := body[key]; ok {
					return FullRequired
				}
			}
		}
		return Allow
	}
	return AdminRequired
}

// Match resolves exact registered templates without consulting user-supplied operation IDs.
func Match(method, path string) (string, string, Params) {
	path = strings.TrimPrefix(path, "/api")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	best := ""
	score := -1
	env := ""
	var params Params
	for _, route := range Routes {
		if route.Method != method {
			continue
		}
		segs := strings.Split(strings.Trim(route.Path, "/"), "/")
		if len(segs) != len(parts) {
			continue
		}
		n := 0
		valid := true
		e := ""
		v := Params{}
		for i, s := range segs {
			if strings.HasPrefix(s, "{") {
				switch s {
				case "{id}":
					e = parts[i]
				case "{projectId}":
					v.ProjectID = parts[i]
				case "{containerId}":
					v.ContainerID = parts[i]
				case "{runId}":
					v.RunID = parts[i]
				}
			} else if s != parts[i] {
				valid = false
				break
			} else {
				n++
			}
		}
		if valid && n > score {
			score = n
			best = route.Operation
			env = e
			params = v
		}
	}
	return best, env, params
}

type Route struct{ Method, Path, Operation string }
