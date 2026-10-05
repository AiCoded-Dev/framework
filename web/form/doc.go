// Package form holds the typed fields of a page's forms. For <ssr:form name="add"> in a
// template, aicoded generate writes FormAddValues: a [BaseFormValues] and one field for each
// <ssr:input>, <ssr:select> and <ssr:textarea> in the form, such as Input[string], Select[int]
// or File. Generated code reads the posted values into the fields; the page's data provider sets
// initial values and options and reads the results in two hooks:
//
//   - InitAdd runs every time the page shows the form, and on a post after the form's token is
//     checked and before the posted values are read. Set default values with SetValue and the
//     options of every select with SetOptions.
//   - ProcessAdd runs only for the posted form, and only when every field is valid: required
//     fields are filled, numbers parse and each select value is one of its enabled options.
//     Check there what only the server knows, such as whether a login is taken. Set an error on
//     a field with its SetError, or on the whole form with [BaseFormValues.SetError], and return
//     nil to show the form again with the errors; return web.Redirect to show the result.
//
// A posted form is at most 32 MiB. A form with a file field is posted as multipart/form-data;
// read an uploaded file with the Open method of its [FileHeader].
//
// Read more in the guide docs/guides/forms.md and the task docs/tasks/add-form.md, which aicoded
// explain and the MCP tool howto print as guides/forms and tasks/add-form.
package form
