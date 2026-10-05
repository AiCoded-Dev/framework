package run

import (
	"context"
	"database/sql"
	"reflect"
)

// caller calls a function, as reflect.Value does.
type caller interface {
	Call(in []reflect.Value) []reflect.Value
}

// Query runs the SQL q on db through reflection.
func Query(ctx context.Context, db *sql.DB, q string) []reflect.Value {
	v := reflect.ValueOf(db)
	_ = v.Method(0)
	return v.MethodByName("QueryContext").Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(q)})
}

// Spread calls f with the elements of args.
func Spread(f, args reflect.Value) []reflect.Value { return f.CallSlice([]reflect.Value{args}) }

// Through calls f through an interface.
func Through(f caller) []reflect.Value { return f.Call(nil) }

// Func returns the method of db named name as a function.
func Func(db *sql.DB, name string) any {
	m, _ := reflect.TypeOf(db).MethodByName(name)
	_ = reflect.TypeOf(db).Method(0)
	return m.Func.Interface()
}

// Kind reads the kind of x, which reflection may do.
func Kind(x any) reflect.Kind { return reflect.TypeOf(x).Kind() }

// ScanType reads a column's scan type, which hands out a reflect.Type.
func ScanType(ct *sql.ColumnType) reflect.Type { return ct.ScanType() }
