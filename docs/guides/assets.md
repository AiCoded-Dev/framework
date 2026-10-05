# Scripts, styles and images

A page's script is `index.ts` and its styles `index.css`, next to its template. `aicoded
generate` builds them with esbuild, which is built into `aicoded`, copies the page's images, and
embeds everything in the app, so there is no Node.js, npm or `node_modules` and no build step of
your own.

## The files

```text
pages/
  index.html
  index.css          styles of the layout, loaded on every page inside it
  home/
    index.html
    index.ts         the home page's script
    logo.svg         an image the template shows
assets/
  team.png           an image any page shows as /assets/team.png
```

- `index.ts` is TypeScript. The build removes the types without checking them; your editor
  checks them.
- `index.css` is plain CSS. Native nesting works and is lowered for older browsers.
- A script imports only files of the app, in its `pages/` and `assets/` folders, by a relative
  path such as `./label`. Packages, files in a `node_modules` folder and URLs are refused, since
  code from outside would skip the security checks (E-GEN-036). `tsconfig.json` is ignored.
- A page with live values or calls also gets a generated `reactive_gen.ts`, which its
  `index.ts` imports as `./reactive_gen`; see the [TypeScript API](typescript-api.md).
- The build targets Chrome 111, Firefox 115 and Safari 16.4, minifies everything and writes no
  source maps.

## Loading them

`<ssr:assets/>` writes the `<link rel="stylesheet">` and `<script defer>` tags of its own page
and of every page shown inside it, each once. Put it in the `<head>` of the top layout, usually
`pages/index.html`:

```html
<head>
<meta charset="utf-8">
<ssr:access role="*"/>
<title>People</title>
<ssr:assets/>
</head>
```

- A page whose scripts or styles no `<ssr:assets/>` would write is refused (E-GEN-050): it
  needs one in its own template or in a layout above it. A page below a gate is not shown inside
  the gate, so the gate's `<ssr:assets/>` does not count for it.
- `<ssr:assets/>` cannot be inside `ssr:if` or `ssr:for` (E-GEN-051), or inside a live part of the
  page (E-GEN-046).
- On a page with live values or calls, it also writes the framework's own script and styles for
  the live connection.
- Scripts run in the order of the tags, after the page is parsed: a layout's script before the
  script of the page inside it.

## Images

An `<img src>` that names a file next to the template, such as `src="logo.svg"`, or
`src="/assets/<file>"` in the app's `assets/` folder, is copied into the app with a hash in its
name, and the tag points at the copy.

- Only images and fonts are copied: `.gif`, `.ico`, `.jpeg`, `.jpg`, `.png`, `.svg`, `.webp`,
  `.woff` and `.woff2`, so a source file is never published by mistake.
- A file that is missing, outside `pages/` and `assets/`, or not one of these is refused
  (E-GEN-035). Its folder and file names use letters, digits, spaces and
  ``!#$%&()+,-.=@[]^_{}~``.
- Images and fonts that `index.css` names with `url()` are copied the same way.
- A source computed with `{{ }}`, one that starts with a scheme such as `https:` or with `//`,
  and any other path starting with `/` are left as they are. The page's Content-Security-Policy
  loads images only from the app and from `data:` URLs, so a browser shows no image from another
  site.

## How they are served

The build writes to `pages/assets_gen/`, and the generated `pages/assets_gen.go` embeds that
folder in the app binary. Both are committed, like every generated file. The app serves them at
`/_aicoded/assets/<path>`:

- with `Cache-Control: public, max-age=31536000, immutable`, since each name holds a hash of the
  content, and an `ETag`;
- gzipped for scripts, styles and SVG when the browser accepts it;
- to `GET` and `HEAD` only.

`aicoded dev` builds them again when a file changes. To build them yourself, run
`aicoded generate`.

## The Content-Security-Policy

Every page carries this policy, which the app sets and a hook cannot change:

```text
default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'
```

No inline script, inline style or code from another site runs, and no other site can frame the
page. That is why the generator refuses `<script>` bodies, `<style>`, `style` and `on…`
attributes: they would not run anyway.

## See also

- [TypeScript API](typescript-api.md): what a page's script can do with its live values and calls.
- [Template syntax](template-syntax.md): what the generator refuses in a template.
- [people/pages/index.css](../../examples/people/pages/index.css): the people example's styles,
  with native nesting.
