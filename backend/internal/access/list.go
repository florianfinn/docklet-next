// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package access

import (
	"cmp"
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"

	"github.com/florianfinn/docklet-next/types/base"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func ProjectList(ctx context.Context, p Principal, env string, q url.Values) (base.Paginated[projectDTO.Details], error) {
	c, _ := ctx.Value(controllerKey{}).(*Controller)
	if c == nil || c.Projects == nil {
		return base.Paginated[projectDTO.Details]{}, Unavailable
	}
	filters := url.Values{}
	for k, v := range q {
		if k != "start" && k != "limit" && k != "search" && k != "sort" && k != "order" {
			filters[k] = slices.Clone(v)
		}
	}
	rows, err := c.Projects(ctx, env, filters)
	if err != nil {
		return base.Paginated[projectDTO.Details]{}, Unavailable
	}
	ids := p.ProjectIDs(env)
	out := []projectDTO.Details{}
	for _, r := range rows {
		if slices.Contains(ids, r.ID) {
			out = append(out, r)
		}
	}
	grand := len(out)
	term := strings.ToLower(strings.TrimSpace(q.Get("search")))
	out = slices.DeleteFunc(out, func(r projectDTO.Details) bool {
		return term != "" && !strings.Contains(strings.ToLower(r.Name+" "+r.Path+" "+r.Status), term)
	})
	key := func(r projectDTO.Details) string {
		switch q.Get("sort") {
		case "status":
			return r.Status
		case "createdAt", "created_at":
			return r.CreatedAt
		case "updatedAt", "updated_at":
			return r.UpdatedAt
		case "id":
			return r.ID
		case "path":
			return r.Path
		}
		return r.Name
	}
	slices.SortFunc(out, func(a, b projectDTO.Details) int {
		n := cmp.Compare(key(a), key(b))
		if n == 0 {
			n = cmp.Compare(a.ID, b.ID)
		}
		if q.Get("order") == "desc" {
			return -n
		}
		return n
	})
	total := len(out)
	start, _ := strconv.Atoi(q.Get("start"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit == 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if limit != -1 {
		start = min(max(start, 0), len(out))
		out = out[start:min(start+max(limit, 1), len(out))]
	}
	page := pagination.BuildResponse(int64(total), int64(grand), pagination.QueryParams{Params: pagination.Params{Start: start, Limit: limit}})
	return base.Paginated[projectDTO.Details]{Success: true, Data: out, Pagination: base.PaginationResponse{TotalPages: page.TotalPages, TotalItems: page.TotalItems, CurrentPage: page.CurrentPage, ItemsPerPage: page.ItemsPerPage, GrandTotalItems: page.GrandTotalItems}}, nil
}
