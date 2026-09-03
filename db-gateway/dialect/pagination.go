package dialect

import (
	"fmt"
	"strings"
)

// RewritePositional converts '?' placeholders into Oracle-style :N binds
// (:1, :2, ...). go-ora rejects '?' and only understands :name/:N binds.
// Text inside single-quoted string literals is left untouched.
func RewritePositional(sql string, count int) string {
	if count <= 0 || !strings.ContainsRune(sql, '?') {
		return sql
	}
	var b strings.Builder
	b.Grow(len(sql) + 8*count)
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
		if ch == '?' && !inStr && n < count {
			n++
			fmt.Fprintf(&b, ":%d", n)
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func WrapPagination(driver, sql string, page, pageSize int) string {
	if page <= 0 || pageSize <= 0 {
		return sql
	}
	offset := (page - 1) * pageSize
	switch driver {
	case "mysql", "opengauss", "postgres", "ob-mysql", "gd-mysql", "sqlite", "clickhouse":
		return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, pageSize, offset)
	case "oracle", "ob-oracle", "gd-oracle", "sqlserver":
		return fmt.Sprintf("%s OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", sql, offset, pageSize)
	default:
		return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, pageSize, offset)
	}
}
