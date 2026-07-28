package driver

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/sijms/go-ora/v2"
)

type OpenFunc func(dsn string) (*sql.DB, error)

var registry = map[string]OpenFunc{
	"mysql": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
	"postgres": func(dsn string) (*sql.DB, error) {
		return sql.Open("pgx", dsn)
	},
	"opengauss": func(dsn string) (*sql.DB, error) {
		return sql.Open("pgx", dsn)
	},
	"oracle": func(dsn string) (*sql.DB, error) {
		return sql.Open("oracle", dsn)
	},
	"ob-mysql": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
	"ob-oracle": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
	"gd-mysql": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
	"gd-oracle": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
}

func Open(driver, dsn string) (*sql.DB, error) {
	fn, ok := registry[driver]
	if !ok {
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
	return fn(dsn)
}

func Supported(driver string) bool {
	_, ok := registry[driver]
	return ok
}
