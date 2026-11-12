// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original event-local bounded timestamp pagination.

package event

import (
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	kit "go.getarcane.app/kit/pkg"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Huma parses typed time inputs after middleware and normally answers 422 on invalid input.
// This event-only boundary keeps malformed window requests at the promised 400 status.
func validateEventWindow(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		url := ctx.URL()
		if raw, present := url.Query()["since"]; present {
			if len(raw) != 1 {
				_ = huma.WriteErr(api, ctx, http.StatusBadRequest, "Invalid since timestamp")
				return
			}
			if _, err := time.Parse(time.RFC3339Nano, raw[0]); err != nil {
				_ = huma.WriteErr(api, ctx, http.StatusBadRequest, "Invalid since timestamp")
				return
			}
		}
		next(ctx)
	}
}

// Huma parameter wrappers preserve absence separately from an explicit zero value.
type eventQueryParam[T any] struct {
	Value T
	Set   bool
}

func (eventQueryParam[T]) Schema(registry huma.Registry) *huma.Schema {
	return huma.SchemaFromType(registry, reflect.TypeFor[T]())
}

func (p *eventQueryParam[T]) Receiver() reflect.Value    { return reflect.ValueOf(&p.Value).Elem() }
func (p *eventQueryParam[T]) OnParamSet(set bool, _ any) { p.Set = set }

func eventPaginationParams(start int, limit eventQueryParam[int], sort, order, search string, since eventQueryParam[time.Time]) (pagination.QueryParams, error) {
	size := 20
	if since.Set {
		size = 500
	}
	if limit.Set {
		size = limit.Value
	}
	if since.Set && (size < 1 || size > 500) {
		return pagination.QueryParams{}, huma.Error400BadRequest("With since, limit must be between 1 and 500")
	}
	params := handlerutil.PaginationParams(start, size, sort, order, search)
	if since.Set {
		params.Filters["since"] = since.Value.UTC().Format(time.RFC3339Nano)
	}
	return params, nil
}

// The same filtered query backs count and page, so totalItems exposes truncation.
// Unfiltered reads still use the shared paginator and its unchanged 100-row clamp.
func paginateWindowEvents(params pagination.QueryParams, q *gorm.DB, result *[]Event) (pagination.Response, error) {
	since, err := time.Parse(time.RFC3339Nano, params.Filters["since"])
	if err != nil {
		return pagination.Response{}, err
	}
	// SQLite persists integer microseconds. Round the lower bound up so truncation
	// cannot include a stored event that precedes a higher-precision since instant.
	if floor := since.Truncate(time.Microsecond); floor.Before(since) {
		since = floor.Add(time.Microsecond)
	}
	q = q.Where("timestamp >= ?", since)
	if params.Sort != "" {
		field, found := reflect.TypeFor[Event]().FieldByName(kit.Capitalize(params.Sort))
		sortable, _ := strconv.ParseBool(field.Tag.Get("sortable"))
		columns := []clause.OrderByColumn{}
		if found && sortable {
			columns = append(columns, clause.OrderByColumn{Column: clause.Column{Name: kit.SnakeCase(params.Sort)}, Desc: params.Order == "desc"})
		}
		if params.Sort != "id" || !sortable {
			columns = append(columns, clause.OrderByColumn{Column: clause.Column{Name: "id"}})
		}
		q = q.Clauses(clause.OrderBy{Columns: columns})
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return pagination.Response{}, err
	}
	offset, limit := max(0, params.Start), params.Limit
	if err := q.Offset(offset).Limit(limit).Find(result).Error; err != nil {
		return pagination.Response{}, err
	}
	return pagination.Response{TotalItems: total, TotalPages: max(1, (total+int64(limit)-1)/int64(limit)), CurrentPage: offset/limit + 1, ItemsPerPage: limit}, nil
}
