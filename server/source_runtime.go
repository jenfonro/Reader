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

type bookInfoRuntimeRequest struct {
	SourceURL string      `json:"sourceUrl"`
	BookURL   string      `json:"bookUrl"`
	Book      legado.Book `json:"book"`
	CanReName bool        `json:"canReName"`
}

type tocRuntimeRequest struct {
	SourceURL string      `json:"sourceUrl"`
	TocURL    string      `json:"tocUrl"`
	Book      legado.Book `json:"book"`
}

type contentRuntimeRequest struct {
	SourceURL  string         `json:"sourceUrl"`
	ChapterURL string         `json:"chapterUrl"`
	Book       legado.Book    `json:"book"`
	Chapter    legado.Chapter `json:"chapter"`
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
		var input bookInfoRuntimeRequest
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		input.SourceURL = firstNonEmpty(input.SourceURL, r.URL.Query().Get("sourceUrl"), r.URL.Query().Get("source"))
		input.BookURL = firstNonEmpty(input.BookURL, r.URL.Query().Get("bookUrl"), r.URL.Query().Get("url"), input.Book.BookURL)
		if strings.TrimSpace(input.SourceURL) == "" || strings.TrimSpace(input.BookURL) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源地址和书籍地址不能为空"})
			return
		}
		source, ok := loadRuntimeSource(w, database, input.SourceURL)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		book, err := legado.NewEngine().BookInfo(ctx, source, legado.BookInfoOptions{BookURL: input.BookURL, Book: input.Book, CanReName: input.CanReName})
		if handleRuntimeError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"book": book})
	}
}

func tocHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		var input tocRuntimeRequest
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		input.SourceURL = firstNonEmpty(input.SourceURL, r.URL.Query().Get("sourceUrl"), r.URL.Query().Get("source"))
		input.TocURL = firstNonEmpty(input.TocURL, r.URL.Query().Get("tocUrl"), r.URL.Query().Get("url"), input.Book.TocURL)
		if strings.TrimSpace(input.SourceURL) == "" || strings.TrimSpace(input.TocURL) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源地址和目录地址不能为空"})
			return
		}
		source, ok := loadRuntimeSource(w, database, input.SourceURL)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		result, err := legado.NewEngine().Toc(ctx, source, legado.TocOptions{TocURL: input.TocURL, Book: input.Book})
		if handleRuntimeError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func contentHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		var input contentRuntimeRequest
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		input.SourceURL = firstNonEmpty(input.SourceURL, r.URL.Query().Get("sourceUrl"), r.URL.Query().Get("source"))
		input.ChapterURL = firstNonEmpty(input.ChapterURL, r.URL.Query().Get("chapterUrl"), r.URL.Query().Get("url"), input.Chapter.URL)
		if strings.TrimSpace(input.SourceURL) == "" || strings.TrimSpace(input.ChapterURL) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源地址和章节地址不能为空"})
			return
		}
		source, ok := loadRuntimeSource(w, database, input.SourceURL)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		result, err := legado.NewEngine().Content(ctx, source, legado.ContentOptions{ChapterURL: input.ChapterURL, Book: input.Book, Chapter: input.Chapter})
		if handleRuntimeError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, result)
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
