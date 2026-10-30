# E-PUB-005: another builder's app

The first publish of an app's name in your organisation creates the app, owned by the builder who published it, and only its owner can publish it again, see its publishes, read its logs and traces, or run its preview. An app of this name exists in your organisation and is not yours, or `aicoded status`, `aicoded logs`, `aicoded traces` or `aicoded preview` asked about such an app. Before `aicoded` says so, it refreshes your sign-in once, which brings the apps you own now, such as one that an administrator has just transferred to you. The message quotes the platform.

A preview needs the scope `app:<id>:preview` of your own app. A sign-in made before previews existed lacks it, and the refresh brings it, so you do not sign in again for it; a sign-in whose refresh the platform refuses has ended, which is E-CLI-004.

**Fix:** publish under a name of your own: change `app:` in `aicoded.yaml`, or ask your administrator to transfer the app to you. To read its logs, ask its owner, or your administrator to transfer it to you.
