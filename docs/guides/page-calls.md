# Page calls: functions a page's script calls

A page call lets the page's script ask its server for something without a reload or a separate
API. The page declares the call with typed arguments and result, the data provider answers it,
and the access rules and Guards of the page run again before every call.

## Declaring a call

```html
<!-- pages/notes/n_id/index.html -->
<ssr:access role="*" guard="true"/>
<ssr:var name="note" type="Note"/>
<ssr:call name="star" in="StarIn" out="StarOut"/>
<h1>{{ note.Title }}</h1>
<button id="star" type="button">Star</button>
<p id="star-result"></p>
```

- `name` is a lower-case letter followed by letters and digits, such as `star` or `setTitle`,
  once per template.
- `in` and `out` are Go types the page's package can name, as for `<ssr:var>`. `struct{}` stands
  for no arguments or no result. A `web` type, a func or a chan cannot be sent as JSON and is
  refused (E-GEN-045, E-GEN-040).
- The tag cannot be inside `ssr:if`, `ssr:for` or `<ssr:form>`: the page can always make the
  call (E-GEN-040).

The types travel as JSON, with the names `encoding/json` gives them, so give their fields `json`
tags:

```go
// pages/notes/n_id/types.go

// StarIn is what the page sends to star a note.
type StarIn struct {
	Starred bool `json:"starred"`
}

// StarOut is what the page gets back.
type StarOut struct {
	Starred bool   `json:"starred"`
	Title   string `json:"title"`
}
```

## Answering it

`aicoded generate` adds `Call<Name>` to the page's data provider. Its stub refuses every call
with 403 until you write it:

```go
// CallStar stars or unstars the note. The page's Guard has checked the owner for this call.
func (p *DP) CallStar(ctx context.Context, r *web.Request, in StarIn) (StarOut, error) {
	n, err := p.d.Note(ctx, r.URLParamInt("id"))
	if err != nil {
		return StarOut{}, err
	}
	if err := p.d.Star(ctx, n.ID, in.Starred); err != nil {
		return StarOut{}, err
	}
	return StarOut{Starred: in.Starred, Title: n.Title}, nil
}
```

- Before every call, the access rules and `Guard`s of every page on the path run again, so a
  `Guard` sees a change at once, such as a note given to someone else. The roles are those the
  viewer had when the page's live connection opened; a change of roles takes effect when it
  connects again, at the latest after 10 minutes.
- `Data` does not run for a call.
- The arguments must match `in` exactly: an unknown field or anything after the JSON answers
  400 "The call has the wrong arguments.". They still come from the browser, so check their
  values in `Call<Name>`.
- `r.URLParam` and `r.URLParamInt` read the page's parameters, as in its other hooks.

## Calling it from the script

`index.ts` next to the template imports the page's generated `reactive_gen.ts`:

```ts
import { ssr, type CallError } from "./reactive_gen";

const out = document.getElementById("star-result");
document.getElementById("star")?.addEventListener("click", () => {
  ssr
    .call("star", { starred: true })
    .then((r) => {
      if (out) out.textContent = `Starred ${r.title}.`;
    })
    .catch((e: CallError) => {
      if (out) out.textContent = e.message;
    });
});
```

`ssr.call` returns a promise of the result, typed from `StarOut`, and rejects with a
`CallError`, `{status, message}`:

| The call | `status` | `message` |
|---|---|---|
| returns `web.Error(status, message)`, `web.Forbidden()` or `web.NotFound()` | its status | its message |
| fails a page's access rule or `Guard` | 403, or what the `Guard` returned | the error's message |
| names a call that the open page does not have | 404 | "This call is not on this page." |
| takes longer than 30 seconds | 504 | "The call took too long." |
| has wrong arguments | 400 | "The call has the wrong arguments." |
| is the fifth at once of this page | 429 | "Too many calls at once." |
| returns `web.Redirect` (E-WEB-005), another error, or panics | 500 | "Something went wrong.", and the error is logged |
| was sent when the connection dropped | 0 | "The connection was lost." |

- A call made before the live connection opens waits for it. A call in flight when the
  connection drops fails with status 0 and is not sent again.
- To move to another page after a call, return the path in the result and call
  `location.assign(path)` in the script.

## In the permission list

`aicoded generate` lists each page's calls, and those of the layouts it is shown in, under
`calls` in the page's entry of the `access` section of
[the permission list](permission-list.md):

```yaml
access:
  "/notes/{id}":
    require: ["*"]
    guard: true
    calls: ["star"]
```

## See also

- [Live values](live-values.md): the live connection that carries calls.
- [TypeScript API](typescript-api.md): the `ssr` object.
- [Access](access.md): the checks that run before every call.
