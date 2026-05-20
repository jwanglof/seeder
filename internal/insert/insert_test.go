package insert_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/infer"
	"github.com/mickamy/seeder/internal/insert"
	"github.com/mickamy/seeder/internal/introspect"
)

type mockDriver struct {
	bulkCalls []bulkCall
}

type bulkCall struct {
	table string
	rows  int
}

func (m *mockDriver) Close(_ context.Context) error                { return nil }
func (m *mockDriver) Truncate(_ context.Context, _ []string) error { return nil }
func (m *mockDriver) BulkInsert(_ context.Context, table string, _ []string, rows [][]any) (int64, error) {
	m.bulkCalls = append(m.bulkCalls, bulkCall{table: table, rows: len(rows)})

	return int64(len(rows)), nil
}

func (m *mockDriver) ColumnValues(_ context.Context, _ string, _ []string) (map[string][]any, error) {
	return map[string][]any{}, nil
}

func TestIsSerialDefault(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   *string
		want bool
	}{
		{"nil (no default)", nil, false},
		{"empty default", new(""), false},
		{"literal zero", new("0"), false},
		{"current_timestamp", new("current_timestamp"), false},
		{"string literal", new("'foo'::text"), false},
		{"nextval regclass", new("nextval('users_id_seq'::regclass)"), true},
		{"nextval bare", new("nextval('seq')"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := insert.IsSerialDefault(tc.in); got != tc.want {
				t.Errorf("IsSerialDefault(%v) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestInsertTable_PerTableRowsOverride(t *testing.T) {
	t.Parallel()

	usersTable := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "id", Kind: introspect.KindInt, IsIdentity: true},
			{Name: "name", Kind: introspect.KindString},
		},
	}
	ordersTable := introspect.Table{
		Name: "orders",
		Columns: []introspect.Column{
			{Name: "id", Kind: introspect.KindInt, IsIdentity: true},
			{Name: "title", Kind: introspect.KindString},
		},
	}

	opts := insert.Options{
		Rows:        100,
		RowsByTable: map[string]int{"users": 5},
	}

	drv := &mockDriver{}
	faker := gofakeit.New(42)
	pool := insert.NewPool(0)

	usersStats, err := insert.InsertTable(t.Context(), drv, usersTable, opts, faker, pool, nil, io.Discard)
	if err != nil {
		t.Fatalf("insertTable users: %v", err)
	}
	ordersStats, err := insert.InsertTable(t.Context(), drv, ordersTable, opts, faker, pool, nil, io.Discard)
	if err != nil {
		t.Fatalf("insertTable orders: %v", err)
	}

	if usersStats.Rows != 5 {
		t.Errorf("users Rows = %d; want 5 (from RowsByTable)", usersStats.Rows)
	}
	if ordersStats.Rows != 100 {
		t.Errorf("orders Rows = %d; want 100 (from default Options.Rows)", ordersStats.Rows)
	}

	if len(drv.bulkCalls) != 2 {
		t.Fatalf("BulkInsert calls = %d; want 2", len(drv.bulkCalls))
	}
	if drv.bulkCalls[0].table != "users" || drv.bulkCalls[0].rows != 5 {
		t.Errorf("call[0] = %+v; want {users 5}", drv.bulkCalls[0])
	}
	if drv.bulkCalls[1].table != "orders" || drv.bulkCalls[1].rows != 100 {
		t.Errorf("call[1] = %+v; want {orders 100}", drv.bulkCalls[1])
	}
}

func TestPlanColumns_GeneratorOverrideAppliesToNonFKColumn(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "email", Kind: introspect.KindString},
		},
	}
	overrides := map[string]insert.ColumnOverride{
		"email": {Generator: "Email"},
	}

	cols, err := insert.PlanColumns(table, gofakeit.New(42), infer.LocaleEN, overrides)
	if err != nil {
		t.Fatalf("PlanColumns: %v", err)
	}
	if len(cols) != 1 || cols[0].Gen() == nil {
		t.Fatalf("len(cols) = %d, gen-nil = %v; want 1 with non-nil gen", len(cols), cols[0].Gen() == nil)
	}
	got, ok := cols[0].Gen()().(string)
	if !ok || !strings.Contains(got, "@") {
		t.Errorf("override-generated value = %v; want email-like string", got)
	}
}

func TestPlanColumns_OverrideIgnoredForFKColumn(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "orders",
		Columns: []introspect.Column{
			{Name: "user_id", Kind: introspect.KindInt},
		},
		ForeignKeys: []introspect.ForeignKey{
			{Columns: []string{"user_id"}, ReferencedTable: "users", ReferencedColumns: []string{"id"}},
		},
	}
	overrides := map[string]insert.ColumnOverride{
		"user_id": {Value: 999},
	}

	cols, err := insert.PlanColumns(table, gofakeit.New(42), infer.LocaleEN, overrides)
	if err != nil {
		t.Fatalf("PlanColumns: %v", err)
	}
	if len(cols) != 1 {
		t.Fatalf("len(cols) = %d; want 1", len(cols))
	}
	if cols[0].Gen() != nil {
		t.Errorf("FK column should not carry a generator; got gen = %v", cols[0].Gen())
	}
}

func TestPlanColumns_ValueOverrideStableAcrossCalls(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "country", Kind: introspect.KindString},
		},
	}
	overrides := map[string]insert.ColumnOverride{
		"country": {Value: "JP"},
	}

	cols, err := insert.PlanColumns(table, gofakeit.New(42), infer.LocaleEN, overrides)
	if err != nil {
		t.Fatalf("PlanColumns: %v", err)
	}
	if cols[0].Gen() == nil {
		t.Fatal("gen is nil; want value-override generator")
	}
	for range 5 {
		if got := cols[0].Gen()(); got != "JP" {
			t.Errorf("value override = %v; want stable JP literal", got)
		}
	}
}

func TestPlanColumns_OverrideBeatsSerialDefault(t *testing.T) {
	t.Parallel()

	table := introspect.Table{
		Name: "users",
		Columns: []introspect.Column{
			{Name: "rank", Kind: introspect.KindInt, Default: new("nextval('rank_seq'::regclass)")},
		},
	}
	overrides := map[string]insert.ColumnOverride{
		"rank": {Value: 7},
	}

	cols, err := insert.PlanColumns(table, gofakeit.New(42), infer.LocaleEN, overrides)
	if err != nil {
		t.Fatalf("PlanColumns: %v", err)
	}
	if len(cols) != 1 || cols[0].Gen() == nil {
		t.Fatalf("len(cols) = %d; want 1 with non-nil gen (override beats serial-default skip)", len(cols))
	}
	if got := cols[0].Gen()(); got != 7 {
		t.Errorf("value override = %v; want 7", got)
	}
}
