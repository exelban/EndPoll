package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecoverer(t *testing.T) {
	t.Run("normal request passes through", func(t *testing.T) {
		handler := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		}))

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		require.Equal(t, "ok", rr.Body.String())
	})

	t.Run("panic returns 500", func(t *testing.T) {
		handler := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		}))

		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

func TestCORS(t *testing.T) {
	handler := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("sets CORS headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		require.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "GET", rr.Header().Get("Access-Control-Allow-Methods"))
		require.Equal(t, "Content-Type", rr.Header().Get("Access-Control-Allow-Headers"))
		require.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("OPTIONS returns 200 without calling handler", func(t *testing.T) {
		called := false
		h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))

		req := httptest.NewRequest("OPTIONS", "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.False(t, called)
	})
}

func TestPing(t *testing.T) {
	t.Run("default path /ping", func(t *testing.T) {
		handler := Ping()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		req := httptest.NewRequest("GET", "/ping", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Equal(t, "ok", rr.Body.String())
	})

	t.Run("custom path", func(t *testing.T) {
		handler := Ping("/health")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		req := httptest.NewRequest("GET", "/health", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		require.Equal(t, "ok", rr.Body.String())
	})

	t.Run("non-ping path passes through", func(t *testing.T) {
		handler := Ping()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("real handler"))
		}))

		req := httptest.NewRequest("GET", "/api/data", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Equal(t, "real handler", rr.Body.String())
	})

	t.Run("multiple paths", func(t *testing.T) {
		handler := Ping("/ping", "/health", "/ready")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))

		for _, path := range []string{"/ping", "/health", "/ready"} {
			req := httptest.NewRequest("GET", path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			require.Equal(t, http.StatusOK, rr.Code)
		}
	})
}

func TestInfo(t *testing.T) {
	handler := Info("TestApp", "1.2.3")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, "TestApp", rr.Header().Get("App-Name"))
	require.Equal(t, "1.2.3", rr.Header().Get("App-Version"))
	require.Equal(t, http.StatusOK, rr.Code)
}

func TestRouter_Middleware(t *testing.T) {
	t.Run("middlewares applied in order", func(t *testing.T) {
		order := make([]int, 0)

		m1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, 1)
				next.ServeHTTP(w, r)
			})
		}
		m2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, 2)
				next.ServeHTTP(w, r)
			})
		}

		router := NewRouter(m1, m2)
		router.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
			order = append(order, 3)
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()
		router.mux.ServeHTTP(rr, req)

		require.Equal(t, []int{1, 2, 3}, order)
	})
}
