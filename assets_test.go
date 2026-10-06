package eyeball

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

// Htmx must be the release VERSION names. A vendored file edited by hand, or
// reformatted, would no longer be the code its version number promises.
func Test_htmxIsTheVendoredRelease(t *testing.T) {
	g := NewWithT(t)
	version, err := os.ReadFile("web/vendor/VERSION")
	g.Expect(err).NotTo(HaveOccurred())
	sum := sha256.Sum256(Htmx)
	g.Expect(string(version)).To(ContainSubstring(hex.EncodeToString(sum[:]) + "  htmx.min.js"))
}

// The template class check cannot see a class the client adds at run time, so
// every such class is in web/ts/classes.ts and held to app.css here.
func Test_scriptClassesAreInTheStylesheet(t *testing.T) {
	g := NewWithT(t)
	src, err := fs.ReadFile(Scripts, "web/ts/classes.ts")
	g.Expect(err).NotTo(HaveOccurred())
	values := regexp.MustCompile(`:\s*"([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	g.Expect(values).NotTo(BeEmpty())
	css := string(Stylesheet)
	for _, v := range values {
		class := regexp.MustCompile(`\.` + regexp.QuoteMeta(v[1]) + `\b`)
		g.Expect(class.MatchString(css)).To(BeTrue(), "app.css has no ."+v[1])
	}
	g.Expect(strings.Count(string(src), `"`)/2).To(Equal(len(values)), "every string in classes.ts is a class")
}

func TestBundle(t *testing.T) {
	g := NewWithT(t)
	out, err := build("web/ts/main.ts")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(out)).To(ContainSubstring("prefers-color-scheme"))
}

func TestWorker(t *testing.T) {
	g := NewWithT(t)
	out, err := Worker()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(out)).To(ContainSubstring("notificationclick"))
}
