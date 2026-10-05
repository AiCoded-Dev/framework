# E-GEN-021: `<style>` element

The template has a `<style>` element, at any depth, including inside `<svg>`. Pages run under a
strict Content-Security-Policy that blocks inline code, so this would not run in the browser
either. The generator builds `index.css` and serves it with the page.

**Fix:** move the rules to index.css next to the template
