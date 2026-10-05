package node_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
)

var reactive = map[string]bool{"counter": true, "name": true}

func v(name string) *node.Variable { return &node.Variable{Name: name} }

func TestCollectVarRefs(t *testing.T) {
	both := []string{"counter", "name"}
	for _, c := range []struct {
		n    node.Node
		want []string
	}{
		{v("counter"), []string{"counter"}},
		{v("other"), []string{}},
		{&node.String{Text: `"hello"`}, []string{}},
		{&node.Number{Text: "42"}, []string{}},
		{&node.Text{Text: "hello world"}, []string{}},
		{&node.HtmlRaw{Data: "<!DOCTYPE html>"}, []string{}},
		{&node.Operator{Op: "+", Left: v("counter"), Right: v("name")}, both},
		{&node.Operator{Op: "!", Right: v("counter")}, []string{"counter"}},
		{&node.Operator{Op: "+", Left: v("counter"), Right: v("counter")}, []string{"counter"}},
		{&node.Parentheses{Value: v("counter")}, []string{"counter"}},
		{&node.StructField{Expr: v("counter"), FieldName: "Value"}, []string{"counter"}},
		{&node.StructField{Expr: v("user"), FieldName: "Name"}, []string{}},
		{&node.Indexed{Expr: v("counter"), Index: v("name")}, both},
		{&node.Function{Expr: v("counter"), Arguments: &node.ExpressionsList{Values: []node.Node{v("name")}}}, both},
		{&node.TernaryIf{Cond: v("counter"), T: &node.String{Text: `"yes"`}, F: v("name")}, both},
		{&node.Loop{Array: v("counter"), Variable: "item", Children: []node.Node{v("name")}}, both},
		{&node.Loop{Array: v("counter"), Variable: "item", Children: []node.Node{&node.Text{Text: "static"}}}, []string{"counter"}},
		{&node.Loop{Array: v("nonReactive"), Variable: "item", Children: []node.Node{v("counter")}}, []string{"counter"}},
		{&node.ExpressionsList{Values: []node.Node{v("counter"), &node.Number{Text: "42"}, v("name")}}, both},
		{&node.Expression{Value: v("counter")}, []string{"counter"}},
		{&node.RawExpression{Value: v("name")}, []string{"name"}},
		{&node.Content{Children: []node.Node{v("counter"), &node.Text{Text: "static"}, v("name")}}, both},
		{&node.HtmlElement{
			TagName:    "div",
			Attributes: []node.HtmlAttribute{{Key: "class", Values: []node.Node{v("counter")}}},
			Children:   []node.Node{v("name")},
		}, both},
		{&node.SsrCondition{
			Conditions: []node.SsrConditionData{{Condition: v("counter"), Body: &node.Text{Text: "visible"}}},
			ElseBody:   v("name"),
		}, both},
		{&node.SsrJSON{Value: v("counter")}, []string{"counter"}},
		{&node.SsrContent{}, []string{}},
		{&node.SsrAssets{}, []string{}},
		{&node.SsrForm{}, []string{}},
		{&node.SsrInput{}, []string{}},
		{&node.SsrSelect{}, []string{}},
		{&node.SsrTextarea{}, []string{}},
	} {
		assert.ElementsMatch(t, c.want, c.n.CollectVarRefs(reactive), "%T", c.n)
		assert.NotNil(t, c.n.CollectVarRefs(reactive), "%T", c.n)
	}
}
