package run

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"reflect"
)

// Raw runs q on db's driver connection, with no method of database/sql that takes SQL.
func Raw(ctx context.Context, db *sql.DB, q string) (driver.Rows, error) {
	f := reflect.ValueOf(db).Elem().FieldByName("connector")
	c := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem().Interface().(driver.Connector)
	conn, err := c.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return conn.(driver.QueryerContext).QueryContext(ctx, q, nil)
}

// Pointers reaches the pointers that reflect hands out.
func Pointers(v reflect.Value) (uintptr, uintptr) {
	return v.Pointer(), v.UnsafeAddr()
}

// OpenConn opens a connection through the driver interface.
func OpenConn(d driver.Driver, name string) (driver.Conn, error) { return d.Open(name) }

// Prepare prepares a statement through the driver interface.
func Prepare(conn driver.Conn, q string) (driver.Stmt, error) { return conn.Prepare(q) }

// RunStmt runs a prepared statement through the driver interface.
func RunStmt(ctx context.Context, s driver.StmtQueryContext) (driver.Rows, error) {
	return s.QueryContext(ctx, nil)
}

// Value implements driver.Valuer, which stays allowed.
func Value(n int) (driver.Value, error) { return int64(n), nil }
