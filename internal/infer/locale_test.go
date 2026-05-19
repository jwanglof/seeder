package infer_test

import (
	"testing"

	"github.com/mickamy/seeder/internal/infer"
)

func TestParseLocale(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		want    infer.Locale
		wantErr bool
	}{
		{"", infer.LocaleEN, false},
		{"en", infer.LocaleEN, false},
		{"ja", infer.LocaleJA, false},
		{"EN", "", true},
		{"jp", "", true},
		{"fr", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := infer.ParseLocale(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseLocale(%q) err = %v; wantErr %v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("ParseLocale(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}
