package sqlh

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/validation"
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

// AddForceFilter adds server-owned query restrictions.
func (f *ForceFilters) AddForceFilter(filters ...ForceFilter) {
	f.filters = append(f.filters, filters...)
}

// GetForceFilters returns a copy of the filters currently stored on the carrier.
func (f ForceFilters) GetForceFilters() []ForceFilter {
	return append([]ForceFilter(nil), f.filters...)
}

// ListPage contains the limit and offset for a list request.
type ListPage struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// JsonFilter describes a public field filter using SQLC's existing operators.
// Field is an API name resolved by the resource, never a database identifier.
type JsonFilter struct {
	Type   sqlc.JsonFilterType `json:"type"`
	Field  string              `json:"field,omitempty"`
	Value  any                 `json:"value,omitempty"`
	Values []any               `json:"values,omitempty"`
	From   any                 `json:"from,omitempty"`
	To     any                 `json:"to,omitempty"`
	Fields []JsonFilter        `json:"fields,omitempty"`
}

// JsonOrder orders a public field. An omitted direction means ascending.
type JsonOrder struct {
	Field     string `json:"field"`
	Direction string `json:"direction,omitempty"`
}

// No node in the wire tree implements UnmarshalJSON, so encoding/json decodes
// and validates the complete tree once rather than reparsing every subtree.
type jsonFilterWire struct {
	Type   sqlc.JsonFilterType `json:"type"`
	Field  string              `json:"field,omitempty"`
	Value  any                 `json:"value,omitempty"`
	Values []any               `json:"values,omitempty"`
	From   any                 `json:"from,omitempty"`
	To     any                 `json:"to,omitempty"`
	Fields []jsonFilterWire    `json:"fields,omitempty"`
}

func (f jsonFilterWire) publicFilter() JsonFilter {
	result := JsonFilter{Type: f.Type, Field: f.Field, Value: f.Value, Values: f.Values, From: f.From, To: f.To}
	if f.Fields != nil {
		result.Fields = make([]JsonFilter, len(f.Fields))
		for i := range f.Fields {
			result.Fields[i] = f.Fields[i].publicFilter()
		}
	}

	return result
}

// UnmarshalJSON rejects obsolete raw-column and unknown filter dialects.
func (f *JsonFilter) UnmarshalJSON(data []byte) error {
	var value jsonFilterWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*f = value.publicFilter()

	return nil
}

// UnmarshalJSON rejects obsolete raw-column ordering.
func (o *JsonOrder) UnmarshalJSON(data []byte) error {
	type wireOrder JsonOrder
	var value wireOrder
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*o = JsonOrder(value)

	return nil
}

// ListInput is the standard public-field list request.
type ListInput struct {
	ForceFilters
	Filter  JsonFilter  `json:"filter"`
	OrderBy []JsonOrder `json:"orderBy,omitempty"`
	Page    ListPage    `json:"page,omitempty"`
}

// GetFilter returns the public request filter.
func (i ListInput) GetFilter() JsonFilter { return i.Filter }

// GetOrderBy returns the public request ordering.
func (i ListInput) GetOrderBy() []JsonOrder { return i.OrderBy }

// ValidateQueryModifiers validates ordering syntax; resource resolution also
// validates every public field before invoking query or count callbacks.
func (i ListInput) ValidateQueryModifiers() error {
	for index, key := range i.OrderBy {
		if _, err := (sqlc.JsonOrder{Column: "field", Direction: key.Direction}).ToExpression(); err != nil {
			return validation.NewError(fmt.Errorf("orderBy[%d]: %w", index, err))
		}
	}

	return nil
}

// ApplyPagination applies the standard page limit and offset fields.
func (i ListInput) ApplyPagination(qb *sqlr.QueryBuilderSelect) {
	if qb == nil {
		return
	}
	if i.Page.Limit > 0 {
		qb.Limit(i.Page.Limit)
	}
	if i.Page.Offset > 0 {
		qb.Offset(i.Page.Offset)
	}
}

// ValidatePagination validates the standard page limit and offset fields.
func (i ListInput) ValidatePagination() error {
	if i.Page.Limit < 0 {
		return validation.NewError(fmt.Errorf("limit must not be negative"))
	}
	if i.Page.Offset < 0 {
		return validation.NewError(fmt.Errorf("offset must not be negative"))
	}
	if i.Page.Offset > 0 && i.Page.Limit == 0 {
		return validation.NewError(fmt.Errorf("offset requires a positive limit"))
	}

	return nil
}

// ListInputSource is the minimal contract required by a typed list operation.
type ListInputSource interface {
	ForceFilterSource
	GetFilter() JsonFilter
	GetOrderBy() []JsonOrder
	ApplyPagination(*sqlr.QueryBuilderSelect)
	ValidatePagination() error
	ValidateQueryModifiers() error
}

// QueryScope applies the non-pagination part of a list query. It is passed to
// both list and count callbacks so they cannot accidentally diverge in scope.
type QueryScope func(qb *sqlr.QueryBuilderSelect) error

// QueryPlan exposes the SQLR relation-tag builder, shared list scope, row-only
// query modifiers, and pagination to custom query and count callbacks.
type QueryPlan struct {
	// ApplyBuilder installs relation-tag defaults. It is always safe to call.
	ApplyBuilder        func(qb *sqlr.QueryBuilderSelect)
	ApplyScope          QueryScope
	ApplyQueryModifiers func(qb *sqlr.QueryBuilderSelect)
	ApplyPagination     func(qb *sqlr.QueryBuilderSelect)
}

func applyForceFilters(source ForceFilterSource, qb *sqlr.QueryBuilderSelect) {
	if source == nil || qb == nil {
		return
	}

	for _, filter := range source.GetForceFilters() {
		filter(qb)
	}
}
