package legado

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
)

type requestSpec struct {
	URL            string
	Method         string
	Body           string
	Charset        string
	Type           string
	WebJS          string
	SourceRegex    string
	Headers        map[string]string
	UseCookieJar   bool
	Timeout        time.Duration
	Retry          int
	WebView        bool
	SourceKey      string
	ConcurrentRate string
}

type httpResult struct {
	URL        string
	Body       string
	Headers    http.Header
	StatusCode int
}

type Engine struct {
	client *http.Client
}

type concurrentRecord struct {
	concurrent bool
	time       time.Time
	frequency  int
}

var concurrentRecords = struct {
	sync.Mutex
	items map[string]*concurrentRecord
}{items: map[string]*concurrentRecord{}}

func NewEngine() *Engine {
	jar, _ := cookiejar.New(nil)
	return &Engine{
		client: &http.Client{
			Timeout: 20 * time.Second,
			Jar:     jar,
		},
	}
}

func (e *Engine) clientOrDefault() *http.Client {
	if e != nil && e.client != nil {
		return e.client
	}
	return NewEngine().client
}

func buildRequestSpec(ctx context.Context, rc *runContext, raw string) (requestSpec, error) {
	if strings.TrimSpace(raw) == "" {
		return requestSpec{}, errors.New("request url empty")
	}
	return requestSpecFromText(ctx, rc, raw)
}

func (rc *runContext) evalURLSpec(ctx context.Context, raw string) (string, error) {
	rule := strings.TrimSpace(raw)
	if rule == "" {
		return "", nil
	}
	if before, code, after, ok := splitJSBlock(rule); ok {
		if strings.TrimSpace(code) != "" {
			value, err := rc.evalJS(ctx, code, rc.result)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(after) == "" && strings.TrimSpace(before) == "" && strings.TrimSpace(anyString(value)) != "" {
				return rc.replaceURLPageChoices(anyString(value)), nil
			}
		}
		processed, err := rc.replaceTemplates(ctx, before+after, rc.result)
		if err != nil {
			return "", err
		}
		return rc.replaceURLPageChoices(processed), nil
	}
	if before, code, ok := splitAtJS(rule); ok {
		value, err := rc.evalJS(ctx, code, rc.result)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(before) == "" {
			return rc.replaceURLPageChoices(anyString(value)), nil
		}
		processed, err := rc.replaceTemplates(ctx, before+anyString(value), rc.result)
		if err != nil {
			return "", err
		}
		return rc.replaceURLPageChoices(processed), nil
	}
	processed, err := rc.replaceTemplates(ctx, rule, rc.result)
	if err != nil {
		return "", err
	}
	return rc.replaceURLPageChoices(processed), nil
}

func (rc *runContext) replaceURLPageChoices(rule string) string {
	if rc == nil || rc.page <= 0 || !strings.Contains(rule, "<") || !strings.Contains(rule, ">") {
		return rule
	}
	var out strings.Builder
	for {
		start := strings.Index(rule, "<")
		if start < 0 {
			out.WriteString(rule)
			return out.String()
		}
		end := strings.Index(rule[start+1:], ">")
		if end < 0 {
			out.WriteString(rule)
			return out.String()
		}
		end += start + 1
		inside := rule[start+1 : end]
		if inside == "" || strings.ContainsAny(inside, "<>{}") {
			out.WriteString(rule[:end+1])
			rule = rule[end+1:]
			continue
		}
		choices := strings.Split(inside, ",")
		choiceIndex := rc.page - 1
		if choiceIndex < 0 {
			choiceIndex = 0
		}
		if choiceIndex >= len(choices) {
			choiceIndex = len(choices) - 1
		}
		out.WriteString(rule[:start])
		out.WriteString(strings.TrimSpace(choices[choiceIndex]))
		rule = rule[end+1:]
	}
}

func (rc *runContext) sourceHeaders(ctx context.Context) (map[string]string, error) {
	headers := map[string]string{}
	raw := strings.TrimSpace(rc.source.Header)
	if raw == "" {
		return headers, nil
	}
	value, err := rc.evalRule(ctx, raw, rc.result)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(anyString(value))
	if text == "" {
		return headers, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		parsed, err = parseJSObject(ctx, text)
		if err != nil {
			return nil, err
		}
	}
	for key, value := range parsed {
		headers[key] = anyString(value)
	}
	return headers, nil
}

func (rc *runContext) getRequestSpec(ctx context.Context, target string) (requestSpec, error) {
	return requestSpecFromText(ctx, rc, target)
}

func (rc *runContext) requestTimeout() time.Duration {
	if rc == nil || rc.source.RespondTime <= 0 {
		return 20 * time.Second
	}
	return time.Duration(rc.source.RespondTime) * time.Millisecond
}

func (e *Engine) doRequest(ctx context.Context, spec requestSpec) (httpResult, error) {
	release, err := enterConcurrentRate(ctx, spec.SourceKey, spec.ConcurrentRate)
	if err != nil {
		return httpResult{}, err
	}
	defer release()
	if spec.WebView {
		return httpResult{}, NeedVerificationError{URL: spec.URL, Message: "webView source requires user verification"}
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	attempts := spec.Retry + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		res, err := e.doRequestOnce(ctx, spec)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return httpResult{}, ctx.Err()
		}
	}
	return httpResult{}, lastErr
}

func (e *Engine) doRequestOnce(ctx context.Context, spec requestSpec) (httpResult, error) {
	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if spec.Body != "" {
		body = strings.NewReader(spec.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, spec.URL, body)
	if err != nil {
		return httpResult{}, err
	}
	for key, value := range spec.Headers {
		if strings.TrimSpace(key) != "" {
			req.Header.Set(key, value)
		}
	}
	if spec.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Reader/1.0")
	}

	client := e.clientOrDefault()
	if !spec.UseCookieJar {
		cloned := *client
		cloned.Jar = nil
		client = &cloned
	}
	resp, err := client.Do(req)
	if err != nil {
		return httpResult{}, err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return httpResult{}, err
	}
	text := string(content)
	if spec.Type != "" {
		text = hex.EncodeToString(content)
	} else if strings.Contains(spec.Charset, "gbk") || strings.Contains(spec.Charset, "gb2312") {
		decoded, decodeErr := simplifiedchinese.GBK.NewDecoder().String(text)
		if decodeErr == nil {
			text = decoded
		}
	}
	return httpResult{URL: resp.Request.URL.String(), Body: text, Headers: resp.Header.Clone(), StatusCode: resp.StatusCode}, nil
}

func (e *Engine) simpleRequest(ctx context.Context, rc *runContext, method string, target string, bodyText string, headers map[string]string) (httpResult, error) {
	spec := requestSpec{
		URL:          normalizeRequestURL(rc.resolveURL(target)),
		Method:       method,
		Body:         bodyText,
		Headers:      headers,
		UseCookieJar: rc.source.EnabledCookieJar,
		Timeout:      rc.requestTimeout(),
	}
	return e.doRequest(ctx, spec)
}

func splitRequestOptions(spec string) (string, string) {
	depth := 0
	quote := rune(0)
	escaped := false
	for index, char := range spec {
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if char == '\\' {
				escaped = true
				continue
			}
			if char == quote {
				quote = 0
			}
			continue
		}
		switch char {
		case '\'', '"', '`':
			quote = char
		case '{', '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				right := strings.TrimSpace(spec[index+1:])
				if strings.HasPrefix(right, "{") {
					return spec[:index], right
				}
			}
		}
	}
	return spec, ""
}

func normalizeRequestURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	if parsed.RawQuery != "" {
		parsed.RawQuery = parsed.Query().Encode()
	}
	return parsed.String()
}

func isAbsURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func joinURL(base string, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "//") {
		return "https:" + ref
	}
	if isAbsURL(ref) {
		return ref
	}
	parsedBase, err := url.Parse(base)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return ref
	}
	parsedRef, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return parsedBase.ResolveReference(parsedRef).String()
}

func requestSpecFromText(ctx context.Context, rc *runContext, specText string) (requestSpec, error) {
	return requestSpecFromTextWithContent(ctx, rc, specText, "", "")
}

func requestSpecFromTextWithContent(ctx context.Context, rc *runContext, specText string, contentWebJS string, sourceRegex string) (requestSpec, error) {
	specText = strings.TrimSpace(specText)
	if specText == "" {
		return requestSpec{}, errors.New("request url empty")
	}
	specText, err := rc.evalURLSpec(ctx, specText)
	if err != nil {
		return requestSpec{}, err
	}
	urlText, optionText := splitRequestOptions(strings.TrimSpace(specText))
	urlText = strings.TrimSpace(urlText)
	options := map[string]any{}
	if optionText != "" {
		parsed, err := parseJSObject(ctx, optionText)
		if err != nil {
			return requestSpec{}, err
		}
		options = parsed
	}
	headers, err := rc.sourceHeaders(ctx)
	if err != nil {
		return requestSpec{}, err
	}
	if optionHeaders, ok := anyMap(options["headers"]); ok {
		for key, value := range optionHeaders {
			headers[key] = anyString(value)
		}
	}
	method := strings.ToUpper(strings.TrimSpace(anyString(options["method"])))
	body := anyString(options["body"])
	charset := strings.ToLower(strings.TrimSpace(anyString(options["charset"])))
	retry := anyInt(options["retry"])
	requestType := strings.TrimSpace(anyString(options["type"]))
	webView := anyTruthy(options["webView"])
	webJS := strings.TrimSpace(anyString(options["webJs"]))
	if webJS == "" {
		webJS = strings.TrimSpace(contentWebJS)
	}
	if method == "" {
		if body != "" {
			method = http.MethodPost
		} else {
			method = http.MethodGet
		}
	}
	requestURL := normalizeRequestURL(rc.resolveURL(urlText))
	if jsRule := strings.TrimSpace(anyString(options["js"])); jsRule != "" {
		value, err := rc.evalJS(ctx, jsRule, requestURL)
		if err != nil {
			return requestSpec{}, err
		}
		if urlValue := strings.TrimSpace(anyString(value)); urlValue != "" {
			requestURL = normalizeRequestURL(rc.resolveURL(urlValue))
		}
	}
	return requestSpec{
		URL:            requestURL,
		Method:         method,
		Body:           body,
		Charset:        charset,
		Type:           requestType,
		WebJS:          webJS,
		SourceRegex:    strings.TrimSpace(sourceRegex),
		Headers:        headers,
		UseCookieJar:   rc.source.EnabledCookieJar,
		Timeout:        rc.requestTimeout(),
		Retry:          retry,
		WebView:        webView,
		SourceKey:      rc.source.BookSourceURL,
		ConcurrentRate: rc.source.ConcurrentRate,
	}, nil
}

func enterConcurrentRate(ctx context.Context, sourceKey string, rawRate string) (func(), error) {
	rate := strings.TrimSpace(rawRate)
	if sourceKey == "" || rate == "" {
		return func() {}, nil
	}
	wait, concurrent := reserveConcurrentSlot(sourceKey, rate)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			releaseConcurrentSlot(sourceKey, concurrent)
			return func() {}, ctx.Err()
		case <-timer.C:
		}
	}
	return func() { releaseConcurrentSlot(sourceKey, concurrent) }, nil
}

func reserveConcurrentSlot(sourceKey string, rate string) (time.Duration, bool) {
	now := time.Now()
	concurrentRecords.Lock()
	defer concurrentRecords.Unlock()
	record := concurrentRecords.items[sourceKey]
	rateIndex := strings.Index(rate, "/")
	if record == nil {
		record = &concurrentRecord{concurrent: rateIndex > 0, time: now, frequency: 1}
		concurrentRecords.items[sourceKey] = record
		return 0, record.concurrent
	}
	if rateIndex < 0 {
		interval, err := strconv.Atoi(rate)
		if err != nil || interval <= 0 {
			return 0, false
		}
		if record.frequency > 0 {
			record.frequency++
			return time.Duration(interval) * time.Millisecond, false
		}
		next := record.time.Add(time.Duration(interval) * time.Millisecond)
		if !now.Before(next) {
			record.time = now
			record.frequency = 1
			return 0, false
		}
		record.frequency = 1
		return time.Until(next), false
	}
	limit, err1 := strconv.Atoi(strings.TrimSpace(rate[:rateIndex]))
	window, err2 := strconv.Atoi(strings.TrimSpace(rate[rateIndex+1:]))
	if err1 != nil || err2 != nil || limit <= 0 || window <= 0 {
		return 0, true
	}
	next := record.time.Add(time.Duration(window) * time.Millisecond)
	if !now.Before(next) {
		record.time = now
		record.frequency = 1
		return 0, true
	}
	record.frequency++
	if record.frequency > limit {
		return time.Until(next), true
	}
	return 0, true
}

func releaseConcurrentSlot(sourceKey string, concurrent bool) {
	if sourceKey == "" || concurrent {
		return
	}
	concurrentRecords.Lock()
	defer concurrentRecords.Unlock()
	if record := concurrentRecords.items[sourceKey]; record != nil && record.frequency > 0 {
		record.frequency--
	}
}

func anyInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func anyTruthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s != "" && s != "false" && s != "0" && s != "null"
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	default:
		return value != nil
	}
}
