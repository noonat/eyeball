package convention_test

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/noonat/eyeball/internal/convention"
)

func Test_checksFlagFixtures(t *testing.T) {
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

	setup.Expect(convention.ProseChecks).NotTo(BeEmpty())
	for _, check := range convention.ProseChecks {
		t.Run(check.Name, func(t *testing.T) {
			g := NewWithT(t)
			path, body, err := convention.ProseFixture(check.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(check.Run(path, body)).NotTo(BeEmpty())
		})
	}

	setup.Expect(convention.TearoutChecks).NotTo(BeEmpty())
	for _, check := range convention.TearoutChecks {
		t.Run(check.Name, func(t *testing.T) {
			g := NewWithT(t)
			tear, err := convention.TearoutFixture(check.Name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(check.Run(tear)).NotTo(BeEmpty())
		})
	}
}

func Test_fixturesHaveChecks(t *testing.T) {
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
	for _, check := range convention.ProseChecks {
		registered[check.Name] = struct{}{}
	}
	for _, check := range convention.TearoutChecks {
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

func Test_repoFollowsConventions(t *testing.T) {
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

	files, err := convention.MarkdownFiles()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(files).NotTo(BeEmpty())

	var prose []string
	for _, name := range files {
		body, err := convention.ReadMarkdown(name)
		g.Expect(err).NotTo(HaveOccurred())
		for _, check := range convention.ProseChecks {
			for _, f := range check.Run(name, body) {
				prose = append(prose, f.String())
			}
		}
	}
	g.Expect(prose).To(BeEmpty())

	tear, err := convention.LoadTearout()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(tear.Pages).NotTo(BeEmpty())

	var markup []string
	for _, check := range convention.TearoutChecks {
		for _, f := range check.Run(tear) {
			markup = append(markup, f.String())
		}
	}
	g.Expect(markup).To(BeEmpty())
}
