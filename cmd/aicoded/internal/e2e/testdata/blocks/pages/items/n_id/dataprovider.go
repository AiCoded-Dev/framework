package n_id

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"aicoded.dev/framework/web"

	"blocks/deps"
)

var _ RouteDataProvider = &DP{}

// DP shows one item and its document.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data reads the item from the database and its document from the store.
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	id := r.URLParamInt("id")
	err := p.d.DB.QueryRowContext(ctx, "SELECT id, title FROM items WHERE id = ?", id).Scan(&data.Item.ID, &data.Item.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return web.NotFound()
	}
	if err != nil {
		return err
	}
	doc, err := p.d.Docs.ReadFile(fmt.Sprintf("item-%d.txt", id))
	if err != nil {
		return err
	}
	data.Doc = string(doc)
	return nil
}
