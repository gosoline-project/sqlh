package sqlh

import (
	"fmt"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
)

// ForceFilter is a server-owned query restriction. SQLH applies every force
// filter in addition to request filters and pagination.
type ForceFilter func(qb *sqlr.QueryBuilderSelect)

// ForceFilterSource exposes request-local force filters to SQLH operations.
// Inputs used by scoped operations should embed [ForceFilters].
type ForceFilterSource interface {
	GetForceFilters() []ForceFilter
}

// ForceFilters stores server-owned query restrictions. It is intended to be
// embedded in HTTP input structs. The unexported slice prevents HTTP binders
// from populating or replacing the filters.
type ForceFilters struct {
	filters []ForceFilter
}

// AddForceFilter adds one or more server-owned query restrictions. Nil filters
// are ignored so callers can build optional scopes without branching.
func (f *ForceFilters) AddForceFilter(filters ...ForceFilter) {
	if f == nil {
		return
	}

	for _, filter := range filters {
		if filter != nil {
			f.filters = append(f.filters, filter)
		}
	}
}

// filtersCopy returns a defensive copy without relying on the promoted
// ForceFilters method, which is shadowed by the embedded field on request
// inputs.
func (f ForceFilters) filtersCopy() []ForceFilter {
	return append([]ForceFilter(nil), f.filters...)
}

// GetForceFilters returns a copy of the filters currently stored on the carrier.
func (f ForceFilters) GetForceFilters() []ForceFilter {
	return f.filtersCopy()
}

// ForceFilters returns a copy of the filters currently stored on the carrier.
// It is retained on the carrier itself for direct use; request input types
// expose the unambiguous GetForceFilters method because their embedded field is
// named ForceFilters.
func (f ForceFilters) ForceFilters() []ForceFilter {
	return f.filtersCopy()
}

// ListInput is the standard SQLH list input. Applications can embed it in a
// domain-specific input to retain the predefined filter, limit, offset, and
// force-filter behavior.
type ListInput struct {
	ForceFilters

	Filter sqlc.JsonFilter `json:"filter"`
	Limit  int             `json:"limit,omitempty"`
	Offset int             `json:"offset,omitempty"`
}

// GetForceFilters returns a copy of the server-owned filters carried by the list input.
func (i ListInput) GetForceFilters() []ForceFilter {
	return i.filtersCopy()
}

// ApplyFilters applies all server-owned force filters before the user filter. It
// does not apply pagination, so the same scope can be reused for a count query.
func (i ListInput) ApplyFilters(qb *sqlr.QueryBuilderSelect) error {
	if qb == nil {
		return fmt.Errorf("query builder is nil")
	}

	applyForceFilters(i, qb)

	expression, err := i.Filter.ToExpression()
	if err != nil {
		return fmt.Errorf("failed to convert list filter: %w", err)
	}
	if expression != nil {
		qb.Where(expression)
	}

	return nil
}

// ApplyPagination applies the standard limit and offset fields.
func (i ListInput) ApplyPagination(qb *sqlr.QueryBuilderSelect) {
	if qb == nil {
		return
	}
	if i.Limit > 0 {
		qb.Limit(i.Limit)
	}
	if i.Offset > 0 {
		qb.Offset(i.Offset)
	}
}

// ValidatePagination validates the standard limit and offset fields.
func (i ListInput) ValidatePagination() error {
	if i.Limit < 0 {
		return fmt.Errorf("limit must not be negative")
	}
	if i.Offset < 0 {
		return fmt.Errorf("offset must not be negative")
	}

	return nil
}

// ListInputSource is the minimal contract required by a typed list operation.
type ListInputSource interface {
	ForceFilterSource
	ApplyFilters(*sqlr.QueryBuilderSelect) error
	ApplyPagination(*sqlr.QueryBuilderSelect)
	ValidatePagination() error
}

// QueryScope applies the non-pagination part of a list query. It is passed to
// both list and count callbacks so they cannot accidentally diverge in scope.
type QueryScope func(qb *sqlr.QueryBuilderSelect) error

// QueryPlan exposes the composed SQLR builder hooks, shared list scope, and
// page application functions to custom query and count callbacks.
type QueryPlan struct {
	// ApplyBuilder installs relation-tag defaults followed by the definition's
	// custom query builder.
	ApplyBuilder    func(qb *sqlr.QueryBuilderSelect)
	ApplyScope      QueryScope
	ApplyPagination func(qb *sqlr.QueryBuilderSelect)
}

func applyForceFilters(source ForceFilterSource, qb *sqlr.QueryBuilderSelect) {
	if source == nil || qb == nil {
		return
	}

	for _, filter := range source.GetForceFilters() {
		if filter != nil {
			filter(qb)
		}
	}
}
