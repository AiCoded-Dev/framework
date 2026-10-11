# E-DEV-021: persona name in dev.yaml is empty or too long

A persona in `dev.yaml` has no name, an empty one, or one longer than 512 bytes. A persona's name
is the viewer's `Subject` while you use it, and a `Subject` is 1 to 512 bytes, so the framework
would refuse every request of such a persona. Every workspace's personas are checked. The message
names the line and never quotes the name.

**Fix:** give the persona a name of 1 to 512 bytes
