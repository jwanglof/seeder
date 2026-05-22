//nolint:testpackage // exercises unexported colSpec, newCompositeUniques, compositeKey
package insert

import (
	"slices"
	"testing"
)

func TestNewCompositeUniques_ResolvesColumnIndexes(t *testing.T) {
	t.Parallel()

	cols := []colSpec{
		{name: "id"}, {name: "user_id"}, {name: "role"}, {name: "created_at"},
	}
	got := newCompositeUniques(cols, [][]string{
		{"user_id", "role"},
		{"role", "created_at"},
	})
	if len(got) != 2 {
		t.Fatalf("got %d specs; want 2", len(got))
	}
	if !slices.Equal(got[0].colIdx, []int{1, 2}) {
		t.Errorf("first spec colIdx = %v; want [1 2]", got[0].colIdx)
	}
	if !slices.Equal(got[1].colIdx, []int{2, 3}) {
		t.Errorf("second spec colIdx = %v; want [2 3]", got[1].colIdx)
	}
}

func TestNewCompositeUniques_SkipsConstraintsReferencingUnwrittenColumns(t *testing.T) {
	t.Parallel()

	cols := []colSpec{{name: "id"}, {name: "user_id"}}
	got := newCompositeUniques(cols, [][]string{
		{"user_id", "missing"},
		{"user_id"},
	})
	if len(got) != 1 {
		t.Fatalf("got %d specs; want 1 (only the all-resolved one)", len(got))
	}
	if !slices.Equal(got[0].colIdx, []int{1}) {
		t.Errorf("colIdx = %v; want [1]", got[0].colIdx)
	}
}

func TestNewCompositeUniques_EmptyConstraintsReturnsNil(t *testing.T) {
	t.Parallel()

	cols := []colSpec{{name: "id"}}
	if got := newCompositeUniques(cols, nil); got != nil {
		t.Errorf("got %v; want nil", got)
	}
}

func TestCompositeKey_DistinguishesTuples(t *testing.T) {
	t.Parallel()

	cols := []int{0, 1}
	a := compositeKey([]any{"a", "bc"}, cols)
	b := compositeKey([]any{"ab", "c"}, cols)
	if a == b {
		t.Errorf("compositeKey collides for ('a','bc') vs ('ab','c'); both got %q", a)
	}
}

func TestCompositeKey_HandlesMixedTypes(t *testing.T) {
	t.Parallel()

	cols := []int{0, 1, 2}
	a := compositeKey([]any{42, "x", true}, cols)
	b := compositeKey([]any{42, "x", true}, cols)
	if a != b {
		t.Errorf("same tuple produced different keys: %q vs %q", a, b)
	}
}
