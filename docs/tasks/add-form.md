# Add a form

Add a form whose values the server checks before the page acts on them. The example is the
contact form of the people example, at `/contact`: it mails the team, then shows a banner.

## Steps

1. Declare the form in the page's template with `<ssr:form name="…">`, and its fields with
   `<ssr:input>`, `<ssr:select>` and `<ssr:textarea>`. Show each field's error next to it:

   <!-- code: examples/people/pages/contact/index.html -->
   ```html
   <ssr:var name="sent" type="bool"/>
   <h1>Contact us</h1>
   <p>Your message goes to the team by mail.</p>
   <p id="sent" class="alert success" ssr:if="sent">Your message was sent.</p>

   <ssr:form name="contact">
     <div class="field">
       <label for="contact-name">Your name</label>
       <ssr:input name="name" type="text" required maxlength="100" id="contact-name"
                  class="{{ form.IsValidated() ? input.HasError() ? 'invalid' : 'valid' : '' }}"/>
       <p id="contact-name-error" class="error" ssr:if="form.Name.HasError()">{{ form.Name.GetError() }}</p>
     </div>
     <div class="field">
       <label for="contact-email">Your email address</label>
       <ssr:input name="email" type="email" required maxlength="254" id="contact-email"
                  class="{{ form.IsValidated() ? input.HasError() ? 'invalid' : 'valid' : '' }}"/>
       <p id="contact-email-error" class="error" ssr:if="form.Email.HasError()">{{ form.Email.GetError() }}</p>
     </div>
     <div class="field">
       <label for="contact-topic">Topic</label>
       <ssr:select name="topic" required id="contact-topic"
                   class="{{ form.IsValidated() ? input.HasError() ? 'invalid' : 'valid' : '' }}"/>
       <p id="contact-topic-error" class="error" ssr:if="form.Topic.HasError()">{{ form.Topic.GetError() }}</p>
     </div>
     <div class="field">
       <label for="contact-message">Message</label>
       <ssr:textarea name="message" required maxlength="2000" rows="6" id="contact-message"
                     class="{{ form.IsValidated() ? textarea.HasError() ? 'invalid' : 'valid' : '' }}"/>
       <p id="contact-message-error" class="error" ssr:if="form.Message.HasError()">{{ form.Message.GetError() }}</p>
     </div>
     <button type="submit">Send message</button>
   </ssr:form>
   ```

   The server checks `required`, the field's Go type (its `gotype`, `string` when left out), and
   that a select's value is one of its options. `maxlength`, `min`, `max` and `type="email"`
   only help the browser, so check them again in `Process`. Every form carries a token, and a
   post without a valid one is refused with 403; there is nothing to add for it.
2. Run `aicoded generate`, or save under `aicoded dev`. It writes `FormContactValues`, with one
   typed field per input, and adds `InitContact` and `ProcessContact` to the page's hooks. When
   `dataprovider.go` exists already, the Go compiler names the methods it lacks.
3. `Init<Form>` sets the starting values of the fields and the options of a select. It runs
   every time the page shows or receives the form, on a `GET` too, so it must not change data;
   that is the work of `Process<Form>`. The contact form's `Init` offers the topics:

   <!-- code: examples/people/pages/contact/dataprovider.go DP.InitContact -->
   ```go
   // InitContact offers the topics.
   func (p *DP) InitContact(_ context.Context, _ *web.Request, _ web.ResponseWriter, f *FormContactValues) error {
   	f.Topic.SetOptions([]form.SelectOptionElement[string]{
   		form.SelectOption[string]{Value: "General question", Label: "General question"},
   		form.SelectOption[string]{Value: "Technical support", Label: "Technical support"},
   		form.SelectOption[string]{Value: "Billing", Label: "Billing"},
   		form.SelectOption[string]{Value: "Feedback", Label: "Feedback"},
   	})
   	return nil
   }
   ```

4. `Process<Form>` runs only when every field passed the server's checks. Check what only your
   code knows: set an error on a field with `SetError` and return nil, and the page shows the
   form again with the viewer's values. When the form is done, return `web.Redirect`, so that
   reloading the page does not post it again:

   <!-- code: examples/people/pages/contact/dataprovider.go DP.ProcessContact -->
   ```go
   // ProcessContact mails the message to the team, then shows the banner. It stores and logs
   // nothing. The same viewer sending the same message again sends no second mail.
   func (p *DP) ProcessContact(ctx context.Context, _ *web.Request, _ web.ResponseWriter, f *FormContactValues) error {
   	name := strings.TrimSpace(f.Name.GetValue())
   	email := strings.TrimSpace(f.Email.GetValue())
   	message := strings.TrimSpace(f.Message.GetValue())
   	switch {
   	case name == "":
   		f.Name.SetError("Enter your name.")
   	case utf8.RuneCountInString(name) > 100:
   		f.Name.SetError("Use at most 100 characters.")
   	}
   	switch {
   	case !plainAddress(email):
   		f.Email.SetError("Enter an email address.")
   	case len(email) > 254:
   		f.Email.SetError("Use at most 254 characters.")
   	}
   	switch {
   	case message == "":
   		f.Message.SetError("Enter a message.")
   	case utf8.RuneCountInString(message) > 2000:
   		f.Message.SetError("Use at most 2000 characters.")
   	}
   	if f.HasError() {
   		return nil
   	}
   	topic := f.Topic.GetValue()
   	_, err := mailer.Send(ctx, mailer.Message{
   		IdempotencyKey: contactKey(auth.Viewer(ctx).Subject, topic, message),
   		To:             []mailer.Address{{Address: p.d.TeamAddress}},
   		Subject:        "Contact form: " + topic,
   		Text:           fmt.Sprintf("From: %s <%s>\nTopic: %s\n\n%s\n", name, email, topic, message),
   	})
   	if err != nil {
   		return err
   	}
   	return web.Redirect("/contact?sent=1")
   }
   ```

   The address goes into the mail's text, so it must be an address and nothing else:

   <!-- code: examples/people/pages/contact/dataprovider.go plainAddress -->
   ```go
   // plainAddress reports whether s is an email address and nothing else: no display name, angle
   // brackets or comment, which would reach the mail's text.
   func plainAddress(s string) bool {
   	a, err := mail.ParseAddress(s)
   	return err == nil && a.Name == "" && a.Address == s
   }
   ```

   It stores and logs nothing about the form, and its mail carries an idempotency key, which
   [Send mail after a write](email-after-a-write.md) explains.
5. `Data` reads the address the form redirected to:

   <!-- code: examples/people/pages/contact/dataprovider.go DP.Data -->
   ```go
   // Data shows the banner after a message was sent.
   func (p *DP) Data(_ context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
   	data.Sent = r.URL.Query().Get("sent") == "1"
   	return nil
   }
   ```

## Check it

- `aicoded check` generates, builds, vets, lints and tests the app.
- Open `http://people.localhost:8080/contact`, send the form, and read the message on the Mail
  page of the dev UI. Sending the same message again sends no second mail.
- The Logs and Traces pages of the dev UI show none of the values you sent.

## See also

- [Forms](../guides/forms.md)
- [Mail](../guides/mail.md)
- [The web API](../guides/web-api.md)
