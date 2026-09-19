//go:build all

package driver

import (
	"errors"
	"strings"
	"testing"
)

func TestSupportedExtra(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "ob-mysql", input: "ob-mysql", want: true},
		{name: "ob-oracle", input: "ob-oracle", want: true},
		{name: "gd-mysql", input: "gd-mysql", want: true},
		{name: "gd-oracle", input: "gd-oracle", want: true},
		{name: "sqlite", input: "sqlite", want: true},
		{name: "clickhouse", input: "clickhouse", want: true},
		{name: "sqlserver", input: "sqlserver", want: true},
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

func TestOpenReturnsDBExtra(t *testing.T) {
	cases := []struct {
		name   string
		driver string
		dsn    string
	}{
		{name: "ob-mysql", driver: "ob-mysql", dsn: "root:pass@tcp(127.0.0.1:2883)/test"},
		{name: "ob-oracle", driver: "ob-oracle", dsn: "user@tenant/pass@127.0.0.1:2883/test"},
		{name: "ob-oracle-wire", driver: "ob-oracle", dsn: "mysql://user@tenant:pass@127.0.0.1:2883/test?cluster=obcluster"},
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

func TestIsOceanBaseOracleTenantMySQLDriverError(t *testing.T) {
	match := errors.New("Error 1235 (0A000): Oracle tenant for current client driver is not supported")
	if !isOceanBaseOracleTenantMySQLDriverError(match) {
		t.Errorf("expected OB Error 1235 to be detected")
	}
	lower := errors.New("error 1235 (0a000): oracle tenant for current client driver is not supported")
	if !isOceanBaseOracleTenantMySQLDriverError(lower) {
		t.Errorf("expected case-insensitive OB Error 1235 to be detected")
	}
	unrelated := errors.New("dial tcp 127.0.0.1:1521: connect: connection refused")
	if isOceanBaseOracleTenantMySQLDriverError(unrelated) {
		t.Errorf("did not expect unrelated error to be detected")
	}
	if isOceanBaseOracleTenantMySQLDriverError(nil) {
		t.Errorf("nil error must not be detected")
	}
}

func TestIsOBOracleTNSHandshakeFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "closed-network", err: errors.New("use of closed network connection"), want: true},
		{name: "unexpected-packet", err: errors.New("unexpected packet header"), want: true},
		{name: "tns-keyword", err: errors.New("TNS: could not resolve the connection identifier"), want: true},
		{name: "protocol-error", err: errors.New("protocol error"), want: true},
		{name: "refused", err: errors.New("dial tcp: connection refused"), want: false},
		{name: "timeout", err: errors.New("i/o timeout"), want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOBOracleTNSHandshakeFailure(tc.err); got != tc.want {
				t.Errorf("isOBOracleTNSHandshakeFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsMissingPortInAddressError(t *testing.T) {
	if !isMissingPortInAddressError(errors.New("missing port in address")) {
		t.Errorf("expected missing port error to be detected")
	}
	if isMissingPortInAddressError(errors.New("dial tcp: connection refused")) {
		t.Errorf("did not expect unrelated error to be detected")
	}
	if isMissingPortInAddressError(nil) {
		t.Errorf("nil error must not be detected")
	}
}

func TestAnnotateConnectError(t *testing.T) {
	err1235 := errors.New("Error 1235 (0A000): Oracle tenant for current client driver is not supported")

	annotated := AnnotateConnectError("ob-oracle", "oracle://user:pass@127.0.0.1:2883/test", err1235)
	text := annotated.Error()
	if !strings.Contains(text, "oboracle://") {
		t.Errorf("expected hint to mention oboracle:// DSN, got: %s", text)
	}
	if !strings.Contains(text, err1235.Error()) {
		t.Errorf("expected original error to be preserved, got: %s", text)
	}

	missingPort := AnnotateConnectError("ob-oracle", "user@tenant/password@127.0.0.1:2883/test", errors.New("missing port in address"))
	if !strings.Contains(missingPort.Error(), "oracle://user@tenant:password@host:port") {
		t.Errorf("expected hint to mention oracle:// DSN format, got: %s", missingPort.Error())
	}

	handshake := AnnotateConnectError("ob-oracle", "oracle://user:pass@127.0.0.1:2883/test", errors.New("use of closed network connection"))
	if !strings.Contains(handshake.Error(), "oboracle://") {
		t.Errorf("expected handshake hint to mention oboracle:// DSN, got: %s", handshake.Error())
	}

	obMySQL1235 := AnnotateConnectError("ob-mysql", "root:pass@tcp(127.0.0.1:2883)/test", err1235)
	if !strings.Contains(obMySQL1235.Error(), "ob-oracle") {
		t.Errorf("expected ob-mysql 1235 hint to suggest ob-oracle, got: %s", obMySQL1235.Error())
	}

	other := errors.New("dial tcp: connection refused")
	if got := AnnotateConnectError("ob-oracle", "oracle://user:pass@127.0.0.1:2883/test", other); got != other {
		t.Errorf("unrelated error should pass through unchanged, got: %v", got)
	}
	nonOB := AnnotateConnectError("mysql", "root:pass@tcp(127.0.0.1:3306)/test", err1235)
	if nonOB != err1235 {
		t.Errorf("non OB driver should pass through unchanged")
	}
}
