package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type BookSourceRow struct {
	BookSourceURL     string
	BookSourceName    string
	BookSourceGroup   *string
	BookSourceType    int64
	BookURLPattern    *string
	CustomOrder       int64
	Enabled           bool
	EnabledExplore    bool
	JSLib             *string
	EnabledCookieJar  bool
	ConcurrentRate    *string
	Header            *string
	LoginURL          *string
	LoginUI           *string
	LoginCheckJS      *string
	CoverDecodeJS     *string
	BookSourceComment *string
	VariableComment   *string
	LastUpdateTime    int64
	RespondTime       int64
	Weight            int64
	ExploreURL        *string
	ExploreScreen     *string
	SearchURL         *string
	RuleSearch        *string
	RuleExplore       *string
	RuleBookInfo      *string
	RuleToc           *string
	RuleContent       *string
	RuleReview        *string
	RawJSON           string
	CreatedAt         int64
	UpdatedAt         int64
}

type BookSourceImportItem struct {
	BookSourceURL     string
	BookSourceName    string
	BookSourceGroup   *string
	BookSourceType    int64
	BookURLPattern    *string
	CustomOrder       int64
	Enabled           bool
	EnabledExplore    bool
	JSLib             *string
	EnabledCookieJar  bool
	ConcurrentRate    *string
	Header            *string
	LoginURL          *string
	LoginUI           *string
	LoginCheckJS      *string
	CoverDecodeJS     *string
	BookSourceComment *string
	VariableComment   *string
	LastUpdateTime    int64
	RespondTime       int64
	Weight            int64
	ExploreURL        *string
	ExploreScreen     *string
	SearchURL         *string
	RuleSearch        *string
	RuleExplore       *string
	RuleBookInfo      *string
	RuleToc           *string
	RuleContent       *string
	RuleReview        *string
	RawJSON           string
}

type BookSourceImportResult struct {
	Created int
	Updated int
}

var ErrBookSourceURLExists = errors.New("book source url exists")

const bookSourceURLQueryBatchSize = 500

func (d *DB) ListBookSources() ([]BookSourceRow, error) {
	if d == nil || d.db == nil {
		return nil, errors.New("db nil")
	}
	rows, err := d.db.Query(`
		SELECT book_source_url, book_source_name, book_source_group, book_source_type,
			custom_order, enabled, enabled_explore, enabled_cookie_jar,
			last_update_time, respond_time, weight, created_at, updated_at
		FROM book_sources
		ORDER BY custom_order ASC, book_source_name COLLATE NOCASE ASC, book_source_url ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]BookSourceRow, 0)
	for rows.Next() {
		var row BookSourceRow
		var bookSourceGroup sql.NullString
		var enabled, enabledExplore, enabledCookieJar int64
		if err := rows.Scan(
			&row.BookSourceURL,
			&row.BookSourceName,
			&bookSourceGroup,
			&row.BookSourceType,
			&row.CustomOrder,
			&enabled,
			&enabledExplore,
			&enabledCookieJar,
			&row.LastUpdateTime,
			&row.RespondTime,
			&row.Weight,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		row.BookSourceGroup = nullableString(bookSourceGroup)
		row.Enabled = enabled != 0
		row.EnabledExplore = enabledExplore != 0
		row.EnabledCookieJar = enabledCookieJar != 0
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (d *DB) GetBookSourceByURL(bookSourceURL string) (BookSourceRow, error) {
	if d == nil || d.db == nil {
		return BookSourceRow{}, errors.New("db nil")
	}
	url := strings.TrimSpace(bookSourceURL)
	if url == "" {
		return BookSourceRow{}, errors.New("book source url empty")
	}

	var row BookSourceRow
	var bookSourceGroup, bookURLPattern, jsLib, concurrentRate, header sql.NullString
	var loginURL, loginUI, loginCheckJS, coverDecodeJS sql.NullString
	var bookSourceComment, variableComment, exploreURL, exploreScreen sql.NullString
	var searchURL, ruleSearch, ruleExplore, ruleBookInfo, ruleToc, ruleContent, ruleReview sql.NullString
	var enabled, enabledExplore, enabledCookieJar int64
	err := d.db.QueryRow(`
		SELECT book_source_url, book_source_name, book_source_group, book_source_type, book_url_pattern,
			custom_order, enabled, enabled_explore, js_lib, enabled_cookie_jar, concurrent_rate, header,
			login_url, login_ui, login_check_js, cover_decode_js, book_source_comment, variable_comment,
			last_update_time, respond_time, weight, explore_url, explore_screen, search_url, rule_search,
			rule_explore, rule_book_info, rule_toc, rule_content, rule_review, created_at, updated_at
		FROM book_sources
		WHERE book_source_url=?
		LIMIT 1
	`, url).Scan(
		&row.BookSourceURL,
		&row.BookSourceName,
		&bookSourceGroup,
		&row.BookSourceType,
		&bookURLPattern,
		&row.CustomOrder,
		&enabled,
		&enabledExplore,
		&jsLib,
		&enabledCookieJar,
		&concurrentRate,
		&header,
		&loginURL,
		&loginUI,
		&loginCheckJS,
		&coverDecodeJS,
		&bookSourceComment,
		&variableComment,
		&row.LastUpdateTime,
		&row.RespondTime,
		&row.Weight,
		&exploreURL,
		&exploreScreen,
		&searchURL,
		&ruleSearch,
		&ruleExplore,
		&ruleBookInfo,
		&ruleToc,
		&ruleContent,
		&ruleReview,
		&row.CreatedAt,
		&row.UpdatedAt,
	)
	if err != nil {
		return BookSourceRow{}, err
	}
	row.BookSourceGroup = nullableString(bookSourceGroup)
	row.BookURLPattern = nullableString(bookURLPattern)
	row.JSLib = nullableString(jsLib)
	row.Enabled = enabled != 0
	row.EnabledExplore = enabledExplore != 0
	row.EnabledCookieJar = enabledCookieJar != 0
	row.ConcurrentRate = nullableString(concurrentRate)
	row.Header = nullableString(header)
	row.LoginURL = nullableString(loginURL)
	row.LoginUI = nullableString(loginUI)
	row.LoginCheckJS = nullableString(loginCheckJS)
	row.CoverDecodeJS = nullableString(coverDecodeJS)
	row.BookSourceComment = nullableString(bookSourceComment)
	row.VariableComment = nullableString(variableComment)
	row.ExploreURL = nullableString(exploreURL)
	row.ExploreScreen = nullableString(exploreScreen)
	row.SearchURL = nullableString(searchURL)
	row.RuleSearch = nullableString(ruleSearch)
	row.RuleExplore = nullableString(ruleExplore)
	row.RuleBookInfo = nullableString(ruleBookInfo)
	row.RuleToc = nullableString(ruleToc)
	row.RuleContent = nullableString(ruleContent)
	row.RuleReview = nullableString(ruleReview)
	return row, nil
}

func (d *DB) ExistingBookSourceURLs(urls []string) (map[string]bool, error) {
	if d == nil || d.db == nil {
		return nil, errors.New("db nil")
	}
	existing := make(map[string]bool)
	uniqueURLs := uniqueBookSourceURLs(urls)
	for start := 0; start < len(uniqueURLs); start += bookSourceURLQueryBatchSize {
		end := start + bookSourceURLQueryBatchSize
		if end > len(uniqueURLs) {
			end = len(uniqueURLs)
		}
		batch := uniqueURLs[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, len(batch))
		for index, url := range batch {
			args[index] = url
		}
		rows, err := d.db.Query(`SELECT book_source_url FROM book_sources WHERE book_source_url IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var url string
			if err := rows.Scan(&url); err != nil {
				rows.Close()
				return nil, err
			}
			existing[url] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return existing, nil
}

func (d *DB) ImportBookSources(items []BookSourceImportItem) (BookSourceImportResult, error) {
	if d == nil || d.db == nil {
		return BookSourceImportResult{}, errors.New("db nil")
	}
	if len(items) == 0 {
		return BookSourceImportResult{}, nil
	}

	urls := make([]string, 0, len(items))
	for _, item := range items {
		urls = append(urls, item.BookSourceURL)
	}
	existing, err := d.ExistingBookSourceURLs(urls)
	if err != nil {
		return BookSourceImportResult{}, err
	}

	tx, err := d.db.Begin()
	if err != nil {
		return BookSourceImportResult{}, err
	}
	defer tx.Rollback()

	stmt, err := prepareBookSourceUpsert(tx)
	if err != nil {
		return BookSourceImportResult{}, err
	}
	defer stmt.Close()

	result := BookSourceImportResult{}
	now := time.Now().Unix()
	seenInBatch := make(map[string]bool)
	for _, item := range items {
		url := strings.TrimSpace(item.BookSourceURL)
		if url == "" || item.RawJSON == "" {
			continue
		}
		if err := execBookSourceUpsert(stmt, item, now, now); err != nil {
			return BookSourceImportResult{}, err
		}
		if existing[url] || seenInBatch[url] {
			result.Updated++
		} else {
			result.Created++
		}
		seenInBatch[url] = true
	}
	if err := tx.Commit(); err != nil {
		return BookSourceImportResult{}, err
	}
	return result, nil
}

func (d *DB) SaveBookSource(originalURL string, item BookSourceImportItem) (BookSourceRow, bool, error) {
	if d == nil || d.db == nil {
		return BookSourceRow{}, false, errors.New("db nil")
	}
	url := strings.TrimSpace(item.BookSourceURL)
	if url == "" || item.RawJSON == "" {
		return BookSourceRow{}, false, errors.New("invalid book source")
	}
	original := strings.TrimSpace(originalURL)

	tx, err := d.db.Begin()
	if err != nil {
		return BookSourceRow{}, false, err
	}
	defer tx.Rollback()

	created := true
	if original != "" && original != url {
		var exists string
		err := tx.QueryRow(`SELECT book_source_url FROM book_sources WHERE book_source_url=? LIMIT 1`, url).Scan(&exists)
		switch {
		case err == nil:
			return BookSourceRow{}, false, ErrBookSourceURLExists
		case !errors.Is(err, sql.ErrNoRows):
			return BookSourceRow{}, false, err
		}
		result, err := tx.Exec(`DELETE FROM book_sources WHERE book_source_url=?`, original)
		if err != nil {
			return BookSourceRow{}, false, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return BookSourceRow{}, false, err
		}
		created = affected == 0
	} else {
		var existingURL string
		err := tx.QueryRow(`SELECT book_source_url FROM book_sources WHERE book_source_url=? LIMIT 1`, url).Scan(&existingURL)
		switch {
		case err == nil:
			created = false
		case errors.Is(err, sql.ErrNoRows):
			created = true
		default:
			return BookSourceRow{}, false, err
		}
	}

	stmt, err := prepareBookSourceUpsert(tx)
	if err != nil {
		return BookSourceRow{}, false, err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	if err := execBookSourceUpsert(stmt, item, now, now); err != nil {
		return BookSourceRow{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return BookSourceRow{}, false, err
	}
	row, err := d.GetBookSourceByURL(url)
	return row, created, err
}

func (d *DB) DeleteBookSources(urls []string) (int64, error) {
	if d == nil || d.db == nil {
		return 0, errors.New("db nil")
	}
	uniqueURLs := uniqueBookSourceURLs(urls)
	if len(uniqueURLs) == 0 {
		return 0, nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var deleted int64
	for start := 0; start < len(uniqueURLs); start += bookSourceURLQueryBatchSize {
		end := start + bookSourceURLQueryBatchSize
		if end > len(uniqueURLs) {
			end = len(uniqueURLs)
		}
		batch := uniqueURLs[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		args := make([]any, len(batch))
		for index, url := range batch {
			args[index] = url
		}
		result, err := tx.Exec(`DELETE FROM book_sources WHERE book_source_url IN (`+placeholders+`)`, args...)
		if err != nil {
			return 0, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += affected
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (d *DB) UpdateBookSourceEnabled(bookSourceURL string, enabled bool) (BookSourceRow, error) {
	if d == nil || d.db == nil {
		return BookSourceRow{}, errors.New("db nil")
	}
	url := strings.TrimSpace(bookSourceURL)
	if url == "" {
		return BookSourceRow{}, sql.ErrNoRows
	}

	var rawJSON string
	if err := d.db.QueryRow(`SELECT raw_json FROM book_sources WHERE book_source_url=? LIMIT 1`, url).Scan(&rawJSON); err != nil {
		return BookSourceRow{}, err
	}
	updatedRawJSON := rawJSON
	var rawMap map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &rawMap); err == nil && rawMap != nil {
		rawMap["enabled"] = enabled
		if content, err := json.Marshal(rawMap); err == nil {
			updatedRawJSON = string(content)
		}
	}

	result, err := d.db.Exec(
		`UPDATE book_sources SET enabled=?, raw_json=?, updated_at=? WHERE book_source_url=?`,
		boolToInt(enabled),
		updatedRawJSON,
		time.Now().Unix(),
		url,
	)
	if err != nil {
		return BookSourceRow{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return BookSourceRow{}, err
	}
	if affected == 0 {
		return BookSourceRow{}, sql.ErrNoRows
	}
	return d.GetBookSourceByURL(url)
}

func prepareBookSourceUpsert(tx *sql.Tx) (*sql.Stmt, error) {
	return tx.Prepare(`
		INSERT INTO book_sources(
			book_source_url, book_source_name, book_source_group, book_source_type, book_url_pattern,
			custom_order, enabled, enabled_explore, js_lib, enabled_cookie_jar, concurrent_rate, header,
			login_url, login_ui, login_check_js, cover_decode_js, book_source_comment, variable_comment,
			last_update_time, respond_time, weight, explore_url, explore_screen, search_url, rule_search,
			rule_explore, rule_book_info, rule_toc, rule_content, rule_review, raw_json, created_at, updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(book_source_url) DO UPDATE SET
			book_source_name=excluded.book_source_name,
			book_source_group=excluded.book_source_group,
			book_source_type=excluded.book_source_type,
			book_url_pattern=excluded.book_url_pattern,
			custom_order=excluded.custom_order,
			enabled=excluded.enabled,
			enabled_explore=excluded.enabled_explore,
			js_lib=excluded.js_lib,
			enabled_cookie_jar=excluded.enabled_cookie_jar,
			concurrent_rate=excluded.concurrent_rate,
			header=excluded.header,
			login_url=excluded.login_url,
			login_ui=excluded.login_ui,
			login_check_js=excluded.login_check_js,
			cover_decode_js=excluded.cover_decode_js,
			book_source_comment=excluded.book_source_comment,
			variable_comment=excluded.variable_comment,
			last_update_time=excluded.last_update_time,
			respond_time=excluded.respond_time,
			weight=excluded.weight,
			explore_url=excluded.explore_url,
			explore_screen=excluded.explore_screen,
			search_url=excluded.search_url,
			rule_search=excluded.rule_search,
			rule_explore=excluded.rule_explore,
			rule_book_info=excluded.rule_book_info,
			rule_toc=excluded.rule_toc,
			rule_content=excluded.rule_content,
			rule_review=excluded.rule_review,
			raw_json=excluded.raw_json,
			updated_at=excluded.updated_at
	`)
}

func execBookSourceUpsert(stmt *sql.Stmt, item BookSourceImportItem, createdAt, updatedAt int64) error {
	url := strings.TrimSpace(item.BookSourceURL)
	if url == "" || item.RawJSON == "" {
		return nil
	}
	name := strings.TrimSpace(item.BookSourceName)
	if name == "" {
		name = url
	}
	_, err := stmt.Exec(
		url,
		name,
		nullableTextArg(item.BookSourceGroup),
		item.BookSourceType,
		nullableTextArg(item.BookURLPattern),
		item.CustomOrder,
		boolToInt(item.Enabled),
		boolToInt(item.EnabledExplore),
		nullableTextArg(item.JSLib),
		boolToInt(item.EnabledCookieJar),
		nullableTextArg(item.ConcurrentRate),
		nullableTextArg(item.Header),
		nullableTextArg(item.LoginURL),
		nullableTextArg(item.LoginUI),
		nullableTextArg(item.LoginCheckJS),
		nullableTextArg(item.CoverDecodeJS),
		nullableTextArg(item.BookSourceComment),
		nullableTextArg(item.VariableComment),
		item.LastUpdateTime,
		item.RespondTime,
		item.Weight,
		nullableTextArg(item.ExploreURL),
		nullableTextArg(item.ExploreScreen),
		nullableTextArg(item.SearchURL),
		nullableTextArg(item.RuleSearch),
		nullableTextArg(item.RuleExplore),
		nullableTextArg(item.RuleBookInfo),
		nullableTextArg(item.RuleToc),
		nullableTextArg(item.RuleContent),
		nullableTextArg(item.RuleReview),
		item.RawJSON,
		createdAt,
		updatedAt,
	)
	return err
}

func uniqueBookSourceURLs(urls []string) []string {
	uniqueURLs := make([]string, 0, len(urls))
	seen := make(map[string]bool)
	for _, value := range urls {
		url := strings.TrimSpace(value)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		uniqueURLs = append(uniqueURLs, url)
	}
	return uniqueURLs
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullableTextArg(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
