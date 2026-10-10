# Add a page

Add a page to an app: a folder under `pages/` with a template that declares its values, an
access rule on its path, and a data provider that fills the values. The example is the info tab
of a user's card in the people example, at `/users/<login>/info`.

## Steps

1. Make the page's folder. Its path under `pages/` is the page's address. A folder named
   `s_<name>` captures one part of the address as a string, and `n_<name>` a whole number, so
   `pages/users/s_login/info/` is `/users/<login>/info`, such as `/users/alice/info`. Each folder
   is a Go package: name it with ASCII letters, digits and `_`, starting with a letter, and not
   `main`, `testdata` or a Go keyword (E-GEN-043).
2. Write the template, `index.html`. Declare every value it shows with `<ssr:var>` and a Go type,
   and use the values in `{{ }}`, `ssr:if` and `ssr:for`:

   <!-- code: examples/people/pages/users/s_login/info/index.html -->
   ```html
   <ssr:var name="user" type="User"/>
   <ssr:var name="lastSeen" type="string" reactive="true"/>
   <h2>Age group:
     <span id="age-group" ssr:if="user.Age <= 18">0-18</span>
     <span id="age-group" ssr:else-if="user.Age <= 30">19-30</span>
     <!-- the older groups -->
     <span id="age-group" ssr:else-if="user.Age <= 60">31-60</span>
     <span id="age-group" ssr:else>60+</span>
   </h2>
   <dl>
     <dt>Team</dt>
     <dd id="team">{{ user.Team }}</dd>
     <dt>Languages</dt>
     <dd id="languages">{{ user.Languages == '' ? 'none given' : user.Languages }}</dd>
     <dt>Works</dt>
     <dd id="works">
       <span ssr:if="user.OnSite && user.Remote">on site and remotely</span>
       <span ssr:else-if="user.Remote">remotely</span>
       <span ssr:else>on site</span>
     </dd>
     <dt>Shift</dt>
     <dd id="shift">{{ user.Shift == 2 ? 'night' : 'day' }}</dd>
   </dl>
   <p id="bio">{{ user.Bio }}</p>
   <p class="muted">Last opened a page: <span id="last-seen">{{ lastSeen }}</span></p>
   ```

   Every page needs an access rule on its path, in its own template or in a parent's. This page
   has none of its own: `pages/users/index.html` holds `<ssr:access role="staff"/>`, which covers
   every page below `/users`. A page outside such a folder starts with its own rule, such as
   `<ssr:access role="staff"/>`, or `<ssr:access role="*"/>` for every viewer. A page with an id
   in its URL also says whose records it shows, with `guard="true"` or `shared="true"`
   (E-GEN-054): this one inherits `shared="true"` from the card above it,
   `pages/users/s_login/index.html`, since every member of `staff` sees every profile.
3. Declare the types the template names, such as `User`, in a `.go` file next to it:

   <!-- code: examples/people/pages/users/s_login/info/types.go -->
   ```go
   package info

   import "people/deps"

   // User is the user whose details this page shows.
   type User = deps.User
   ```

4. Run `aicoded generate`; under `aicoded dev`, saving the file is enough. It writes
   `route_gen.go`, with the `RouteData` struct of the page's values, and a `dataprovider.go`
   whose `Data` fills nothing yet. `dataprovider.go` is yours: `aicoded generate` writes it only
   when it is missing. Fill `Data`:

   <!-- code: examples/people/pages/users/s_login/info/dataprovider.go DP.Data -->
   ```go
   // Data shows the user's details and when they last opened a page.
   func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
   	u, err := p.d.User(ctx, r.URLParam("login"))
   	if errors.Is(err, deps.ErrNoUser) {
   		return web.NotFound()
   	}
   	if err != nil {
   		return err
   	}
   	data.User = u
   	data.LastSeen = seen(p.d.Seen.At(u.Login))
   	return nil
   }
   ```

   <!-- code: examples/people/pages/users/s_login/info/dataprovider.go seen -->
   ```go
   // seen tells when a user last opened a page: at, when ok.
   func seen(at time.Time, ok bool) string {
   	if !ok {
   		return "not since the app started"
   	}
   	return at.UTC().Format("2006-01-02 15:04:05 UTC")
   }
   ```

   `r.URLParam("login")` reads the folder's parameter, and returning `web.NotFound()` answers
   404.
5. Link to the page. It shows inside the layouts above it, where they hold `<ssr:content/>`. The
   card, `pages/users/s_login/index.html`, links to the tab and holds
   `<ssr:content default="info"/>`, so `/users/alice` opens it.

The page's `lastSeen` is a live value, which [Add a live value](add-live-value.md) explains.

## Check it

- `aicoded check` generates, builds, vets, lints and tests the app.
- `aicoded generate` records the page in the `access` section of `aicoded.yaml` as
  `"/users/{login}/info"` with `require: ["staff"]` and `shared: true`, and `aicoded describe`
  lists it.
- Open `http://people.localhost:8080/users/alice/info` as a persona with the `staff` role. A
  persona without it gets 403, and a login that no user has gets 404.

## See also

- [Routing](../guides/routing.md)
- [Template syntax](../guides/template-syntax.md)
- [Access rules](../guides/access.md)
- [The web API](../guides/web-api.md)
