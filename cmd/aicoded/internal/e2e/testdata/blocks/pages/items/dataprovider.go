package items

import (
	"context"
	"fmt"

	"aicoded.dev/framework/mailer"
	"aicoded.dev/framework/web"

	"blocks/deps"
)

var _ RouteDataProvider = &DP{}

// DP lists the items and adds new ones.
type DP struct{ d *deps.Deps }

// NewDP returns the page's data provider.
func NewDP(d *deps.Deps) *DP { return &DP{d: d} }

// Data lists every item.
func (p *DP) Data(ctx context.Context, _ *web.Request, _ web.ResponseWriter, data *RouteData) error {
	rows, err := p.d.DB.QueryContext(ctx, "SELECT id, title FROM items ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var it deps.Item
		if err := rows.Scan(&it.ID, &it.Title); err != nil {
			return err
		}
		data.Items = append(data.Items, it)
	}
	return rows.Err()
}

// InitAdd needs nothing.
func (p *DP) InitAdd(context.Context, *web.Request, web.ResponseWriter, *FormAddValues) error {
	return nil
}

// ProcessAdd stores the item, writes its document and tells the team.
func (p *DP) ProcessAdd(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormAddValues) error {
	title := f.Title.GetValue()
	res, err := p.d.DB.ExecContext(ctx, "INSERT INTO items (title) VALUES (?)", title)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if err := p.d.Docs.WriteFile(fmt.Sprintf("item-%d.txt", id), []byte(title)); err != nil {
		return err
	}
	_, err = mailer.Send(ctx, mailer.Message{
		IdempotencyKey: fmt.Sprintf("item-%d", id),
		To:             []mailer.Address{{Address: "team@blocks.test"}},
		Subject:        "New item",
		Text:           title,
	})
	if err != nil {
		return err
	}
	return web.Redirect(fmt.Sprintf("/items/%d", id))
}
