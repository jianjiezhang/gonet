package mysql

import (
	"database/sql"
	"errors"

	mysqldriver "github.com/go-sql-driver/mysql"
)

var (
	errEmptyDSN = errors.New("gonet/mysql: DSN 为空")
	errClosed   = errors.New("gonet/mysql: 连接已关闭")
	ErrNoRows   = sql.ErrNoRows
)

func IsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func IsDuplicate(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// IsDupColumn 是 ALTER TABLE 时列已经存在（1060）。
func IsDupColumn(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1060
}
