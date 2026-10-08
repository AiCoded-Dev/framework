# E-GATE-024: the app reaches a capability without a building block

This is a note: it does not stop the publish. capslock, which the delivery pipeline runs over the app's code, follows the calls from each function of the app and found one that reaches a capability, such as the network, files, other programs or the system's state, through the standard library or a third-party module rather than through a building block of the framework. Apps may not reach the network, files, programs or the system directly: the building blocks are how the runner holds an app to its permission list, such as `sqldb` for the database, `filestore` for files, `mailer` for mail, `secrets` for secrets and `rpc` for other apps. A capability reached around them escapes those limits, so security sees each one. The note is at the line of the app's function, and the message names the capability and the call it goes through. The change record counts the note.

```go
cfg, err := ini.Load("rates.ini") // a note: FILES, through a module that reads the disk itself

data, err := store.ReadFile("rates.ini") // no note: the file comes through a filestore.Store
```

**Fix:** reach it only through a building block: apps may not reach the network, files, programs or the system directly.
