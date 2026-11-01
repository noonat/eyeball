package server

import (
	"net/http"

	. "github.com/onsi/gomega"
	xhtml "golang.org/x/net/html"

	"github.com/noonat/eyeball/internal/viewdoc"
)

// scopedPage says an address is one of the project, tree and file pages, whose
// view is read through viewPage.
func scopedPage(address string) bool {
	return viewdoc.Scoped(address)
}

// viewPage reads an address's view from /api and draws it as the template pages
// once did, so a test about one of these screens reads its content by the old
// class names.
func (f *fixture) viewPage(g *WithT, address string, status int) *xhtml.Node {
	g.THelper()
	return viewDocument(g, f.s.Surface(), address, status)
}

// viewDocument is viewPage for any surface handler.
func viewDocument(g *WithT, h http.Handler, address string, status int) *xhtml.Node {
	g.THelper()
	res := serveSurface(h, http.MethodGet, "/api"+address)
	g.Expect(res.Code).To(Equal(status), res.Body.String())
	doc, err := viewdoc.Page(res.Code, res.Body.Bytes())
	g.Expect(err).NotTo(HaveOccurred())
	return parse(g, doc)
}

// docFromView draws a view's JSON as the template pages drew it.
func docFromView(g *WithT, body []byte) *xhtml.Node {
	g.THelper()
	doc, err := viewdoc.Page(http.StatusOK, body)
	g.Expect(err).NotTo(HaveOccurred())
	return parse(g, doc)
}
