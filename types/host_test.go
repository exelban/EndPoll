package types

import (
	"crypto/md5"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHost_Status(t *testing.T) {
	t.Run("code", func(t *testing.T) {
		h := Host{
			Conditions: &Success{
				Code: []int{1, 2, 3},
			},
		}

		require.False(t, h.Status(0, nil))
		require.True(t, h.Status(1, nil))
		require.True(t, h.Status(2, nil))
		require.True(t, h.Status(3, nil))
		require.False(t, h.Status(4, nil))
	})

	t.Run("body", func(t *testing.T) {
		str := "ok"
		h := Host{
			Conditions: &Success{
				Code: []int{200},
				Body: &str,
			},
		}

		require.False(t, h.Status(200, nil))
		require.False(t, h.Status(200, []byte("not ok")))
		require.True(t, h.Status(200, []byte(str)))
	})
}

func TestHost_String(t *testing.T) {
	n := "name"
	name := Host{
		Name: &n,
		URL:  "url",
	}
	url := Host{
		URL: "url",
	}

	require.Equal(t, "name (url)", name.String())
	require.Equal(t, "url", url.String())
}

func TestHost_GenerateID(t *testing.T) {
	url := "url"
	group := "group"

	t.Run("url only", func(t *testing.T) {
		h := Host{
			URL: url,
		}
		hasher := md5.New()
		hasher.Write([]byte(url))
		hash := hasher.Sum(nil)
		expected := base64.URLEncoding.EncodeToString(hash)[:6]
		require.Equal(t, expected, h.GenerateID())
	})

	t.Run("url and group", func(t *testing.T) {
		h := Host{
			URL:   url,
			Group: &group,
		}
		hasher := md5.New()
		input := append([]byte(url), []byte(group)...)
		hasher.Write(input)
		hash := hasher.Sum(nil)
		expected := base64.URLEncoding.EncodeToString(hash)[:6]
		require.Equal(t, expected, h.GenerateID())
	})
}

func TestHost_GetType(t *testing.T) {
	t.Run("explicit type takes priority", func(t *testing.T) {
		h := Host{URL: "http://example.com", Type: ICMPType}
		require.Equal(t, ICMPType, h.GetType())
	})
	t.Run("http url", func(t *testing.T) {
		h := Host{URL: "http://example.com"}
		require.Equal(t, HttpType, h.GetType())
	})
	t.Run("https url", func(t *testing.T) {
		h := Host{URL: "https://example.com"}
		require.Equal(t, HttpType, h.GetType())
	})
	t.Run("mongodb url", func(t *testing.T) {
		h := Host{URL: "mongodb://localhost:27017"}
		require.Equal(t, MongoType, h.GetType())
	})
	t.Run("ipv4 address", func(t *testing.T) {
		h := Host{URL: "192.168.1.1"}
		require.Equal(t, ICMPType, h.GetType())
	})
	t.Run("hostname defaults to http", func(t *testing.T) {
		h := Host{URL: "example.com"}
		require.Equal(t, HttpType, h.GetType())
	})
	t.Run("ipv4 with leading zeros", func(t *testing.T) {
		h := Host{URL: "0.0.0.0"}
		require.Equal(t, ICMPType, h.GetType())
	})
	t.Run("ipv4 boundary 255.255.255.255", func(t *testing.T) {
		h := Host{URL: "255.255.255.255"}
		require.Equal(t, ICMPType, h.GetType())
	})
}

func TestIsIPv4(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		require.True(t, isIPv4("192.168.1.1"))
		require.True(t, isIPv4("0.0.0.0"))
		require.True(t, isIPv4("255.255.255.255"))
		require.True(t, isIPv4("10.0.0.1"))
	})
	t.Run("invalid", func(t *testing.T) {
		require.False(t, isIPv4("256.0.0.1"))
		require.False(t, isIPv4("1.2.3"))
		require.False(t, isIPv4("1.2.3.4.5"))
		require.False(t, isIPv4("abc.def.ghi.jkl"))
		require.False(t, isIPv4(""))
		require.False(t, isIPv4("-1.0.0.1"))
	})
}

func TestHost_SecureURL(t *testing.T) {
	t.Run("http url unchanged", func(t *testing.T) {
		h := Host{URL: "https://example.com/api"}
		require.Equal(t, "https://example.com/api", h.SecureURL())
	})
	t.Run("mongo url without credentials", func(t *testing.T) {
		h := Host{URL: "mongodb://localhost:27017"}
		require.Equal(t, "mongodb://localhost:27017", h.SecureURL())
	})
	t.Run("mongo url with credentials", func(t *testing.T) {
		h := Host{URL: "mongodb://user:password@localhost:27017"}
		url := h.SecureURL()
		require.Contains(t, url, "*****")
		require.NotContains(t, url, "password")
	})
	t.Run("mongo url without @ sign", func(t *testing.T) {
		h := Host{URL: "mongodb://localhost:27017/db"}
		require.Equal(t, "mongodb://localhost:27017/db", h.SecureURL())
	})
}

func TestHost_Status_NilConditions(t *testing.T) {
	h := Host{}
	require.True(t, h.Status(200, nil))
	require.False(t, h.Status(500, nil))
}

func TestHost_GenerateID_Uniqueness(t *testing.T) {
	t.Run("different urls produce different ids", func(t *testing.T) {
		h1 := Host{URL: "http://a.com"}
		h2 := Host{URL: "http://b.com"}
		require.NotEqual(t, h1.GenerateID(), h2.GenerateID())
	})
	t.Run("same url different group produce different ids", func(t *testing.T) {
		g1 := "group1"
		g2 := "group2"
		h1 := Host{URL: "http://a.com", Group: &g1}
		h2 := Host{URL: "http://a.com", Group: &g2}
		require.NotEqual(t, h1.GenerateID(), h2.GenerateID())
	})
	t.Run("same url with and without group produce different ids", func(t *testing.T) {
		g := "group"
		h1 := Host{URL: "http://a.com"}
		h2 := Host{URL: "http://a.com", Group: &g}
		require.NotEqual(t, h1.GenerateID(), h2.GenerateID())
	})
	t.Run("deterministic", func(t *testing.T) {
		h := Host{URL: "http://a.com"}
		id1 := h.GenerateID()
		id2 := h.GenerateID()
		require.Equal(t, id1, id2)
	})
}
