package eyeball

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

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

func TestWorker(t *testing.T) {
	g := NewWithT(t)
	out, err := Worker()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(string(out)).To(ContainSubstring("notificationclick"))
}

// The worker carries the stamp the daemon reports, and the stamp follows the
// script: a builder whose output differs gets a different stamp, and the
// first pass sees the placeholder, so the hash never includes itself.
func TestWorker_stamp(t *testing.T) {
	g := NewWithT(t)
	out, err := Worker()
	g.Expect(err).NotTo(HaveOccurred())
	id, err := WorkerStamp()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(id).To(MatchRegexp(`^[0-9a-f]{12}$`))
	g.Expect(string(out)).To(ContainSubstring(`"` + id + `"`))
	g.Expect(string(out)).NotTo(ContainSubstring(stampPlaceholder))

	var passes []string
	versioned := func(source string) builder {
		return func(stamp string) ([]byte, error) {
			passes = append(passes, stamp)
			return []byte(source + stamp), nil
		}
	}
	_, one, err := stamped(versioned("one;"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(passes).To(Equal([]string{stampPlaceholder, one}))
	_, two, err := stamped(versioned("two;"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(two).NotTo(Equal(one))
	_, again, err := stamped(versioned("one;"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(again).To(Equal(one))
}
