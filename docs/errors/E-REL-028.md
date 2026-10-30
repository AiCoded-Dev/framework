# E-REL-028: the operator cannot set another organisation's secret

The platform's operator asked `admin secrets put` or `admin secrets delete` for an app of another
organisation. The operator sets and deletes only the secrets of its own organisation's apps: the
values of an organisation's apps are set by the app's owner or a security admin of that
organisation, in the console. Nothing was set or deleted. `admin secrets list` still shows the
names of any organisation's secrets, never a value.

```
admin secrets put <slug> <app> <name>   # refused: the app is of another organisation
```

**Fix:** the app's owner or a security admin sets it in the console
