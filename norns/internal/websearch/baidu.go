package websearch

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const baiduSearchURL = "https://www.baidu.com/s"

type baiduBackend struct{ endpoint string }

func newBaiduBackend() *baiduBackend { return &baiduBackend{endpoint: baiduSearchURL} }
func (b *baiduBackend) Name() string { return "baidu" }

func (b *baiduBackend) Search(ctx context.Context, client *http.Client, query string, maximum int) ([]Result, error) {
	results := make([]Result, 0, min(maximum, 10))
	seen := make(map[string]struct{}, min(maximum, 10))
	for page := 0; page < pageCount(maximum) && len(results) < maximum; page++ {
		params := url.Values{
			"wd": {query},
			"ie": {"utf-8"},
			"rn": {"10"},
			"pn": {strconv.Itoa(page * 10)},
		}
		headers := http.Header{
			"Accept-Language": {"zh-CN,zh;q=0.9,en;q=0.7"},
			"User-Agent":      {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"},
		}
		body, err := fetchHTML(ctx, client, b.endpoint, params, headers)
		if err != nil {
			return nil, fmt.Errorf("baidu search request failed: %w", err)
		}
		pageResults, err := parseBaiduResults(body)
		if err != nil {
			return nil, err
		}
		var added int
		results, added = appendPage(results, pageResults, seen, maximum)
		if len(pageResults) == 0 || added == 0 {
			break
		}
	}
	return results, nil
}

func parseBaiduResults(body []byte) ([]Result, error) {
	if bytes.Contains(body, []byte("百度安全验证")) {
		return nil, fmt.Errorf("Baidu security verification required")
	}
	root, err := parseHTML(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cannot parse Baidu results: %w", err)
	}
	results := []Result{}
	walk(root, func(node *html.Node) {
		if !isElement(node, "div") || !hasClass(node, "c-container") {
			return
		}
		heading := firstDescendant(node, func(candidate *html.Node) bool { return isElement(candidate, "h3") })
		if heading == nil {
			return
		}
		anchor := firstDescendant(heading, func(candidate *html.Node) bool { return isElement(candidate, "a") })
		if anchor == nil {
			return
		}
		href := strings.TrimSpace(attr(node, "mu"))
		if href == "" {
			href = strings.TrimSpace(attr(anchor, "href"))
		}
		if strings.HasPrefix(href, "//") {
			href = "https:" + href
		}
		if isBaiduRedirect(href) {
			return
		}
		bodyNode := firstDescendant(node, func(candidate *html.Node) bool {
			return attr(candidate, "data-module") == "abstract"
		})
		if bodyNode == nil {
			bodyNode = firstDescendant(node, func(candidate *html.Node) bool { return hasClass(candidate, "c-abstract") })
		}
		if bodyNode == nil {
			bodyNode = firstDescendant(node, func(candidate *html.Node) bool { return hasClass(candidate, "content-right") })
		}
		result := Result{Title: nodeText(anchor), Href: href}
		if bodyNode != nil {
			result.Body = nodeText(bodyNode)
		}
		results = append(results, result)
	})
	return results, nil
}

func isBaiduRedirect(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	return (host == "baidu.com" || host == "www.baidu.com") && parsed.Path == "/link"
}
