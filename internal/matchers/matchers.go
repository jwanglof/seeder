package matchers

import (
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Matcher asserts something about v and reports failures via t.
type Matcher func(t *testing.T, v any)

// Member asserts v is a string contained in list.
func Member(list []string) Matcher {
	return func(t *testing.T, v any) {
		t.Helper()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("got %T; want string", v)
		}
		if !slices.Contains(list, s) {
			t.Errorf("got %q; not in expected list", s)
		}
	}
}

// Equal asserts v deeply equals want; safe for slices, maps, and other
// non-comparable types.
func Equal(want any) Matcher {
	return func(t *testing.T, v any) {
		t.Helper()
		if !reflect.DeepEqual(v, want) {
			t.Errorf("got %v; want %v", v, want)
		}
	}
}

// Match asserts v is a string matched by re.
func Match(re *regexp.Regexp) Matcher {
	return func(t *testing.T, v any) {
		t.Helper()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("got %T; want string", v)
		}
		if !re.MatchString(s) {
			t.Errorf("got %q; want match %s", s, re)
		}
	}
}

// MatchPrefix asserts v is a string starting with prefix.
func MatchPrefix(prefix string) Matcher {
	return func(t *testing.T, v any) {
		t.Helper()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("got %T; want string", v)
		}
		if !strings.HasPrefix(s, prefix) {
			t.Errorf("got %q; want prefix %q", s, prefix)
		}
	}
}

// Type asserts v has dynamic type T.
func Type[T any]() Matcher {
	return func(t *testing.T, v any) {
		t.Helper()
		if _, ok := v.(T); !ok {
			var zero T
			t.Errorf("got %T; want %T", v, zero)
		}
	}
}

// NonEmpty asserts v is a non-empty string.
func NonEmpty() Matcher {
	return func(t *testing.T, v any) {
		t.Helper()
		s, ok := v.(string)
		if !ok {
			t.Fatalf("got %T; want string", v)
		}
		if s == "" {
			t.Errorf("got empty string")
		}
	}
}
