# E-GEN-011: file input needs multipart

A form with an `<ssr:input type="file">` sets an `enctype` other than `multipart/form-data`.
Browsers send files only in multipart forms, so the file would never arrive. A form with a file
input and no `enctype` gets `multipart/form-data`.

**Fix:** remove `enctype` or set it to `multipart/form-data`
