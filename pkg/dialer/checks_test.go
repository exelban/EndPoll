package dialer

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestDialer_tcpCall(t *testing.T) {
	d := New(1)
	ctx := context.Background()
	timeout := 2 * time.Second

	t.Run("open port", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer ln.Close()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()

		h := &types.Host{URL: "tcp://" + ln.Addr().String(), TimeoutInterval: &timeout}
		h.Type = h.GetType()
		require.Equal(t, types.TCPType, h.Type)

		resp := d.Dial(ctx, h)
		require.True(t, resp.OK)
		require.Equal(t, http.StatusOK, resp.Code)
		require.False(t, resp.Timestamp.IsZero())
	})
	t.Run("closed port", func(t *testing.T) {
		h := &types.Host{URL: "tcp://127.0.0.1:1", Type: types.TCPType, TimeoutInterval: &timeout}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.Equal(t, 523, resp.Code)
		require.NotEmpty(t, resp.Body)
	})
	t.Run("invalid address", func(t *testing.T) {
		h := &types.Host{URL: "tcp://localhost", Type: types.TCPType}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.Equal(t, http.StatusBadRequest, resp.Code)
	})
}

func TestDialer_dnsCall(t *testing.T) {
	d := New(1)
	ctx := context.Background()
	timeout := 3 * time.Second

	t.Run("resolves", func(t *testing.T) {
		h := &types.Host{URL: "dns://localhost", TimeoutInterval: &timeout}
		h.Type = h.GetType()
		require.Equal(t, types.DNSType, h.Type)

		resp := d.Dial(ctx, h)
		require.True(t, resp.OK, resp.Body)
		require.Equal(t, http.StatusOK, resp.Code)
		require.Contains(t, resp.Body, "127.0.0.1")
	})
	t.Run("expected record present", func(t *testing.T) {
		h := &types.Host{URL: "dns://localhost", Type: types.DNSType, TimeoutInterval: &timeout,
			DNS: &types.DNSCheck{Type: "a", Expect: []string{"127.0.0.1"}}}
		resp := d.Dial(ctx, h)
		require.True(t, resp.OK, resp.Body)
	})
	t.Run("expected record missing", func(t *testing.T) {
		h := &types.Host{URL: "dns://localhost", Type: types.DNSType, TimeoutInterval: &timeout,
			DNS: &types.DNSCheck{Expect: []string{"10.0.0.1"}}}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.Equal(t, http.StatusExpectationFailed, resp.Code)
		require.Contains(t, resp.Body, "10.0.0.1")
	})
	t.Run("unsupported record type", func(t *testing.T) {
		h := &types.Host{URL: "dns://localhost", Type: types.DNSType, TimeoutInterval: &timeout,
			DNS: &types.DNSCheck{Type: "SRV"}}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.Contains(t, resp.Body, "unsupported record type")
	})
	t.Run("invalid address", func(t *testing.T) {
		h := &types.Host{URL: "dns://", Type: types.DNSType}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.Equal(t, http.StatusBadRequest, resp.Code)
	})
	t.Run("unreachable resolver", func(t *testing.T) {
		short := 500 * time.Millisecond
		h := &types.Host{URL: "dns://example.com", Type: types.DNSType, TimeoutInterval: &short,
			DNS: &types.DNSCheck{Resolver: "127.0.0.1:1"}}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.False(t, resp.Timestamp.IsZero())
	})
}

// fakeRedis - a minimal RESP server: AUTH with the given password, SELECT, PING
func fakeRedis(t *testing.T, password string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				authed := password == ""
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if !strings.HasPrefix(line, "*") {
						continue
					}
					n := int(line[1] - '0')
					args := make([]string, 0, n)
					for i := 0; i < n; i++ {
						_, _ = r.ReadString('\n') // $len
						a, _ := r.ReadString('\n')
						args = append(args, strings.TrimRight(a, "\r\n"))
					}
					switch strings.ToUpper(args[0]) {
					case "AUTH":
						if args[len(args)-1] == password {
							authed = true
							_, _ = c.Write([]byte("+OK\r\n"))
						} else {
							_, _ = c.Write([]byte("-WRONGPASS invalid username-password pair\r\n"))
						}
					case "SELECT":
						_, _ = c.Write([]byte("+OK\r\n"))
					case "PING":
						if !authed {
							_, _ = c.Write([]byte("-NOAUTH Authentication required.\r\n"))
						} else {
							_, _ = c.Write([]byte("+PONG\r\n"))
						}
					default:
						_, _ = c.Write([]byte("-ERR unknown command\r\n"))
					}
				}
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func TestDialer_redisCall(t *testing.T) {
	d := New(1)
	ctx := context.Background()
	timeout := 2 * time.Second

	t.Run("ping", func(t *testing.T) {
		addr := fakeRedis(t, "")
		h := &types.Host{URL: "redis://" + addr + "/2", TimeoutInterval: &timeout}
		h.Type = h.GetType()
		require.Equal(t, types.RedisType, h.Type)

		resp := d.Dial(ctx, h)
		require.True(t, resp.OK, resp.Body)
		require.Equal(t, http.StatusOK, resp.Code)
	})
	t.Run("auth ok", func(t *testing.T) {
		addr := fakeRedis(t, "secret")
		h := &types.Host{URL: "redis://:secret@" + addr, Type: types.RedisType, TimeoutInterval: &timeout}
		resp := d.Dial(ctx, h)
		require.True(t, resp.OK, resp.Body)
	})
	t.Run("wrong password", func(t *testing.T) {
		addr := fakeRedis(t, "secret")
		h := &types.Host{URL: "redis://default:wrong@" + addr, Type: types.RedisType, TimeoutInterval: &timeout}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.Equal(t, http.StatusUnauthorized, resp.Code)
		require.Contains(t, resp.Body, "WRONGPASS")
	})
	t.Run("auth required", func(t *testing.T) {
		addr := fakeRedis(t, "secret")
		h := &types.Host{URL: "redis://" + addr, Type: types.RedisType, TimeoutInterval: &timeout}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.Contains(t, resp.Body, "NOAUTH")
	})
	t.Run("closed port", func(t *testing.T) {
		h := &types.Host{URL: "redis://127.0.0.1:1", Type: types.RedisType, TimeoutInterval: &timeout}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.Equal(t, 523, resp.Code)
	})
	t.Run("password is masked", func(t *testing.T) {
		h := &types.Host{URL: "redis://:secret@localhost:6379"}
		require.NotContains(t, h.SecureURL(), "secret")
	})
}

func TestDialer_sql(t *testing.T) {
	d := New(1)
	ctx := context.Background()
	timeout := 2 * time.Second

	t.Run("dsn", func(t *testing.T) {
		h := &types.Host{URL: "mysql://user:pass@db.local/app?parseTime=true", TimeoutInterval: &timeout}
		h.Type = h.GetType()
		require.Equal(t, types.MySQLType, h.Type)
		driver, dsn, err := sqlDSN(h)
		require.NoError(t, err)
		require.Equal(t, "mysql", driver)
		require.Equal(t, "user:pass@tcp(db.local:3306)/app?parseTime=true&timeout=2s", dsn)

		h = &types.Host{URL: "postgresql://user:pass@db.local:5433/app?sslmode=disable", TimeoutInterval: &timeout}
		h.Type = h.GetType()
		require.Equal(t, types.PostgresType, h.Type)
		driver, dsn, err = sqlDSN(h)
		require.NoError(t, err)
		require.Equal(t, "pgx", driver)
		require.Contains(t, dsn, "postgres://user:pass@db.local:5433/app?")
		require.Contains(t, dsn, "connect_timeout=3")
		require.Contains(t, dsn, "sslmode=disable")

		h = &types.Host{URL: "postgres://", Type: types.PostgresType}
		_, _, err = sqlDSN(h)
		require.Error(t, err)
	})
	t.Run("postgres unreachable", func(t *testing.T) {
		h := &types.Host{URL: "postgres://user:pass@127.0.0.1:1/app?sslmode=disable", Type: types.PostgresType, TimeoutInterval: &timeout}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.NotEmpty(t, resp.Body)
		require.NotContains(t, resp.Body, "pass@", "the error must not leak the password")
		require.False(t, resp.Timestamp.IsZero())
	})
	t.Run("mysql unreachable", func(t *testing.T) {
		h := &types.Host{URL: "mysql://user:pass@127.0.0.1:1/app", Type: types.MySQLType, TimeoutInterval: &timeout}
		resp := d.Dial(ctx, h)
		require.False(t, resp.OK)
		require.NotEmpty(t, resp.Body)
	})
}

func TestHost_GetType_Schemes(t *testing.T) {
	cases := map[string]types.HostType{
		"tcp://host:1":              types.TCPType,
		"dns://example.com":         types.DNSType,
		"redis://localhost":         types.RedisType,
		"rediss://localhost":        types.RedisType,
		"postgres://localhost/db":   types.PostgresType,
		"postgresql://localhost/db": types.PostgresType,
		"mysql://localhost/db":      types.MySQLType,
		"mongodb+srv://cluster/db":  types.MongoType,
		"HTTPS://example.com":       types.HttpType,
		"192.168.1.1":               types.ICMPType,
		"example.com":               types.HttpType,
	}
	for u, want := range cases {
		h := &types.Host{URL: u}
		require.Equal(t, want, h.GetType(), u)
	}
}
