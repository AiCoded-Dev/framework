# E-RUN-011: secret has no value where the app runs

The app asked for a secret that its permission list declares, and the platform holds no value for
it where the app runs. Rather than hand the app an empty secret, the runner refuses the call:
nothing was read. The values of an app's secrets are set in the console by the app's owner or a
security admin, never by the app or its builder.

**Fix:** ask the app's owner or a security admin to set the secret's value in the console, then call again; the app need not be published or released again
