# E-PUB-005: another builder's app

The first publish of an app's name in your organisation creates the app, owned by the builder who published it, and only its owner can publish it again, see its publishes, or read its logs and traces. An app of this name exists in your organisation and is not yours, or `aicoded status`, `aicoded logs` or `aicoded traces` asked about such an app. Before `aicoded` says so, it refreshes your sign-in once, which brings the apps you own now, such as one that an administrator has just transferred to you. The message quotes the platform.

`aicoded preview` and the `preview_publish` and `preview_end` tools need the scope `app:<id>:preview` of your own app, which a sign-in made before previews existed lacks: the platform then answers "the app belongs to another builder, or your sign-in does not carry it yet", and running `aicoded login` again brings the scope.

**Fix:** publish under a name of your own: change `app:` in `aicoded.yaml`, or ask your administrator to transfer the app to you. To read its logs, ask its owner, or your administrator to transfer it to you. To preview your own app, run `aicoded login` again.
