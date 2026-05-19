package insert_test

import (
	"context"
	"io"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

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
	pool := make(map[string]map[string][]any)

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
