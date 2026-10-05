# Template syntax: tags and attributes

A page's `index.html` is HTML with `ssr:` tags, `ssr:` attributes and `{{ }}` expressions.
`aicoded generate` turns it into Go code, checks it, and refuses anything that could run code in
the browser or write a value where escaping cannot make it safe.

## Tags

Every tag is written `<ssr:name …/>`. An unknown `ssr:` tag is an error (E-GEN-004), and `ssr:`
attributes do not work on `ssr:` tags (E-GEN-005).

| Tag | What it does |
|---|---|
| `<ssr:access role="a,b" guard="true"/>` | who may open the page; see [access](access.md) |
| `<ssr:var name="…" type="…"/>` | declares a value the page's data provider fills |
| `<ssr:content default="…"/>` | where the page below a layout is shown; see [routing](routing.md) |
| `<ssr:assets/>` | the script and style tags of the page and the pages inside it; see [assets](assets.md) |
| `<ssr:form name="…">` | a form that posts to the page; see [forms](forms.md) |
| `<ssr:input>`, `<ssr:select>`, `<ssr:textarea>` | the fields of a form |
| `<ssr:json name="…" value="…"/>` | passes a value to the page's script as JSON |
| `<ssr:call name="…" in="…" out="…"/>` | a function the page's script may call; see [page calls](page-calls.md) |

### `<ssr:var>`

```html
<ssr:var name="users" type="[]User"/>
<ssr:var name="intro" type="web.SafeHTML"/>
<ssr:var name="tabClass" type="func(string) string"/>
<ssr:var name="userCount" type="int" reactive="true"/>
<ssr:var name="displayName" type="string" reactive="true" client-writable="true"/>
```

- `name` is a Go name, used in expressions. It becomes an exported field of the page's
  `RouteData`: `userCount` is `RouteData.UserCount`.
- `type` is a Go type that the page's package can name: a predeclared type such as `string` or
  `int64`, a type declared in a `.go` file next to the template, an exported type of package
  `web`, or a pointer, slice, array, map or func of these (E-GEN-045). A type of another
  package, such as `time.Time`, or a `struct{…}` with fields, is refused: declare a type next to
  the template, such as `type User = deps.User`.
- `reactive="true"` makes it a live value, and `client-writable="true"` lets the page write it
  back; see [live values](live-values.md).

Declarations may stand anywhere in the template; where they stand changes nothing.

### `<ssr:json>`

```html
<ssr:var name="langs" type="[]string"/>
<ssr:json name="langs-data" value="langs"/>
```

It renders `<script type="application/json" id="langs-data">["Go","TypeScript","Rust"]</script>`.
A script reads it with `JSON.parse(document.getElementById("langs-data")?.textContent ?? "[]")`.
The name is lower-case letters, digits and `-`, used once per template, and the value is an
expression written without `{{ }}` (E-GEN-033). Browsers never run it, so it is the way to hand
data to a script.

## Attributes on HTML elements

| Attribute | What it does |
|---|---|
| `ssr:if="expr"` | renders the element only when `expr` is true |
| `ssr:else-if="expr"` | follows an `ssr:if` or `ssr:else-if` element |
| `ssr:else` | follows an `ssr:if` or `ssr:else-if` element |
| `ssr:for="item in list"`, `ssr:for="i, item in list"` | repeats the element for each item |
| `ssr:bind="name"` | keeps a native input in step with a client-writable live value |

```html
<span ssr:if="user.Age <= 18">0-18</span>
<span ssr:else-if="user.Age <= 30">19-30</span>
<!-- the older groups -->
<span ssr:else>31+</span>

<li ssr:for="u in users"><a href="/users/{{ u.Login }}">{{ u.Name }}</a></li>
<li ssr:for="i, lang in langs">#{{ i }}: {{ lang }}</li>
```

- `ssr:else-if` and `ssr:else` go on the element right after the one they continue; only
  whitespace and comments may stand between them (E-GEN-006).
- `ssr:for` compiles to Go's `for i, item := range list`, so it works on slices, arrays, maps
  and strings. For a map, `i` is the key; for a string, the byte index, and `item` a rune. An
  element cannot have both `ssr:for` and `ssr:if` (E-GEN-007): put one on an element around the
  other.
- The loop variables exist only inside the element.

## Expressions

`{{ expr }}` writes a value into text or into a quoted attribute value, escaped for where it
lands. `{{$ expr }}` writes markup as it is, and takes only `web.SafeHTML`. An attribute value
may mix fixed text and several expressions:

```html
<p>Total: {{ price * quantity }}</p>
<a class="{{ u.Login == current ? 'active' : '' }}" href="/users/{{ u.Login }}">{{ u.Name }}</a>
```

See [expressions](expressions.md) for the language and the escaping.

## What the generator refuses

Pages run under a strict Content-Security-Policy, which blocks inline code, and the generator
refuses it before the browser would:

- a `<script>` with a body, and `srcdoc` (E-GEN-020); load scripts from `index.ts` and pass data
  with `<ssr:json>`;
- a `<style>` element (E-GEN-021) and a `style` attribute (E-GEN-023); use `index.css`;
- an `on…` attribute such as `onclick` (E-GEN-022); attach handlers in `index.ts`;
- a `javascript:` or `vbscript:` URL (E-GEN-028).

It also refuses a value where no escaping makes it safe:

- in an attribute value without quotes (E-GEN-024);
- in a tag or attribute name (E-GEN-025);
- inside `<iframe>`, `<noscript>`, `<noembed>`, `<noframes>`, `<xmp>` or `<plaintext>` (E-GEN-026);
- in an attribute of `<script>`, `<base>`, `<meta>`, `<link>`, `<object>`, `<embed>` or an SVG
  animation, or in `srcset`, `imagesrcset` or `http-equiv` (E-GEN-027);
- where it could choose a URL's scheme, as in `href="{{ proto }}://x"` (E-GEN-038).

Attributes starting with `data-ssr-` are the generated code's own (E-GEN-042).

## Whitespace, comments and the document

- Outside `<pre>`, `<textarea>` and the raw-text elements, a run of whitespace becomes one
  space, as the browser shows it.
- HTML comments are dropped, so they never reach the browser.
- The doctype is kept. Keep `<!doctype html>` first in the top layout.
- The page a viewer opens must have an `<html>` element, in its own template or a layout's
  (E-GEN-052). `<html>`, `<ssr:assets/>` and `<ssr:content/>` cannot be inside `ssr:if` or
  `ssr:for` (E-GEN-051).
- A template that is not valid HTML, such as a tag never closed or an attribute given twice, is
  refused (E-GEN-001).

## Images

An `<img src>` that names a file next to the template, such as `src="logo.svg"`, or
`src="/assets/<file>"` in the app's `assets/` folder, is copied into the app with a hash in its
name and served from there. See [assets](assets.md).

## See also

- [Expressions](expressions.md): the `{{ }}` language.
- [people/pages/home/index.html](../../examples/people/pages/home/index.html): a page that uses
  most of this guide.
