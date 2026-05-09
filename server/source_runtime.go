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

type searchRuntimeRequest struct {
	SourceURL string `json:"sourceUrl"`
	Keyword   string `json:"keyword"`
	Page      int    `json:"page"`
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

func searchHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		input := searchRuntimeRequest{Page: 1}
		if err := readRuntimeInput(r, &input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		input.SourceURL = firstNonEmpty(input.SourceURL, r.URL.Query().Get("sourceUrl"), r.URL.Query().Get("source"))
		input.Keyword = firstNonEmpty(input.Keyword, r.URL.Query().Get("keyword"), r.URL.Query().Get("key"), r.URL.Query().Get("q"))
		if page := parsePositiveInt(r.URL.Query().Get("page")); page > 0 {
			input.Page = page
		}
		if strings.TrimSpace(input.Keyword) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "搜索关键词不能为空"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		engine := legado.NewEngine()
		if strings.TrimSpace(input.SourceURL) != "" {
			source, ok := loadRuntimeSource(w, database, input.SourceURL)
			if !ok {
				return
			}
			result, err := engine.Search(ctx, source, legado.SearchOptions{Keyword: input.Keyword, Page: input.Page})
			if handleRuntimeError(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"keyword": input.Keyword, "page": input.Page, "source": result.Source, "books": result.Books})
			return
		}
		rows, err := database.ListBookSources()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "读取书源失败"})
			return
		}
		results := make([]legado.SearchResult, 0)
		for _, row := range rows {
			if !row.Enabled {
				continue
			}
			detail, err := database.GetBookSourceByURL(row.BookSourceURL)
			if err != nil {
				continue
			}
			result, err := engine.Search(ctx, legado.SourceFromDB(detail), legado.SearchOptions{Keyword: input.Keyword, Page: input.Page})
			if err != nil {
				continue
			}
			if len(result.Books) > 0 {
				results = append(results, result)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"keyword": input.Keyword, "page": input.Page, "results": results})
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
