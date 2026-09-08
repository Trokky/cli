package cmd

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBuildListQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   listQuery
		want    url.Values
		wantErr string
	}{
		{
			name:  "sort defaults to ascending",
			query: listQuery{Sort: "title"},
			want:  url.Values{"sort": {"title"}},
		},
		{
			name:  "sort asc is explicit",
			query: listQuery{Sort: "title", Order: "asc"},
			want:  url.Values{"sort": {"title"}},
		},
		{
			name:  "sort desc gets a dash prefix",
			query: listQuery{Sort: "_createdAt", Order: "desc"},
			want:  url.Values{"sort": {"-_createdAt"}},
		},
		{
			name:  "order is case-insensitive",
			query: listQuery{Sort: "_createdAt", Order: "DESC"},
			want:  url.Values{"sort": {"-_createdAt"}},
		},
		{
			name:  "prefixed sort passes through unchanged",
			query: listQuery{Sort: "-_createdAt", Order: "asc"},
			want:  url.Values{"sort": {"-_createdAt"}},
		},
		{
			name:  "field.desc passes through unchanged",
			query: listQuery{Sort: "_createdAt.desc"},
			want:  url.Values{"sort": {"_createdAt.desc"}},
		},
		{
			name:  "field.asc passes through unchanged",
			query: listQuery{Sort: "title.asc", Order: "desc"},
			want:  url.Values{"sort": {"title.asc"}},
		},
		{
			name:    "invalid order errors",
			query:   listQuery{Sort: "title", Order: "sideways"},
			wantErr: "invalid order",
		},
		{
			name:  "status alone becomes a JSON filter",
			query: listQuery{Status: "published"},
			want:  url.Values{"filter": {`{"_status":"published"}`}},
		},
		{
			name:  "status merges into an existing filter",
			query: listQuery{Filter: `{"featured":true}`, Status: "draft"},
			want:  url.Values{"filter": {`{"_status":"draft","featured":true}`}},
		},
		{
			name:  "filter alone is passed through verbatim",
			query: listQuery{Filter: `{"featured":true}`},
			want:  url.Values{"filter": {`{"featured":true}`}},
		},
		{
			name:    "non-object filter combined with status errors",
			query:   listQuery{Filter: `[1,2,3]`, Status: "published"},
			wantErr: "--filter must be a JSON object",
		},
		{
			name:  "pagination, search, expand and count",
			query: listQuery{Limit: 10, Offset: 5, Page: 2, Search: "hello", Expand: "author", Count: true},
			want: url.Values{
				"limit":  {"10"},
				"offset": {"5"},
				"page":   {"2"},
				"search": {"hello"},
				"expand": {"author"},
				"count":  {"true"},
			},
		},
		{
			name:  "zero values are omitted",
			query: listQuery{Limit: 0, Offset: 0, Page: 0},
			want:  url.Values{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildListQuery(tt.query)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (params %v)", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Encode() != tt.want.Encode() {
				t.Errorf("got %q, want %q", got.Encode(), tt.want.Encode())
			}
		})
	}
}

// The v2 server expects prefix notation for sort, never a JSON object.
func TestBuildListQuerySortIsNeverJSON(t *testing.T) {
	cases := []listQuery{
		{Sort: "title"},
		{Sort: "title", Order: "asc"},
		{Sort: "_createdAt", Order: "desc"},
		{Sort: "-_createdAt"},
		{Sort: "_createdAt.desc"},
		{Sort: "title", Order: "desc", Filter: `{"featured":true}`, Status: "published", Limit: 5},
	}

	for _, q := range cases {
		params, err := buildListQuery(q)
		if err != nil {
			t.Fatalf("buildListQuery(%+v): %v", q, err)
		}
		sortValue := params.Get("sort")
		if strings.ContainsAny(sortValue, "{}") {
			t.Errorf("sort %q contains JSON braces", sortValue)
		}
		encodedSort := url.Values{"sort": {sortValue}}.Encode()
		if strings.Contains(encodedSort, "%7B") {
			t.Errorf("encoded sort %q contains %%7B", encodedSort)
		}
	}
}

func TestDocumentsListSendsV2QueryParams(t *testing.T) {
	// --url/--token take priority over the config file, but point HOME at a
	// temp dir so a developer's ~/.trokky/config.yaml can never leak in.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TROKKY_URL", "")
	t.Setenv("TROKKY_TOKEN", "")
	t.Setenv("TROKKY_INSTANCE", "")

	var gotPath string
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"documents":[]}}`))
	}))
	defer server.Close()

	rootCmd.SetArgs([]string{
		"documents", "list", "posts",
		"--url", server.URL,
		"--token", "t",
		"--sort", "_createdAt",
		"--order", "desc",
		"--limit", "5",
		"-q",
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if gotPath != "/collections/posts" {
		t.Errorf("path = %q, want %q", gotPath, "/collections/posts")
	}
	if got := gotQuery.Get("sort"); got != "-_createdAt" {
		t.Errorf("sort = %q, want %q", got, "-_createdAt")
	}
	if got := gotQuery.Get("limit"); got != "5" {
		t.Errorf("limit = %q, want %q", got, "5")
	}
	if got := gotQuery.Encode(); got != "limit=5&sort=-_createdAt" {
		t.Errorf("query = %q, want %q", got, "limit=5&sort=-_createdAt")
	}
}
