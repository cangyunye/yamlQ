package driver

import "testing"

func TestSupported(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "mysql", input: "mysql", want: true},
		{name: "postgres", input: "postgres", want: true},
		{name: "opengauss", input: "opengauss", want: true},
		{name: "oracle", input: "oracle", want: true},
		{name: "empty", input: "", want: false},
		{name: "unsupported", input: "nonexistent", want: false},
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
		{name: "mysql", driver: "mysql", dsn: "root:pass@tcp(127.0.0.1:3306)/test"},
		{name: "postgres", driver: "postgres", dsn: "postgres://user:pass@127.0.0.1:5432/test"},
		{name: "oracle", driver: "oracle", dsn: "oracle://user:pass@127.0.0.1:1521/XE"},
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
