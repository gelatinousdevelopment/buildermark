package db

import (
	"context"
	"database/sql/driver"
	"fmt"
	"log"
	"strings"
	"time"

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
	start := time.Now()
	if ec, ok := c.conn.(driver.ExecerContext); ok {
		res, err := ec.ExecContext(ctx, query, args)
		if LogQueries {
			logQuery(time.Since(start), query, namedValuesToValues(args))
		}
		return res, err
	}
	return nil, driver.ErrSkip
}

func (c *loggingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	start := time.Now()
	if qc, ok := c.conn.(driver.QueryerContext); ok {
		rows, err := qc.QueryContext(ctx, query, args)
		if LogQueries {
			logQuery(time.Since(start), query, namedValuesToValues(args))
		}
		return rows, err
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
	start := time.Now()
	res, err := s.stmt.Exec(args) //nolint:staticcheck
	if LogQueries {
		logQuery(time.Since(start), s.query, args)
	}
	return res, err
}

func (s *loggingStmt) Query(args []driver.Value) (driver.Rows, error) {
	start := time.Now()
	rows, err := s.stmt.Query(args) //nolint:staticcheck
	if LogQueries {
		logQuery(time.Since(start), s.query, args)
	}
	return rows, err
}

func namedValuesToValues(named []driver.NamedValue) []driver.Value {
	vals := make([]driver.Value, len(named))
	for i, nv := range named {
		vals[i] = nv.Value
	}
	return vals
}

func logQuery(d time.Duration, query string, args []driver.Value) {
	ms := d.Milliseconds()
	if ms >= 1 {
		log.Printf("[SQL] %dms %s", ms, minimizeQuery(interpolateQuery(query, args)))
	}
}

// minimizeQuery collapses all whitespace runs into a single space.
func minimizeQuery(q string) string {
	var b strings.Builder
	inSpace := false
	for _, c := range q {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(c)
			inSpace = false
		}
	}
	return strings.TrimSpace(b.String())
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
