package infer

import "fmt"

// Locale selects the dictionary used by name-rule generators. Empty / "en"
// keeps the gofakeit defaults; richer locales (e.g., "ja") swap in
// language-specific dictionaries while leaving locale-neutral rules unchanged.
type Locale string

const (
	LocaleEN Locale = ""
	LocaleJA Locale = "ja"
	LocaleSV Locale = "sv"
)

// ParseLocale accepts the user-facing flag value and returns the canonical
// Locale. "" and "en" both resolve to LocaleEN so the flag can be omitted.
func ParseLocale(s string) (Locale, error) {
	switch s {
	case "", "en":
		return LocaleEN, nil
	case "ja":
		return LocaleJA, nil
	case "sv":
		return LocaleSV, nil
	default:
		return "", fmt.Errorf("unknown locale %q (supported: en, ja, sv)", s)
	}
}
