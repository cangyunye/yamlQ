package dialect

import "fmt"

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
