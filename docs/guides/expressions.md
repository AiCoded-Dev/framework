# Expressions: the `{{ }}` language

Expressions are compiled to Go, not interpreted: what you write between the braces becomes a Go
expression in the page's generated code, so the Go compiler checks its names and types. Each
value is escaped for the place in the page where it lands.

## Where expressions go

```html
<p>Total: {{ price * quantity }}</p>              <!-- element text -->
<span class="badge {{ tone }}">…</span>           <!-- a quoted attribute value -->
<a href="/users/{{ u.Login }}">…</a>              <!-- part of a URL -->
<div ssr:if="user.Age >= 18">…</div>              <!-- a condition, without braces -->
<li ssr:for="i, lang in langs">…</li>             <!-- a loop, without braces -->
<ssr:json name="langs-data" value="langs"/>       <!-- a value for scripts, without braces -->
```

An attribute value may mix fixed text with any number of expressions. Attribute values with an
expression must be in quotes (E-GEN-024).

## Escaping

`{{ expr }}` picks its escaper from where it stands, when the page is generated:

| Where | Example | What is written |
|---|---|---|
| element text, also in `<title>` and `<textarea>` | `<p>{{ note }}</p>` | the text with `&`, `<`, `>`, `"` and `'` as character references |
| a quoted attribute value | `class="{{ tone }}"` | the same |
| a whole URL attribute | `href="{{ link }}"` | the URL, percent-encoded where needed; a URL whose scheme is not `http`, `https` or `mailto` becomes `about:invalid#aicoded-unsafe-url` |
| a URL after fixed text with a path or scheme | `href="/notes/{{ id }}"` | the value percent-encoded, keeping `/`, `?` and the other URL characters |
| a URL right after its leading `/` | `href="/{{ slug }}"` | the same, but a leading `/` is encoded and an empty value is `.`, so the URL never starts with `//` |
| a URL after `?` or `#` | `href="/find?q={{ q }}"` | the value percent-encoded as one query part, `&` and `=` included |

URL attributes are `href`, `src`, `action`, `formaction`, `poster`, `cite` and the like, and every
attribute whose name contains `src`, `uri` or `url`. Inline scripts, styles, event handlers and
attributes that load code take no expressions at all; see
[template syntax](template-syntax.md).

`{{$ expr }}` writes markup as it is. It takes only a `web.SafeHTML`, and only in element text:
never in an attribute, `<title>` or `<textarea>` (E-GEN-029). A `web.SafeHTML` is made in Go with
`web.HTMLConst` from a string constant, `web.EscapeHTML` from any text, or `web.JoinHTML` from
other `web.SafeHTML` values, so a viewer's text can only reach it escaped:

```go
data.Intro = web.JoinHTML(web.HTMLConst("You are signed in as <strong>"),
	web.EscapeHTML(auth.Viewer(ctx).Name), web.HTMLConst("</strong>."))
```

```html
<ssr:var name="intro" type="web.SafeHTML"/>
<p>{{$ intro }}</p>
```

## Literals and operators

| Kind | Examples |
|---|---|
| strings | `'text'`, `"text"`, `` `text` ``; single quotes read best inside attributes |
| numbers | `42`, `-7`, `3.14` |
| names | variables, loop variables, Go's `true`, `false` and `nil` |

| Operators | |
|---|---|
| arithmetic | `+` `-` `*` `/` `%` |
| comparison | `==` `!=` `<` `<=` `>` `>=` |
| logic | `&&` `||` `!` |
| grouping | `( … )` |
| fields and methods | `user.Name`, `user.FullName()` |
| indexing | `users[0]`, `scores['alice']` |
| calls | `formatMoney(balance, 2)`, `len(users)` |
| conditional | `cond ? whenTrue : whenFalse` |

Go's rules apply: no conversion between number types, no truthiness, and `+` on strings joins
them. The binary operators are written into Go as they are, so Go's precedence holds:
`{{ 1 + 2 * 3 }}` is 7.

## The conditional operator

`cond ? a : b` compiles to `render.If(cond, a, b)`, a function call:

- `a` and `b` must have the same type.
- Both are always evaluated. A branch that would panic, such as `users[0].Name` on an empty
  list, panics even when `cond` is false; use `ssr:if` instead.
- Its bounds come from the template's own grammar, in which every operator has one level and
  groups to the left. `{{ a ? b : c + 1 }}` is `render.If(a, b, c) + 1`, and
  `{{ x ? 'a' : y ? 'b' : 'c' }}` nests to the left. Put parentheses around a conditional inside
  another expression: `{{ x ? 'a' : (y ? 'b' : 'c') }}`.

The usual nesting for a field's state reads well because it nests in the middle:

```html
class="{{ form.IsValidated() ? input.HasError() ? 'invalid' : 'valid' : '' }}"
```

## Names you can use

- The variables the template declares with `<ssr:var>`, and the loop variables of `ssr:for`.
- Fields and methods of any value they reach.
- Functions declared in the page's package, in a `.go` file next to the template, and Go's
  built-in functions such as `len`.
- A variable of func type, declared as `<ssr:var name="tabClass" type="func(string) string"/>`
  and called as `{{ tabClass('info') }}`.

There are no imports in templates. To use another package, wrap it in a function next to the
template, or pass the result in a variable.

An expression is app code: `aicoded check` holds the Go code written from it to the same
[rules](rules.md) as the Go code next to the template, such as constant SQL (E-LINT-007) and no
personal data or secret in a log or span (E-LINT-008), and points to the template's line.

Some names are the generated code's own and refused (E-GEN-042): `w`, `s`, `render`, `web`,
`form` (outside a form), `reactive`, `io`, `context`, `http`, `json`, `strings`, `sync`,
`errors`, `slog`, and names starting with `_html`; and the names the page's generated files
declare: `routeKey`, `route`, `state`, `NewRoute`, `RouteData`, `RouteDataProvider`,
`ReactiveState`, `NewHandler`, `assets`, `assetsDir`, names starting with `renderBlock_`, and a
form's `Form<Name>Values`.

## How values are written

- Strings are written as they are, then escaped.
- Integers, `bool` and floats are formatted directly. Floats use the shortest form that reads
  back the same value, so `9.99 * 3` shows `29.97`; format money and other fixed-point values
  yourself.
- Anything else goes through `fmt.Sprint`, so a type with a `String()` method shows through it.

## In forms

Inside `<ssr:form>`, `form` is the form's values, and `input` (in `<ssr:input>` and
`<ssr:select>`) or `textarea` (in `<ssr:textarea>`) is the field being written:

```html
<ssr:input name="login" type="text" required
           class="{{ form.IsValidated() ? input.HasError() ? 'invalid' : 'valid' : '' }}"/>
<p class="error" ssr:if="form.Login.HasError()">{{ form.Login.GetError() }}</p>
```

See [forms](forms.md).

## Live expressions

An expression that reads a variable declared with `reactive="true"` becomes a live part of the
page, which the server renders again and sends when the value changes. See
[live values](live-values.md).

## See also

- [Template syntax](template-syntax.md): the tags and attributes around expressions.
- [people/pages/home/index.html](../../examples/people/pages/home/index.html): arithmetic,
  conditions, loops and `{{$ }}` on one page.
