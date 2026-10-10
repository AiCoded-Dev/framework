# E-GATE-031: another viewer saw a record through a guarded page

The simulated attacks (L7), which the delivery pipeline runs on the app it built, test each guarded page, one with `guard="true"` in its `<ssr:access>`, with two viewers. The first holds every role the app names and enters records through the app's forms. The second is another viewer, who holds only the roles that the access rules ask for up to the page that declares the `Guard`, such as `staff`, and so no role that may see everyone's records. The second viewer opened a URL of the page that the first had reached, such as `/trips/7`, and the page answered 200 and showed a value the first had entered. So any viewer with those roles can read other people's records by changing the id in the URL. The problem is at the page's template, at line 1, and the message names the request and the second viewer's roles. The page's `Guard` decides who sees each record: it runs after the access rules and before the forms, `Data` and the render, so it loads the record the URL names and refuses a viewer who may not see it. A `Guard` that also lets in a role that may see every record, such as `travel-desk` below, passes, since the second viewer does not hold that role. See [access](../guides/access.md) and [simulated attacks](../guides/simulated-attacks.md).

```go
func (p *DP) Guard(ctx context.Context, r *web.Request) error {
	return nil // wrong: every viewer the rule admits sees every trip
}

func (p *DP) Guard(ctx context.Context, r *web.Request) error { // right
	viewer := auth.Viewer(ctx)
	trip, err := p.d.Trip(ctx, r.URLParamInt("id"))
	if err != nil {
		return err
	}
	if trip.Owner != viewer.Subject && !viewer.HasRole("travel-desk") {
		return web.Forbidden()
	}
	return nil
}
```

**Fix:** in the page's Guard, load the record from the id in the URL and return web.Forbidden() unless the viewer owns it or holds a role that may see every record.
