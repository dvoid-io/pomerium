package config

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pomerium/pomerium/pkg/cryptutil"
	"github.com/pomerium/pomerium/pkg/grpc/session"
)

func TestCookieDomainFor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		configured string
		host       string
		want       string
	}{
		{"apex", "dvoid.io", "dvoid.io", "dvoid.io"},
		{"subdomain", "dvoid.io", "app.dvoid.io", "dvoid.io"},
		{"nested subdomain", "dvoid.io", "a.b.dvoid.io", "dvoid.io"},
		{"host with port", "dvoid.io", "app.dvoid.io:8443", "dvoid.io"},
		{"uppercase host", "dvoid.io", "APP.DVOID.IO", "dvoid.io"},
		{"uppercase configured is returned verbatim", "DVOID.IO", "app.dvoid.io", "DVOID.IO"},
		{"trailing dot on host", "dvoid.io", "app.dvoid.io.", "dvoid.io"},
		{"trailing dot on host with port", "dvoid.io", "app.dvoid.io.:443", "dvoid.io"},
		{"trailing dot on configured is returned verbatim", "dvoid.io.", "app.dvoid.io", "dvoid.io."},
		{"leading dot on configured is returned verbatim", ".dvoid.io", "app.dvoid.io", ".dvoid.io"},
		{"leading dot on configured matches apex", ".dvoid.io", "dvoid.io", ".dvoid.io"},

		{"unrelated domain", "dvoid.io", "admin.avesa.online", ""},
		{"unrelated domain with port", "dvoid.io", "admin.avesa.online:443", ""},
		{"sibling suffix attack", "dvoid.io", "evil-dvoid.io", ""},
		{"suffix without label boundary", "dvoid.io", "evildvoid.io", ""},
		{"configured as a prefix", "dvoid.io", "dvoid.io.evil.com", ""},
		{"parent of configured", "dvoid.io", "io", ""},
		{"ipv4 never suffix-matches", "0.1", "10.0.0.1", ""},
		{"ipv4 exact match", "10.0.0.1", "10.0.0.1:443", "10.0.0.1"},
		{"ipv6 host", "dvoid.io", "[::1]:443", ""},
		{"empty host", "dvoid.io", "", ""},
		{"empty configured", "", "app.dvoid.io", ""},
		{"only a dot configured", ".", "app.dvoid.io", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, cookieDomainFor(tc.configured, tc.host))
		})
	}
}

func TestOptions_GetCookieDomain(t *testing.T) {
	t.Parallel()

	req := func(host string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "https://"+host+"/", nil)
		r.Host = host
		return r
	}

	t.Run("unset", func(t *testing.T) {
		t.Parallel()
		o := NewDefaultOptions()
		assert.Empty(t, o.GetCookieDomain(req("app.dvoid.io")))
		assert.Empty(t, o.NewCookie(req("app.dvoid.io")).Domain)
	})
	t.Run("nil request", func(t *testing.T) {
		t.Parallel()
		o := NewDefaultOptions()
		o.CookieDomain = "dvoid.io"
		assert.Empty(t, o.GetCookieDomain(nil))
	})
	t.Run("request host", func(t *testing.T) {
		t.Parallel()
		o := NewDefaultOptions()
		o.CookieDomain = "dvoid.io"
		assert.Equal(t, "dvoid.io", o.GetCookieDomain(req("app.dvoid.io")))
		assert.Equal(t, "dvoid.io", o.NewCookie(req("app.dvoid.io")).Domain)
		assert.Empty(t, o.GetCookieDomain(req("admin.avesa.online")))
		assert.Empty(t, o.NewCookie(req("admin.avesa.online")).Domain)
	})
	t.Run("internal authenticate host is judged by the external one", func(t *testing.T) {
		t.Parallel()
		o := NewDefaultOptions()
		o.CookieDomain = "dvoid.io"
		o.AuthenticateURLString = "https://authenticate.dvoid.io"
		o.AuthenticateInternalURLString = "https://pomerium-authenticate.internal:8443"
		assert.Equal(t, "dvoid.io", o.GetCookieDomain(req("pomerium-authenticate.internal:8443")))
		assert.Equal(t, "dvoid.io", o.GetCookieDomain(req("authenticate.dvoid.io")))
		assert.Empty(t, o.GetCookieDomain(req("admin.avesa.online")))
	})
	t.Run("internal authenticate host without the mapping", func(t *testing.T) {
		t.Parallel()
		o := NewDefaultOptions()
		o.CookieDomain = "dvoid.io"
		o.AuthenticateURLString = "https://authenticate.dvoid.io"
		assert.Empty(t, o.GetCookieDomain(req("pomerium-authenticate.internal:8443")))
	})
}

// TestSessionStore_CookieDomain proves that the session cookie the proxy sets
// and clears carries cookie_domain only for hosts inside it, and that setting
// and clearing always agree.
func TestSessionStore_CookieDomain(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		configured string
		host       string
		wantDomain string
	}{
		{"dvoid.io", "app.dvoid.io", "dvoid.io"},
		{"dvoid.io", "dvoid.io", "dvoid.io"},
		{"dvoid.io", "admin.avesa.online", ""},
		{"dvoid.io", "evil-dvoid.io", ""},
		{"", "app.dvoid.io", ""},
		{"", "admin.avesa.online", ""},
	} {
		t.Run(tc.configured+"/"+tc.host, func(t *testing.T) {
			t.Parallel()

			options := NewDefaultOptions()
			options.SharedKey = base64.StdEncoding.EncodeToString(cryptutil.NewKey())
			options.CookieDomain = tc.configured
			store, err := NewSessionStore(options)
			require.NoError(t, err)

			r := httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/.pomerium/callback/", nil)

			set := httptest.NewRecorder()
			require.NoError(t, store.WriteSessionHandle(set, r, &session.Handle{Id: "xyz"}))
			assertSessionCookieDomain(t, set, options.CookieName, tc.wantDomain)

			cleared := httptest.NewRecorder()
			store.ClearSessionHandle(cleared, r)
			c := assertSessionCookieDomain(t, cleared, options.CookieName, tc.wantDomain)
			assert.Negative(t, c.MaxAge, "clearing must expire the cookie")
		})
	}
}

func assertSessionCookieDomain(t *testing.T, w *httptest.ResponseRecorder, name, wantDomain string) *http.Cookie {
	t.Helper()

	raw := w.Header().Values("Set-Cookie")
	require.Len(t, raw, 1)
	if wantDomain == "" {
		assert.NotContains(t, raw[0], "Domain=", "expected a host-only cookie")
	} else {
		assert.Contains(t, raw[0], "; Domain="+wantDomain+";")
	}

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, name, cookies[0].Name)
	assert.Equal(t, wantDomain, cookies[0].Domain)
	return cookies[0]
}
