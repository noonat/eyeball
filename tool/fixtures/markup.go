package main

import (
	"bytes"

	"github.com/cockroachdb/errors"
	"golang.org/x/net/html"
)

// queueMarkup is the part of the template queue page a component draws: the top
// bar and the scrolling column.
//
// What the page's script fills in or shows is left out, since a component takes
// it from props: the first-run steps and the lapsed notice, both hidden until
// push.ts decides, and the attributes the stream script reads.
func queueMarkup(doc []byte) ([]byte, error) {
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
		return nil, errors.Newf("the queue page has %d of the top bar and scroll column, want 2", len(keep))
	}
	var out bytes.Buffer
	for _, n := range keep {
		prune(n)
		if err := html.Render(&out, n); err != nil {
			return nil, errors.WithStack(err)
		}
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// prune removes what a component does not draw from a subtree.
func prune(n *html.Node) {
	var drop []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (attr(c, "id") == "firstrun" || attr(c, "id") == "lapsed") {
			drop = append(drop, c)
			continue
		}
		prune(c)
	}
	for _, c := range drop {
		n.RemoveChild(c)
	}
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != "data-stream" && a.Key != "data-stamp" {
			kept = append(kept, a)
		}
	}
	n.Attr = kept
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

// attr is an element's attribute, or empty.
func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}
