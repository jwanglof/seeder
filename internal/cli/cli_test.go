package cli_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mickamy/seeder/internal/cli"
	"github.com/mickamy/seeder/internal/introspect"
)

func TestReorderArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{
			name: "flags then dsn",
			in:   []string{"--rows", "10", "--dry-run", "postgres://x"},
			want: []string{"--rows", "10", "--dry-run", "postgres://x"},
		},
		{
			name: "dsn first",
			in:   []string{"postgres://x", "--rows", "10", "--dry-run"},
			want: []string{"--rows", "10", "--dry-run", "postgres://x"},
		},
		{
			name: "equals form",
			in:   []string{"postgres://x", "--rows=10"},
			want: []string{"--rows=10", "postgres://x"},
		},
		{
			name: "double-dash terminator preserves trailing dashes",
			in:   []string{"--rows", "10", "--", "-weird-dsn-with-dashes"},
			want: []string{"--rows", "10", "-weird-dsn-with-dashes"},
		},
		{
			name: "bool flag is not greedy",
			in:   []string{"--dry-run", "postgres://x"},
			want: []string{"--dry-run", "postgres://x"},
		},
		{
			name:    "value flag missing value",
			in:      []string{"--rows"},
			want:    nil,
			wantErr: true,
		},
		{
			name: "negative seed value",
			in:   []string{"postgres://x", "--seed", "-42"},
			want: []string{"--seed", "-42", "postgres://x"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := cli.ReorderArgs(tc.in, cli.ValueFlags)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil; out=%v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v; want %v", got, tc.want)
			}
		})
	}
}

func TestSplitTrim(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want []string
	}{
		{"users,orders", []string{"users", "orders"}},
		{"users, orders, comments", []string{"users", "orders", "comments"}},
		{"users,,orders", []string{"users", "orders"}},
		{"  ", []string{}},
		{"", []string{}},
	}
	for _, tc := range cases {
		got := cli.SplitTrim(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SplitTrim(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

func TestIncludeTables(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{{Name: "users"}, {Name: "orders"}, {Name: "comments"}}

	got, missing := cli.IncludeTables(tables, []string{"users", "comments"})
	if len(missing) != 0 {
		t.Errorf("missing = %v; want []", missing)
	}
	gotNames := tableNames(got)
	wantNames := []string{"users", "comments"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Errorf("got %v; want %v (input order preserved)", gotNames, wantNames)
	}

	_, missing = cli.IncludeTables(tables, []string{"users", "bogus"})
	if !slices.Contains(missing, "bogus") {
		t.Errorf("missing = %v; want bogus included", missing)
	}
}

func TestExcludeTables(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{{Name: "users"}, {Name: "orders"}, {Name: "comments"}}

	got, missing := cli.ExcludeTables(tables, []string{"orders"})
	if len(missing) != 0 {
		t.Errorf("missing = %v; want []", missing)
	}
	gotNames := tableNames(got)
	wantNames := []string{"users", "comments"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Errorf("got %v; want %v", gotNames, wantNames)
	}

	_, missing = cli.ExcludeTables(tables, []string{"bogus"})
	if !slices.Contains(missing, "bogus") {
		t.Errorf("missing = %v; want bogus included", missing)
	}
}

func TestOrphanFKs_MissingParentNotNull(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{ordersTable(false)}
	got := cli.OrphanFKs(tables)
	if len(got) != 1 {
		t.Fatalf("orphan count = %d; want 1", len(got))
	}
	if got[0].FromTable != "orders" || got[0].FromCol != "user_id" || got[0].ToTable != "users" {
		t.Errorf("orphan = %+v; want orders.user_id -> users.id", got[0])
	}
}

func TestOrphanFKs_NullableIgnored(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{ordersTable(true)}
	if got := cli.OrphanFKs(tables); len(got) != 0 {
		t.Errorf("nullable orphan should be ignored; got %v", got)
	}
}

func TestOrphanFKs_ParentPresent(t *testing.T) {
	t.Parallel()

	tables := []introspect.Table{ordersTable(false), {Name: "users"}}
	if got := cli.OrphanFKs(tables); len(got) != 0 {
		t.Errorf("with parent present orphan count = %d; want 0", len(got))
	}
}

func TestRun_MutuallyExclusiveFilters(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder
	code := cli.Run([]string{"postgres://x", "--tables", "a", "--exclude", "b"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d; want 2", code)
	}
	if !strings.Contains(stderr.String(), "mutually exclusive") {
		t.Errorf("stderr = %q; want mention of mutual exclusion", stderr.String())
	}
}

func TestRun_MissingDSN(t *testing.T) {
	t.Parallel()

	var stdout, stderr strings.Builder
	code := cli.Run(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code = %d; want 2 (Usage)", code)
	}
	if !strings.Contains(stderr.String(), "missing <dsn>") {
		t.Errorf("stderr = %q; want missing dsn message", stderr.String())
	}
}

func ordersTable(nullable bool) introspect.Table {
	return introspect.Table{
		Name: "orders",
		Columns: []introspect.Column{
			{Name: "id", Kind: introspect.KindInt},
			{Name: "user_id", Kind: introspect.KindInt, Nullable: nullable},
		},
		ForeignKeys: []introspect.ForeignKey{
			{Name: "fk_user", Columns: []string{"user_id"}, ReferencedTable: "users", ReferencedColumns: []string{"id"}},
		},
	}
}

func tableNames(tables []introspect.Table) []string {
	out := make([]string, 0, len(tables))
	for _, t := range tables {
		out = append(out, t.Name)
	}

	return out
}
