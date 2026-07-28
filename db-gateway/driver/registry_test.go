package driver

import "testing"

func TestSupported(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   bool
	}{
		{name: "mysql", input: "mysql", want: true},
		{name: "postgres", input: "postgres", want: true},
		{name: "oracle", input: "oracle", want: true},
		{name: "opengauss", input: "opengauss", want: true},
		{name: "ob-mysql", input: "ob-mysql", want: true},
		{name: "ob-oracle", input: "ob-oracle", want: true},
		{name: "sqlite", input: "sqlite", want: true},
		{name: "clickhouse", input: "clickhouse", want: true},
		{name: "sqlserver", input: "sqlserver", want: true},
		{name: "gd-mysql", input: "gd-mysql", want: true},
		{name: "gd-oracle", input: "gd-oracle", want: true},
		{name: "empty", input: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Supported(tc.input)
			if got != tc.want {
				t.Errorf("Supported(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestOpenReturnsDB(t *testing.T) {
	cases := []struct {
		name   string
		driver string
		dsn    string
	}{
		{name: "ob-mysql", driver: "ob-mysql", dsn: "root:pass@tcp(127.0.0.1:2883)/test"},
		{name: "ob-oracle", driver: "ob-oracle", dsn: "user@tenant:pass@tcp(127.0.0.1:2883)/test"},
		{name: "gd-mysql", driver: "gd-mysql", dsn: "root:pass@tcp(127.0.0.1:3306)/test"},
		{name: "gd-oracle", driver: "gd-oracle", dsn: "root:pass@tcp(127.0.0.1:3306)/test"},
		{name: "sqlite", driver: "sqlite", dsn: ":memory:"},
		{name: "clickhouse", driver: "clickhouse", dsn: "clickhouse://127.0.0.1:9000/default"},
		{name: "sqlserver", driver: "sqlserver", dsn: "sqlserver://sa:pass@127.0.0.1:1433?database=test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := Open(tc.driver, tc.dsn)
			if err != nil {
				t.Fatalf("Open(%q, %q) unexpected error: %v", tc.driver, tc.dsn, err)
			}
			if db == nil {
				t.Fatalf("Open(%q, %q) returned nil *sql.DB", tc.driver, tc.dsn)
			}
			db.Close()
		})
	}
}
