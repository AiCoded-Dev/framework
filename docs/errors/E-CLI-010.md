# E-CLI-010: the platform is older than this aicoded

`aicoded publish` sends with every publish whether it asks for the release at once (`--release`)
or is held for its owner's preview, the default. The platform refused that: it is older than this
`aicoded` and does not hold publishes, so it would have released a publish that you meant to try
first. Nothing was published. The platform is updated by its operator.

**Fix:** use the aicoded version the platform's docs name, or wait for the platform update
