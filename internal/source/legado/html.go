package legado

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"
)

type htmlNode struct {
	HTML string
}

func (n htmlNode) String() string { return n.HTML }

func (rc *runContext) evalHTMLRule(rule string, input any) (any, error) {
	rule = strings.TrimSpace(strings.TrimLeft(rule, "@"))
	if rule == "" {
		return "", nil
	}
	selection, err := selectionFromInput(input)
	if err != nil {
		return "", err
	}
	segments := splitHTMLSegments(rule)
	if len(segments) == 0 {
		return "", nil
	}
	current := selection
	for index, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		last := index == len(segments)-1
		if isTerminalSegment(segment) {
			return terminalValue(current, segment), nil
		}
		current = applyHTMLSegment(current, segment)
		if last {
			return nodesFromSelection(current), nil
		}
	}
	return nodesFromSelection(current), nil
}

func evalXPath(rule string, input any) any {
	content := htmlString(input)
	if strings.TrimSpace(content) == "" {
		return ""
	}
	doc, err := htmlquery.Parse(strings.NewReader(content))
	if err != nil {
		return ""
	}
	nodes, err := htmlquery.QueryAll(doc, rule)
	if err != nil || len(nodes) == 0 {
		return ""
	}
	values := make([]any, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		switch node.Type {
		case html.TextNode:
			values = append(values, strings.TrimSpace(node.Data))
		case html.ElementNode:
			values = append(values, htmlNode{HTML: outerHTML(node)})
		default:
			text := strings.TrimSpace(node.Data)
			if text != "" {
				values = append(values, text)
			}
		}
	}
	if len(values) == 1 {
		return values[0]
	}
	return values
}

func selectionFromInput(input any) (*goquery.Selection, error) {
	content := htmlString(input)
	if strings.TrimSpace(content) == "" {
		content = "<html></html>"
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return nil, err
	}
	switch input.(type) {
	case htmlNode:
		first := doc.Selection.Find("body").Children().First()
		if first.Length() == 0 {
			first = doc.Selection.Find("*").First()
		}
		return first, nil
	}
	return doc.Selection, nil
}

func htmlString(input any) string {
	switch v := input.(type) {
	case nil:
		return ""
	case string:
		return v
	case htmlNode:
		return v.HTML
	case []htmlNode:
		var b strings.Builder
		for _, node := range v {
			b.WriteString(node.HTML)
			b.WriteByte('\n')
		}
		return b.String()
	case []any:
		var b strings.Builder
		for _, item := range v {
			b.WriteString(htmlString(item))
			b.WriteByte('\n')
		}
		return b.String()
	default:
		return anyString(v)
	}
}

func splitHTMLSegments(rule string) []string {
	parts := strings.Split(rule, "@")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func isTerminalSegment(segment string) bool {
	segment = strings.TrimSpace(segment)
	if strings.HasPrefix(segment, "data-") {
		return true
	}
	switch segment {
	case "text", "textNodes", "ownText", "html", "href", "src", "content", "children":
		return true
	}
	return false
}

func applyHTMLSegment(selection *goquery.Selection, segment string) *goquery.Selection {
	segment = normalizeHTMLSegment(segment)
	if segment == "children" {
		return selection.Children()
	}
	if strings.HasPrefix(segment, "text.") {
		needle := strings.TrimSpace(strings.TrimPrefix(segment, "text."))
		matched := selection.Find("*").FilterFunction(func(_ int, item *goquery.Selection) bool {
			return strings.Contains(strings.TrimSpace(item.Text()), needle)
		})
		if matched.Length() == 0 && strings.Contains(strings.TrimSpace(selection.Text()), needle) {
			return selection
		}
		return matched
	}
	selector, index, hasIndex := segmentSelector(segment)
	if strings.TrimSpace(selector) == "" {
		return selection
	}
	matched := selection.Find(selector)
	if matched.Length() == 0 && selection.Is(selector) {
		matched = selection.Filter(selector)
	}
	if hasIndex {
		if index < 0 {
			index = matched.Length() + index
		}
		if index < 0 || index >= matched.Length() {
			return matched.Slice(0, 0)
		}
		return matched.Eq(index)
	}
	return matched
}

func normalizeHTMLSegment(segment string) string {
	segment = strings.TrimSpace(strings.TrimLeft(segment, "@"))
	segment = strings.ReplaceAll(segment, "!0", "")
	if strings.HasPrefix(segment, "children") {
		return "children"
	}
	if strings.HasPrefix(segment, "tag.") || strings.HasPrefix(segment, "class.") || strings.HasPrefix(segment, "id.") || strings.HasPrefix(segment, "text.") {
		return segment
	}
	return segment
}

func segmentSelector(segment string) (string, int, bool) {
	if strings.HasPrefix(segment, "text.") {
		return segment, 0, false
	}
	if strings.HasPrefix(segment, "tag.") {
		name := strings.TrimPrefix(segment, "tag.")
		selector, index, ok := selectorIndex(name)
		return selector, index, ok
	}
	if strings.HasPrefix(segment, "class.") {
		name := strings.TrimPrefix(segment, "class.")
		selector, index, ok := selectorIndex(name)
		return "." + selector, index, ok
	}
	if strings.HasPrefix(segment, "id.") {
		name := strings.TrimPrefix(segment, "id.")
		selector, index, ok := selectorIndex(name)
		return "#" + selector, index, ok
	}
	return selectorIndex(segment)
}

func selectorIndex(raw string) (string, int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, false
	}
	if strings.Contains(raw, "[") && strings.HasSuffix(raw, "]") {
		left := raw[:strings.LastIndex(raw, "[")]
		inside := raw[strings.LastIndex(raw, "[")+1 : len(raw)-1]
		if strings.Contains(inside, ":") || strings.Contains(inside, "!") {
			return left, 0, false
		}
		if n, err := strconv.Atoi(inside); err == nil {
			return left, n, true
		}
	}
	lastDot := strings.LastIndex(raw, ".")
	if lastDot > 0 && lastDot < len(raw)-1 {
		suffix := raw[lastDot+1:]
		if n, err := strconv.Atoi(suffix); err == nil {
			selector := raw[:lastDot]
			return selector, n, true
		}
	}
	return raw, 0, false
}

func terminalValue(selection *goquery.Selection, terminal string) any {
	terminal = strings.TrimSpace(terminal)
	if terminal == "children" {
		return nodesFromSelection(selection.Children())
	}
	values := make([]any, 0, selection.Length())
	selection.Each(func(_ int, item *goquery.Selection) {
		switch terminal {
		case "text", "textNodes":
			values = append(values, strings.TrimSpace(item.Text()))
		case "ownText":
			values = append(values, strings.TrimSpace(ownText(item)))
		case "html":
			htmlValue, _ := item.Html()
			values = append(values, strings.TrimSpace(htmlValue))
		case "href", "src", "content":
			value, _ := item.Attr(terminal)
			values = append(values, strings.TrimSpace(value))
		default:
			if strings.HasPrefix(terminal, "data-") {
				value, _ := item.Attr(terminal)
				values = append(values, strings.TrimSpace(value))
			}
		}
	})
	cleaned := make([]any, 0, len(values))
	for _, value := range values {
		if text := anyString(value); strings.TrimSpace(text) != "" {
			cleaned = append(cleaned, text)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 {
		return cleaned[0]
	}
	parts := make([]string, 0, len(cleaned))
	for _, value := range cleaned {
		parts = append(parts, anyString(value))
	}
	return strings.Join(parts, "\n")
}

func ownText(selection *goquery.Selection) string {
	var parts []string
	selection.Contents().Each(func(_ int, item *goquery.Selection) {
		for _, node := range item.Nodes {
			if node.Type == html.TextNode {
				if text := strings.TrimSpace(node.Data); text != "" {
					parts = append(parts, text)
				}
			}
		}
	})
	return strings.Join(parts, "\n")
}

func nodesFromSelection(selection *goquery.Selection) []htmlNode {
	nodes := make([]htmlNode, 0, selection.Length())
	selection.Each(func(_ int, item *goquery.Selection) {
		for _, node := range item.Nodes {
			nodes = append(nodes, htmlNode{HTML: outerHTML(node)})
		}
	})
	return nodes
}

func outerHTML(node *html.Node) string {
	if node == nil {
		return ""
	}
	var b bytes.Buffer
	if err := html.Render(&b, node); err != nil {
		return fmt.Sprint(node.Data)
	}
	return b.String()
}
