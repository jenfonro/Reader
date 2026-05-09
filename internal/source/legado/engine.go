package legado

import (
	"context"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"
)

func (e *Engine) Search(ctx context.Context, source Source, options SearchOptions) (SearchResult, error) {
	if strings.TrimSpace(source.SearchURL) == "" {
		return SearchResult{Source: source.Summary()}, nil
	}
	if _, err := requireRuleObject(source.RuleSearch, "搜索"); err != nil {
		return SearchResult{}, err
	}
	rc := newRunContext(e, source)
	rc.key = options.Keyword
	rc.page = options.Page
	if rc.page <= 0 {
		rc.page = 1
	}
	spec, err := buildRequestSpec(ctx, rc, source.SearchURL)
	if err != nil {
		return SearchResult{}, err
	}
	res, err := e.doRequest(ctx, spec)
	if err != nil {
		return SearchResult{}, err
	}
	res, err = rc.applyLoginCheck(ctx, res)
	if err != nil {
		return SearchResult{}, err
	}
	rc.setResponse(res)
	books, err := rc.analyzeBookList(ctx, true, res.URL)
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Source: source.Summary(), Books: books}, nil
}

func (e *Engine) Explore(ctx context.Context, source Source, options ExploreOptions) (ExploreResult, error) {
	if !source.EnabledExplore {
		return ExploreResult{Source: source.Summary()}, nil
	}
	requestRule := strings.TrimSpace(options.URL)
	if requestRule == "" {
		requestRule = firstExploreURL(source.ExploreURL)
	}
	if requestRule == "" {
		return ExploreResult{Source: source.Summary(), URL: requestRule}, nil
	}
	rc := newRunContext(e, source)
	rc.page = options.Page
	if rc.page <= 0 {
		rc.page = 1
	}
	spec, err := buildRequestSpec(ctx, rc, requestRule)
	if err != nil {
		return ExploreResult{}, err
	}
	res, err := e.doRequest(ctx, spec)
	if err != nil {
		return ExploreResult{}, err
	}
	res, err = rc.applyLoginCheck(ctx, res)
	if err != nil {
		return ExploreResult{}, err
	}
	rc.setResponse(res)
	books, err := rc.analyzeBookList(ctx, false, res.URL)
	if err != nil {
		return ExploreResult{}, err
	}
	return ExploreResult{Source: source.Summary(), URL: spec.URL, Books: books}, nil
}

func (e *Engine) BookInfo(ctx context.Context, source Source, options BookInfoOptions) (Book, error) {
	if strings.TrimSpace(options.BookURL) == "" {
		return Book{}, errors.New("book url empty")
	}
	rules, err := requireRuleObject(source.RuleBookInfo, "详情")
	if err != nil {
		return Book{}, err
	}
	rc := newRunContext(e, source)
	bookURL := rc.resolveURL(options.BookURL)
	spec, err := rc.getRequestSpec(ctx, bookURL)
	if err != nil {
		return Book{}, err
	}
	res, err := e.doRequest(ctx, spec)
	if err != nil {
		return Book{}, err
	}
	res, err = rc.applyLoginCheck(ctx, res)
	if err != nil {
		return Book{}, err
	}
	rc.setResponse(res)
	pageValue := rc.result
	if initRule := strings.TrimSpace(rules["init"]); initRule != "" {
		pageValue, err = rc.evalRule(ctx, initRule, rc.result)
		if err != nil {
			return Book{}, err
		}
		rc.result = pageValue
	}
	book := options.Book
	book.BookURL = bookURL
	book.SourceURL = source.BookSourceURL
	book.SourceName = source.BookSourceName
	if err := rc.applyBookInfoRules(ctx, rules, pageValue, &book, bookURL, res.Body, options.CanReName); err != nil {
		return Book{}, err
	}
	return book, nil
}

func (e *Engine) Toc(ctx context.Context, source Source, options TocOptions) (TocResult, error) {
	tocURL := strings.TrimSpace(options.TocURL)
	if tocURL == "" {
		tocURL = strings.TrimSpace(options.Book.TocURL)
	}
	if tocURL == "" {
		return TocResult{}, errors.New("toc url empty")
	}
	rules, err := requireRuleObject(source.RuleToc, "目录")
	if err != nil {
		return TocResult{}, err
	}
	rc := newRunContext(e, source)
	rc.book = options.Book
	if strings.TrimSpace(options.Book.BookURL) != "" {
		rc.baseURL = options.Book.BookURL
	}
	if preUpdateJS := strings.TrimSpace(rules["preUpdateJs"]); preUpdateJS != "" {
		if _, err := rc.evalJS(ctx, preUpdateJS, rc.result); err != nil {
			return TocResult{}, err
		}
	}
	listRule, reverse := trimLeadingRuleFlag(rules["chapterList"])
	chapters := make([]Chapter, 0)
	seen := map[string]bool{}
	queue := []string{rc.resolveURL(tocURL)}
	firstURL := queue[0]
	for page := 0; page < 10 && len(queue) > 0; page++ {
		nextURL := queue[0]
		queue = queue[1:]
		if strings.TrimSpace(nextURL) == "" || seen[nextURL] {
			continue
		}
		seen[nextURL] = true
		cachedBody := ""
		if page == 0 && strings.TrimSpace(options.Book.TocHTML) != "" {
			cachedBody = options.Book.TocHTML
		}
		pageChapters, foundNext, err := e.tocPage(ctx, rc, rules, listRule, nextURL, cachedBody)
		if err != nil {
			return TocResult{}, err
		}
		chapters = append(chapters, pageChapters...)
		for _, found := range foundNext {
			resolved := rc.resolveURL(found)
			if resolved != "" && !seen[resolved] {
				queue = append(queue, resolved)
			}
		}
	}
	if reverse {
		reverseChapters(chapters)
	}
	chapters = dedupeChapters(chapters)
	return TocResult{Source: source.Summary(), TocURL: firstURL, Chapters: chapters}, nil
}

func (e *Engine) tocPage(ctx context.Context, rc *runContext, rules map[string]string, listRule string, tocURL string, cachedBody string) ([]Chapter, []string, error) {
	if strings.TrimSpace(cachedBody) != "" {
		rc.setResponse(httpResult{URL: tocURL, Body: cachedBody})
	} else {
		spec, err := rc.getRequestSpec(ctx, tocURL)
		if err != nil {
			return nil, nil, err
		}
		res, err := e.doRequest(ctx, spec)
		if err != nil {
			return nil, nil, err
		}
		res, err = rc.applyLoginCheck(ctx, res)
		if err != nil {
			return nil, nil, err
		}
		rc.setResponse(res)
	}
	items, err := rc.evalRule(ctx, listRule, rc.result)
	if err != nil {
		return nil, nil, err
	}
	chapters := make([]Chapter, 0)
	for _, item := range itemsList(items) {
		chapter := Chapter{}
		if rule := strings.TrimSpace(rules["chapterName"]); rule != "" {
			chapter.Name, _ = rc.evalRuleString(ctx, rule, item)
		}
		if rule := strings.TrimSpace(rules["isVolume"]); rule != "" {
			value, _ := rc.evalRule(ctx, rule, item)
			chapter.IsVolume = anyBool(value)
		}
		if rule := strings.TrimSpace(rules["isVip"]); rule != "" {
			value, _ := rc.evalRule(ctx, rule, item)
			chapter.IsVIP = anyBool(value)
		}
		if rule := strings.TrimSpace(rules["updateTime"]); rule != "" {
			chapter.UpdateTime, _ = rc.evalRuleString(ctx, rule, item)
		}
		if rule := strings.TrimSpace(rules["chapterUrl"]); rule != "" {
			chapter.URL, _ = rc.evalRuleString(ctx, rule, item)
		}
		if strings.TrimSpace(chapter.URL) == "" {
			if chapter.IsVolume {
				chapter.URL = chapter.Name + intString(len(chapters))
			} else {
				chapter.URL = tocURL
			}
		} else if !chapter.IsVolume || !strings.HasPrefix(chapter.URL, chapter.Name) {
			chapter.URL = rc.resolveURL(chapter.URL)
		}
		if chapter.IsVIP && chapter.Name != "" && !strings.HasPrefix(chapter.Name, "🔒") {
			chapter.Name = "🔒" + chapter.Name
		}
		if strings.TrimSpace(chapter.Name) != "" {
			chapters = append(chapters, chapter)
		}
	}
	nextURLs := []string{}
	if rule := strings.TrimSpace(rules["nextTocUrl"]); rule != "" {
		value, _ := rc.evalRule(ctx, rule, rc.result)
		for _, next := range stringsFromValue(value) {
			if strings.TrimSpace(next) != "" && next != tocURL {
				nextURLs = append(nextURLs, next)
			}
		}
	}
	return chapters, nextURLs, nil
}

func (e *Engine) Content(ctx context.Context, source Source, options ContentOptions) (ContentResult, error) {
	chapterURL := strings.TrimSpace(options.ChapterURL)
	if chapterURL == "" {
		chapterURL = strings.TrimSpace(options.Chapter.URL)
	}
	if chapterURL == "" {
		return ContentResult{}, errors.New("chapter url empty")
	}
	rules, err := requireRuleObject(source.RuleContent, "正文")
	if err != nil {
		return ContentResult{}, err
	}
	imageStyle := strings.TrimSpace(rules["imageStyle"])
	if strings.TrimSpace(rules["content"]) == "" {
		return ContentResult{Source: source.Summary(), ChapterURL: chapterURL, Content: chapterURL, ImageStyle: imageStyle}, nil
	}
	if options.Chapter.IsVolume && strings.HasPrefix(chapterURL, options.Chapter.Name) {
		return ContentResult{Source: source.Summary(), ChapterURL: chapterURL, Content: options.Chapter.UpdateTime, ImageStyle: imageStyle}, nil
	}
	rc := newRunContext(e, source)
	rc.book = options.Book
	rc.chapter = options.Chapter
	if strings.TrimSpace(options.Book.TocURL) != "" {
		rc.baseURL = options.Book.TocURL
	}
	parts := make([]string, 0, 2)
	seen := map[string]bool{}
	queue := []string{rc.resolveURL(chapterURL)}
	visited := []string{}
	firstURL := queue[0]
	for page := 0; page < 10 && len(queue) > 0; page++ {
		nextURL := queue[0]
		queue = queue[1:]
		if strings.TrimSpace(nextURL) == "" || seen[nextURL] {
			continue
		}
		seen[nextURL] = true
		visited = append(visited, nextURL)
		content, foundNext, err := e.contentPage(ctx, rc, rules, nextURL)
		if err != nil {
			return ContentResult{}, err
		}
		if strings.TrimSpace(content) != "" {
			parts = append(parts, content)
		}
		for _, found := range foundNext {
			resolved := rc.resolveURL(found)
			if resolved != "" && !seen[resolved] {
				queue = append(queue, resolved)
			}
		}
	}
	content := strings.Join(parts, "\n")
	if rule := strings.TrimSpace(rules["replaceRegex"]); rule != "" {
		value, err := rc.evalRule(ctx, rule, content)
		if err != nil {
			return ContentResult{}, err
		}
		content = anyString(value)
	}
	content = html.UnescapeString(content)
	return ContentResult{Source: source.Summary(), ChapterURL: firstURL, Content: content, ImageStyle: imageStyle, NextURLs: visited}, nil
}

func (e *Engine) contentPage(ctx context.Context, rc *runContext, rules map[string]string, chapterURL string) (string, []string, error) {
	spec, err := requestSpecFromTextWithContent(ctx, rc, chapterURL, rules["webJs"], rules["sourceRegex"])
	if err != nil {
		return "", nil, err
	}
	res, err := e.doRequest(ctx, spec)
	if err != nil {
		return "", nil, err
	}
	rc.setResponse(res)
	content, err := rc.evalRuleString(ctx, rules["content"], rc.result)
	if err != nil {
		return "", nil, err
	}
	nextURLs := []string{}
	if rule := strings.TrimSpace(rules["nextContentUrl"]); rule != "" {
		value, _ := rc.evalRule(ctx, rule, rc.result)
		nextURLs = append(nextURLs, stringsFromValue(value)...)
	}
	return content, nextURLs, nil
}

func (rc *runContext) analyzeBookList(ctx context.Context, isSearch bool, baseURL string) ([]Book, error) {
	if matchesBookURLPattern(rc.source.BookURLPattern, baseURL) {
		return rc.detailBookFromCurrentPage(ctx, baseURL)
	}
	rules, err := rc.bookListRules(isSearch)
	if err != nil {
		return nil, err
	}
	listRule, reverse := trimLeadingRuleFlag(rules["bookList"])
	items, err := rc.evalRule(ctx, listRule, rc.result)
	if err != nil {
		return nil, err
	}
	if itemsIsEmpty(items) && strings.TrimSpace(rc.source.BookURLPattern) == "" {
		return rc.detailBookFromCurrentPage(ctx, baseURL)
	}
	books := rc.booksFromRules(ctx, rules, items, baseURL)
	if reverse {
		reverseBooks(books)
	}
	return books, nil
}

func (rc *runContext) bookListRules(isSearch bool) (map[string]string, error) {
	searchRules, err := requireRuleObject(rc.source.RuleSearch, "搜索")
	if err != nil {
		return nil, err
	}
	if isSearch {
		return searchRules, nil
	}
	exploreRules, err := requireRuleObject(rc.source.RuleExplore, "发现")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(exploreRules["bookList"]) == "" {
		return searchRules, nil
	}
	return exploreRules, nil
}

func (rc *runContext) detailBookFromCurrentPage(ctx context.Context, baseURL string) ([]Book, error) {
	rules, err := requireRuleObject(rc.source.RuleBookInfo, "详情")
	if err != nil {
		return nil, err
	}
	pageValue := rc.result
	if initRule := strings.TrimSpace(rules["init"]); initRule != "" {
		pageValue, err = rc.evalRule(ctx, initRule, rc.result)
		if err != nil {
			return nil, err
		}
		rc.result = pageValue
	}
	book := Book{SourceURL: rc.source.BookSourceURL, SourceName: rc.source.BookSourceName, BookURL: baseURL, InfoHTML: anyString(rc.result)}
	if err := rc.applyBookInfoRules(ctx, rules, pageValue, &book, baseURL, anyString(rc.result), false); err != nil {
		return nil, err
	}
	if strings.TrimSpace(book.Name) == "" {
		return nil, nil
	}
	return []Book{book}, nil
}

func (rc *runContext) applyBookInfoRules(ctx context.Context, rules map[string]string, pageValue any, book *Book, bookURL string, body string, canReName bool) error {
	parsed, err := rc.bookFromRules(ctx, rules, pageValue)
	if err != nil {
		return err
	}
	allowRename := canReName && strings.TrimSpace(rules["canReName"]) != ""
	if strings.TrimSpace(rules["canReName"]) != "" {
		ensureBookExtra(book)["canReName"] = true
	}
	if parsed.Name != "" && (allowRename || book.Name == "") {
		book.Name = parsed.Name
	}
	if parsed.Author != "" && (allowRename || book.Author == "") {
		book.Author = parsed.Author
	}
	if parsed.Intro != "" {
		book.Intro = parsed.Intro
	}
	if parsed.Kind != "" {
		book.Kind = parsed.Kind
	}
	if parsed.LastChapter != "" {
		book.LastChapter = parsed.LastChapter
	}
	if parsed.UpdateTime != "" {
		book.UpdateTime = parsed.UpdateTime
	}
	if parsed.CoverURL != "" {
		book.CoverURL = rc.resolveURL(parsed.CoverURL)
	}
	if parsed.WordCount != "" {
		book.WordCount = parsed.WordCount
	}
	if parsed.DownloadURLs != "" {
		book.DownloadURLs = parsed.DownloadURLs
	}
	if parsed.TocURL != "" {
		book.TocURL = rc.resolveURL(parsed.TocURL)
	} else {
		book.TocURL = bookURL
	}
	book.SourceURL = rc.source.BookSourceURL
	book.SourceName = rc.source.BookSourceName
	book.BookURL = bookURL
	if sameURL(book.TocURL, bookURL) {
		book.TocHTML = body
	}
	return nil
}

func (rc *runContext) bookFromRules(ctx context.Context, rules map[string]string, item any) (Book, error) {
	book := Book{}
	var err error
	if rule := strings.TrimSpace(rules["name"]); rule != "" {
		book.Name, err = rc.evalRuleString(ctx, rule, item)
		if err != nil {
			return Book{}, err
		}
	}
	if rule := strings.TrimSpace(rules["author"]); rule != "" {
		book.Author, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["bookUrl"]); rule != "" {
		book.BookURL, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["coverUrl"]); rule != "" {
		book.CoverURL, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["intro"]); rule != "" {
		book.Intro, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["kind"]); rule != "" {
		book.Kind, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["lastChapter"]); rule != "" {
		book.LastChapter, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["updateTime"]); rule != "" {
		book.UpdateTime, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["tocUrl"]); rule != "" {
		book.TocURL, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["wordCount"]); rule != "" {
		book.WordCount, _ = rc.evalRuleString(ctx, rule, item)
	}
	if rule := strings.TrimSpace(rules["downloadUrls"]); rule != "" {
		book.DownloadURLs, _ = rc.evalRuleString(ctx, rule, item)
	}
	if book.CoverURL != "" && strings.TrimSpace(rc.source.CoverDecodeJS) != "" {
		if value, err := rc.evalJS(ctx, rc.source.CoverDecodeJS, book.CoverURL); err == nil && strings.TrimSpace(anyString(value)) != "" {
			book.CoverURL = anyString(value)
		}
	}
	if rule := strings.TrimSpace(rules["checkKeyWord"]); rule != "" {
		book.CheckKeyword = rule
	}
	return book, nil
}

func (rc *runContext) booksFromRules(ctx context.Context, rules map[string]string, items any, baseURL string) []Book {
	books := make([]Book, 0)
	for _, item := range itemsList(items) {
		book, err := rc.bookFromRules(ctx, rules, item)
		if err != nil {
			continue
		}
		if book.BookURL != "" {
			book.BookURL = rc.resolveURL(book.BookURL)
		} else if strings.TrimSpace(book.Name) != "" {
			book.BookURL = baseURL
		}
		if book.CoverURL != "" {
			book.CoverURL = rc.resolveURL(book.CoverURL)
		}
		book.SourceURL = rc.source.BookSourceURL
		book.SourceName = rc.source.BookSourceName
		if hasBookValue(book) {
			books = append(books, book)
		}
	}
	return books
}

func (rc *runContext) applyLoginCheck(ctx context.Context, res httpResult) (httpResult, error) {
	checkJS := strings.TrimSpace(rc.source.LoginCheckJS)
	if checkJS == "" {
		return res, nil
	}
	value, err := rc.evalJS(ctx, checkJS, res)
	if err != nil {
		return httpResult{}, err
	}
	return mergeHTTPResult(res, value), nil
}

func mergeHTTPResult(res httpResult, value any) httpResult {
	switch v := value.(type) {
	case map[string]any:
		if body := anyString(v["body"]); body != "" {
			res.Body = body
		}
		if rawURL := anyString(v["url"]); rawURL != "" {
			res.URL = rawURL
		}
		if status := anyInt(v["statusCode"]); status > 0 {
			res.StatusCode = status
		}
	case string:
		if strings.TrimSpace(v) != "" {
			res.Body = v
		}
	}
	return res
}

func firstExploreURL(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if parts := strings.SplitN(line, "::", 2); len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
		return line
	}
	return ""
}

func itemsList(value any) []any {
	switch v := value.(type) {
	case nil:
		return nil
	case []any:
		return v
	case []htmlNode:
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
		return items
	case []string:
		items := make([]any, 0, len(v))
		for _, item := range v {
			items = append(items, item)
		}
		return items
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []any{v}
	default:
		return []any{v}
	}
}

func hasBookValue(book Book) bool {
	return strings.TrimSpace(book.Name) != "" || strings.TrimSpace(book.BookURL) != ""
}

func trimLeadingRuleFlag(rule string) (string, bool) {
	rule = strings.TrimSpace(rule)
	reverse := false
	if strings.HasPrefix(rule, "-") {
		reverse = true
		rule = strings.TrimSpace(strings.TrimPrefix(rule, "-"))
	}
	if strings.HasPrefix(rule, "+") {
		rule = strings.TrimSpace(strings.TrimPrefix(rule, "+"))
	}
	return rule, reverse
}

func itemsIsEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	case []htmlNode:
		return len(v) == 0
	case []string:
		return len(v) == 0
	default:
		return false
	}
}

func stringsFromValue(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return compactNonEmpty(strings.Split(v, "\n")...)
	case []string:
		return compactNonEmpty(v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, stringsFromValue(item)...)
		}
		return out
	case []htmlNode:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, strings.TrimSpace(item.HTML))
		}
		return compactNonEmpty(out...)
	default:
		return compactNonEmpty(anyString(v))
	}
}

func reverseBooks(items []Book) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reverseChapters(items []Chapter) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func dedupeChapters(items []Chapter) []Chapter {
	seen := map[string]bool{}
	out := make([]Chapter, 0, len(items))
	for _, item := range items {
		key := item.URL
		if key == "" {
			key = item.Name
		}
		if key != "" && seen[key] {
			continue
		}
		if key != "" {
			seen[key] = true
		}
		out = append(out, item)
	}
	return out
}

func matchesBookURLPattern(pattern string, target string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || strings.TrimSpace(target) == "" {
		return false
	}
	matched, err := regexp.MatchString(pattern, target)
	return err == nil && matched
}

func ensureBookExtra(book *Book) map[string]any {
	if book.Extra == nil {
		book.Extra = map[string]any{}
	}
	return book.Extra
}

func sameURL(left string, right string) bool {
	return strings.TrimRight(strings.TrimSpace(left), "/") == strings.TrimRight(strings.TrimSpace(right), "/")
}

func intString(value int) string {
	return strconv.Itoa(value)
}
