package authenticate

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pomerium/pomerium/internal/middleware"
	"github.com/pomerium/pomerium/internal/urlutil"
	"github.com/pomerium/pomerium/pkg/cryptutil"
)

// dvoid fork: the account chooser is honoured only when the sign-in URL that
// carries it verifies, i.e. authorize put it there.
func TestRequestedPrompt(t *testing.T) {
	t.Parallel()
	key := cryptutil.NewKey()
	verify := func(r *http.Request) error { return middleware.ValidateRequestURL(r, key) }
	signIn := func(prompt string) *url.URL {
		u, _ := url.Parse("https://authenticate.example.com/.pomerium/sign_in")
		q := url.Values{urlutil.QueryRedirectURI: {"https://clip.example.com/"}}
		if prompt != "" {
			q.Set(urlutil.QueryPrompt, prompt)
		}
		u.RawQuery = q.Encode()
		return u
	}
	req := func(u string) *http.Request { return httptest.NewRequest(http.MethodGet, u, nil) }
	signed := func(u *url.URL) string { return urlutil.NewSignedURL(key, u).String() }

	assert.Equal(t, "select_account", requestedPrompt(req(signed(signIn("select_account"))), verify),
		"signed by authorize: honoured")
	assert.Equal(t, "", requestedPrompt(req(signIn("select_account").String()), verify),
		"unsigned: ignored")
	tampered := signed(signIn("")) + "&" + urlutil.QueryPrompt + "=select_account"
	assert.Equal(t, "", requestedPrompt(req(tampered), verify),
		"added by the client to a signed URL: the signature fails, ignored")
	assert.Equal(t, "", requestedPrompt(req(signed(signIn("login"))), verify),
		"any other value, even signed: ignored")
	assert.Equal(t, "", requestedPrompt(req(signed(signIn(""))), verify),
		"no prompt: none")
}
