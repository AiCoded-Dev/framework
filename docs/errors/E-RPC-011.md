# E-RPC-011: called app not running

The app called a function of an app that is not running in this workspace. `aicoded dev` runs every app below the folder it starts in and carries calls only between those apps once each has started. It starts each app after the apps it calls, so an app may call them in `OnStart`. Of apps that call each other, directly or through other apps, the one whose folder comes first starts first, before an app it calls. An app that failed to build or start, that exited, or that you stopped on the dev UI, is not running until it starts again. An app that runs on its own, outside `aicoded dev`, cannot call at all.

**Fix:** run `aicoded dev` in a folder above both apps, and never call, in `OnStart`, an app that calls this app too.
