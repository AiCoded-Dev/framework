# E-GATE-034: a page served what someone entered as markup

The simulated attacks (L7), which the delivery pipeline runs on the app it built, post each form they used again, with markup in every text field: an element, an attribute, a link that runs a script, and a script, each with a value of its own. Then they load every page they reached again and read it as a browser does. A page served one of them as markup, not as text: the element or attribute was in the page, a link or a form's address ran the script, or the script was in the page. So whoever enters such a value runs their own script in the browser of everyone who opens the page, with that viewer's access (cross-site scripting). The problem is at the template of the page that served it, at line 1, and the message names the form, the field and the kind of markup. Template expressions such as `{{ note }}` escape a value for where it lands, in text, an attribute or a URL, and `{{$ }}` writes as markup only a `web.SafeHTML`, which holds a viewer's text only through `web.EscapeHTML`; so look for markup that reaches the page another way. When the page shows what people enter only through template expressions and building blocks, tell your administrator, since the framework's escaping failed. The attacks read the page as the app sends it, and do not run its scripts. See [expressions](../guides/expressions.md) and [simulated attacks](../guides/simulated-attacks.md).

```html
<p>{{ note }}</p>                  <!-- right: the note is shown as text -->
<a href="{{ link }}">Open</a>      <!-- right: a link that is not http, https or mailto is made harmless -->
```

**Fix:** show what people enter only through template expressions and building blocks, never as markup the app builds.
