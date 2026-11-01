package main

import (
	"bytes"

	"github.com/cockroachdb/errors"
	"golang.org/x/net/html"
)

// pageMarkup is the part of a template page a component draws: the top bar and
// the scrolling column. The rest of the page is the layout's hidden notices and
// its scripts.
func pageMarkup(doc []byte) ([]byte, error) {
	root, err := html.Parse(bytes.NewReader(doc))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var keep []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" && (hasClass(n, "topbar") || hasClass(n, "scroll")) {
			keep = append(keep, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if len(keep) != 2 {
		return nil, errors.Newf("the page has %d of the top bar and scroll column, want 2", len(keep))
	}
	var out bytes.Buffer
	for _, n := range keep {
		if err := html.Render(&out, n); err != nil {
			return nil, errors.WithStack(err)
		}
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// hasClass says whether an element carries a class.
func hasClass(n *html.Node, class string) bool {
	for _, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		for _, c := range bytes.Fields([]byte(a.Val)) {
			if string(c) == class {
				return true
			}
		}
	}
	return false
}
