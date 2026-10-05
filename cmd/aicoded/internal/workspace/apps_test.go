package workspace_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/workspace"
	"aicoded.dev/framework/internal/errs"
)

func TestApps(t *testing.T) {
	root := t.TempDir()
	shop := app(t, root, "b-shop", "shop")
	billing := app(t, root, "a/billing", "billing")
	apps, err := workspace.Apps(root)
	require.NoError(t, err)
	assert.Equal(t, []workspace.App{{Name: "billing", Dir: billing}, {Name: "shop", Dir: shop}}, apps, "in folder order")
	assert.Equal(t, map[string]string{"billing": billing, "shop": shop}, workspace.Names(apps))
}

func TestAppsRefuses(t *testing.T) {
	root := t.TempDir()
	_, err := workspace.Apps(root)
	assert.Equal(t, "E-MAN-006", errs.Code(err), "no app")

	a, b := app(t, root, "a", "shop"), app(t, root, "b", "shop")
	_, err = workspace.Apps(root)
	assert.Equal(t, "E-DEV-010", errs.Code(err))
	require.ErrorContains(t, err, a+" and "+b)

	app(t, root, "b", "bad name")
	_, err = workspace.Apps(root)
	assert.Equal(t, "E-MAN-004", errs.Code(err), "a permission list that does not load")
}

func TestSelect(t *testing.T) {
	apps := []workspace.App{{Name: "billing", Dir: "/w/billing"}, {Name: "shop", Dir: "/w/shop"}}
	got, err := workspace.Select(apps, "")
	require.NoError(t, err)
	assert.Equal(t, apps, got, "every app")
	got, err = workspace.Select(apps, "shop")
	require.NoError(t, err)
	assert.Equal(t, apps[1:], got)

	for _, name := range []string{"ledger", "Shop", "../shop", "/w/shop", "shop\n"} {
		_, err := workspace.Select(apps, name)
		assert.Equal(t, "E-DEV-014", errs.Code(err), name)
	}
	var e *errs.Error
	require.ErrorAs(t, workspace.UnknownApp("a\nb"+strings.Repeat("x", 100)), &e)
	assert.Equal(t, `no app named "a\nb`+strings.Repeat("x", 61)+`…" in this workspace`, e.Msg, "quoted and cut")
	assert.Equal(t, "name an app from the app: line of an aicoded.yaml in this workspace", e.Fix)
}
