package config

import (
	"net"
	"net/http"
	"strings"

	"github.com/pomerium/pomerium/internal/urlutil"
)

// GetCookieDomain returns the Domain attribute for a cookie set or cleared in
// response to r.
//
// The configured cookie_domain is applied only when the request host is that
// domain or a subdomain of it. For any other host the cookie is host-only (no
// Domain attribute): a browser must reject a Set-Cookie whose Domain does not
// domain-match the request host (RFC 6265 section 5.3, step 6), so stamping
// cookie_domain onto a response for a route on another domain would make it
// impossible to establish a session there.
//
// A request that arrives on the internal authenticate URL is judged by the
// external authenticate URL, the host the browser actually sent it to.
func (o *Options) GetCookieDomain(r *http.Request) string {
	if o.CookieDomain == "" {
		return ""
	}
	var host string
	if r != nil {
		host = r.Host
	}
	return cookieDomainFor(o.CookieDomain, o.externalAuthenticateHost(host))
}

// externalAuthenticateHost maps the internal authenticate host to the external
// one, and returns every other host unchanged.
func (o *Options) externalAuthenticateHost(host string) string {
	if o.AuthenticateInternalURLString == "" {
		return host
	}
	internalURL, err := o.GetInternalAuthenticateURL()
	if err != nil || !strings.EqualFold(urlutil.StripPort(host), internalURL.Hostname()) {
		return host
	}
	externalURL, err := o.GetAuthenticateURL()
	if err != nil {
		return host
	}
	return externalURL.Host
}

// cookieDomainFor returns configured when host (which may carry a port) is
// configured itself or a subdomain of it, and "" otherwise. Matching is
// case-insensitive, respects label boundaries (evil-example.com is not inside
// example.com), and tolerates a trailing dot on either side and a leading dot
// on configured. An IP address only ever matches exactly (RFC 6265 section
// 5.1.3). The returned value is configured verbatim, so a cookie for a host
// inside the domain is exactly what it was before this check existed.
func cookieDomainFor(configured, host string) string {
	domain := normalizeCookieHost(strings.TrimPrefix(configured, "."))
	host = normalizeCookieHost(urlutil.StripPort(host))
	if domain == "" || host == "" {
		return ""
	}
	if host == domain {
		return configured
	}
	if net.ParseIP(host) == nil && strings.HasSuffix(host, "."+domain) {
		return configured
	}
	return ""
}

func normalizeCookieHost(s string) string {
	return strings.ToLower(strings.TrimSuffix(s, "."))
}
