# E-RUN-011: secret has no value where the app runs

The app asked for a secret that its permission list declares, and the platform holds no value for
it where the app runs. Rather than hand the app an empty secret, the runner refuses the call:
nothing was read. The values of an app's secrets are set by the platform's operator, never by the
app or its builder.

**Fix:** ask the platform's operator to set the secret's value for the app, then call again; the app need not be published or released again
