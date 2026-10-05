package generate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/internal/errs"
)

const invoiceSnapshot = `{
  "app": "billing",
  "methods": {
    "GetInvoice": {"in": "InvoiceID", "out": "Invoice"}
  },
  "types": {
    "Invoice": {"fields": {"ID": {"number": 1, "type": "int64"}, "Total": {"number": 2, "type": "int64"}}},
    "InvoiceID": {"fields": {"ID": {"number": 1, "type": "int64"}}}
  }
}
`

const ledgerSnapshot = `{"app": "ledger", "methods": {"Post": {"in": "Entry", "out": "Entry"}}, "types": {"Entry": {"fields": {"ID": {"number": 1, "type": "int64"}}}}}`

func writeFile(t *testing.T, path, data string) {
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
}

func TestCheckSnapshots(t *testing.T) {
	root := t.TempDir()
	shop, billing := filepath.Join(root, "shop"), filepath.Join(root, "billing")
	held := filepath.Join(shop, ".aicoded", "services", "billing.json")
	writeFile(t, held, invoiceSnapshot)
	writeFile(t, filepath.Join(shop, ".aicoded", "services", "ledger.json"), ledgerSnapshot)
	writeFile(t, filepath.Join(billing, ".aicoded", "rpc.json"), invoiceSnapshot)
	apps := map[string]string{"shop": shop, "billing": billing}

	warnings, err := CheckSnapshots(apps)
	require.NoError(t, err)
	assert.Equal(t, []string{"shop holds a snapshot of ledger, which is not in this workspace"}, warnings)

	writeFile(t, filepath.Join(billing, ".aicoded", "rpc.json"), `{
  "app": "billing",
  "methods": {"GetInvoice": {"in": "InvoiceID", "out": "Invoice"}},
  "types": {
    "Invoice": {"fields": {"ID": {"number": 1, "type": "int64"}}, "retired": [2]},
    "InvoiceID": {"fields": {"ID": {"number": 1, "type": "int64"}}}
  }
}
`)
	_, err = CheckSnapshots(apps)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-RPC-013", e.Code)
	assert.Equal(t, held+":4", e.Pos, "the line of the broken function in the held snapshot")
	assert.Contains(t, e.Msg, "Total")
	assert.Equal(t, "restore both the rpc/ folder and .aicoded/rpc.json of billing from the version history, "+
		"or run aicoded rpc add billing in shop and update its calls", e.Fix, "aicoded dev has renumbered billing's snapshot already")

	require.NoError(t, os.Remove(filepath.Join(billing, ".aicoded", "rpc.json")))
	_, err = CheckSnapshots(apps)
	assert.Equal(t, "E-RPC-013", errs.Code(err), "billing serves no function any more")
}

func TestCheckSnapshotsRefusesAnInvalidSnapshot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "shop", ".aicoded", "services", "billing.json"), "{")
	writeFile(t, filepath.Join(root, "billing", ".aicoded", "rpc.json"), invoiceSnapshot)
	_, err := CheckSnapshots(map[string]string{"shop": filepath.Join(root, "shop"), "billing": filepath.Join(root, "billing")})
	assert.Equal(t, "E-RPC-005", errs.Code(err))
}

func TestCheckSnapshotsInAnyFolder(t *testing.T) {
	shop := filepath.Join(t.TempDir(), "[shop]*")
	writeFile(t, filepath.Join(shop, ".aicoded", "services", "billing.json"), invoiceSnapshot)
	_, err := CheckSnapshots(map[string]string{"shop": shop, "billing": t.TempDir()})
	assert.Equal(t, "E-RPC-013", errs.Code(err), "a folder name is not a pattern")
}

func TestMethodLine(t *testing.T) {
	assert.Equal(t, 4, methodLine([]byte(invoiceSnapshot), "GetInvoice"))
	assert.Equal(t, 1, methodLine([]byte(invoiceSnapshot), "Invoice"), "a type is not a method")
	assert.Equal(t, 1, methodLine([]byte("{"), "GetInvoice"))
}

func TestCheckHeld(t *testing.T) {
	root := t.TempDir()
	apps := map[string]string{}
	for _, app := range []string{"billing", "desk", "shop"} {
		apps[app] = filepath.Join(root, app)
		writeFile(t, filepath.Join(apps[app], "aicoded.yaml"), "app: "+app+"\n")
	}
	writeFile(t, filepath.Join(apps["desk"], ".aicoded", "services", "billing.json"), invoiceSnapshot)
	held := filepath.Join(apps["shop"], ".aicoded", "services", "billing.json")
	writeFile(t, held, invoiceSnapshot)
	writeFile(t, filepath.Join(apps["shop"], ".aicoded", "services", "ledger.json"), ledgerSnapshot)

	_, err := CheckHeld("shop", apps)
	var e *errs.Error
	require.ErrorAs(t, err, &e)
	assert.Equal(t, "E-RPC-013", e.Code, "billing serves no function")
	assert.Equal(t, held+":4", e.Pos, "shop's own snapshot, not the one of desk, which sorts first")

	require.NoError(t, os.Remove(held))
	warnings, err := CheckHeld("shop", apps)
	require.NoError(t, err)
	assert.Equal(t, []string{"shop holds a snapshot of ledger, which is not in this workspace"}, warnings)
	warnings, err = CheckHeld("billing", apps)
	require.NoError(t, err)
	assert.Empty(t, warnings, "billing holds nothing")
}
