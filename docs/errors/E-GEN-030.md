# E-GEN-030: page without an access rule

No template on the path from `pages/` to this page has an `<ssr:access>` rule, so nothing says
who may open the page. Pages are closed by default: a page nobody may open is safer than a page
everybody may open by mistake. The rule can sit in the page's own template or in the template of
any parent folder, and every rule on the path applies. `role="*"` opens a page to every viewer
the company login lets into the app.

**Fix:** add `<ssr:access role="…"/>` to this template or to a parent folder's template
