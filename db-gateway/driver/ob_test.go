package driver

import (
	"net/url"
	"testing"
)

func TestOracleBinds(t *testing.T) {
	cases := []struct {
		name   string
		driver string
		dsn    string
		want   bool
	}{
		{"plain oracle always :N", "oracle", "user:pw@host:1521/svc", true},
		{"ob-mysql never :N", "ob-mysql", "u:p@tcp(h:2883)/db", false},
		{"ob-oracle legacy simple string", "ob-oracle", "scott@oracle_tenant/password@192.168.1.100:2883/test_db", true},
		{"ob-oracle oracle url", "ob-oracle", "oracle://scott:pw@obproxy:1521/ORCL", true},
		{"ob-oracle bare go-sql dsn stays go-ora", "ob-oracle", "scott:pw@tcp(127.0.0.1:2883)/db", true},
		{"ob-oracle mysql url is wire", "ob-oracle", "mysql://scott@oracle_tenant:pw@127.0.0.1:2881/oracle_tenant", false},
		{"ob-oracle oboracle url is wire", "ob-oracle", "oboracle://scott@oracle_tenant:pw@127.0.0.1:2883/oracle_tenant?preset=oboracle", false},
		{"ob-oracle oceanbase-oracle url is wire", "ob-oracle", "oceanbase-oracle://scott@oracle_tenant:pw@127.0.0.1:2883/oracle_tenant", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OracleBinds(tc.driver, tc.dsn); got != tc.want {
				t.Errorf("OracleBinds(%q, %q) = %v, want %v", tc.driver, tc.dsn, got, tc.want)
			}
		})
	}
}

func TestResolveOBOracle(t *testing.T) {
	userOf := func(t *testing.T, raw string) string {
		t.Helper()
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("resolved DSN %q unparseable: %v", raw, err)
		}
		if u.User == nil {
			return ""
		}
		return u.User.Username()
	}
	queryOf := func(t *testing.T, raw string) url.Values {
		t.Helper()
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("resolved DSN %q unparseable: %v", raw, err)
		}
		return u.Query()
	}

	cases := []struct {
		name       string
		dsn        string
		wantDriver string
		check      func(t *testing.T, dsn string)
	}{
		{
			name:       "legacy simple string keeps go-ora",
			dsn:        "scott@oracle_tenant/password@192.168.1.100:2883/test_db",
			wantDriver: "oracle",
			check: func(t *testing.T, dsn string) {
				if dsn != "scott@oracle_tenant/password@192.168.1.100:2883/test_db" {
					t.Errorf("DSN mutated on go-ora path: %q", dsn)
				}
			},
		},
		{
			name:       "oracle url keeps go-ora",
			dsn:        "oracle://scott@oracle_tenant:pw@obproxy:1521/ORCL",
			wantDriver: "oracle",
			check: func(t *testing.T, dsn string) {
				if dsn != "oracle://scott@oracle_tenant:pw@obproxy:1521/ORCL" {
					t.Errorf("DSN mutated on oracle:// path: %q", dsn)
				}
			},
		},
		{
			name:       "mysql url rewrites to oboracle",
			dsn:        "mysql://sys:pw@127.0.0.1:2881/oracle_tenant",
			wantDriver: "oboracle",
			check: func(t *testing.T, dsn string) {
				u, err := url.Parse(dsn)
				if err != nil {
					t.Fatalf("unparseable: %v", err)
				}
				if u.Scheme != "oboracle" {
					t.Errorf("scheme = %q, want oboracle", u.Scheme)
				}
				if got := queryOf(t, dsn).Get("preset"); got != "oboracle" {
					t.Errorf("preset = %q, want oboracle", got)
				}
			},
		},
		{
			name:       "existing preset preserved",
			dsn:        "oboracle://sys:pw@127.0.0.1:2883/oracle_tenant?preset=oboracle",
			wantDriver: "oboracle",
			check: func(t *testing.T, dsn string) {
				if dsn != "oboracle://sys:pw@127.0.0.1:2883/oracle_tenant?preset=oboracle" {
					t.Errorf("DSN mutated: %q", dsn)
				}
			},
		},
		{
			name:       "cluster folded into user on 2883",
			dsn:        "mysql://sys@oracle_tenant:pw@127.0.0.1:2883/oracle_tenant?cluster=obcluster",
			wantDriver: "oboracle",
			check: func(t *testing.T, dsn string) {
				if got := userOf(t, dsn); got != "sys@oracle_tenant#obcluster" {
					t.Errorf("user = %q, want sys@oracle_tenant#obcluster", got)
				}
				if q := queryOf(t, dsn); q.Get("cluster") != "" {
					t.Errorf("cluster query param not stripped: %v", q)
				}
			},
		},
		{
			name:       "cluster ignored on direct 2881",
			dsn:        "mysql://sys@oracle_tenant:pw@127.0.0.1:2881/oracle_tenant?cluster=obcluster",
			wantDriver: "oboracle",
			check: func(t *testing.T, dsn string) {
				if got := userOf(t, dsn); got != "sys@oracle_tenant" {
					t.Errorf("user = %q, want sys@oracle_tenant", got)
				}
			},
		},
		{
			name:       "charset param survives rewrite",
			dsn:        "mysql://sys:pw@127.0.0.1:2883/db?charset=utf8mb4",
			wantDriver: "oboracle",
			check: func(t *testing.T, dsn string) {
				if got := queryOf(t, dsn).Get("charset"); got != "utf8mb4" {
					t.Errorf("charset = %q, want utf8mb4", got)
				}
			},
		},
		{name: "scheme without host rejected", dsn: "oboracle://", wantDriver: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotDriver, gotDSN, err := ResolveOBOracle(tc.dsn)
			if tc.wantDriver == "" {
				if err == nil {
					t.Fatalf("expected error for %q, got driver %q dsn %q", tc.dsn, gotDriver, gotDSN)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveOBOracle(%q) unexpected error: %v", tc.dsn, err)
			}
			if gotDriver != tc.wantDriver {
				t.Errorf("driver = %q, want %q", gotDriver, tc.wantDriver)
			}
			if tc.check != nil {
				tc.check(t, gotDSN)
			}
		})
	}
}
