// Package prompt carries a per-request OIDC prompt (dvoid fork) from the
// authenticate service to the identity provider's sign-in redirect.
//
// The only prompt a request may ask for is select_account: it adds the IdP's
// account chooser and never removes an interaction the configuration requires.
// The authenticate service sets it only when authorize put it in the signed
// sign-in URL (authenticate.requestedPrompt).
package prompt

import (
	"context"
	"strings"
)

// SelectAccount asks the identity provider to show its account chooser.
const SelectAccount = "select_account"

type ctxKey struct{}

// WithSelectAccount returns a context that asks the sign-in redirect for the
// account chooser.
func WithSelectAccount(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKey{}, SelectAccount)
}

// FromContext returns the prompt the request asked for, or "".
func FromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKey{}).(string)
	return v
}

// Merge adds the requested prompt to the configured one (OIDC Core 3.1.2.1: a
// space-delimited list). It never replaces a configured value, so a request can
// only add an interaction, and it leaves `none` alone, because `none` must
// stand alone and the configuration chose it.
func Merge(configured, requested string) string {
	values := strings.Fields(configured)
	for _, v := range values {
		if v == "none" || v == requested {
			return strings.Join(values, " ")
		}
	}
	return strings.Join(append(values, requested), " ")
}
