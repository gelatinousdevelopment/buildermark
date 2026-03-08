package db

import (
	"context"
	"database/sql/driver"
	"fmt"
	"log"
	"strings"

	"github.com/mattn/go-sqlite3"
)

// LogQueries enables SQL query logging when true. Set via LOG_SQL=1 env var.
var LogQueries bool

type loggingConnector struct {
	dsn    string
	driver *sqlite3.SQLiteDriver
}

func (c *loggingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &loggingConn{conn: conn}, nil
}

func (c *loggingConnector) Driver() driver.Driver {
	return c.driver
}

type loggingConn struct {
	conn driver.Conn
}

func (c *loggingConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &loggingStmt{stmt: stmt, query: query}, nil
}

func (c *loggingConn) Close() error {
	return c.conn.Close()
}

func (c *loggingConn) Begin() (driver.Tx, error) {
	return c.conn.Begin() //nolint:staticcheck
}

func (c *loggingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bt, ok := c.conn.(driver.ConnBeginTx); ok {
		return bt.BeginTx(ctx, opts)
	}
	return c.conn.Begin() //nolint:staticcheck
}

func (c *loggingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if LogQueries {
		logQuery(query, namedValuesToValues(args))
	}
	if ec, ok := c.conn.(driver.ExecerContext); ok {
		return ec.ExecContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func (c *loggingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if LogQueries {
		logQuery(query, namedValuesToValues(args))
	}
	if qc, ok := c.conn.(driver.QueryerContext); ok {
		return qc.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

type loggingStmt struct {
	stmt  driver.Stmt
	query string
}

func (s *loggingStmt) Close() error {
	return s.stmt.Close()
}

func (s *loggingStmt) NumInput() int {
	return s.stmt.NumInput()
}

func (s *loggingStmt) Exec(args []driver.Value) (driver.Result, error) {
	if LogQueries {
		logQuery(s.query, args)
	}
	return s.stmt.Exec(args) //nolint:staticcheck
}

func (s *loggingStmt) Query(args []driver.Value) (driver.Rows, error) {
	if LogQueries {
		logQuery(s.query, args)
	}
	return s.stmt.Query(args) //nolint:staticcheck
}

func namedValuesToValues(named []driver.NamedValue) []driver.Value {
	vals := make([]driver.Value, len(named))
	for i, nv := range named {
		vals[i] = nv.Value
	}
	return vals
}

func logQuery(query string, args []driver.Value) {
	log.Printf("[SQL] %s", interpolateQuery(query, args))
}

func interpolateQuery(query string, args []driver.Value) string {
	if len(args) == 0 {
		return query
	}
	var b strings.Builder
	argIdx := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' && argIdx < len(args) {
			b.WriteString(formatValue(args[argIdx]))
			argIdx++
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

func formatValue(v driver.Value) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case string:
		return "'" + strings.ReplaceAll(val, "'", "''") + "'"
	case []byte:
		return fmt.Sprintf("X'%x'", val)
	default:
		return fmt.Sprintf("'%v'", val)
	}
}
