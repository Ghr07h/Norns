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

const googleSearchURL = "https://www.google.com/wml/search"

type googleBackend struct{ endpoint string }

func newGoogleBackend() *googleBackend { return &googleBackend{endpoint: googleSearchURL} }
func (g *googleBackend) Name() string  { return "google" }

func (g *googleBackend) Search(ctx context.Context, client *http.Client, query string, maximum int) ([]Result, error) {
	results := make([]Result, 0, min(maximum, 10))
	seen := make(map[string]struct{}, min(maximum, 10))
	for page := 0; page < pageCount(maximum) && len(results) < maximum; page++ {
		params := url.Values{
			"q":       {query},
			"sca_esv": {"1"},
			"filter":  {"1"},
			"start":   {strconv.Itoa(page * 10)},
			"hl":      {"en-US"},
			"lr":      {"lang_en"},
			"cr":      {"countryUS"},
		}
		headers := http.Header{
			"User-Agent": {"NokiaN72/5.0819.4.0.1 Series60/2.8 Profile/MIDP-2.0 Configuration/CLDC-1.1/UC Browser7.9.1.122/27/351/UCWEB"},
			"Cookie":     {"CONSENT=YES+"},
		}
		body, err := fetchHTML(ctx, client, g.endpoint, params, headers)
		if err != nil {
			return nil, fmt.Errorf("google search request failed: %w", err)
		}
		pageResults, err := parseGoogleResults(body)
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

func parseGoogleResults(body []byte) ([]Result, error) {
	lower := bytes.ToLower(body)
	if bytes.Contains(lower, []byte("unusual traffic")) || bytes.Contains(lower, []byte("/sorry/")) {
		return nil, fmt.Errorf("Google security verification required")
	}
	root, err := parseHTML(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cannot parse Google results: %w", err)
	}
	results := []Result{}
	walk(root, func(node *html.Node) {
		if !isElement(node, "div") {
			return
		}
		children := elementChildren(node, "div")
		if len(children) < 2 {
			return
		}
		anchors := elementChildren(children[0], "a")
		tables := elementChildren(children[1], "table")
		if len(anchors) == 0 || len(tables) == 0 {
			return
		}
		anchor := anchors[0]
		href := normalizeGoogleURL(attr(anchor, "href"))
		if href == "" {
			return
		}
		results = append(results, Result{Title: nodeText(anchor), Href: href, Body: nodeText(children[1])})
	})
	return results, nil
}

func normalizeGoogleURL(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "/url?") {
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		value = parsed.Query().Get("q")
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return ""
	}
	return value
}
