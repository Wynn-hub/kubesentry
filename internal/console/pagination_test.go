package console

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"testing"
)

var testPageSpec = pageSpec{
	DefaultSortBy:    "name",
	DefaultSortOrder: "asc",
	AllowedSort:      stringSet("name", "phase"),
}

func TestParsePageQueryDefaults(t *testing.T) {
	got, err := parsePageQuery(url.Values{}, testPageSpec)
	if err != nil {
		t.Fatalf("parse defaults: %v", err)
	}
	want := pageQuery{Page: 1, PageSize: 20, SortBy: "name", SortOrder: "asc"}
	if got != want {
		t.Fatalf("query = %+v, want %+v", got, want)
	}
}

func TestParsePageQueryAcceptsSupportedValues(t *testing.T) {
	for _, pageSize := range []string{"10", "20", "50", "100"} {
		t.Run(pageSize, func(t *testing.T) {
			got, err := parsePageQuery(url.Values{
				"page":      {"3"},
				"pageSize":  {pageSize},
				"sortBy":    {"phase"},
				"sortOrder": {"desc"},
			}, testPageSpec)
			if err != nil {
				t.Fatalf("parse query: %v", err)
			}
			if got.Page != 3 || got.SortBy != "phase" || got.SortOrder != "desc" {
				t.Fatalf("unexpected query: %+v", got)
			}
		})
	}
}

func TestParsePageQueryRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		values url.Values
	}{
		{name: "page text", values: url.Values{"page": {"nope"}}},
		{name: "page zero", values: url.Values{"page": {"0"}}},
		{name: "page negative", values: url.Values{"page": {"-1"}}},
		{name: "page offset overflow", values: url.Values{"page": {strconv.Itoa(int(^uint(0) >> 1))}, "pageSize": {"100"}}},
		{name: "page size text", values: url.Values{"pageSize": {"nope"}}},
		{name: "page size zero", values: url.Values{"pageSize": {"0"}}},
		{name: "page size unsupported", values: url.Values{"pageSize": {"30"}}},
		{name: "page size too large", values: url.Values{"pageSize": {"101"}}},
		{name: "sort field", values: url.Values{"sortBy": {"createdAt"}}},
		{name: "sort order", values: url.Values{"sortOrder": {"sideways"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parsePageQuery(tt.values, testPageSpec); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestParsePageQuerySupportsDescendingDefault(t *testing.T) {
	spec := pageSpec{
		DefaultSortBy:    "createdAt",
		DefaultSortOrder: "desc",
		AllowedSort:      stringSet("createdAt"),
	}
	got, err := parsePageQuery(url.Values{}, spec)
	if err != nil {
		t.Fatal(err)
	}
	if got.SortBy != "createdAt" || got.SortOrder != "desc" {
		t.Fatalf("query = %+v", got)
	}
}

func TestPaginate(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}
	got := paginate(items, pageQuery{Page: 2, PageSize: 2})
	if got.Total != 5 || got.Page != 2 || got.PageSize != 2 {
		t.Fatalf("metadata = %+v", got)
	}
	if !reflect.DeepEqual(got.Items, []string{"c", "d"}) {
		t.Fatalf("items = %v", got.Items)
	}
}

func TestPaginateBeyondLastPageReturnsEmptyArray(t *testing.T) {
	got := paginate([]string{"a"}, pageQuery{Page: 2, PageSize: 20})
	if got.Total != 1 || len(got.Items) != 0 || got.Items == nil {
		t.Fatalf("result = %+v", got)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"items":[],"total":1,"page":2,"pageSize":20}` {
		t.Fatalf("json = %s", b)
	}
}

func TestPaginateNilInputReturnsEmptyArray(t *testing.T) {
	got := paginate[string](nil, pageQuery{Page: 1, PageSize: 20})
	if got.Items == nil || got.Total != 0 {
		t.Fatalf("result = %+v", got)
	}
}
