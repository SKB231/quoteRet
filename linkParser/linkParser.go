package linkParser

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

func returnNodeType(node *html.Node) string {
	nodeTypeName := map[html.NodeType]string{0: "ErrorNode", 1: "TextNode", 2: "DocNode", 3: "ElementNode", 4: "CommentNode", 5: "DocTypeNode"}
	return nodeTypeName[node.Type]
}

type ParseResult struct {
	Price string
	Time  string
}

// dfs method takes a single node, and a boolean and goes over every available node in the dfs Tree.
// Only set the childAnchor to true if the next node is the child of an anchor tag.
func dfs(node *html.Node, quote string, result *ParseResult) {
	if node == nil {
		// Should find a better way to do this for sure
		// Marks end of DFS.
		return
	}

	if node.Type == html.ElementNode && node.Data == "span" {
		dataTestID := ""
		for _, attr := range node.Attr {
			if attr.Key == "data-testid" {
				dataTestID = attr.Val
				break
			}
		}

		if dataTestID == "qsp-price" && result.Price == "" {
			result.Price = strings.TrimSpace(nodeText(node))
		}
	} else if node.Type == html.ElementNode && node.Data == "div" {
		id := ""
		for _, attr := range node.Attr {
			switch attr.Key {
			case "slot":
				id = attr.Val
			}
		}
		if id == "marketTimeNotice" {
			if node.FirstChild != nil && node.FirstChild.FirstChild != nil {
				result.Time = node.FirstChild.FirstChild.Data
			}
		}
	}

	if node.FirstChild != nil {
		dfs(node.FirstChild, quote, result)
	}
	if node.NextSibling != nil {
		dfs(node.NextSibling, quote, result)
	}
}

func nodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}

	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(nodeText(child))
	}
	return builder.String()
}

// Parse function
// @param r io.Reader this is the reader to the text containing the html to read. An array containing link Result structs is returned.
func Parse(r io.Reader, quote string) (ParseResult, error) {

	doc, err := html.Parse(r)
	var parseRes ParseResult // Not the best way to send back data but we only have 2 strings so not expensive.
	if err != nil {
		return ParseResult{}, err
	}
	dfs(doc, quote, &parseRes)
	return parseRes, err
}
