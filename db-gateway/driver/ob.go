package driver

import (
	"fmt"
	"net/url"
	"strings"
)

// OracleBinds reports whether a logical driver connection uses Oracle-style
// ":N" positional binds (go-ora). go-ora rejects '?'; the OceanBase MySQL
// wire protocol (obconnector-go) accepts '?' natively even though SQL syntax
// stays Oracle-style. "oracle" always binds :N. "ob-oracle" binds :N only
// when the DSN selects the go-ora driver — a scheme-less go-ora simple
// connection string (e.g. user@tenant/password@host:port/service), a bare
// non-URL DSN, or an explicit oracle:// URL. Any other URL scheme selects
// obconnector-go and must keep '?'.
func OracleBinds(driverName, dsn string) bool {
	if driverName == "oracle" {
		return true
	}
	if driverName != "ob-oracle" {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return true
	}
	return u.Scheme == "" || u.Opaque != "" || u.Scheme == "oracle"
}

// ResolveOBOracle picks the underlying database/sql driver and DSN for a
// logical "ob-oracle" connection, mirroring the proven go-owl-migrate
// dispatch:
//   - go-ora ("oracle", Oracle TNS wire, ":N" binds) for scheme-less go-ora
//     connection strings and oracle:// URLs (OBProxy Oracle-protocol
//     endpoint) — unchanged legacy behaviour;
//   - obconnector-go ("oboracle", OceanBase MySQL wire, "?" binds, Oracle SQL
//     mode via preset=oboracle) for any other URL scheme — direct OBServer
//     :2881 or OBProxy :2883.
func ResolveOBOracle(dsn string) (string, string, error) {
	if OracleBinds("ob-oracle", dsn) {
		return "oracle", dsn, nil
	}
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("ob-oracle MySQL-wire DSN must be a URL like mysql://user@tenant:pass@host:2883/db: %q", dsn)
	}
	// Cluster semantics differ by endpoint:
	//   - 2881 (or unset) = direct OBServer: no cluster is needed; a cluster
	//     name folded into the username may even be rejected by the server.
	//   - 2883 (OBProxy) = multi-cluster routing: the cluster must travel in
	//     the username as `user@tenant#cluster` (obconnector-go decodes the
	//     percent-encoded URL userinfo). A bare `cluster` query parameter is
	//     silently ignored by the driver, so fold it in where it is read.
	if cluster := strings.TrimSpace(u.Query().Get("cluster")); cluster != "" {
		user, pass := "", ""
		if u.User != nil {
			user = u.User.Username()
			pass, _ = u.User.Password()
		}
		q := u.Query()
		q.Del("cluster")
		if p := u.Port(); p != "" && p != "2881" {
			if !strings.Contains(user, "#") {
				user += "#" + cluster
			}
			u.User = url.UserPassword(user, pass)
		}
		u.RawQuery = q.Encode()
	}
	if u.Scheme != "oboracle" {
		u.Scheme = "oboracle"
	}
	q := u.Query()
	if q.Get("preset") == "" {
		q.Set("preset", "oboracle")
	}
	u.RawQuery = q.Encode()
	return "oboracle", u.String(), nil
}
