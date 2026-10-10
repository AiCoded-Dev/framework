# E-GATE-032: a form on a guarded page acted for another viewer

The simulated attacks (L7), which the delivery pipeline runs on the app it built, test each guarded page, one with `guard="true"` in its `<ssr:access>`, with two viewers. The first holds every role the app names and enters records through the app's forms. The second is another viewer, who holds only the roles that the access rules ask for up to the page that declares the `Guard`, such as `staff`, and so no role that may see everyone's records. The first viewer saw a form on a URL of the page, such as the form `withdraw` on `/trips/7`. The second viewer posted that form to that URL, with the first viewer's values and a form token of their own, and the page did not refuse it with 403 or 404: it answered with a redirect, as a form that worked does, or with anything else. So any viewer with those roles can change or delete other people's records. The problem is at the page's template, at line 1, and the message names the request, the form, the second viewer's roles, what the page answered and what was expected. A check in `Data` comes too late for a form: on a post, `Init` and `Process` of the forms run before `Data`, so the form has acted by the time `Data` refuses. A check in `Process` covers that one form only, and only once its fields are valid. The `Guard` runs before every form, `Data`, the live connection and every page call of the page and of the pages below it, so one check there covers them all. See [access](../guides/access.md) and [simulated attacks](../guides/simulated-attacks.md).

```go
func (p *DP) Data(ctx context.Context, r *web.Request, _ web.ResponseWriter, data *RouteData) error {
	trip, err := p.d.Trip(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	if trip.Owner != auth.Viewer(ctx).Subject {
		return web.Forbidden() // wrong: on a post, the form withdrew the trip before this runs
	}
	data.Trip = trip
	return nil
}

func (p *DP) Guard(ctx context.Context, r *web.Request) error { // right: runs before every form
	trip, err := p.d.Trip(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	if trip.Owner != auth.Viewer(ctx).Subject {
		return web.Forbidden()
	}
	return nil
}
```

**Fix:** check ownership in the page's Guard, which runs before every form, not in Data or Process.
