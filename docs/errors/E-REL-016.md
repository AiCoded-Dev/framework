# E-REL-016: app still released

The platform's operator asked to rename or archive an app that has a release live, or pending on
its server. The app's host, its entry at the hosting's gateway and the instance its server runs are
made under the name it was released with, and the platform cannot stop a release yet, so an app with
a release live or pending keeps its name and stays active until releases can be stopped. The message
names the release. Nothing changed.

**Fix:** keep the app's name and leave it active; to publish under another name, publish the app as a new app with aicoded publish
