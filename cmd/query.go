package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// listQuery holds the user-supplied options for `documents list` before they are
// translated into the query parameters the Trokky v2 server expects.
type listQuery struct {
	Limit  int
	Offset int
	Page   int
	Filter string
	Sort   string
	Order  string
	Status string
	Expand string
	Search string
	Count  bool
}

// buildListQuery converts a listQuery into the v2 server's query params.
//
// Sorting is sent in prefix notation: "-field" for descending, "field" for
// ascending. A value the user already wrote in a directional form ("-field",
// "field.desc", "field.asc") is passed through untouched.
func buildListQuery(q listQuery) (url.Values, error) {
	params := url.Values{}

	if q.Limit > 0 {
		params.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Offset > 0 {
		params.Set("offset", strconv.Itoa(q.Offset))
	}
	if q.Page > 0 {
		params.Set("page", strconv.Itoa(q.Page))
	}
	if q.Filter != "" {
		params.Set("filter", q.Filter)
	}

	if q.Sort != "" {
		sortParam, err := buildSortParam(q.Sort, q.Order)
		if err != nil {
			return nil, err
		}
		params.Set("sort", sortParam)
	}

	if q.Status != "" {
		filter, err := mergeStatusFilter(q.Filter, q.Status)
		if err != nil {
			return nil, err
		}
		params.Set("filter", filter)
	}

	if q.Search != "" {
		params.Set("search", q.Search)
	}
	if q.Expand != "" {
		params.Set("expand", q.Expand)
	}
	if q.Count {
		params.Set("count", "true")
	}

	return params, nil
}

// buildSortParam renders a sort field plus order as the server's prefix notation.
func buildSortParam(sort, order string) (string, error) {
	// Already directional — pass through unchanged.
	if strings.HasPrefix(sort, "-") || strings.HasSuffix(sort, ".desc") || strings.HasSuffix(sort, ".asc") {
		return sort, nil
	}

	switch strings.ToLower(order) {
	case "", "asc":
		return sort, nil
	case "desc":
		return "-" + sort, nil
	default:
		return "", fmt.Errorf("invalid order %q: must be 'asc' or 'desc'", order)
	}
}

// mergeStatusFilter folds --status into the JSON filter object as "_status".
func mergeStatusFilter(filter, status string) (string, error) {
	obj := map[string]interface{}{}
	if filter != "" {
		if err := json.Unmarshal([]byte(filter), &obj); err != nil {
			return "", fmt.Errorf("--filter must be a JSON object when combined with --status")
		}
	}
	obj["_status"] = status
	merged, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(merged), nil
}
