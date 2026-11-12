// Source: docklet-next/issues/303
// Revision: 8696c990fba68be0dd3ca9ac4de61434f6c56131
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original explicit event list order.

package event

import (
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// paginateEventsInternal paginates an event list query with the explicit event order.
func paginateEventsInternal(params pagination.QueryParams, q *gorm.DB, result *[]Event) (pagination.Response, error) {
	q, params = orderEventListInternal(q, params)
	if params.Filters["since"] != "" {
		return paginateWindowEvents(params, q, result)
	}
	return pagination.PaginateAndSortDB(params, q, result)
}

// orderEventListInternal makes the default event order newest first with the ID as tie-breaker in the
// same direction; other sortable columns keep the shared pagination order.
func orderEventListInternal(q *gorm.DB, params pagination.QueryParams) (*gorm.DB, pagination.QueryParams) {
	if params.Sort != "" && params.Sort != "timestamp" {
		return q, params
	}
	desc := params.Sort == "" || params.Order != "asc"
	params.Sort = ""
	return q.Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{
		{Column: clause.Column{Table: clause.CurrentTable, Name: "timestamp"}, Desc: desc},
		{Column: clause.Column{Table: clause.CurrentTable, Name: "id"}, Desc: desc},
	}}), params
}
