package authorize

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pomerium/pomerium/authorize/evaluator"
	"github.com/pomerium/pomerium/config"
	"github.com/pomerium/pomerium/internal/urlutil"
	hpke_handlers "github.com/pomerium/pomerium/pkg/hpke/handlers"
)

// dvoid fork: pomerium_prompt=select_account on a navigation travels into the
// signed sign-in URL, never into the URL the user returns to; no other value
// passes.
func TestRequireLogin_prompt(t *testing.T) {
	t.Parallel()

	opt := config.NewDefaultOptions()
	opt.DataBroker.ServiceURL = "https://databroker.example.com"
	opt.SharedKey = "E8wWIMnihUx+AUfRegAQDNs8eRb3UrB5G3zlJW9XJDM="
	hpkePrivateKey, err := opt.GetHPKEPrivateKey()
	require.NoError(t, err)
	authnSrv := httptest.NewServer(hpke_handlers.HPKEPublicKeyHandler(hpkePrivateKey.PublicKey()))
	t.Cleanup(authnSrv.Close)
	opt.AuthenticateURLString = authnSrv.URL
	a, err := New(t.Context(), config.New(opt))
	require.NoError(t, err)

	signIn := func(t *testing.T, path string) (signInQuery url.Values, returnTo *url.URL) {
		res, err := a.requireLoginResponse(t.Context(), &envoy_service_auth_v3.CheckRequest{
			Attributes: &envoy_service_auth_v3.AttributeContext{
				Request: &envoy_service_auth_v3.AttributeContext_Request{
					Http: &envoy_service_auth_v3.AttributeContext_HttpRequest{
						Scheme: "https", Host: "clip.example.com", Path: path, Method: "GET",
						Headers: map[string]string{"accept": "text/html"},
					},
				},
			},
		}, &evaluator.Request{})
		require.NoError(t, err)
		require.Equal(t, http.StatusFound, int(res.GetDeniedResponse().GetStatus().GetCode()))
		var location string
		for _, h := range res.GetDeniedResponse().GetHeaders() {
			if h.GetHeader().GetKey() == "Location" {
				location = h.GetHeader().GetValue()
			}
		}
		u, err := url.Parse(location)
		require.NoError(t, err)
		returnTo, err = url.Parse(u.Query().Get(urlutil.QueryRedirectURI))
		require.NoError(t, err)
		return u.Query(), returnTo
	}

	t.Run("select_account rides the signed sign-in URL, not the return URL", func(t *testing.T) {
		t.Parallel()
		q, back := signIn(t, "/panel?pomerium_prompt=select_account&x=1")
		assert.Equal(t, "select_account", q.Get(urlutil.QueryPrompt))
		assert.NotEmpty(t, q.Get(urlutil.QueryHmacSignature), "the prompt is inside the signature")
		assert.Equal(t, "/panel", back.Path)
		assert.Equal(t, url.Values{"x": {"1"}}, back.Query())
	})
	t.Run("any other value is dropped, from both", func(t *testing.T) {
		t.Parallel()
		q, back := signIn(t, "/?pomerium_prompt=login&x=1")
		assert.Empty(t, q.Get(urlutil.QueryPrompt))
		assert.Equal(t, url.Values{"x": {"1"}}, back.Query())
		q, _ = signIn(t, "/?pomerium_prompt=none")
		assert.Empty(t, q.Get(urlutil.QueryPrompt))
	})
	t.Run("no prompt asked: none sent", func(t *testing.T) {
		t.Parallel()
		q, back := signIn(t, "/?x=1")
		assert.Empty(t, q.Get(urlutil.QueryPrompt))
		assert.Equal(t, url.Values{"x": {"1"}}, back.Query())
	})
}
