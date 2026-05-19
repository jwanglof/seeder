package dsn

import (
	"errors"
	"strings"
)

var ErrMissingScheme = errors.New("missing scheme in DSN: expected postgres://<...>")

func Scheme(dsn string) string {
	scheme, _, ok := strings.Cut(dsn, "://")
	if !ok {
		return ""
	}

	return scheme
}
