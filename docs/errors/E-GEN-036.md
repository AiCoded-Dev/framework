# E-GEN-036: script or style does not build

A page's `index.ts` or `index.css`, or a file it imports, does not build. Scripts are
TypeScript, bundled by esbuild without type checking: types are removed, not checked. Styles
are plain CSS with native nesting, which the build lowers for older browsers. Scripts and
styles may load only files in the app's `pages/` and `assets/` folders, by a path such as
`./label`: packages, anything in a `node_modules` folder and URLs are refused, because
outside code would skip the security checks. A page with live values or calls imports its
generated `./reactive_gen`; any other `reactive_gen.ts` is left from an earlier run and is
refused. Images and fonts that styles use are copied into the app binary, so their names
follow the rules of E-GEN-035. `tsconfig.json` files are ignored. The message is esbuild's
or the generator's, at the file, line and column where the build stopped.

**Fix:** fix the file at this line
