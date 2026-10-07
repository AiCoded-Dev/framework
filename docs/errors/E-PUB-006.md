# E-PUB-006: the app is archived

An administrator archived the app of this name in your organisation, so it takes no publish, and its name stays taken. A sign-in made or refreshed after the archive no longer carries the app, so the platform says so when `aicoded publish` asks for the app and when `aicoded status` asks for one of its publishes. The message quotes the platform.

**Fix:** publish under a new name: change `app:` in `aicoded.yaml`, or ask your administrator about the archived app.
