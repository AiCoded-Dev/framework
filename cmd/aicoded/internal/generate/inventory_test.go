package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	noParams = []ParamInfo{}
	noCalls  = []string{}
	noForms  = []FormInfo{}
)

func TestInventoryAccess(t *testing.T) {
	rs, err := discover(newApp(t, map[string]string{
		"pages/index.html":                        `<ssr:access role="staff"/><ssr:content/>`,
		"pages/trips/report/index.html":           `<p>report</p>`,
		"pages/trips/n_id/index.html":             `<ssr:access role="staff" guard="true"/><p>trip</p>`,
		"pages/trips/n_id/close/index.html":       `<ssr:access role="ops"/><p>close</p>`,
		"pages/users/s_login/index.html":          `<ssr:access role="staff" shared="true"/><ssr:content default="info"/>`,
		"pages/users/s_login/info/index.html":     `<p>info</p>`,
		"pages/users/s_login/contacts/index.html": `<ssr:access role="staff" guard="true"/><p>contacts</p>`,
	}), noImages)
	require.NoError(t, err)
	staff := []string{"staff"}
	id := []ParamInfo{{Name: "id", Kind: "number"}}
	login := []ParamInfo{{Name: "login", Kind: "string"}}
	assert.Equal(t, []RouteInfo{
		{Path: "/", Template: "pages/index.html", Layout: true, Params: noParams, Require: staff, Calls: noCalls, Forms: noForms},
		{Path: "/trips/report", Template: "pages/trips/report/index.html", Params: noParams, Require: staff, Calls: noCalls, Forms: noForms},
		{Path: "/trips/{id}", Template: "pages/trips/n_id/index.html", Params: id, Require: staff,
			Guard: true, GuardAt: "/trips/{id}", Calls: noCalls, Forms: noForms},
		{Path: "/trips/{id}/close", Template: "pages/trips/n_id/close/index.html", Params: id, Require: []string{"staff", "ops"},
			Guard: true, GuardAt: "/trips/{id}", Calls: noCalls, Forms: noForms},
		{Path: "/users/{login}", Template: "pages/users/s_login/index.html", Layout: true, Params: login, Require: staff,
			Shared: true, Calls: noCalls, Forms: noForms},
		{Path: "/users/{login}/contacts", Template: "pages/users/s_login/contacts/index.html", Params: login, Require: staff,
			Guard: true, GuardAt: "/users/{login}/contacts", Calls: noCalls, Forms: noForms},
		{Path: "/users/{login}/info", Template: "pages/users/s_login/info/index.html", Params: login, Require: staff,
			Shared: true, Calls: noCalls, Forms: noForms},
	}, inventory(rs), "sorted by pattern; a Guard above counts; a page with its own Guard under a shared layout is not shared")
}

func TestInventoryLiveAndForms(t *testing.T) {
	rs, err := discover(newApp(t, map[string]string{
		"pages/index.html":             `<ssr:access role="*"/><ssr:content/>`,
		"pages/board/index.html":       `<ssr:var name="n" type="int" reactive="true"/><p>{{ n }}</p><ssr:content/>`,
		"pages/board/notes/index.html": `<p>notes</p>`,
		"pages/chat/index.html":        `<ssr:call name="send" in="string" out="int"/><p>chat</p>`,
		"pages/frame/index.html":       `<p>frame</p><ssr:content/>`,
		"pages/add/index.html": `<ssr:form name="add">
<ssr:input name="title" required/><ssr:input name="age" type="number" gotype="uint8"/><ssr:textarea name="body"/>
<ssr:select name="tags" multiple/><ssr:input name="photo" type="file"/>
</ssr:form><ssr:form name="remove"></ssr:form>`,
	}), noImages)
	require.NoError(t, err)
	all := []string{"*"}
	assert.Equal(t, []RouteInfo{
		{Path: "/", Template: "pages/index.html", Layout: true, Params: noParams, Require: all, Calls: noCalls, Forms: noForms},
		{Path: "/add", Template: "pages/add/index.html", Params: noParams, Require: all, Calls: noCalls, Forms: []FormInfo{
			{Name: "add", Fields: []FieldInfo{
				{Name: "title", Kind: "input", GoType: "string", Required: true},
				{Name: "age", Kind: "input", GoType: "uint8"},
				{Name: "body", Kind: "textarea", GoType: "string"},
				{Name: "tags", Kind: "select", GoType: "string", Multiple: true},
				{Name: "photo", Kind: "file", GoType: "string"},
			}},
			{Name: "remove", Fields: []FieldInfo{}},
		}},
		{Path: "/board", Template: "pages/board/index.html", Layout: true, Params: noParams, Require: all, Live: true, Calls: noCalls, Forms: noForms},
		{Path: "/board/notes", Template: "pages/board/notes/index.html", Params: noParams, Require: all, Live: true, Calls: noCalls, Forms: noForms},
		{Path: "/chat", Template: "pages/chat/index.html", Params: noParams, Require: all, Live: true, Calls: []string{"send"}, Forms: noForms},
		{Path: "/frame", Template: "pages/frame/index.html", Params: noParams, Require: all, Calls: noCalls, Forms: noForms},
	}, inventory(rs), "a page under a live layout opens a live connection too; a layout with no pages below is a page")
}
