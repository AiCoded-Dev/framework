# E-GEN-020: inline script or page

The template has a `<script>` element with a body, or a `srcdoc` attribute. A `<script>` may
only load a file with `src`. Pages run under a strict Content-Security-Policy that blocks inline
code, so this would not run in the browser either. `srcdoc` holds a whole page inline, as markup
in an attribute that the generator does not check; a frame loads a page of the app with `src`
instead. Values that page scripts need go in an `<ssr:json>` element, which holds them as data
that never runs.

**Fix:** move the code to index.ts next to the template and a frame's page into a page of the
app loaded with src; pass data with `<ssr:json>`
