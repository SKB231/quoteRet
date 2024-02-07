package linkParser

import (
	"fmt"
	"io"

	"golang.org/x/net/html"
)

func returnNodeType(node *html.Node) string {
	nodeTypeName := map[html.NodeType]string{0: "ErrorNode", 1: "TextNode", 2: "DocNode", 3: "ElementNode", 4: "CommentNode", 5: "DocTypeNode"}
	return nodeTypeName[node.Type]
}


type ParseResult struct {
	Price string
	Time string
}


// dfs method takes a single node, and a boolean and goes over every available node in the dfs Tree.
// Only set the childAnchor to true if the next node is the child of an anchor tag.
func dfs(node *html.Node, quote string, result *ParseResult) {
	if node == nil {
		// Should find a better way to do this for sure
		// Marks end of DFS.
		return
	}


	if node.Type == 3 && node.Data == "fin-streamer" {
		// We have reached an fin-streamer tag. Extract the available tags and their values. One of them contains the required data.

		dataFieldVal := ""
		dataFieldType := ""
		dataSymbolField := ""
		for _, attr := range node.Attr {
			switch attr.Key {
			case "data-field":
				dataFieldType = attr.Val
			case "value":
				dataFieldVal = attr.Val
			case "data-symbol":
				dataSymbolField = attr.Val
			}
		}
		if dataFieldType == "regularMarketPrice" && dataFieldVal != "" && dataSymbolField == quote{
			result.Price = dataFieldVal
		}
		
	} else if node.Type == 3 && node.Data == "div" {
		id := ""
		for _, attr := range node.Attr {
			switch attr.Key {
			case "id":
				id = attr.Val
			}
		}
		if id == "quote-market-notice"{
			if node.FirstChild != nil && node.FirstChild.FirstChild != nil{
				result.Time = node.FirstChild.FirstChild.Data;
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

// Parse function
// @param r io.Reader this is the reader to the text containing the html to read. An array containing link Result structs is returned.
func Parse(r io.Reader, quote string) (ParseResult, error) {
	doc, err := html.Parse(r)
	var parseRes ParseResult // Not the best way to send back data but we only have 2 strings so not expensive.
	if err != nil {
		fmt.Println("Error parsing document")
		fmt.Println(err)
		return ParseResult{}, err
	}
	dfs(doc, quote, &parseRes)
	return parseRes, err
}
