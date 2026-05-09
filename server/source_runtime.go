package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jenfonro/reader/internal/db"
	"github.com/jenfonro/reader/internal/source/legado"
)

type searchSourcePayload struct {
	SourceID    string `json:"sourceId"`
	Name        string `json:"name"`
	Group       string `json:"group,omitempty"`
	Type        int64  `json:"type"`
	Order       int64  `json:"order"`
	Weight      int64  `json:"weight"`
	RespondTime int64  `json:"respondTime"`
}

type searchSourcesResponse struct {
	Sources []searchSourcePayload `json:"sources"`
	Total   int                   `json:"total"`
}

type searchRequest struct {
	SourceID string `json:"sourceId"`
	Keyword  string `json:"keyword"`
	Page     int    `json:"page"`
}

type searchResponse struct {
	Keyword string              `json:"keyword"`
	Page    int                 `json:"page"`
	Total   int                 `json:"total"`
	Books   []searchBookPayload `json:"books"`
	Source  searchSourceStatus  `json:"source"`
}

type searchStreamEvent struct {
	Type             string              `json:"type"`
	Keyword          string              `json:"keyword,omitempty"`
	Page             int                 `json:"page,omitempty"`
	TotalSources     int                 `json:"totalSources,omitempty"`
	CompletedSources int                 `json:"completedSources,omitempty"`
	TotalBooks       int                 `json:"totalBooks,omitempty"`
	Books            []searchBookPayload `json:"books,omitempty"`
	Source           searchSourceStatus  `json:"source,omitempty"`
	Message          string              `json:"message,omitempty"`
}

type searchBookPayload struct {
	Name          string   `json:"name,omitempty"`
	Author        string   `json:"author,omitempty"`
	BookURL       string   `json:"bookUrl,omitempty"`
	CoverURL      string   `json:"coverUrl,omitempty"`
	ImageURL      string   `json:"imageUrl,omitempty"`
	Intro         string   `json:"intro,omitempty"`
	Kind          string   `json:"kind,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	LastChapter   string   `json:"lastChapter,omitempty"`
	LatestChapter string   `json:"latestChapter,omitempty"`
	UpdateTime    string   `json:"updateTime,omitempty"`
	Time          string   `json:"time,omitempty"`
	WordCount     string   `json:"wordCount,omitempty"`
	SourceID      string   `json:"sourceId"`
	SourceName    string   `json:"sourceName"`
	SourceGroup   string   `json:"sourceGroup,omitempty"`
}

type searchSourceStatus struct {
	SourceID         string `json:"sourceId"`
	SourceName       string `json:"sourceName,omitempty"`
	SourceGroup      string `json:"sourceGroup,omitempty"`
	Success          bool   `json:"success"`
	Count            int    `json:"count"`
	Message          string `json:"message,omitempty"`
	NeedVerification bool   `json:"needVerification,omitempty"`
	VerificationURL  string `json:"verificationUrl,omitempty"`
}

type exploreRuntimeRequest struct {
	SourceURL string `json:"sourceUrl"`
	URL       string `json:"url"`
	Page      int    `json:"page"`
}

type bookInfoRequest struct {
	SourceID string            `json:"sourceId"`
	BookURL  string            `json:"bookUrl"`
	Book     searchBookPayload `json:"book"`
}

type bookInfoResponse struct {
	Book   bookInfoPayload    `json:"book"`
	Source searchSourceStatus `json:"source"`
}

type bookInfoPayload struct {
	Name          string   `json:"name,omitempty"`
	Author        string   `json:"author,omitempty"`
	BookURL       string   `json:"bookUrl,omitempty"`
	TocURL        string   `json:"tocUrl,omitempty"`
	CoverURL      string   `json:"coverUrl,omitempty"`
	ImageURL      string   `json:"imageUrl,omitempty"`
	Intro         string   `json:"intro,omitempty"`
	Kind          string   `json:"kind,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	LastChapter   string   `json:"lastChapter,omitempty"`
	LatestChapter string   `json:"latestChapter,omitempty"`
	UpdateTime    string   `json:"updateTime,omitempty"`
	Time          string   `json:"time,omitempty"`
	WordCount     string   `json:"wordCount,omitempty"`
	DownloadURLs  string   `json:"downloadUrls,omitempty"`
	SourceID      string   `json:"sourceId"`
	SourceName    string   `json:"sourceName"`
	SourceGroup   string   `json:"sourceGroup,omitempty"`
}

type tocRequest struct {
	SourceID string          `json:"sourceId"`
	BookURL  string          `json:"bookUrl"`
	TocURL   string          `json:"tocUrl"`
	Book     bookInfoPayload `json:"book"`
}

type tocResponse struct {
	TocURL   string             `json:"tocUrl"`
	Total    int                `json:"total"`
	Chapters []chapterPayload   `json:"chapters"`
	Source   searchSourceStatus `json:"source"`
}

type chapterPayload struct {
	Index      int    `json:"index"`
	Name       string `json:"name,omitempty"`
	Title      string `json:"title,omitempty"`
	ChapterURL string `json:"chapterUrl,omitempty"`
	URL        string `json:"url,omitempty"`
	IsVolume   bool   `json:"isVolume,omitempty"`
	IsVIP      bool   `json:"isVip,omitempty"`
	UpdateTime string `json:"updateTime,omitempty"`
	Time       string `json:"time,omitempty"`
}

type contentRequest struct {
	SourceID   string          `json:"sourceId"`
	ChapterURL string          `json:"chapterUrl"`
	Book       bookInfoPayload `json:"book"`
	Chapter    chapterPayload  `json:"chapter"`
}

type contentResponse struct {
	ChapterURL string             `json:"chapterUrl"`
	Chapter    chapterPayload     `json:"chapter"`
	Content    string             `json:"content"`
	ImageStyle string             `json:"imageStyle,omitempty"`
	NextURLs   []string           `json:"nextUrls,omitempty"`
	Source     searchSourceStatus `json:"source"`
}

func searchSourcesHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		rows, err := database.ListBookSources()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "读取书源失败"})
			return
		}
		sources := make([]searchSourcePayload, 0, len(rows))
		for _, row := range rows {
			if !row.Enabled {
				continue
			}
			sources = append(sources, searchSourcePayload{
				SourceID:    bookSourceID(row.BookSourceURL),
				Name:        row.BookSourceName,
				Group:       derefString(row.BookSourceGroup),
				Type:        row.BookSourceType,
				Order:       row.CustomOrder,
				Weight:      row.Weight,
				RespondTime: row.RespondTime,
			})
		}
		writeJSON(w, http.StatusOK, searchSourcesResponse{Sources: sources, Total: len(sources)})
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func searchStreamHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}

		input := searchRequest{Page: 1}
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		fillSearchInputFromQuery(&input, r)
		input.Keyword = strings.TrimSpace(input.Keyword)
		if input.Keyword == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "搜索关键词不能为空"})
			return
		}
		if input.Page <= 0 {
			input.Page = 1
		}

		sources, err := loadSearchSources(database, input.SourceID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "读取书源失败"})
			return
		}
		if len(sources) == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "没有可搜索的书源"})
			return
		}

		concurrency, err := database.SearchConcurrency()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "读取设置失败"})
			return
		}
		concurrency = normalizeSearchConcurrency(concurrency, len(sources))

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "当前环境不支持流式响应"})
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		encoder := json.NewEncoder(w)
		if err := encoder.Encode(searchStreamEvent{
			Type:         "start",
			Keyword:      input.Keyword,
			Page:         input.Page,
			TotalSources: len(sources),
		}); err != nil {
			return
		}
		flusher.Flush()

		streamCtx, cancel := context.WithCancel(r.Context())
		defer cancel()

		events := make(chan searchStreamEvent)
		go runSearchStream(streamCtx, sources, input, concurrency, events)

		completedSources := 0
		totalBooks := 0
		for event := range events {
			if event.Type == "source" {
				completedSources++
				totalBooks += len(event.Books)
				event.CompletedSources = completedSources
				event.TotalSources = len(sources)
			}
			if err := encoder.Encode(event); err != nil {
				return
			}
			flusher.Flush()
		}

		_ = encoder.Encode(searchStreamEvent{
			Type:             "done",
			Keyword:          input.Keyword,
			Page:             input.Page,
			TotalSources:     len(sources),
			CompletedSources: completedSources,
			TotalBooks:       totalBooks,
		})
		flusher.Flush()
	}
}

func runSearchStream(ctx context.Context, sources []legado.Source, input searchRequest, concurrency int, events chan<- searchStreamEvent) {
	defer close(events)

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
scheduleLoop:
	for _, source := range sources {
		select {
		case <-ctx.Done():
			break scheduleLoop
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(source legado.Source) {
			defer wg.Done()
			defer func() { <-sem }()

			event := searchOneSource(ctx, source, input)
			select {
			case <-ctx.Done():
			case events <- event:
			}
		}(source)
	}
	wg.Wait()
}

func searchOneSource(ctx context.Context, source legado.Source, input searchRequest) searchStreamEvent {
	searchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	status := searchStatusFromSource(source)
	result, err := legado.NewEngine().Search(searchCtx, source, legado.SearchOptions{Keyword: input.Keyword, Page: input.Page})
	if err != nil {
		status.Message = err.Error()
		var verification legado.NeedVerificationError
		if errors.As(err, &verification) {
			status.NeedVerification = true
			status.VerificationURL = verification.URL
		}
		return searchStreamEvent{Type: "source", Source: status, Books: []searchBookPayload{}}
	}

	books := make([]searchBookPayload, 0, len(result.Books))
	for _, book := range result.Books {
		books = append(books, searchBookFromLegado(source, book))
	}
	status.Success = true
	status.Count = len(books)
	return searchStreamEvent{Type: "source", Source: status, Books: books}
}

func normalizeSearchConcurrency(value int, sourceCount int) int {
	if value <= 0 {
		value = db.DefaultSearchConcurrency
	}
	if sourceCount > 0 && value > sourceCount {
		return sourceCount
	}
	return value
}

func loadSearchSources(database *db.DB, sourceID string) ([]legado.Source, error) {
	rows, err := database.ListEnabledBookSourceDetails()
	if err != nil {
		return nil, err
	}
	sourceID = strings.TrimSpace(sourceID)
	sources := make([]legado.Source, 0, len(rows))
	for _, row := range rows {
		if sourceID != "" && !sameBookSourceID(sourceID, row.BookSourceURL) {
			continue
		}
		sources = append(sources, legado.SourceFromDB(row))
	}
	return sources, nil
}

func searchHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		input := searchRequest{Page: 1}
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		fillSearchInputFromQuery(&input, r)
		if strings.TrimSpace(input.Keyword) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "搜索关键词不能为空"})
			return
		}
		if strings.TrimSpace(input.SourceID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源 ID 不能为空"})
			return
		}
		if input.Page <= 0 {
			input.Page = 1
		}

		source, ok := loadSearchSourceByID(w, database, input.SourceID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()

		status := searchStatusFromSource(source)
		result, err := legado.NewEngine().Search(ctx, source, legado.SearchOptions{Keyword: input.Keyword, Page: input.Page})
		if err != nil {
			status.Message = err.Error()
			var verification legado.NeedVerificationError
			if errors.As(err, &verification) {
				status.NeedVerification = true
				status.VerificationURL = verification.URL
			}
			writeJSON(w, http.StatusOK, searchResponse{
				Keyword: input.Keyword,
				Page:    input.Page,
				Total:   0,
				Books:   []searchBookPayload{},
				Source:  status,
			})
			return
		}

		books := make([]searchBookPayload, 0, len(result.Books))
		for _, book := range result.Books {
			books = append(books, searchBookFromLegado(source, book))
		}
		status.Success = true
		status.Count = len(books)
		writeJSON(w, http.StatusOK, searchResponse{
			Keyword: input.Keyword,
			Page:    input.Page,
			Total:   len(books),
			Books:   books,
			Source:  status,
		})
	}
}

func fillSearchInputFromQuery(input *searchRequest, r *http.Request) {
	query := r.URL.Query()
	input.Keyword = firstNonEmpty(input.Keyword, query.Get("keyword"), query.Get("key"), query.Get("q"))
	input.SourceID = firstNonEmpty(input.SourceID, query.Get("sourceId"), query.Get("id"))
	if page := parsePositiveInt(query.Get("page")); page > 0 {
		input.Page = page
	}
}

func loadSearchSourceByID(w http.ResponseWriter, database *db.DB, sourceID string) (legado.Source, bool) {
	rows, err := database.ListBookSources()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "读取书源失败"})
		return legado.Source{}, false
	}
	for _, row := range rows {
		if !sameBookSourceID(sourceID, row.BookSourceURL) {
			continue
		}
		detail, err := database.GetBookSourceByURL(row.BookSourceURL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "读取书源失败"})
			return legado.Source{}, false
		}
		return legado.SourceFromDB(detail), true
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"message": "书源不存在"})
	return legado.Source{}, false
}

func searchStatusFromSource(source legado.Source) searchSourceStatus {
	return searchSourceStatus{
		SourceID:    bookSourceID(source.BookSourceURL),
		SourceName:  source.BookSourceName,
		SourceGroup: source.BookSourceGroup,
	}
}

func searchBookFromLegado(source legado.Source, book legado.Book) searchBookPayload {
	sourceName := firstNonEmpty(book.SourceName, source.BookSourceName)
	return searchBookPayload{
		Name:          book.Name,
		Author:        book.Author,
		BookURL:       book.BookURL,
		CoverURL:      book.CoverURL,
		ImageURL:      book.CoverURL,
		Intro:         book.Intro,
		Kind:          book.Kind,
		Tags:          splitSearchTags(book.Kind),
		LastChapter:   book.LastChapter,
		LatestChapter: book.LastChapter,
		UpdateTime:    book.UpdateTime,
		Time:          book.UpdateTime,
		WordCount:     book.WordCount,
		SourceID:      bookSourceID(source.BookSourceURL),
		SourceName:    sourceName,
		SourceGroup:   source.BookSourceGroup,
	}
}

func splitSearchTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := map[string]bool{}
	tags := make([]string, 0)
	for _, tag := range strings.FieldsFunc(raw, isSearchTagSeparator) {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

func isSearchTagSeparator(char rune) bool {
	switch char {
	case ',', '，', ';', '；', '|', '/', '\\', '\n', '\t':
		return true
	default:
		return false
	}
}

func exploreHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		input := exploreRuntimeRequest{Page: 1}
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		input.SourceURL = firstNonEmpty(input.SourceURL, r.URL.Query().Get("sourceUrl"), r.URL.Query().Get("source"))
		input.URL = firstNonEmpty(input.URL, r.URL.Query().Get("url"), r.URL.Query().Get("exploreUrl"))
		if page := parsePositiveInt(r.URL.Query().Get("page")); page > 0 {
			input.Page = page
		}
		if strings.TrimSpace(input.SourceURL) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源地址不能为空"})
			return
		}
		source, ok := loadRuntimeSource(w, database, input.SourceURL)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		result, err := legado.NewEngine().Explore(ctx, source, legado.ExploreOptions{URL: input.URL, Page: input.Page})
		if handleRuntimeError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func bookInfoHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		var input bookInfoRequest
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		fillBookInfoInputFromQuery(&input, r)
		input.SourceID = firstNonEmpty(input.SourceID, input.Book.SourceID)
		input.BookURL = firstNonEmpty(input.BookURL, input.Book.BookURL)
		if strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.BookURL) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源 ID 和书籍地址不能为空"})
			return
		}

		source, ok := loadSearchSourceByID(w, database, input.SourceID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		seed := legadoBookFromSearchPayload(input.Book)
		book, err := legado.NewEngine().BookInfo(ctx, source, legado.BookInfoOptions{BookURL: input.BookURL, Book: seed, CanReName: true})
		if handleRuntimeError(w, err) {
			return
		}
		status := searchStatusFromSource(source)
		status.Success = true
		status.Count = 1
		writeJSON(w, http.StatusOK, bookInfoResponse{
			Book:   bookInfoFromLegado(source, book),
			Source: status,
		})
	}
}

func fillBookInfoInputFromQuery(input *bookInfoRequest, r *http.Request) {
	query := r.URL.Query()
	input.SourceID = firstNonEmpty(input.SourceID, query.Get("sourceId"), query.Get("id"))
	input.BookURL = firstNonEmpty(input.BookURL, query.Get("bookUrl"), query.Get("url"))
}

func legadoBookFromSearchPayload(book searchBookPayload) legado.Book {
	return legado.Book{
		Name:        book.Name,
		Author:      book.Author,
		BookURL:     book.BookURL,
		CoverURL:    firstNonEmpty(book.CoverURL, book.ImageURL),
		Intro:       book.Intro,
		Kind:        firstNonEmpty(book.Kind, strings.Join(book.Tags, ",")),
		LastChapter: firstNonEmpty(book.LastChapter, book.LatestChapter),
		UpdateTime:  firstNonEmpty(book.UpdateTime, book.Time),
		WordCount:   book.WordCount,
		SourceName:  book.SourceName,
	}
}

func bookInfoFromLegado(source legado.Source, book legado.Book) bookInfoPayload {
	sourceName := firstNonEmpty(book.SourceName, source.BookSourceName)
	kind := book.Kind
	return bookInfoPayload{
		Name:          book.Name,
		Author:        book.Author,
		BookURL:       book.BookURL,
		TocURL:        book.TocURL,
		CoverURL:      book.CoverURL,
		ImageURL:      book.CoverURL,
		Intro:         book.Intro,
		Kind:          kind,
		Tags:          splitSearchTags(kind),
		LastChapter:   book.LastChapter,
		LatestChapter: book.LastChapter,
		UpdateTime:    book.UpdateTime,
		Time:          book.UpdateTime,
		WordCount:     book.WordCount,
		DownloadURLs:  book.DownloadURLs,
		SourceID:      bookSourceID(source.BookSourceURL),
		SourceName:    sourceName,
		SourceGroup:   source.BookSourceGroup,
	}
}

func tocHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		var input tocRequest
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		fillTocInputFromQuery(&input, r)
		input.SourceID = firstNonEmpty(input.SourceID, input.Book.SourceID)
		input.BookURL = firstNonEmpty(input.BookURL, input.Book.BookURL)
		input.TocURL = firstNonEmpty(input.TocURL, input.Book.TocURL, input.BookURL)
		if strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.TocURL) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源 ID 和目录地址不能为空"})
			return
		}

		source, ok := loadSearchSourceByID(w, database, input.SourceID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		book := legadoBookFromInfoPayload(input.Book)
		if book.BookURL == "" {
			book.BookURL = input.BookURL
		}
		if book.TocURL == "" {
			book.TocURL = input.TocURL
		}
		result, err := legado.NewEngine().Toc(ctx, source, legado.TocOptions{TocURL: input.TocURL, Book: book})
		if handleRuntimeError(w, err) {
			return
		}
		chapters := chaptersFromLegado(result.Chapters)
		status := searchStatusFromSource(source)
		status.Success = true
		status.Count = len(chapters)
		writeJSON(w, http.StatusOK, tocResponse{
			TocURL:   result.TocURL,
			Total:    len(chapters),
			Chapters: chapters,
			Source:   status,
		})
	}
}

func fillTocInputFromQuery(input *tocRequest, r *http.Request) {
	query := r.URL.Query()
	input.SourceID = firstNonEmpty(input.SourceID, query.Get("sourceId"), query.Get("id"))
	input.BookURL = firstNonEmpty(input.BookURL, query.Get("bookUrl"))
	input.TocURL = firstNonEmpty(input.TocURL, query.Get("tocUrl"), query.Get("url"))
}

func contentHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		var input contentRequest
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		fillContentInputFromQuery(&input, r)
		input.SourceID = firstNonEmpty(input.SourceID, input.Book.SourceID)
		input.ChapterURL = firstNonEmpty(input.ChapterURL, input.Chapter.ChapterURL, input.Chapter.URL)
		if strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.ChapterURL) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源 ID 和章节地址不能为空"})
			return
		}

		source, ok := loadSearchSourceByID(w, database, input.SourceID)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		book := legadoBookFromInfoPayload(input.Book)
		chapter := legadoChapterFromPayload(input.Chapter)
		if chapter.URL == "" {
			chapter.URL = input.ChapterURL
		}
		result, err := legado.NewEngine().Content(ctx, source, legado.ContentOptions{ChapterURL: input.ChapterURL, Book: book, Chapter: chapter})
		if handleRuntimeError(w, err) {
			return
		}
		status := searchStatusFromSource(source)
		status.Success = true
		status.Count = 1
		responseChapter := chapterPayloadFromLegado(chapter, input.Chapter.Index)
		if result.ChapterURL != "" {
			responseChapter.ChapterURL = result.ChapterURL
			responseChapter.URL = result.ChapterURL
		}
		writeJSON(w, http.StatusOK, contentResponse{
			ChapterURL: result.ChapterURL,
			Chapter:    responseChapter,
			Content:    result.Content,
			ImageStyle: result.ImageStyle,
			NextURLs:   result.NextURLs,
			Source:     status,
		})
	}
}

func fillContentInputFromQuery(input *contentRequest, r *http.Request) {
	query := r.URL.Query()
	input.SourceID = firstNonEmpty(input.SourceID, query.Get("sourceId"), query.Get("id"))
	input.ChapterURL = firstNonEmpty(input.ChapterURL, query.Get("chapterUrl"), query.Get("url"))
}

func legadoBookFromInfoPayload(book bookInfoPayload) legado.Book {
	return legado.Book{
		Name:         book.Name,
		Author:       book.Author,
		BookURL:      book.BookURL,
		TocURL:       book.TocURL,
		CoverURL:     firstNonEmpty(book.CoverURL, book.ImageURL),
		Intro:        book.Intro,
		Kind:         firstNonEmpty(book.Kind, strings.Join(book.Tags, ",")),
		LastChapter:  firstNonEmpty(book.LastChapter, book.LatestChapter),
		UpdateTime:   firstNonEmpty(book.UpdateTime, book.Time),
		WordCount:    book.WordCount,
		DownloadURLs: book.DownloadURLs,
		SourceName:   book.SourceName,
	}
}

func legadoChapterFromPayload(chapter chapterPayload) legado.Chapter {
	return legado.Chapter{
		Name:       firstNonEmpty(chapter.Name, chapter.Title),
		URL:        firstNonEmpty(chapter.ChapterURL, chapter.URL),
		IsVolume:   chapter.IsVolume,
		IsVIP:      chapter.IsVIP,
		UpdateTime: firstNonEmpty(chapter.UpdateTime, chapter.Time),
	}
}

func chaptersFromLegado(chapters []legado.Chapter) []chapterPayload {
	items := make([]chapterPayload, 0, len(chapters))
	for index, chapter := range chapters {
		items = append(items, chapterPayloadFromLegado(chapter, index))
	}
	return items
}

func chapterPayloadFromLegado(chapter legado.Chapter, index int) chapterPayload {
	name := chapter.Name
	return chapterPayload{
		Index:      index,
		Name:       name,
		Title:      name,
		ChapterURL: chapter.URL,
		URL:        chapter.URL,
		IsVolume:   chapter.IsVolume,
		IsVIP:      chapter.IsVIP,
		UpdateTime: chapter.UpdateTime,
		Time:       chapter.UpdateTime,
	}
}

func readRuntimeInput(r *http.Request, value any) error {
	if r.Method == http.MethodGet || r.Body == nil {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := decoder.Decode(value); err != nil {
		return err
	}
	return nil
}

func loadRuntimeSource(w http.ResponseWriter, database *db.DB, sourceURL string) (legado.Source, bool) {
	row, err := database.GetBookSourceByURL(sourceURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "书源不存在"})
			return legado.Source{}, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "读取书源失败"})
		return legado.Source{}, false
	}
	return legado.SourceFromDB(row), true
}

func handleRuntimeError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var verification legado.NeedVerificationError
	if errors.As(err, &verification) {
		writeJSON(w, http.StatusPreconditionRequired, map[string]any{
			"status":  "need_verification",
			"url":     verification.URL,
			"message": verification.Message,
		})
		return true
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"message": err.Error()})
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parsePositiveInt(value string) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
