package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jenfonro/reader/internal/db"
)

const maxBookSourceImportBytes = 20 << 20

type bookSourceNetworkImportRequest struct {
	URL string `json:"url"`
}

type bookSourceConfirmImportRequest struct {
	Sources []json.RawMessage `json:"sources"`
	Items   []json.RawMessage `json:"items"`
}

type bookSourceSaveRequest struct {
	OriginalURL string          `json:"originalUrl"`
	Source      json.RawMessage `json:"source"`
}

type bookSourceDeleteRequest struct {
	URL  string   `json:"url"`
	URLs []string `json:"urls"`
}

type bookSourcePatchRequest struct {
	URL     string `json:"url"`
	Enabled *bool  `json:"enabled"`
}

func bookSourcesHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(w, r) == nil {
			return
		}

		switch r.Method {
		case http.MethodGet:
			writeBookSourceList(w, database)
		case http.MethodPost, http.MethodPut:
			saveBookSource(w, r, database)
		case http.MethodPatch:
			patchBookSource(w, r, database)
		case http.MethodDelete:
			deleteBookSources(w, r, database)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		}
	}
}

func writeBookSourceList(w http.ResponseWriter, database *db.DB) {
	rows, err := database.ListBookSources()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "请求失败"})
		return
	}
	items := make([]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, bookSourcePayload(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": items})
}

func saveBookSource(w http.ResponseWriter, r *http.Request, database *db.DB) {
	body, err := readLimited(r.Body, maxBookSourceImportBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}

	originalURL := ""
	rawSource := json.RawMessage(bytes.TrimSpace(body))
	var input bookSourceSaveRequest
	if err := json.Unmarshal(body, &input); err == nil && len(bytes.TrimSpace(input.Source)) > 0 {
		originalURL = input.OriginalURL
		rawSource = input.Source
	}
	item, err := parseBookSourceRawItem(rawSource, 0)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	row, created, err := database.SaveBookSource(originalURL, item)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrBookSourceURLExists):
			writeJSON(w, http.StatusConflict, map[string]string{"message": "书源地址已存在"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "保存失败"})
		}
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{"source": bookSourceDetailPayload(row), "created": created})
}

func patchBookSource(w http.ResponseWriter, r *http.Request, database *db.DB) {
	var input bookSourcePatchRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
		return
	}
	if strings.TrimSpace(input.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源地址不能为空"})
		return
	}
	if input.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "没有可修改的书源字段"})
		return
	}
	row, err := database.UpdateBookSourceEnabled(input.URL, *input.Enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "书源不存在"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "修改失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": bookSourcePayload(row)})
}

func deleteBookSources(w http.ResponseWriter, r *http.Request, database *db.DB) {
	var input bookSourceDeleteRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
		return
	}
	urls := input.URLs
	if strings.TrimSpace(input.URL) != "" {
		urls = append(urls, input.URL)
	}
	if len(urls) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请选择要删除的书源"})
		return
	}
	deleted, err := database.DeleteBookSources(urls)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "删除失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

func bookSourceDetailHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(w, r) == nil {
			return
		}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		bookSourceURL := strings.TrimSpace(r.URL.Query().Get("url"))
		if bookSourceURL == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "书源地址不能为空"})
			return
		}
		row, err := database.GetBookSourceByURL(bookSourceURL)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "书源不存在"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"source": bookSourceDetailPayload(row)})
	}
}

func bookSourceImportLocalHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(w, r) == nil {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxBookSourceImportBytes)
		if err := r.ParseMultipartForm(maxBookSourceImportBytes); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "文件过大或格式不正确"})
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请选择书源文件"})
			return
		}
		defer file.Close()

		content, err := readLimited(file, maxBookSourceImportBytes)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		writeBookSourcePreview(w, database, content)
	}
}

func bookSourceImportNetworkHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(w, r) == nil {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}

		var input bookSourceNetworkImportRequest
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		target := strings.TrimSpace(input.URL)
		parsed, err := url.Parse(target)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "网络地址不正确"})
			return
		}

		client := &http.Client{Timeout: 18 * time.Second}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "网络地址不正确"})
			return
		}
		req.Header.Set("Accept", "application/json,text/plain,*/*")
		req.Header.Set("User-Agent", "Reader/1.0")

		resp, err := client.Do(req)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"message": "下载书源失败"})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			writeJSON(w, http.StatusBadGateway, map[string]string{"message": fmt.Sprintf("下载失败：HTTP %d", resp.StatusCode)})
			return
		}
		content, err := readLimited(resp.Body, maxBookSourceImportBytes)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		writeBookSourcePreview(w, database, content)
	}
}

func bookSourceImportConfirmHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(w, r) == nil {
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}

		var input bookSourceConfirmImportRequest
		r.Body = http.MaxBytesReader(w, r.Body, maxBookSourceImportBytes)
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
			return
		}
		rawItems := input.Sources
		if len(rawItems) == 0 {
			rawItems = input.Items
		}
		if len(rawItems) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请选择要导入的书源"})
			return
		}

		items, err := parseBookSourceRawItems(rawItems)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
			return
		}
		result, err := database.ImportBookSources(items)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "导入失败"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"created": result.Created,
			"updated": result.Updated,
			"total":   result.Created + result.Updated,
		})
	}
}

func writeBookSourcePreview(w http.ResponseWriter, database *db.DB, content []byte) {
	items, err := parseBookSourceBytes(content)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	urls := make([]string, 0, len(items))
	for _, item := range items {
		urls = append(urls, item.BookSourceURL)
	}
	existing, err := database.ExistingBookSourceURLs(urls)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "检查书源失败"})
		return
	}

	previews := make([]any, 0, len(items))
	for index, item := range items {
		previews = append(previews, map[string]any{
			"key":             bookSourcePreviewKey(item, index),
			"bookSourceName":  item.BookSourceName,
			"bookSourceUrl":   item.BookSourceURL,
			"bookSourceGroup": item.BookSourceGroup,
			"enabled":         item.Enabled,
			"exists":          existing[item.BookSourceURL],
			"raw":             json.RawMessage(item.RawJSON),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": previews, "total": len(previews)})
}

func parseBookSourceBytes(content []byte) ([]db.BookSourceImportItem, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil, errors.New("书源文件为空")
	}

	var rawItems []json.RawMessage
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &rawItems); err != nil {
			return nil, errors.New("书源 JSON 格式不正确")
		}
	} else if trimmed[0] == '{' {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, errors.New("书源 JSON 格式不正确")
		}
		if raw, ok := envelope["bookSources"]; ok {
			if err := json.Unmarshal(raw, &rawItems); err != nil {
				return nil, errors.New("bookSources 格式不正确")
			}
		} else if _, ok := envelope["bookSourceUrl"]; ok {
			rawItems = []json.RawMessage{append(json.RawMessage(nil), trimmed...)}
		} else {
			return nil, errors.New("未识别到书源数组")
		}
	} else {
		return nil, errors.New("书源文件必须是 JSON")
	}
	return parseBookSourceRawItems(rawItems)
}

func parseBookSourceRawItems(rawItems []json.RawMessage) ([]db.BookSourceImportItem, error) {
	items := make([]db.BookSourceImportItem, 0, len(rawItems))
	for index, raw := range rawItems {
		item, err := parseBookSourceRawItem(raw, index)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, errors.New("未解析到书源")
	}
	return items, nil
}

func parseBookSourceRawItem(raw json.RawMessage, index int) (db.BookSourceImportItem, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return db.BookSourceImportItem{}, fmt.Errorf("第 %d 个书源格式不正确", index+1)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return db.BookSourceImportItem{}, fmt.Errorf("第 %d 个书源格式不正确", index+1)
	}
	bookSourceURL := sourceFieldText(fields, "bookSourceUrl")
	if strings.TrimSpace(bookSourceURL) == "" {
		return db.BookSourceImportItem{}, fmt.Errorf("第 %d 个书源缺少 bookSourceUrl", index+1)
	}
	return db.BookSourceImportItem{
		BookSourceURL:     bookSourceURL,
		BookSourceName:    sourceFieldText(fields, "bookSourceName"),
		BookSourceGroup:   sourceFieldOptionalText(fields, "bookSourceGroup"),
		BookSourceType:    sourceFieldInt(fields, "bookSourceType"),
		BookURLPattern:    sourceFieldOptionalText(fields, "bookUrlPattern"),
		CustomOrder:       sourceFieldInt(fields, "customOrder"),
		Enabled:           sourceFieldBool(fields, "enabled", true),
		EnabledExplore:    sourceFieldBool(fields, "enabledExplore", true),
		JSLib:             sourceFieldOptionalText(fields, "jsLib"),
		EnabledCookieJar:  sourceFieldBool(fields, "enabledCookieJar", false),
		ConcurrentRate:    sourceFieldOptionalText(fields, "concurrentRate"),
		Header:            sourceFieldOptionalText(fields, "header"),
		LoginURL:          sourceFieldOptionalText(fields, "loginUrl"),
		LoginUI:           sourceFieldOptionalText(fields, "loginUi"),
		LoginCheckJS:      sourceFieldOptionalText(fields, "loginCheckJs"),
		CoverDecodeJS:     sourceFieldOptionalText(fields, "coverDecodeJs"),
		BookSourceComment: sourceFieldOptionalText(fields, "bookSourceComment"),
		VariableComment:   sourceFieldOptionalText(fields, "variableComment"),
		LastUpdateTime:    sourceFieldInt(fields, "lastUpdateTime"),
		RespondTime:       sourceFieldIntDefault(fields, "respondTime", 180000),
		Weight:            sourceFieldInt(fields, "weight"),
		ExploreURL:        sourceFieldOptionalText(fields, "exploreUrl"),
		ExploreScreen:     sourceFieldOptionalText(fields, "exploreScreen"),
		SearchURL:         sourceFieldOptionalText(fields, "searchUrl"),
		RuleSearch:        sourceFieldOptionalText(fields, "ruleSearch"),
		RuleExplore:       sourceFieldOptionalText(fields, "ruleExplore"),
		RuleBookInfo:      sourceFieldOptionalText(fields, "ruleBookInfo"),
		RuleToc:           sourceFieldOptionalText(fields, "ruleToc"),
		RuleContent:       sourceFieldOptionalText(fields, "ruleContent"),
		RuleReview:        sourceFieldOptionalText(fields, "ruleReview"),
		RawJSON:           string(trimmed),
	}, nil
}

func sourceFieldText(fields map[string]json.RawMessage, key string) string {
	raw, ok := fields[key]
	if !ok {
		return ""
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err == nil {
		return value
	}
	return string(trimmed)
}

func sourceFieldOptionalText(fields map[string]json.RawMessage, key string) *string {
	raw, ok := fields[key]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err == nil {
		return &value
	}
	value = string(trimmed)
	return &value
}

func sourceFieldInt(fields map[string]json.RawMessage, key string) int64 {
	raw, ok := fields[key]
	if !ok {
		return 0
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		if value, parseErr := number.Int64(); parseErr == nil {
			return value
		}
		if value, parseErr := strconv.ParseFloat(number.String(), 64); parseErr == nil {
			return int64(value)
		}
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		value, parseErr := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if parseErr == nil {
			return value
		}
	}
	var boolean bool
	if err := json.Unmarshal(trimmed, &boolean); err == nil && boolean {
		return 1
	}
	return 0
}

func sourceFieldIntDefault(fields map[string]json.RawMessage, key string, defaultValue int64) int64 {
	if _, ok := fields[key]; !ok {
		return defaultValue
	}
	return sourceFieldInt(fields, key)
}

func sourceFieldBool(fields map[string]json.RawMessage, key string, defaultValue bool) bool {
	raw, ok := fields[key]
	if !ok {
		return defaultValue
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return defaultValue
	}
	var value bool
	if err := json.Unmarshal(trimmed, &value); err == nil {
		return value
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err == nil {
		return number.String() != "0"
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		normalized := strings.ToLower(strings.TrimSpace(text))
		return normalized == "true" || normalized == "1" || normalized == "yes"
	}
	return defaultValue
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, errors.New("读取书源失败")
	}
	if int64(len(content)) > limit {
		return nil, errors.New("书源文件过大")
	}
	return content, nil
}

func bookSourcePreviewKey(item db.BookSourceImportItem, index int) string {
	if strings.TrimSpace(item.BookSourceURL) != "" {
		return item.BookSourceURL
	}
	return fmt.Sprintf("source-%d", index+1)
}

func bookSourceDetailPayload(row db.BookSourceRow) map[string]any {
	payload := bookSourcePayload(row)
	payload["bookUrlPattern"] = row.BookURLPattern
	payload["jsLib"] = row.JSLib
	payload["concurrentRate"] = row.ConcurrentRate
	payload["header"] = row.Header
	payload["loginUrl"] = row.LoginURL
	payload["loginUi"] = row.LoginUI
	payload["loginCheckJs"] = row.LoginCheckJS
	payload["coverDecodeJs"] = row.CoverDecodeJS
	payload["bookSourceComment"] = row.BookSourceComment
	payload["variableComment"] = row.VariableComment
	payload["exploreUrl"] = row.ExploreURL
	payload["exploreScreen"] = row.ExploreScreen
	payload["searchUrl"] = row.SearchURL
	payload["ruleSearch"] = row.RuleSearch
	payload["ruleExplore"] = row.RuleExplore
	payload["ruleBookInfo"] = row.RuleBookInfo
	payload["ruleToc"] = row.RuleToc
	payload["ruleContent"] = row.RuleContent
	payload["ruleReview"] = row.RuleReview
	return payload
}

func bookSourcePayload(row db.BookSourceRow) map[string]any {
	return map[string]any{
		"bookSourceUrl":    row.BookSourceURL,
		"bookSourceName":   row.BookSourceName,
		"bookSourceGroup":  row.BookSourceGroup,
		"bookSourceType":   row.BookSourceType,
		"customOrder":      row.CustomOrder,
		"enabled":          row.Enabled,
		"enabledExplore":   row.EnabledExplore,
		"enabledCookieJar": row.EnabledCookieJar,
		"lastUpdateTime":   row.LastUpdateTime,
		"respondTime":      row.RespondTime,
		"weight":           row.Weight,
		"createdAt":        row.CreatedAt,
		"updatedAt":        row.UpdatedAt,
	}
}
