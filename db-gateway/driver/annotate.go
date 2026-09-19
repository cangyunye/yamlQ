package driver

import (
	"fmt"
	"strings"
)

// isOceanBaseOracleTenantMySQLDriverError 识别 OceanBase 服务端在 MySQL wire 上
// 拒绝普通客户端驱动的错误（Error 1235 / SQLSTATE 0A000：Oracle tenant for current
// client driver is not supported）。
func isOceanBaseOracleTenantMySQLDriverError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "oracle tenant") && strings.Contains(text, "not supported")
}

// isOBOracleTNSHandshakeFailure 识别 go-ora 在 TNS 路径上遇到非 Oracle 协议端口时的
// 握手失败特征（关键字集合参照 GoNavi annotateOceanBaseOracleConnectError）。
// 典型场景：把 ob-oracle 的 oracle:// DSN 指向 OBServer/OBProxy 的 MySQL-wire 端口。
func isOBOracleTNSHandshakeFailure(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, keyword := range []string{
		"tns",
		"protocol error",
		"unexpected packet",
		"got packets out of order",
		"use of closed network connection",
	} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

// isMissingPortInAddressError 识别 go-ora 解析旧式简单 DSN 时的报错。
// 旧式 user@tenant/password@host:port/service 没有 URL scheme 与 userinfo，
// go-ora 不接受，返回 "missing port in address"。
func isMissingPortInAddressError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "missing port in address")
}

const obOracleTNSDSNFormatHint = "TNS 路径 DSN 需使用 oracle:// 形式，例如 oracle://user@tenant:password@host:port/service_name；集群模式在租户名后加 %23cluster（如 oracle://user@tenant%23obcluster:password@host:port/service_name）"

// AnnotateConnectError 为 OB 相关驱动的连接阶段错误附加路径诊断提示。
// 无 build tag，默认构建下 ob-mysql/ob-oracle 未注册，driverName 恒不匹配，原样返回。
func AnnotateConnectError(driverName string, dsn string, err error) error {
	if err == nil {
		return nil
	}
	switch driverName {
	case "ob-oracle":
		switch {
		case isOceanBaseOracleTenantMySQLDriverError(err):
			return fmt.Errorf(
				"%w（OceanBase Error 1235：目标端口是 OceanBase MySQL-wire 入口上的 Oracle 租户，当前 DSN 走的是 Oracle TNS 路径，无法建立连接；请改用 oboracle://user:pass@host:port/db 形式的 DSN）",
				err,
			)
		case isMissingPortInAddressError(err):
			return fmt.Errorf("%w（%s）", err, obOracleTNSDSNFormatHint)
		case isOBOracleTNSHandshakeFailure(err):
			return fmt.Errorf(
				"%w（该端口握手失败：可能不是 Oracle TNS listener，而是 OBServer/OBProxy 的 MySQL-wire 端口上的 OceanBase Oracle 租户；请改用 oboracle://user:pass@host:port/db 形式的 DSN）",
				err,
			)
		}
	case "ob-mysql":
		if isOceanBaseOracleTenantMySQLDriverError(err) {
			return fmt.Errorf(
				"%w（OceanBase Error 1235：该目标看起来是 Oracle 租户而非 MySQL 租户；请改用 db_type=ob-oracle，DSN 使用 oboracle:// 前缀）",
				err,
			)
		}
	}
	return err
}
