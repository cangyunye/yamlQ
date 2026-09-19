package dialect

import "testing"

func TestUsesNumberPlaceholders(t *testing.T) {
	for _, driver := range []string{"postgres", "opengauss"} {
		if !UsesNumberPlaceholders(driver) {
			t.Errorf("UsesNumberPlaceholders(%q) = false, want true", driver)
		}
	}
	for _, driver := range []string{"mysql", "ob-mysql", "ob-oracle", "gd-mysql", "oracle", "sqlite", "clickhouse", "sqlserver", ""} {
		if UsesNumberPlaceholders(driver) {
			t.Errorf("UsesNumberPlaceholders(%q) = true, want false", driver)
		}
	}
}

func TestNumberPlaceholders(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want string
	}{
		{name: "simple", sql: "SELECT * FROM t WHERE a = ? AND b = ?", want: "SELECT * FROM t WHERE a = $1 AND b = $2"},
		{name: "no-placeholder", sql: "SELECT * FROM t WHERE a = 1", want: "SELECT * FROM t WHERE a = 1"},
		{name: "string-literal-kept", sql: "SELECT * FROM t WHERE a LIKE '%' || ? || '%'", want: "SELECT * FROM t WHERE a LIKE '%' || $1 || '%'"},
		{name: "question-in-literal-kept", sql: "SELECT * FROM t WHERE a = 'is ? there' AND b = ?", want: "SELECT * FROM t WHERE a = 'is ? there' AND b = $1"},
		// A bare jsonb '?' operator sits outside any string literal, so it is
		// rewritten too; param-less queries never reach this function because
		// executeOne gates on len(Params) > 0.
		{name: "jsonb-operator-rewritten", sql: "SELECT * FROM t WHERE doc ? 'key' AND a = ?", want: "SELECT * FROM t WHERE doc $1 'key' AND a = $2"},
		{name: "escaped-quote-literal", sql: "SELECT * FROM t WHERE a = 'it''s ? here' AND b = ?", want: "SELECT * FROM t WHERE a = 'it''s ? here' AND b = $1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NumberPlaceholders(tc.sql); got != tc.want {
				t.Errorf("NumberPlaceholders(%q) = %q, want %q", tc.sql, got, tc.want)
			}
		})
	}
}
