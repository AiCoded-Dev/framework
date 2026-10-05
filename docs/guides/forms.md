# Forms

A form is a plain HTML form that posts to its own page. The generated code reads each field into
its Go type, checks required fields and select options, and checks the form token; you write two
hooks, one that prepares the form and one that acts on it once every field is valid.

## Declaring a form

```html
<ssr:form name="contact">
  <label for="contact-name">Your name</label>
  <ssr:input name="name" type="text" required maxlength="100" id="contact-name"/>
  <p class="error" ssr:if="form.Name.HasError()">{{ form.Name.GetError() }}</p>

  <ssr:select name="topic" required/>
  <ssr:textarea name="message" required maxlength="2000"/>
  <button type="submit">Send message</button>
</ssr:form>
```

- The form posts to the page it is on, with two hidden fields: `_aicoded_form`, which names the
  form, and `_aicoded_csrf`, the form token.
- The name is letters, digits and `_`, starting with a letter, once per template (E-GEN-010).
  Forms cannot be nested (E-GEN-009), and fields must be inside one (E-GEN-008).
- Attributes the generator does not use itself, such as `id`, `class`, `maxlength` or
  `placeholder`, pass through to the HTML. `on…` and `style` attributes are refused, as anywhere.
- A page may have several forms. Only the posted form is read and processed.

## What is generated

For the form `contact`, the page's `route_gen.go` gets a struct of its fields, and the data
provider two hooks:

```go
type FormContactValues struct {
	form.BaseFormValues
	Name    form.Input[string]
	Topic   form.Select[string]
	Message form.Textarea
}

InitContact(ctx context.Context, r *web.Request, w web.ResponseWriter, data *FormContactValues) error
ProcessContact(ctx context.Context, r *web.Request, w web.ResponseWriter, data *FormContactValues) error
```

The field types are in the package `aicoded.dev/framework/web/form`, which the
generated code imports as `form`. Import it the same way where your code names them, such as
`form.SelectOption` in `Init`; the framework has no package `aicoded.dev/framework/form`
(E-CHK-002).

A field's Go name is its `name` with the first letter in upper case. Its type follows the tag:

| Field | Type |
|---|---|
| `<ssr:input name="a"/>` | `form.Input[string]` |
| `<ssr:input name="a" type="number" gotype="uint8"/>` | `form.Input[uint8]` |
| radio inputs sharing a name, with `gotype="uint8"` | `form.Input[uint8]` |
| checkbox inputs sharing a name, with `gotype="bool"` | `form.InputMultiple[bool]` |
| `<ssr:input name="a" type="file"/>` | `form.File` |
| `<ssr:input name="a" type="file" multiple/>` | `form.FileMultiple` |
| `<ssr:select name="a"/>` | `form.Select[string]` |
| `<ssr:select name="a" multiple/>` | `form.SelectMultiple[string]` |
| `<ssr:textarea name="a"/>` | `form.Textarea` |

- `gotype` is `string` (the default), `bool`, `int`, `int8` to `int64`, `uint`, `uint8` to
  `uint64`, `float32` or `float64` (E-GEN-012).
- Only radio inputs, or only checkbox inputs, may share a name, with one `gotype` (E-GEN-013,
  E-GEN-014); their `value`s must be values of that type.
- A form with a file input is sent as `multipart/form-data`, the `enctype` the generator gives
  it when it has none; another `enctype` is refused (E-GEN-011).

## The hooks

On every request to the page, `Init<Form>` runs for every form of the page and of its layouts.
It sets starting values and the options of selects, and runs on a `POST` after the form token
was checked, so a forged post never reaches it:

```go
// InitContact offers the topics.
func (p *DP) InitContact(_ context.Context, _ *web.Request, _ web.ResponseWriter, f *FormContactValues) error {
	f.Topic.SetOptions([]form.SelectOptionElement[string]{
		form.SelectOption[string]{Value: "General question", Label: "General question"},
		form.SelectOption[string]{Value: "Billing", Label: "Billing"},
		form.SelectOptionGroup[string]{Label: "Closed", Options: []form.SelectOptionElement[string]{
			form.SelectOption[string]{Value: "Sales", Label: "Sales", Disabled: true},
		}},
	})
	return nil
}
```

`Init<Form>` runs every time the page shows or receives the form: on every `GET` too, and on a
`POST` before `Process<Form>`. So it must never change data: no insert, update or delete, and no
mail, or opening the page would change the data. Change data in `Process<Form>`, which runs only
for a post of that form with every field valid. An action that a link would start, such as
cancelling a booking, is a form with a button whose `Process` acts.

On a `POST`, each field of the posted form is then read into its type:

- a `required` field left empty gets "This field is required";
- a value that does not parse gets "Invalid syntax", and a number that does not fit its type
  "Value out of range";
- a select value that is not one of its enabled options gets "Choose one of the options", so a
  viewer can never post a value you did not offer.

`Process<Form>` runs only when every field is valid. It checks what only your code knows, sets
errors, and acts:

- Set an error on a field with `data.Name.SetError("Enter your name.")`, or on the form with
  `data.SetError(…)`, and return `nil`: the page renders again with the posted values and the
  errors.
- When it succeeds, return `web.Redirect(path)`. The 303 answer keeps a reload from posting the
  form again.
- Any other error answers 500 "Something went wrong." and is logged, so keep it for what went
  wrong on the server.

The people example's contact form checks its fields, mails the team and redirects:

```go
func (p *DP) ProcessContact(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormContactValues) error {
	message := strings.TrimSpace(f.Message.GetValue())
	if message == "" {
		f.Message.SetError("Enter a message.")
	}
	if f.HasError() {
		return nil
	}
	// … send the mail …
	return web.Redirect("/contact?sent=1")
}
```

Its `Data` then reads `?sent=1` from `r.URL.Query()` to show a banner. Its mail's idempotency key
is made from the viewer, the topic and the message alone, so the same message sent again later is
not mailed; a form where repeats are legitimate adds the date to the key, so the same message is
mailed again on another day.

## Reading values

| Method | Of | Returns |
|---|---|---|
| `GetValue()` | `Input[T]`, `Select[T]` | the value, `T` |
| `GetValue()` | `InputMultiple[T]`, `SelectMultiple[T]` | the chosen values, `map[T]struct{}` |
| `GetValue()` | `Textarea` | the text |
| `GetValue()` | `File` | the uploaded file, `*form.FileHeader`, or nil |
| `GetValue()` | `FileMultiple` | the uploaded files, `[]*form.FileHeader` |
| `SetValue(v)` | every field but files | sets the value shown, as `Init` does for a default |
| `SetOptions(o)`, `GetOptions()` | selects | the options |
| `SetError(msg)`, `HasError()`, `GetError()` | every field | the field's error |
| `IsNotNull()` | every field | whether it has a value |
| `GetFormValue()` | `Input[T]` | the text as posted, also when it did not parse |

The form itself has `HasError()` (the form or any field has an error), `GetError()` and
`SetError(msg)` for the form-wide error, and `IsValidated()` (the form was posted and checked).

In the template, `form` is the form's values and `input` (in `<ssr:input>` and `<ssr:select>`)
or `textarea` (in `<ssr:textarea>`) the field being written:

```html
<ssr:input name="login" type="text" required
           class="{{ form.IsValidated() ? input.HasError() ? 'invalid' : 'valid' : '' }}"/>
<p class="error" ssr:if="form.Login.HasError()">{{ form.Login.GetError() }}</p>
<p class="error" ssr:if="form.GetError() != ''">{{ form.GetError() }}</p>
```

## Files

```html
<ssr:form name="add" enctype="multipart/form-data">
  <ssr:input name="photo" type="file" required accept="image/*"/>
  <ssr:input name="photos" type="file" multiple accept="image/*"/>
</ssr:form>
```

`form.FileHeader` embeds `*multipart.FileHeader`, with `Filename`, `Size` and `Open()`. The file
name, the size and the content all come from the browser, so check each before you keep the
file: the people example accepts a plain file name, at most 5 MiB read through
`io.LimitReader`, and content that `http.DetectContentType` sees as an image. A form is at most
32 MiB (413 "The form is too large."), of which 8 MiB is held in memory and the rest in
temporary files.

## The form token

- Every form carries a token, signed with a key the runner gives the app and bound to the app
  and to the viewer. It is valid for 24 hours.
- A `POST` without a valid token answers 403 before any form hook runs. So does a `POST` the
  browser marks as coming from another site.
- A form sent in the wrong encoding answers 400 "The form was sent in the wrong format.", a form
  that is not on the page 400 "This form is not on this page.", and a body that cannot be read
  400 "The form could not be read.".

There is nothing to set up: no key, no cookie, no list of allowed origins.

## Personal data

Form values are often personal data. Never log them or put them in a span:
`slog.Info("contact form", "message", message)` would copy a viewer's message into every place
logs go, and `aicoded check` refuses it (E-LINT-008), also when the value is quoted in a field's
error set with `SetError`. Log that something happened, never what a viewer wrote; see
[telemetry](telemetry.md).

## See also

- [Access](access.md): the checks that run before a form's hooks.
- [Expressions](expressions.md): `form`, `input` and `textarea` in the template.
- [File stores](file-stores.md): where to keep an upload.
- [Mail](mail.md): sending mail from `Process` with an idempotency key.
- [people/pages/contact](../../examples/people/pages/contact): a form that mails the team.
- [people/pages/users/add](../../examples/people/pages/users/add): every kind of field,
  including files.
- [Add a form](../tasks/add-form.md): fields the server checks, `Init` and `Process`, and a
  redirect.
- [Take a file upload](../tasks/upload-a-file.md): file fields, their checks and a file store.
