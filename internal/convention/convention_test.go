package convention_test

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/convention"
)

func Test_eachCheckFlagsItsFixture(t *testing.T) {
	setup := NewWithT(t)
	setup.Expect(convention.Checks).NotTo(BeEmpty())

	for _, check := range convention.Checks {
		t.Run(check.Name, func(t *testing.T) {
			g := NewWithT(t)
			fset := token.NewFileSet()
			pkg, err := convention.ParseFixture(fset, check.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(check.Run(fset, pkg.Files)).NotTo(BeEmpty())
		})
	}
}

func Test_everyFixtureHasACheck(t *testing.T) {
	g := NewWithT(t)
	dirs, err := os.ReadDir(filepath.Join("testdata"))
	g.Expect(err).NotTo(HaveOccurred())

	onDisk := map[string]struct{}{}
	for _, d := range dirs {
		if d.IsDir() {
			onDisk[d.Name()] = struct{}{}
		}
	}
	registered := map[string]struct{}{}
	for _, check := range convention.Checks {
		registered[check.Name] = struct{}{}
	}
	g.Expect(onDisk).To(Equal(registered))
}

func Test_packagesAreListed(t *testing.T) {
	g := NewWithT(t)
	found, err := convention.PackagesListed()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(found).To(BeEmpty())
}

func Test_repoFollowsItsOwnConventions(t *testing.T) {
	g := NewWithT(t)
	fset := token.NewFileSet()
	pkgs, err := convention.ParseRepo(fset)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(pkgs).NotTo(BeEmpty())

	var found []string
	for _, pkg := range pkgs {
		for _, check := range convention.Checks {
			for _, f := range check.Run(fset, pkg.Files) {
				found = append(found, f.String())
			}
		}
	}
	g.Expect(found).To(BeEmpty())
}
