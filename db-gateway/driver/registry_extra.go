//go:build all

package driver

import (
	"database/sql"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	registry["ob-mysql"] = func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	}
	registry["ob-oracle"] = func(dsn string) (*sql.DB, error) {
		return sql.Open("oracle", dsn)
	}
	registry["gd-mysql"] = func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	}
	registry["gd-oracle"] = func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	}
	registry["sqlite"] = func(dsn string) (*sql.DB, error) {
		return sql.Open("sqlite3", dsn)
	}
	registry["clickhouse"] = func(dsn string) (*sql.DB, error) {
		return sql.Open("clickhouse", dsn)
	}
	registry["sqlserver"] = func(dsn string) (*sql.DB, error) {
		return sql.Open("sqlserver", dsn)
	}
}
