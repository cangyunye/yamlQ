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
}

func Open(driver, dsn string) (*sql.DB, error) {
	fn, ok := registry[driver]
	if !ok {
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
	return fn(dsn)
}

// Register adds a driver to the registry. Production drivers register via
// init; this exported hook exists mainly for tests that plug in a fake.
func Register(name string, fn OpenFunc) {
	registry[name] = fn
}

func Supported(driver string) bool {
	_, ok := registry[driver]
	return ok
}
