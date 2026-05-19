//nolint:testpackage // checks the package-private nameRules invariant.
package infer

import "testing"

// LocaleEN acts as the fallback in nameRule.gen; every entry in nameRules must
// register it explicitly, otherwise Pick would call a nil generator for any
// locale that lacks its own override.
func TestNameRules_AllHaveLocaleEN(t *testing.T) {
	t.Parallel()

	for _, r := range nameRules {
		if _, ok := r.gens[LocaleEN]; !ok {
			t.Parallel()

			t.Errorf("rule %q missing LocaleEN entry", r.label)
		}
	}
}
