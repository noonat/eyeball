package namedgomega

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestKind(t *testing.T) {
	NewWithT(t).Expect(1).To(Equal(1))
}
