package legado

import "github.com/jenfonro/reader/internal/db"

type Source struct {
	BookSourceURL     string
	BookSourceName    string
	BookSourceGroup   string
	BookSourceType    int64
	BookURLPattern    string
	CustomOrder       int64
	Enabled           bool
	EnabledExplore    bool
	JSLib             string
	EnabledCookieJar  bool
	ConcurrentRate    string
	Header            string
	LoginURL          string
	LoginUI           string
	LoginCheckJS      string
	CoverDecodeJS     string
	BookSourceComment string
	VariableComment   string
	LastUpdateTime    int64
	RespondTime       int64
	Weight            int64
	ExploreURL        string
	ExploreScreen     string
	SearchURL         string
	RuleSearch        string
	RuleExplore       string
	RuleBookInfo      string
	RuleToc           string
	RuleContent       string
	RuleReview        string
	RawJSON           string
}

type SearchOptions struct {
	Keyword string
	Page    int
}

type ExploreOptions struct {
	URL  string
	Page int
}

type BookInfoOptions struct {
	BookURL   string
	Book      Book
	CanReName bool
}

type TocOptions struct {
	TocURL string
	Book   Book
}

type ContentOptions struct {
	ChapterURL string
	Book       Book
	Chapter    Chapter
}

type SearchResult struct {
	Source SourceSummary `json:"source"`
	Books  []Book        `json:"books"`
}

type ExploreResult struct {
	Source SourceSummary `json:"source"`
	URL    string        `json:"url"`
	Books  []Book        `json:"books"`
}

type SourceSummary struct {
	BookSourceURL   string `json:"bookSourceUrl"`
	BookSourceName  string `json:"bookSourceName"`
	BookSourceGroup string `json:"bookSourceGroup,omitempty"`
	BookSourceType  int64  `json:"bookSourceType"`
	CustomOrder     int64  `json:"customOrder"`
	Enabled         bool   `json:"enabled"`
	EnabledExplore  bool   `json:"enabledExplore"`
	LastUpdateTime  int64  `json:"lastUpdateTime"`
	RespondTime     int64  `json:"respondTime"`
	Weight          int64  `json:"weight"`
}

type Book struct {
	SourceURL    string         `json:"sourceUrl,omitempty"`
	SourceName   string         `json:"sourceName,omitempty"`
	Name         string         `json:"name,omitempty"`
	Author       string         `json:"author,omitempty"`
	BookURL      string         `json:"bookUrl,omitempty"`
	CoverURL     string         `json:"coverUrl,omitempty"`
	Intro        string         `json:"intro,omitempty"`
	Kind         string         `json:"kind,omitempty"`
	LastChapter  string         `json:"lastChapter,omitempty"`
	UpdateTime   string         `json:"updateTime,omitempty"`
	TocURL       string         `json:"tocUrl,omitempty"`
	WordCount    string         `json:"wordCount,omitempty"`
	DownloadURLs string         `json:"downloadUrls,omitempty"`
	CheckKeyword string         `json:"checkKeyword,omitempty"`
	InfoHTML     string         `json:"infoHtml,omitempty"`
	TocHTML      string         `json:"tocHtml,omitempty"`
	Extra        map[string]any `json:"extra,omitempty"`
}

type TocResult struct {
	Source   SourceSummary `json:"source"`
	TocURL   string        `json:"tocUrl"`
	Chapters []Chapter     `json:"chapters"`
}

type Chapter struct {
	Name       string `json:"name,omitempty"`
	URL        string `json:"url,omitempty"`
	IsVolume   bool   `json:"isVolume,omitempty"`
	IsVIP      bool   `json:"isVip,omitempty"`
	UpdateTime string `json:"updateTime,omitempty"`
}

type ContentResult struct {
	Source     SourceSummary `json:"source"`
	ChapterURL string        `json:"chapterUrl"`
	Content    string        `json:"content"`
	ImageStyle string        `json:"imageStyle,omitempty"`
	NextURLs   []string      `json:"nextUrls,omitempty"`
}

type NeedVerificationError struct {
	URL     string
	Message string
}

func (e NeedVerificationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "need verification"
}

func SourceFromDB(row db.BookSourceRow) Source {
	return Source{
		BookSourceURL:     row.BookSourceURL,
		BookSourceName:    row.BookSourceName,
		BookSourceGroup:   deref(row.BookSourceGroup),
		BookSourceType:    row.BookSourceType,
		BookURLPattern:    deref(row.BookURLPattern),
		CustomOrder:       row.CustomOrder,
		Enabled:           row.Enabled,
		EnabledExplore:    row.EnabledExplore,
		JSLib:             deref(row.JSLib),
		EnabledCookieJar:  row.EnabledCookieJar,
		ConcurrentRate:    deref(row.ConcurrentRate),
		Header:            deref(row.Header),
		LoginURL:          deref(row.LoginURL),
		LoginUI:           deref(row.LoginUI),
		LoginCheckJS:      deref(row.LoginCheckJS),
		CoverDecodeJS:     deref(row.CoverDecodeJS),
		BookSourceComment: deref(row.BookSourceComment),
		VariableComment:   deref(row.VariableComment),
		LastUpdateTime:    row.LastUpdateTime,
		RespondTime:       row.RespondTime,
		Weight:            row.Weight,
		ExploreURL:        deref(row.ExploreURL),
		ExploreScreen:     deref(row.ExploreScreen),
		SearchURL:         deref(row.SearchURL),
		RuleSearch:        deref(row.RuleSearch),
		RuleExplore:       deref(row.RuleExplore),
		RuleBookInfo:      deref(row.RuleBookInfo),
		RuleToc:           deref(row.RuleToc),
		RuleContent:       deref(row.RuleContent),
		RuleReview:        deref(row.RuleReview),
		RawJSON:           row.RawJSON,
	}
}

func (s Source) Summary() SourceSummary {
	return SourceSummary{
		BookSourceURL:   s.BookSourceURL,
		BookSourceName:  s.BookSourceName,
		BookSourceGroup: s.BookSourceGroup,
		BookSourceType:  s.BookSourceType,
		CustomOrder:     s.CustomOrder,
		Enabled:         s.Enabled,
		EnabledExplore:  s.EnabledExplore,
		LastUpdateTime:  s.LastUpdateTime,
		RespondTime:     s.RespondTime,
		Weight:          s.Weight,
	}
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
