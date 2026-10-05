package dialer

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
	_ "github.com/go-sql-driver/mysql" // mysql driver
	_ "github.com/jackc/pgx/v5/stdlib" // postgres driver
)

// sqlCall - connects to a SQL database and runs a trivial query
// (url: postgres://user:pass@host:5432/db or mysql://user:pass@host:3306/db)
func (d *Dialer) sqlCall(ctx context.Context, h *types.Host) (response types.HttpResponse) {
	response.Timestamp = time.Now()

	driver, dsn, err := sqlDSN(h)
	if err != nil {
		response.Code = http.StatusBadRequest
		response.Body = err.Error()
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(h))
	defer cancel()

	start := time.Now()
	defer func() {
		response.Time = time.Since(start)
	}()

	db, err := sql.Open(driver, dsn)
	if err != nil {
		response.Code = http.StatusBadRequest
		response.Body = truncate(err.Error(), 512)
		return
	}
	defer func() {
		_ = db.Close()
	}()
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		response.Code = errorCode(err)
		if strings.Contains(strings.ToLower(err.Error()), "password") || strings.Contains(strings.ToLower(err.Error()), "denied") {
			response.Code = http.StatusUnauthorized
		}
		response.Body = truncate(err.Error(), 512)
		return
	}
	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		response.Code = http.StatusBadGateway
		response.Body = truncate(err.Error(), 512)
		return
	}

	response.OK = true
	response.Code = http.StatusOK
	return
}

// sqlDSN - returns the database/sql driver name and the DSN for the host url
func sqlDSN(h *types.Host) (driver, dsn string, err error) {
	u, err := url.Parse(h.URL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("invalid address, expected scheme://user:pass@host:port/db")
	}
	timeout := timeout(h)

	switch h.Type {
	case types.PostgresType:
		q := u.Query()
		if q.Get("connect_timeout") == "" {
			q.Set("connect_timeout", fmt.Sprintf("%d", int(timeout.Seconds())+1))
		}
		u.RawQuery = q.Encode()
		u.Scheme = "postgres"
		return "pgx", u.String(), nil
	case types.MySQLType:
		// mysql dsn: user:pass@tcp(host:port)/db?params
		addr := u.Host
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, "3306")
		}
		q := u.Query()
		if q.Get("timeout") == "" {
			q.Set("timeout", timeout.String())
		}
		auth := ""
		if u.User != nil {
			auth = u.User.String() + "@"
		}
		dsn := fmt.Sprintf("%stcp(%s)/%s", auth, addr, strings.TrimPrefix(u.Path, "/"))
		if len(q) > 0 {
			dsn += "?" + q.Encode()
		}
		return "mysql", dsn, nil
	}
	return "", "", fmt.Errorf("unsupported sql type %q", h.Type)
}
