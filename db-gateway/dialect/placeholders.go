package dialect

import (
	"fmt"
	"strings"
)

// UsesNumberPlaceholders reports whether a driver's wire protocol expects
// PostgreSQL-style "$N" ordinals. pgx's database/sql adapter forwards the SQL
// verbatim, so a '?' placeholder reaches the server as the jsonb `?` operator
// and every parameterized query fails to parse.
func UsesNumberPlaceholders(driver string) bool {
	return driver == "postgres" || driver == "opengauss"
}

// NumberPlaceholders converts '?' placeholders into PostgreSQL-style ordinals
// ($1, $2, ...). Text inside single-quoted string literals is left untouched.
func NumberPlaceholders(sql string) string {
	if !strings.ContainsRune(sql, '?') {
		return sql
	}
	var b strings.Builder
	b.Grow(len(sql) + 8)
	n := 0
	inStr := false
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if ch == '\'' {
			if inStr && i+1 < len(sql) && sql[i+1] == '\'' { // '' escaped quote
				b.WriteByte(ch)
				b.WriteByte(ch)
				i++
				continue
			}
			inStr = !inStr
			b.WriteByte(ch)
			continue
		}
		if ch == '?' && !inStr {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}
