package insert_test

import (
	"reflect"
	"testing"

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
