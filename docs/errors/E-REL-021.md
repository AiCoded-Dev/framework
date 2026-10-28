# E-REL-021: data not allowed by the source registry

Your security admin set the class of one of the app's data sources, in the organisation's source registry, to `health`, `hr` or `card`: health, HR or payment card data. No app may hold such data, whatever its app type and whatever `data` in the permission list says, so the release is *Not allowed*: the publish fails, with this problem in its step `decision`, and a release is refused the same way. The message names the source. See [app types](../guides/app-types.md).

**Fix:** remove the source and the data it holds from the app, or ask your security admin whether the registry's class of the source is right
