# E-GEN-052: page renders without `<html>`

A page a viewer can open is rendered from the layouts above it and the page itself. None of them has an `<html>` element, so the browser would get a bare fragment: no `<head>`, no title, no `<ssr:assets/>`. This happens when the root page is a gate (it has no `<ssr:content/>`) and a page below it writes only its body.

**Fix:** wrap the top layout, usually `pages/index.html`, in `<!doctype html><html>…</html>`; below a root gate, each page writes its own document.
