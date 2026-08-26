package gomegainsubtest

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestKind(t *testing.T) {
	g := NewWithT(t)
	names := []string{"a", "b"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			g.Expect(name).NotTo(BeEmpty())
		})
	}
}
