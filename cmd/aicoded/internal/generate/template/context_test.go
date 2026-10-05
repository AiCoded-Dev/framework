package template

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"aicoded.dev/framework/cmd/aicoded/internal/generate/gobuf"
	"aicoded.dev/framework/cmd/aicoded/internal/generate/template/node"
	"aicoded.dev/framework/internal/errs"
)

func code(t *testing.T, src string) string {
	t.Helper()
	tpl, err := parse(t, src)
	require.NoError(t, err, src)
	b := gobuf.New()
	tpl.WriteGoCode(b)
	b.WriteConsts()
	return b.String()
}

func TestEscaperByContext(t *testing.T) {
	for src, want := range map[string]string{
		`<p>{{ x }}</p>`:                           "render.Text(w, x)",
		`<title>{{ x }}</title>`:                   "render.Text(w, x)",
		`<textarea>{{ x }}</textarea>`:             "render.Text(w, x)",
		`<p class="c {{ x }}"></p>`:                "render.Attr(w, x)",
		`<a href="{{ x }}"></a>`:                   "render.URL(w, x)",
		`<a href=" {{ x }} "></a>`:                 "render.URL(w, x)",
		`<img src="{{ x }}">`:                      "render.URL(w, x)",
		`<button formaction="{{ x }}"></button>`:   "render.URL(w, x)",
		`<a data-href="{{ x }}"></a>`:              "render.URL(w, x)",
		`<div data-thumb-url="{{ x }}"></div>`:     "render.URL(w, x)",
		`<svg><a xlink:href="{{ x }}"></a></svg>`:  "render.URL(w, x)",
		`<a href="/n/{{ x }}"></a>`:                "render.URLPart(w, x)",
		`<a href="n/{{ x }}"></a>`:                 "render.URLPart(w, x)",
		`<a href="mailto:{{ x }}"></a>`:            "render.URLPart(w, x)",
		`<a href="https://h.example/{{ x }}"></a>`: "render.URLPart(w, x)",
		`<a href="tel:{{ x }}"></a>`:               "render.URLPart(w, x)",
		`<a href="/find?q={{ x }}"></a>`:           "render.URLQuery(w, x)",
		`<a href="#{{ x }}"></a>`:                  "render.URLQuery(w, x)",
		`<p>{{$ x }}</p>`:                          "render.HTML(w, x)",
		`<ssr:json name="page-data" value="x"/>`:   "render.JSON(w, x)",
	} {
		assert.Contains(t, code(t, src), want, src)
	}
	out := code(t, `<a href="/{{ x }}/{{ y }}"></a>`)
	assert.Contains(t, out, "render.URLPathStart(w, x)")
	assert.Contains(t, out, "render.URLPart(w, y)")
	for _, src := range []string{`<a href="/{{ x }}"></a>`, `<a href=" /{{ x }}"></a>`, "<a href=\"/\n{{ x }}\"></a>", `<img src="/{{ x }}">`} {
		assert.Contains(t, code(t, src), "render.URLPathStart(w, x)", src)
	}
	out = code(t, `<a href="/find?q={{ x }}&p={{ y }}"></a>`)
	assert.Contains(t, out, "render.URLQuery(w, x)")
	assert.Contains(t, out, "render.URLQuery(w, y)")
	assert.Contains(t, code(t, `<ssr:json name="page-data" value="x"/>`), `<script type=\"application/json\" id=\"page-data\">`)
	assert.Contains(t, code(t, `<a title="&quot;&lt;">x</a>`), `&#34;&lt;`)
}

func TestForbiddenContexts(t *testing.T) {
	for src, want := range map[string]string{
		`<script>alert(1)</script>`:                     "E-GEN-020",
		`<script type="application/json">{}</script>`:   "E-GEN-020",
		`<style>p{}</style>`:                            "E-GEN-021",
		`<svg><style></style></svg>`:                    "E-GEN-021",
		`<a onclick="x()"></a>`:                         "E-GEN-022",
		`<a ONMOUSEOVER="x"></a>`:                       "E-GEN-022",
		`<ssr:form name="a" onsubmit="x()"></ssr:form>`: "E-GEN-022",
		`<p style="color:red"></p>`:                     "E-GEN-023",
		`<ssr:form name="a"><ssr:textarea name="t" style="height:9em"/></ssr:form>`: "E-GEN-023",
		`<a class={{x}}></a>`:                    "E-GEN-024",
		`<a {{x}}="1"></a>`:                      "E-GEN-025",
		`<div{{x}}></div>`:                       "E-GEN-025",
		`<noscript>{{ x }}</noscript>`:           "E-GEN-026",
		`<iframe>{{ x }}</iframe>`:               "E-GEN-026",
		`<xmp>{{ x }}</xmp>`:                     "E-GEN-026",
		`<script src="{{ x }}"></script>`:        "E-GEN-027",
		`<meta content="{{ x }}">`:               "E-GEN-027",
		`<base href="{{ x }}">`:                  "E-GEN-027",
		`<img srcset="{{ x }}">`:                 "E-GEN-027",
		`<iframe srcdoc="{{ x }}"></iframe>`:     "E-GEN-027",
		`<svg><animate values="{{ x }}"/></svg>`: "E-GEN-027",
		`<a href="javascript:alert(1)"></a>`:     "E-GEN-028",
		`<a href=" JavaScript:x"></a>`:           "E-GEN-028",
		`<a href="java&#x09;script:x"></a>`:      "E-GEN-028",
		`<a href="vbscript:x"></a>`:              "E-GEN-028",
		`<a href="javascript:{{ x }}"></a>`:      "E-GEN-028",
		`<a title="{{$ x }}"></a>`:               "E-GEN-029",
		`<title>{{$ x }}</title>`:                "E-GEN-029",
		`<ssr:json name="Bad Name" value="x"/>`:  "E-GEN-033",
		`<ssr:json name="a"/>`:                   "E-GEN-033",
		`<a href="java{{ x }}"></a>`:             "E-GEN-038",
		`<a href="{{ a }}{{ b }}"></a>`:          "E-GEN-038",
		`<a href="{{ base }}/x"></a>`:            "E-GEN-038",
		`<a href="{{ p }}://{{ h }}"></a>`:       "E-GEN-038",
		`<a class={{ x }}></a>`:                  "E-GEN-024",
		`<svg><a><animate attributeName="href" values="javascript:alert(1)"/></a></svg>`: "E-GEN-028",
		`<svg><set attributeName="href" to="/a; JavaScript:x"/></svg>`:                   "E-GEN-028",
		`<ssr:json name="a" value="x" id="b"/>`:                                          "E-GEN-033",
		`<ssr:json name="a" value="{{ x }}"/>`:                                           "E-GEN-033",
		`<ssr:json name="a" value="x"/><p><ssr:json name="a" value="y"/></p>`:            "E-GEN-033",
		`<iframe srcdoc="<p>hi</p>"></iframe>`:                                           "E-GEN-020",
		`<iframe SRCDOC=""></iframe>`:                                                    "E-GEN-020",
		`<link rel="stylesheet" href="{{ x }}">`:                                         "E-GEN-027",
		`<link rel="{{ x }}" href="/a.css">`:                                             "E-GEN-027",
	} {
		_, err := parse(t, src)
		assert.Equal(t, want, errs.Code(err), src)
	}
	for _, ok := range []string{`<script src="/a.js"></script>`, `<script> </script>`, `<a href="/x">x</a>`,
		`<a href="https://x.example">x</a>`, `<a href="tel:+1">x</a>`, `<p data-on="1"></p>`,
		`<img src="data:image/png;base64,AAAA">`, `<svg><animate attributeName="x" values="0;10"/></svg>`,
		`<ssr:json name="a" value="x"/><ssr:json name="b" value="x"/>`, `<iframe src="/x"></iframe>`, `<link rel="stylesheet" href="/a.css">`} {
		_, err := parse(t, ok)
		assert.NoError(t, err, ok)
	}
}

func TestParserAgreesWithBrowsers(t *testing.T) {
	for _, src := range []string{
		`<script src="/a.js"/><p>{{ x }}</p>`,
		`<title/>{{ x }}`,
		`<noscript/><img src=x onerror=alert(1)>`,
		`<svg><noscript><img src=x onerror=alert(1)></noscript></svg>`,
		`<xmp><b>x</b></xmp>`,
		`<a href="/x" href="{{ y }}"></a>`,
		"<a b\x00c=\"1\"></a>",
	} {
		_, err := parse(t, src)
		assert.Equal(t, "E-GEN-001", errs.Code(err), src)
	}
	for _, ok := range []string{`<noscript>Turn on JavaScript &amp; reload</noscript>`, `<svg><title>{{ x }}</title></svg>`} {
		_, err := parse(t, ok)
		assert.NoError(t, err, ok)
	}
}

func TestRCDATA(t *testing.T) {
	for src, want := range map[string]bool{
		`<title>{{ x }}</title>`:       true,
		`<textarea>{{ x }}</textarea>`: true,
		`<p>{{ x }}</p>`:               false,
	} {
		tpl, err := parse(t, src)
		require.NoError(t, err, src)
		el := tpl.nodes[0].(*node.HtmlElement)
		assert.Equal(t, want, el.Children[0].(*node.Expression).RCDATA, src)
	}
}

func TestReservedNames(t *testing.T) {
	for _, src := range []string{
		`<p>{{ w }}</p>`,
		`<p>{{ s.dp.X(w) }}</p>`,
		`<p>{{ x.WriteTo(w) }}</p>`,
		`<p>{{$ render.X }}</p>`,
		`<p title="{{ _html0 }}"></p>`,
		`<p ssr:if="io.EOF == nil"></p>`,
		`<p ssr:for="x in strings.Fields(y)"></p>`,
		`<p ssr:for="render in xs">{{ render }}</p>`,
		`<p ssr:for="i, w in xs"></p>`,
		`<p ssr:for="form, x in xs"></p>`,
		`<ssr:json name="a" value="json.Marshal"/>`,
		`<p>{{ form.X }}</p>`,
		`<ssr:form name="a"></ssr:form><p>{{ form.X }}</p>`,
		`<p>{{ f(context, http, web, reactive, sync) }}</p>`,
		`<p>{{ slog.Info("x") }}</p>`,
		`<p ssr:if="errors.Is(e, x)"></p>`,
		`<ssr:var name="errors" type="[]string"/>`,
		`<ssr:var name="w" type="int"/>`,
		`<ssr:var name="_html1" type="int"/>`,
		`<ssr:var name="a-b" type="int"/>`,
		`<ssr:var name="func" type="int"/>`,
		`<p>{{ routeKey }}</p>`,
		`<p>{{ assets }}</p>`,
		`<p>{{ assetsDir }}</p>`,
		`<p>{{ NewHandler() }}</p>`,
		`<p>{{ NewRoute(nil).Access() }}</p>`,
		`<p ssr:if="state == x"></p>`,
		`<p ssr:for="route in xs"></p>`,
		`<p>{{ RouteData }}</p>`,
		`<p>{{ f(RouteDataProvider, ReactiveState) }}</p>`,
		`<p>{{ renderBlock_x(nil) }}</p>`,
		`<p>{{ FormAddValues }}</p>`,
		`<ssr:var name="state" type="int"/>`,
		`<ssr:var name="v" type="*state"/>`,
		`<ssr:var name="v" type="[]route"/>`,
		`<ssr:var name="v" type="map[string]FormAddValues"/>`,
		`<ssr:call name="a" in="RouteData" out="B"/>`,
		`<ssr:call name="a" in="A" out="*ReactiveState"/>`,
	} {
		_, err := parse(t, "\n"+src)
		var e *errs.Error
		if assert.ErrorAs(t, err, &e, src) {
			assert.Equal(t, "E-GEN-042", e.Code, src)
			assert.Equal(t, "pages/x/index.html:2", e.Pos, src)
		}
	}
	for _, ok := range []string{
		`<ssr:form name="a" data-x="{{ form.ID() }}"><p>{{ form.ID() }}</p><ssr:input name="n" placeholder="{{ form.ID() }}"/></ssr:form>`,
		`<p ssr:for="item in items">{{ item.w }} {{ x.render }}</p>`,
		`<ssr:var name="note" type="Note"/><p>{{ width }} {{ html0 }} {{ _htm }}</p>`,
		`<ssr:var name="v" type="FormatValues"/><p>{{ routes }} {{ x.state }} {{ Forms }} {{ FormValues }} {{ renderBlock }}</p>`,
	} {
		_, err := parse(t, ok)
		assert.NoError(t, err, ok)
	}
}
