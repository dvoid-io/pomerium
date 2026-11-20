package authorize

import (
	"net/http/httptest"
	"testing"

	envoy_service_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pomerium/pomerium/authorize/evaluator"
	"github.com/pomerium/pomerium/config"
	hpke_handlers "github.com/pomerium/pomerium/pkg/hpke/handlers"
	"github.com/pomerium/pomerium/pkg/policy/criteria"
)

// dvoid fork: forbidden_redirect_url sends a denied browser navigation to our
// own page, carrying only the denied host, and changes nothing else.
func TestAuthorize_forbiddenRedirect(t *testing.T) {
	t.Parallel()

	newAuthorize := func(t *testing.T, forbiddenURL string) *Authorize {
		opt := config.NewDefaultOptions()
		opt.DataBroker.ServiceURL = "https://databroker.example.com"
		opt.SharedKey = "E8wWIMnihUx+AUfRegAQDNs8eRb3UrB5G3zlJW9XJDM="
		opt.ForbiddenRedirectURLString = forbiddenURL
		hpkePrivateKey, err := opt.GetHPKEPrivateKey()
		require.NoError(t, err)
		authnSrv := httptest.NewServer(hpke_handlers.HPKEPublicKeyHandler(hpkePrivateKey.PublicKey()))
		t.Cleanup(authnSrv.Close)
		opt.AuthenticateURLString = authnSrv.URL
		a, err := New(t.Context(), config.New(opt))
		require.NoError(t, err)
		return a
	}
	check := func(host, path string, headers map[string]string) *envoy_service_auth_v3.CheckRequest {
		return &envoy_service_auth_v3.CheckRequest{
			Attributes: &envoy_service_auth_v3.AttributeContext{
				Request: &envoy_service_auth_v3.AttributeContext_Request{
					Http: &envoy_service_auth_v3.AttributeContext_HttpRequest{
						Scheme: "https", Host: host, Path: path, Method: "GET", Headers: headers,
					},
				},
			},
		}
	}
	navigation := map[string]string{"accept": "text/html,application/xhtml+xml"}
	denied := &evaluator.Result{Allow: evaluator.NewRuleResult(false, criteria.ReasonClaimUnauthorized)}
	deny := func(a *Authorize, in *envoy_service_auth_v3.CheckRequest) *envoy_service_auth_v3.DeniedHttpResponse {
		res, err := a.handleResult(t.Context(), in, &evaluator.Request{}, denied)
		require.NoError(t, err)
		return res.GetDeniedResponse()
	}
	location := func(r *envoy_service_auth_v3.DeniedHttpResponse) string {
		for _, h := range r.GetHeaders() {
			if h.GetHeader().GetKey() == "Location" {
				return h.GetHeader().GetValue()
			}
		}
		return ""
	}

	t.Run("unset: the error page, unchanged", func(t *testing.T) {
		t.Parallel()
		r := deny(newAuthorize(t, ""), check("clip.example.com", "/", navigation))
		assert.Equal(t, 403, int(r.GetStatus().GetCode()))
		assert.Empty(t, location(r))
	})

	a := newAuthorize(t, "https://app.example.com/forbidden")

	t.Run("a denied navigation goes to the page with only the host", func(t *testing.T) {
		t.Parallel()
		r := deny(a, check("clip.example.com", "/admin/advance?x=1", navigation))
		assert.Equal(t, 302, int(r.GetStatus().GetCode()))
		assert.Equal(t, "https://app.example.com/forbidden?host=clip.example.com", location(r))
		assert.Empty(t, r.GetBody())
	})
	t.Run("a JSON client keeps its 403", func(t *testing.T) {
		t.Parallel()
		r := deny(a, check("clip.example.com", "/api", map[string]string{"accept": "application/json"}))
		assert.Equal(t, 403, int(r.GetStatus().GetCode()))
		assert.Empty(t, location(r))
	})
	t.Run("a Pomerium bearer client keeps its 403", func(t *testing.T) {
		t.Parallel()
		r := deny(a, check("clip.example.com", "/", map[string]string{
			"accept": "text/html", "authorization": "Pomerium some.jwt.value",
		}))
		assert.Equal(t, 403, int(r.GetStatus().GetCode()))
	})
	t.Run("a gRPC client keeps its 403", func(t *testing.T) {
		t.Parallel()
		r := deny(a, check("clip.example.com", "/svc/Method", map[string]string{"content-type": "application/grpc"}))
		assert.NotEqual(t, 302, int(r.GetStatus().GetCode()))
	})
	t.Run("the page itself is never redirected to itself", func(t *testing.T) {
		t.Parallel()
		r := deny(a, check("app.example.com", "/forbidden", navigation))
		assert.Equal(t, 403, int(r.GetStatus().GetCode()))
	})
	t.Run("an unauthenticated user still goes to sign in", func(t *testing.T) {
		t.Parallel()
		res, err := a.handleResult(t.Context(), check("clip.example.com", "/", navigation), &evaluator.Request{},
			&evaluator.Result{Allow: evaluator.NewRuleResult(false, criteria.ReasonUserUnauthenticated)})
		require.NoError(t, err)
		r := res.GetDeniedResponse()
		assert.Equal(t, 302, int(r.GetStatus().GetCode()))
		assert.NotContains(t, location(r), "/forbidden")
	})
}

func TestOptions_forbiddenRedirectURLValidation(t *testing.T) {
	t.Parallel()
	opt := config.NewDefaultOptions()
	opt.ForbiddenRedirectURLString = "not a url"
	u, err := opt.GetForbiddenRedirectURL()
	assert.Error(t, err)
	assert.Nil(t, u)
}
