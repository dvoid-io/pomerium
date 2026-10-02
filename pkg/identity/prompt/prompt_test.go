package prompt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMerge(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ configured, want string }{
		{"", "select_account"},
		{"login", "login select_account"},
		{"consent  login", "consent login select_account"},
		{"select_account consent", "select_account consent"},
		{"none", "none"},
	} {
		assert.Equal(t, tc.want, Merge(tc.configured, SelectAccount), "configured %q", tc.configured)
	}
}

func TestContext(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", FromContext(context.Background()))
	assert.Equal(t, SelectAccount, FromContext(WithSelectAccount(context.Background())))
}
