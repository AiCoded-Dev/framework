# E-GATE-033: ownership was not checked: the attacks reached no record of a guarded page

The simulated attacks (L7), which the delivery pipeline runs on the app it built, check each guarded page, one with `guard="true"` in its `<ssr:access>`, at the URLs that their first viewer reached. That viewer holds every role the app names, starts at `/` and at every page without an id in its URL, follows links and redirects, and posts the forms it finds with valid values, the forms on pages without an id first. It reached no URL of this page, so the attacks could not check that another viewer is kept out of its records, and the publish fails: a `Guard` that nothing checked could let anyone in. A page is reached when a page that the attacks reach links to it, or redirects to it, as a form does with `web.Redirect` once it has created a record. The app's database is empty when the attacks start, so the records must come from a form on a page without an id, and the guarded page must be linked from a page its viewers reach, such as a list of the records. A page below a `Guard` that asks for more roles than the page that declares it, and a page with `shared="true"`, are not checked for ownership and never get this problem. A page the attacks did not reach also gets the note E-GATE-038. See [simulated attacks](../guides/simulated-attacks.md).

```html
<!-- pages/trips/new/index.html: a form whose Process creates the trip and redirects to it -->
<ssr:form name="add">…</ssr:form>
<!-- pages/trips/index.html: a link to each trip -->
<li ssr:for="t in trips"><a href="/trips/{{ t.ID }}">{{ t.Destination }}</a></li>
```

**Fix:** let a form on a page without an id create the record, and link to the guarded page from a page its viewers reach.
