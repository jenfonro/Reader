package legado

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/buke/quickjs-go"
)

func (rc *runContext) evalJS(ctx context.Context, code string, input any) (any, error) {
	rt := quickjs.NewRuntime()
	defer rt.Close()
	rt.SetExecuteTimeout(5000)
	qctx := rt.NewContext()
	defer qctx.Close()

	state := &jsState{}
	if err := rc.bindJS(ctx, qctx, input, state); err != nil {
		return "", err
	}
	if strings.TrimSpace(rc.source.JSLib) != "" {
		val, err := jsEval(qctx, rc.source.JSLib)
		if val != nil {
			val.Free()
		}
		if err != nil {
			return "", err
		}
	}
	val, err := jsEval(qctx, code)
	if val != nil {
		defer val.Free()
	}
	if err != nil {
		return "", err
	}
	if state.verification != nil {
		return "", *state.verification
	}
	if val == nil || val.IsUndefined() || val.IsNull() {
		if state.content != nil {
			rc.result = state.content
			return state.content, nil
		}
		global := qctx.Globals()
		result := global.Get("result")
		defer result.Free()
		return jsValueToGo(qctx, result), nil
	}
	value := jsValueToGo(qctx, val)
	if state.content != nil {
		rc.result = state.content
	}
	return value, nil
}

func jsEval(ctx *quickjs.Context, code string) (*quickjs.Value, error) {
	val := ctx.Eval(code)
	if val == nil {
		if ctx.HasException() {
			return nil, ctx.Exception()
		}
		return nil, nil
	}
	if val.IsException() {
		err := ctx.Exception()
		val.Free()
		return nil, err
	}
	return val, nil
}

func jsValueToGo(ctx *quickjs.Context, value *quickjs.Value) any {
	if value == nil || value.IsUndefined() || value.IsNull() {
		return ""
	}
	if value.IsString() {
		return value.ToString()
	}
	var out any
	if err := ctx.Unmarshal(value, &out); err == nil {
		return out
	}
	jsonText := strings.TrimSpace(value.JSONStringify())
	if jsonText != "" && jsonText != "undefined" {
		if err := json.Unmarshal([]byte(jsonText), &out); err == nil {
			return out
		}
	}
	return value.ToString()
}

type jsState struct {
	verification *NeedVerificationError
	content      any
}

func (rc *runContext) bindJS(ctx context.Context, qctx *quickjs.Context, input any, state *jsState) error {
	globals := qctx.Globals()
	globals.Set("result", rc.jsInputValue(qctx, input, state))
	globals.Set("baseUrl", qctx.NewString(rc.baseURL))
	globals.Set("key", qctx.NewString(rc.key))
	globals.Set("page", qctx.NewInt32(int32(rc.page)))
	globals.Set("source", rc.jsSourceObject(qctx))
	globals.Set("book", rc.jsBookObject(qctx))
	globals.Set("java", rc.jsJavaObject(ctx, qctx, input, state))
	globals.Set("cookie", rc.jsCookieObject(qctx))
	globals.Set("cache", rc.jsCacheObject(qctx))
	return nil
}

func (rc *runContext) jsInputValue(qctx *quickjs.Context, input any, state *jsState) *quickjs.Value {
	switch v := input.(type) {
	case httpResult:
		return jsStrResponseObject(qctx, v)
	default:
		result, err := qctx.Marshal(jsSafeValue(input))
		if err != nil {
			result = qctx.NewString(anyString(input))
		}
		return result
	}
}

func jsSafeValue(input any) any {
	switch v := input.(type) {
	case htmlNode:
		return v.HTML
	case []htmlNode:
		items := make([]string, 0, len(v))
		for _, node := range v {
			items = append(items, node.HTML)
		}
		return items
	default:
		return v
	}
}

func (rc *runContext) jsSourceObject(qctx *quickjs.Context) *quickjs.Value {
	obj := qctx.NewObject()
	obj.Set("bookSourceUrl", qctx.NewString(rc.source.BookSourceURL))
	obj.Set("bookSourceName", qctx.NewString(rc.source.BookSourceName))
	obj.Set("bookSourceGroup", qctx.NewString(rc.source.BookSourceGroup))
	obj.Set("bookSourceType", qctx.NewInt64(rc.source.BookSourceType))
	obj.Set("bookUrlPattern", qctx.NewString(rc.source.BookURLPattern))
	obj.Set("customOrder", qctx.NewInt64(rc.source.CustomOrder))
	obj.Set("enabled", qctx.NewBool(rc.source.Enabled))
	obj.Set("enabledExplore", qctx.NewBool(rc.source.EnabledExplore))
	obj.Set("enabledCookieJar", qctx.NewBool(rc.source.EnabledCookieJar))
	obj.Set("concurrentRate", qctx.NewString(rc.source.ConcurrentRate))
	obj.Set("header", qctx.NewString(rc.source.Header))
	obj.Set("loginUrl", qctx.NewString(rc.source.LoginURL))
	obj.Set("loginUi", qctx.NewString(rc.source.LoginUI))
	obj.Set("loginCheckJs", qctx.NewString(rc.source.LoginCheckJS))
	obj.Set("coverDecodeJs", qctx.NewString(rc.source.CoverDecodeJS))
	obj.Set("bookSourceComment", qctx.NewString(rc.source.BookSourceComment))
	obj.Set("variableComment", qctx.NewString(rc.source.VariableComment))
	obj.Set("lastUpdateTime", qctx.NewInt64(rc.source.LastUpdateTime))
	obj.Set("respondTime", qctx.NewInt64(rc.source.RespondTime))
	obj.Set("weight", qctx.NewInt64(rc.source.Weight))
	obj.Set("exploreUrl", qctx.NewString(rc.source.ExploreURL))
	obj.Set("exploreScreen", qctx.NewString(rc.source.ExploreScreen))
	obj.Set("searchUrl", qctx.NewString(rc.source.SearchURL))
	obj.Set("ruleSearch", qctx.NewString(rc.source.RuleSearch))
	obj.Set("ruleExplore", qctx.NewString(rc.source.RuleExplore))
	obj.Set("ruleBookInfo", qctx.NewString(rc.source.RuleBookInfo))
	obj.Set("ruleToc", qctx.NewString(rc.source.RuleToc))
	obj.Set("ruleContent", qctx.NewString(rc.source.RuleContent))
	obj.Set("ruleReview", qctx.NewString(rc.source.RuleReview))
	obj.Set("getKey", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(rc.source.BookSourceURL)
	}))
	obj.Set("getVariable", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(rc.vars["source.variable"])
	}))
	obj.Set("setVariable", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) > 0 {
			rc.vars["source.variable"] = args[0].ToString()
		}
		return ctx.NewUndefined()
	}))
	obj.Set("getLoginJs", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(extractLoginJS(rc.source.LoginURL))
	}))
	obj.Set("login", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		loginJS := extractLoginJS(rc.source.LoginURL)
		if strings.TrimSpace(loginJS) != "" {
			if _, err := rc.evalJS(context.Background(), loginJS, rc.result); err != nil {
				return ctx.ThrowError(err)
			}
		}
		return ctx.NewUndefined()
	}))
	return obj
}

func (rc *runContext) jsBookObject(qctx *quickjs.Context) *quickjs.Value {
	obj := qctx.NewObject()
	obj.Set("origin", qctx.NewString(rc.origin()))
	obj.Set("bookUrl", qctx.NewString(rc.book.BookURL))
	obj.Set("tocUrl", qctx.NewString(rc.book.TocURL))
	obj.Set("name", qctx.NewString(rc.book.Name))
	obj.Set("author", qctx.NewString(rc.book.Author))
	obj.Set("kind", qctx.NewString(rc.book.Kind))
	obj.Set("latestChapterTitle", qctx.NewString(rc.book.LastChapter))
	obj.Set("lastChapter", qctx.NewString(rc.book.LastChapter))
	obj.Set("wordCount", qctx.NewString(rc.book.WordCount))
	obj.Set("coverUrl", qctx.NewString(rc.book.CoverURL))
	obj.Set("intro", qctx.NewString(rc.book.Intro))
	obj.Set("durChapterTitle", qctx.NewString(rc.chapter.Name))
	obj.Set("getVariable", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		key := "book.variable"
		if len(args) > 0 && strings.TrimSpace(args[0].ToString()) != "" {
			key += "." + args[0].ToString()
		}
		return ctx.NewString(rc.vars[key])
	}))
	obj.Set("setVariable", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) >= 2 {
			rc.vars["book.variable."+args[0].ToString()] = args[1].ToString()
		}
		return ctx.NewUndefined()
	}))
	return obj
}

func (rc *runContext) jsJavaObject(ctx context.Context, qctx *quickjs.Context, input any, state *jsState) *quickjs.Value {
	obj := qctx.NewObject()
	obj.Set("ajax", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return qctx.NewString("")
		}
		res, err := rc.doAjax(ctx, args[0].ToString())
		if err != nil {
			return qctx.ThrowError(err)
		}
		return qctx.NewString(res.Body)
	}))
	obj.Set("get", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return qctx.NewString("")
		}
		target := args[0].ToString()
		if len(args) == 1 && !looksLikeURL(target) {
			switch target {
			case "bookName":
				return qctx.NewString(rc.book.Name)
			case "title":
				return qctx.NewString(rc.chapter.Name)
			default:
				return qctx.NewString(rc.vars[target])
			}
		}
		headers := jsArgHeaders(qctx, args, 1)
		res, err := rc.engine.simpleRequest(ctx, rc, http.MethodGet, target, "", headers)
		if err != nil {
			return qctx.ThrowError(err)
		}
		return jsResponseObject(qctx, res)
	}))
	obj.Set("post", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return jsResponseObject(qctx, httpResult{})
		}
		target := args[0].ToString()
		body := ""
		if len(args) > 1 {
			body = args[1].ToString()
		}
		headers := jsArgHeaders(qctx, args, 2)
		res, err := rc.engine.simpleRequest(ctx, rc, http.MethodPost, target, body, headers)
		if err != nil {
			return qctx.ThrowError(err)
		}
		return jsResponseObject(qctx, res)
	}))
	obj.Set("put", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) >= 2 {
			rc.vars[args[0].ToString()] = args[1].ToString()
		}
		return qctx.NewUndefined()
	}))
	obj.Set("getString", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return qctx.NewString("")
		}
		value, err := rc.evalRule(ctx, args[0].ToString(), input)
		if err != nil {
			return qctx.ThrowError(err)
		}
		return qctx.NewString(anyString(value))
	}))
	obj.Set("getElement", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		rule := ""
		if len(args) > 0 {
			rule = args[0].ToString()
		}
		currentInput := input
		if state.content != nil {
			currentInput = state.content
		}
		value, err := rc.evalHTMLRule(rule, currentInput)
		if err != nil {
			return qctx.ThrowError(err)
		}
		return jsElementList(qctx, toHTMLNodes(value))
	}))
	obj.Set("setContent", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) > 0 {
			state.content = jsValueToGo(qctx, args[0])
			globals := qctx.Globals()
			globals.Set("result", rc.jsInputValue(qctx, state.content, state))
		}
		return qctx.NewUndefined()
	}))
	obj.Set("base64Decode", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString(base64DecodeString(firstArg(args)))
	}))
	obj.Set("base64Encode", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString(base64.StdEncoding.EncodeToString([]byte(firstArg(args))))
	}))
	obj.Set("hexDecodeToString", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		bytes, _ := hex.DecodeString(firstArg(args))
		return qctx.NewString(string(bytes))
	}))
	obj.Set("encodeURI", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString(url.QueryEscape(firstArg(args)))
	}))
	obj.Set("timeFormat", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString(formatJSTime(args, false))
	}))
	obj.Set("timeFormatUTC", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString(formatJSTime(args, true))
	}))
	obj.Set("androidId", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString("reader0000000001")
	}))
	obj.Set("t2s", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString(firstArg(args))
	}))
	obj.Set("getVerificationCode", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString("")
	}))
	obj.Set("startBrowserAwait", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		verification := NeedVerificationError{URL: firstArg(args)}
		if len(args) > 1 {
			verification.Message = args[1].ToString()
		}
		state.verification = &verification
		return jsResponseObject(qctx, httpResult{URL: verification.URL, Body: ""})
	}))
	for _, name := range []string{"toast", "longToast", "log"} {
		obj.Set(name, qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			return qctx.NewUndefined()
		}))
	}
	obj.Set("getCookie", qctx.NewFunction(func(qctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return qctx.NewString("")
	}))
	return obj
}

func (rc *runContext) doAjax(ctx context.Context, specText string) (httpResult, error) {
	spec, err := requestSpecFromText(ctx, rc, specText)
	if err != nil {
		return httpResult{}, err
	}
	return rc.engine.doRequest(ctx, spec)
}

func jsArgHeaders(qctx *quickjs.Context, args []*quickjs.Value, index int) map[string]string {
	if len(args) <= index || args[index] == nil || args[index].IsUndefined() || args[index].IsNull() {
		return map[string]string{}
	}
	var raw map[string]any
	if err := qctx.Unmarshal(args[index], &raw); err != nil {
		return map[string]string{}
	}
	headers := make(map[string]string, len(raw))
	for key, value := range raw {
		headers[key] = anyString(value)
	}
	return headers
}

func jsStrResponseObject(qctx *quickjs.Context, res httpResult) *quickjs.Value {
	obj := qctx.NewObject()
	body := qctx.NewString(res.Body)
	obj.Set("body", body)
	obj.Set("url", qctx.NewString(res.URL))
	obj.Set("statusCode", qctx.NewInt32(int32(res.StatusCode)))
	obj.Set("code", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewInt32(int32(res.StatusCode))
	}))
	obj.Set("bodyString", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(res.Body)
	}))
	obj.Set("header", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return ctx.NewString("")
		}
		return ctx.NewString(res.Headers.Get(args[0].ToString()))
	}))
	obj.Set("urlString", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(res.URL)
	}))
	obj.Set("toString", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(res.Body)
	}))
	return obj
}

func jsResponseObject(qctx *quickjs.Context, res httpResult) *quickjs.Value {
	obj := qctx.NewObject()
	obj.Set("url", qctx.NewString(res.URL))
	obj.Set("statusCode", qctx.NewInt32(int32(res.StatusCode)))
	obj.Set("body", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(res.Body)
	}))
	obj.Set("header", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return ctx.NewString("")
		}
		return ctx.NewString(res.Headers.Get(args[0].ToString()))
	}))
	obj.Set("cookie", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString("")
	}))
	obj.Set("toString", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(res.Body)
	}))
	return obj
}

func (rc *runContext) jsCookieObject(qctx *quickjs.Context) *quickjs.Value {
	obj := qctx.NewObject()
	obj.Set("getCookie", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString("")
	}))
	obj.Set("setCookie", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewUndefined()
	}))
	obj.Set("removeCookie", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewUndefined()
	}))
	return obj
}

func (rc *runContext) jsCacheObject(qctx *quickjs.Context) *quickjs.Value {
	obj := qctx.NewObject()
	obj.Set("get", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString("")
	}))
	obj.Set("put", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewUndefined()
	}))
	return obj
}

func jsElementList(qctx *quickjs.Context, nodes []htmlNode) *quickjs.Value {
	obj := qctx.NewObject()
	obj.Set("length", qctx.NewInt32(int32(len(nodes))))
	obj.Set("toArray", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return jsNodeArray(ctx, nodes)
	}))
	obj.Set("text", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		parts := make([]string, 0, len(nodes))
		for _, node := range nodes {
			parts = append(parts, nodeText(node))
		}
		return ctx.NewString(strings.Join(parts, "\n"))
	}))
	obj.Set("html", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		parts := make([]string, 0, len(nodes))
		for _, node := range nodes {
			parts = append(parts, node.HTML)
		}
		return ctx.NewString(strings.Join(parts, "\n"))
	}))
	obj.Set("toString", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		parts := make([]string, 0, len(nodes))
		for _, node := range nodes {
			parts = append(parts, node.HTML)
		}
		return ctx.NewString(strings.Join(parts, ""))
	}))
	return obj
}

func jsNodeArray(qctx *quickjs.Context, nodes []htmlNode) *quickjs.Value {
	arr := qctx.Eval("[]")
	for index, node := range nodes {
		arr.SetIdx(int64(index), jsNodeObject(qctx, node))
	}
	return arr
}

func jsNodeObject(qctx *quickjs.Context, node htmlNode) *quickjs.Value {
	obj := qctx.NewObject()
	obj.Set("text", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(nodeText(node))
	}))
	obj.Set("attr", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		if len(args) == 0 {
			return ctx.NewString("")
		}
		return ctx.NewString(nodeAttr(node, args[0].ToString()))
	}))
	obj.Set("html", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(node.HTML)
	}))
	obj.Set("toString", qctx.NewFunction(func(ctx *quickjs.Context, this *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
		return ctx.NewString(node.HTML)
	}))
	return obj
}

func toHTMLNodes(value any) []htmlNode {
	switch v := value.(type) {
	case []htmlNode:
		return v
	case htmlNode:
		return []htmlNode{v}
	case []any:
		out := make([]htmlNode, 0, len(v))
		for _, item := range v {
			out = append(out, toHTMLNodes(item)...)
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []htmlNode{{HTML: v}}
	default:
		return []htmlNode{{HTML: anyString(v)}}
	}
}

func nodeText(node htmlNode) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(node.HTML))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(doc.Selection.Text())
}

func nodeAttr(node htmlNode, name string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(node.HTML))
	if err != nil {
		return ""
	}
	value, _ := doc.Selection.Children().First().Attr(name)
	return value
}

func parseJSObject(ctx context.Context, text string) (map[string]any, error) {
	_ = ctx
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return map[string]any{}, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed, nil
	}
	rt := quickjs.NewRuntime()
	defer rt.Close()
	qctx := rt.NewContext()
	defer qctx.Close()
	val, err := jsEval(qctx, "JSON.stringify(("+trimmed+"))")
	if val != nil {
		defer val.Free()
	}
	if err != nil {
		return nil, err
	}
	if val == nil {
		return map[string]any{}, nil
	}
	if err := json.Unmarshal([]byte(val.ToString()), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (rc *runContext) regexReplace(ctx context.Context, input string, pattern string, replace string) (string, error) {
	_ = ctx
	rt := quickjs.NewRuntime()
	defer rt.Close()
	qctx := rt.NewContext()
	defer qctx.Close()
	code := fmt.Sprintf(`String(%s).replace(new RegExp(%s, "g"), %s)`, jsQuote(input), jsQuote(pattern), jsQuote(replace))
	val, err := jsEval(qctx, code)
	if val != nil {
		defer val.Free()
	}
	if err != nil {
		return "", err
	}
	if val == nil {
		return "", nil
	}
	return val.ToString(), nil
}

func jsQuote(value string) string {
	content, _ := json.Marshal(value)
	return string(content)
}

func firstArg(args []*quickjs.Value) string {
	if len(args) == 0 || args[0] == nil {
		return ""
	}
	return args[0].ToString()
}

func base64DecodeString(raw string) string {
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		content, err := encoding.DecodeString(raw)
		if err == nil {
			return string(content)
		}
	}
	return ""
}

func looksLikeURL(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "/") || strings.Contains(value, ",{")
}

func formatJSTime(args []*quickjs.Value, utc bool) string {
	if len(args) == 0 {
		return ""
	}
	millis, _ := strconv.ParseInt(args[0].ToString(), 10, 64)
	pattern := "yyyy-MM-dd"
	if len(args) > 1 && strings.TrimSpace(args[1].ToString()) != "" {
		pattern = args[1].ToString()
	}
	loc := time.Local
	if utc {
		loc = time.UTC
		if len(args) > 2 {
			if offset, err := strconv.Atoi(args[2].ToString()); err == nil {
				loc = time.FixedZone("", offset*3600)
			}
		}
	}
	return time.UnixMilli(millis).In(loc).Format(javaTimeLayout(pattern))
}

func javaTimeLayout(pattern string) string {
	layout := pattern
	replacer := strings.NewReplacer(
		"yyyy", "2006",
		"MM", "01",
		"dd", "02",
		"HH", "15",
		"mm", "04",
		"ss", "05",
	)
	return replacer.Replace(layout)
}

func extractLoginJS(raw string) string {
	login := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(login, "@js:"):
		return strings.TrimSpace(strings.TrimPrefix(login, "@js:"))
	case strings.HasPrefix(login, "<js>") && strings.Contains(login, "</js>"):
		return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(login, "<js>"), "</js>"))
	default:
		return login
	}
}

func unsupported(name string) error {
	return errors.New(name + " unsupported")
}
