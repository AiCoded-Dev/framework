# E-DEV-004: the app stopped

The app the message names exited, or did not report ready within 30 seconds of starting.
`aicoded dev` stops only that app: its address shows this error, and the other apps keep
running. An app whose `OnStart` calls an app that is not running fails this way too. `aicoded dev`
starts the app again when its files change, or when you press Start or Restart on the app's page
of the dev UI.

**Fix:** read the app's log lines printed above this error. A Go `main` must end in
`app.Main(app.Options{...})`, which performs the runner handshake and reports ready.
