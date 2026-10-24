# E-RUN-010: runner too old or too new for the platform

A server's runner and the platform agree on one version of the protocol between them before the
platform sends the runner anything. This runner speaks another version than the platform, as when
the runner was not updated with the platform. The platform ended the connection: it sent the runner
no apps and no access to their resources. The apps the runner serves already keep serving.

**Fix:** update aicoded-runner on the server to the version that matches the platform
