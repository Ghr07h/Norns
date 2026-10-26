package websearch

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

func parseHTML(reader io.Reader) (*html.Node, error) {
	return html.Parse(reader)
}

func walk(root *html.Node, visit func(*html.Node)) {
	if root == nil {
		return
	}
	visit(root)
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		walk(child, visit)
	}
}

func firstDescendant(root *html.Node, match func(*html.Node) bool) *html.Node {
	var found *html.Node
	walk(root, func(node *html.Node) {
		if found == nil && node != root && match(node) {
			found = node
		}
	})
	return found
}

func elementChildren(root *html.Node, tag string) []*html.Node {
	children := []*html.Node{}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && (tag == "" || child.Data == tag) {
			children = append(children, child)
		}
	}
	return children
}

func isElement(node *html.Node, tag string) bool {
	return node.Type == html.ElementNode && node.Data == tag
}

func attr(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

func hasClass(node *html.Node, class string) bool {
	for _, value := range strings.Fields(attr(node, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

func nodeText(root *html.Node) string {
	parts := []string{}
	walk(root, func(node *html.Node) {
		if node.Type == html.TextNode {
			parts = append(parts, node.Data)
		}
	})
	return normalizeText(strings.Join(parts, " "))
}
