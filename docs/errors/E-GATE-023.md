# E-GATE-023: a module's licence is forbidden or not recognised

This is a note: it does not stop the publish. go-licenses, which the delivery pipeline runs over the modules the app uses, read the licence of a module and classed it as forbidden, such as a licence that restricts how a company may use or share the code, or could not recognise it, as when the module has no licence file or an unusual one. Your company may not be allowed to use such a module in its apps. The note is at the module's line in `go.mod`, and the message names the licence and its class. The list of every module's licence and the list of the app's modules (its SBOM) are kept with the change record, which counts the note.

```text
require example.com/charts v1.4.0   // licence: AGPL-3.0, forbidden
```

**Fix:** check with your administrator whether the app may use this module.
