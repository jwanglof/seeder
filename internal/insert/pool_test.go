package insert_test

import (
	"reflect"
	"testing"

	"github.com/brianvoe/gofakeit/v7"

	"github.com/mickamy/seeder/internal/insert"
)

func TestPool_ReplaceRespectsCapacity(t *testing.T) {
	t.Parallel()

	p := insert.NewPool(3)
	p.Replace("users", map[string][]any{
		"id": {1, 2, 3, 4, 5},
	})

	got := p.Values("users", "id")
	want := []any{3, 4, 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Values = %v; want %v (older entries should be dropped)", got, want)
	}
}

func TestPool_ReplaceCopiesInput(t *testing.T) {
	t.Parallel()

	src := []any{1, 2, 3}
	p := insert.NewPool(0)
	p.Replace("users", map[string][]any{"id": src})
	src[0] = 999

	got := p.Values("users", "id")
	if got[0] == 999 {
		t.Errorf("pool shares the caller's slice; mutation leaked through")
	}
}

func TestPool_ValuesUnknownReturnsNil(t *testing.T) {
	t.Parallel()

	p := insert.NewPool(0)
	if got := p.Values("missing", "missing"); got != nil {
		t.Errorf("Values for missing key = %v; want nil", got)
	}
}

func TestPool_DefaultCapacityWhenZero(t *testing.T) {
	t.Parallel()

	p := insert.NewPool(0)
	if p.Capacity <= 0 {
		t.Errorf("NewPool(0).Capacity = %d; want positive default", p.Capacity)
	}
}

func TestPool_PickRowMissingTableReturnsNil(t *testing.T) {
	t.Parallel()

	p := insert.NewPool(0)
	if got := p.PickRow(gofakeit.New(42), "missing"); got != nil {
		t.Errorf("PickRow for missing table = %v; want nil", got)
	}
}

// Confirms that PickRow returns a single tuple — the foundation for composite
// FK pickers, which must read multiple columns at the same row index.
func TestPool_PickRowReturnsConsistentTuple(t *testing.T) {
	t.Parallel()

	p := insert.NewPool(0)
	p.Replace("users", map[string][]any{
		"id":   {1, 2, 3},
		"name": {"a", "b", "c"},
	})

	pairs := map[any]any{1: "a", 2: "b", 3: "c"}
	for i := range 20 {
		row := p.PickRow(gofakeit.New(uint64(i)), "users")
		if row == nil {
			t.Fatalf("PickRow returned nil on attempt %d", i)
		}
		if want, ok := pairs[row["id"]]; !ok || row["name"] != want {
			t.Errorf("tuple desync: id=%v name=%v (want name=%v)", row["id"], row["name"], want)
		}
	}
}
