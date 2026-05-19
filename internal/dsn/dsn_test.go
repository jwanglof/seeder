package dsn_test

import (
	"testing"

	"github.com/mickamy/seeder/internal/dsn"
)

func TestScheme(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"postgres://localhost/db", "postgres"},
		{"postgresql://localhost/db", "postgresql"},
		{"mysql://localhost:3306/db", "mysql"},
		{"no-scheme", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := dsn.Scheme(tc.in); got != tc.want {
			t.Errorf("Scheme(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

//nolint:gosec // G101: URL fixtures use placeholder credentials, not real
func TestToMySQLDSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "full user/pass/host/port/db",
			in:   "mysql://root:pass@localhost:3306/dev",
			want: "root:pass@tcp(localhost:3306)/dev",
		},
		{
			name: "no password",
			in:   "mysql://user@host:3306/db",
			want: "user@tcp(host:3306)/db",
		},
		{
			name: "no user info",
			in:   "mysql://localhost:3306/dev",
			want: "tcp(localhost:3306)/dev",
		},
		{
			name: "with query params",
			in:   "mysql://root:pass@localhost:3306/dev?parseTime=true",
			want: "root:pass@tcp(localhost:3306)/dev?parseTime=true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := dsn.ToMySQLDSN(tc.in)
			if err != nil {
				t.Fatalf("ToMySQLDSN(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ToMySQLDSN(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}
