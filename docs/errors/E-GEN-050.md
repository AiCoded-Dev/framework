# E-GEN-050: scripts or styles that never load

The page has `index.ts`, `index.css`, live values or calls, and `<ssr:assets/>` writes their tags. `<ssr:assets/>` writes the tags of its own page and of every page shown inside it, never of a layout above it. Here neither the page nor a layout above it has `<ssr:assets/>`, so the page would render without its scripts and styles. A page below a gate (a page with pages below it and no `<ssr:content/>`) is not shown inside the gate, so the gate's `<ssr:assets/>` does not count for it.

**Fix:** add `<ssr:assets/>` inside `<head>` of the top layout, such as `pages/index.html`, or of the page itself when no layout is above it.
