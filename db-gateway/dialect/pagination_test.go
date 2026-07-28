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

func TestWrapPaginationOBOracle(t *testing.T) {
	sql := "SELECT * FROM orders ORDER BY id"
	got := WrapPagination("ob-oracle", sql, 3, 15)
	want := "SELECT * FROM orders ORDER BY id OFFSET 30 ROWS FETCH NEXT 15 ROWS ONLY"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
