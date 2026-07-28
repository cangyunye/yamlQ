package dialect

import "testing"

func TestWrapPaginationMySQL(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id"
	got := WrapPagination("mysql", sql, 2, 20)
	want := "SELECT * FROM users ORDER BY id LIMIT 20 OFFSET 20"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationFirstPage(t *testing.T) {
	sql := "SELECT * FROM users"
	got := WrapPagination("mysql", sql, 1, 10)
	want := "SELECT * FROM users LIMIT 10 OFFSET 0"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationNoPage(t *testing.T) {
	sql := "SELECT * FROM users"
	got := WrapPagination("mysql", sql, 0, 10)
	if got != sql {
		t.Fatalf("page=0 should return original SQL, got %q", got)
	}
}

func TestWrapPaginationOBMySQL(t *testing.T) {
	sql := "SELECT * FROM orders ORDER BY id"
	got := WrapPagination("ob-mysql", sql, 2, 20)
	want := "SELECT * FROM orders ORDER BY id LIMIT 20 OFFSET 20"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationGDMysql(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id"
	got := WrapPagination("gd-mysql", sql, 2, 20)
	want := "SELECT * FROM users ORDER BY id LIMIT 20 OFFSET 20"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationGDOracle(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id"
	got := WrapPagination("gd-oracle", sql, 3, 15)
	want := "SELECT * FROM users ORDER BY id OFFSET 30 ROWS FETCH NEXT 15 ROWS ONLY"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationOBOracle(t *testing.T) {
	sql := "SELECT * FROM orders ORDER BY id"
	got := WrapPagination("ob-oracle", sql, 3, 15)
	want := "SELECT * FROM orders ORDER BY id OFFSET 30 ROWS FETCH NEXT 15 ROWS ONLY"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationSQLite(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id"
	got := WrapPagination("sqlite", sql, 2, 20)
	want := "SELECT * FROM users ORDER BY id LIMIT 20 OFFSET 20"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationClickHouse(t *testing.T) {
	sql := "SELECT * FROM events ORDER BY ts"
	got := WrapPagination("clickhouse", sql, 3, 50)
	want := "SELECT * FROM events ORDER BY ts LIMIT 50 OFFSET 100"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationSQLServer(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id"
	got := WrapPagination("sqlserver", sql, 2, 20)
	want := "SELECT * FROM users ORDER BY id OFFSET 20 ROWS FETCH NEXT 20 ROWS ONLY"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationSQLServerFirstPage(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id"
	got := WrapPagination("sqlserver", sql, 1, 10)
	want := "SELECT * FROM users ORDER BY id OFFSET 0 ROWS FETCH NEXT 10 ROWS ONLY"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
