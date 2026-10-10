# E-GATE-038: a page the attacks did not reach

This is a note: it does not stop the publish. The simulated attacks (L7), which the delivery pipeline runs on the app it built, reach pages as a viewer does: as a viewer who holds every role the app names, they start at `/` and at every page without an id in its URL, follow links and redirects up to six steps, and post the forms they find. They never reached this page. They still sent it requests without a session and without a role, at a URL made from its path, such as `/trips/1`, but they could not use its forms, look for markup in what it shows, or, on a guarded page, check that another viewer is kept out; a guarded page they did not reach is the problem E-GATE-033 too. A page that only a mail, a script, a URL typed by hand, or a form whose checks refuse the attacks' values leads to is not reached. The note is at the page's template, at line 1, and the change record lists every page the attacks did not reach, so security sees it. See [simulated attacks](../guides/simulated-attacks.md).

```html
<!-- pages/trips/index.html: a link the attacks follow to /trips/archive -->
<a href="/trips/archive">Archived trips</a>
```

**Fix:** link to the page from a page its viewers reach, so the attacks can check it.
