# E-RUN-014: framework too old for egress

The app was built with a framework that speaks runner protocol 3, which has no way to call an
outside service, and its permission list (`aicoded.yaml`) declares `egress`. The runner refuses
the handshake, so the app does not start: an app calls the hosts in `egress` only with
`outbound.Client()`, which needs runner protocol 4, from framework v0.10.0.

**Fix:** update aicoded.dev/framework to v0.10.0 or newer
