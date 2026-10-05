# E-GEN-051: `<ssr:assets/>`, `<ssr:content/>` or `<html>` inside a condition or loop

`<ssr:assets/>` writes the page's scripts and styles, `<ssr:content/>` shows the page below a layout, and `<html>` holds the page's document. Inside `ssr:if`, `ssr:else-if`, `ssr:else` or `ssr:for` they would render only sometimes, or several times, so the page would sometimes run without its scripts or without its content, or reach the browser without a document.

**Fix:** move `<ssr:assets/>`, `<ssr:content/>` and `<html>` out of `ssr:if` and `ssr:for`, so they always render once.
