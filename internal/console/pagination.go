package console

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
)

var allowedPageSizes = map[int]struct{}{
	10:  {},
	20:  {},
	50:  {},
	100: {},
}

type pageQuery struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

type pageSpec struct {
	DefaultSortBy    string
	DefaultSortOrder string
	AllowedSort      map[string]struct{}
}

type pageResult[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

func stringSet(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func parsePageQuery(values url.Values, spec pageSpec) (pageQuery, error) {
	page, err := positiveInt(values.Get("page"), defaultPage)
	if err != nil {
		return pageQuery{}, fmt.Errorf("invalid page: %w", err)
	}
	pageSize, err := positiveInt(values.Get("pageSize"), defaultPageSize)
	if err != nil {
		return pageQuery{}, fmt.Errorf("invalid pageSize: %w", err)
	}
	if _, ok := allowedPageSizes[pageSize]; !ok {
		return pageQuery{}, fmt.Errorf("invalid pageSize: must be one of 10, 20, 50, 100")
	}
	if page-1 > int(^uint(0)>>1)/pageSize {
		return pageQuery{}, fmt.Errorf("invalid page: value is too large")
	}

	sortBy := values.Get("sortBy")
	if sortBy == "" {
		sortBy = spec.DefaultSortBy
	}
	if _, ok := spec.AllowedSort[sortBy]; !ok {
		return pageQuery{}, fmt.Errorf("invalid sortBy %q", sortBy)
	}

	sortOrder := values.Get("sortOrder")
	if sortOrder == "" {
		sortOrder = spec.DefaultSortOrder
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return pageQuery{}, fmt.Errorf("invalid sortOrder %q", sortOrder)
	}

	return pageQuery{Page: page, PageSize: pageSize, SortBy: sortBy, SortOrder: sortOrder}, nil
}

func (q pageQuery) offset() int {
	return (q.Page - 1) * q.PageSize
}

// sortPageItems applies the selected direction only to the requested field.
// Equal primary values always use the stable comparator in ascending order so
// page boundaries stay deterministic between requests.
func sortPageItems[T any](items []T, query pageQuery, primary, stable func(a, b T) int) {
	sort.SliceStable(items, func(i, j int) bool {
		comparison := primary(items[i], items[j])
		if comparison == 0 {
			return stable(items[i], items[j]) < 0
		}
		if query.SortOrder == "desc" {
			return comparison > 0
		}
		return comparison < 0
	})
}

func positiveInt(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if value < 1 {
		return 0, fmt.Errorf("must be greater than zero")
	}
	return value, nil
}

func paginate[T any](items []T, query pageQuery) pageResult[T] {
	total := len(items)
	result := pageResult[T]{
		Items:    make([]T, 0),
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
	}
	if total == 0 {
		return result
	}

	lastPage := (total-1)/query.PageSize + 1
	if query.Page > lastPage {
		return result
	}
	start := (query.Page - 1) * query.PageSize
	end := min(start+query.PageSize, total)
	result.Items = items[start:end]
	return result
}
