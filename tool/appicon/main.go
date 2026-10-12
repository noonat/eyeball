// Command appicon renders the installed page's icons: the tearouts' badge icon,
// the eye on the paper with a zero in its highlighter iris, at the sizes the
// manifest and iOS ask for.
//
// The PNGs are committed, so the build needs no renderer. This runs when the
// icon or its colors change, with headless Chromium on the PATH, from the
// repository root.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
)

// sizes are the manifest's two icons and iOS's apple-touch-icon.
var sizes = []int{180, 192, 512}

// eyePath finds the open eye's outline in app.css's --eye-open mask, so the
// icon and the top bar's eye are drawn from one path.
var eyePath = regexp.MustCompile(`--eye-open:\s*url\("data:image/svg\+xml,<svg[^>]*><path d='([^']+)'/></svg>"\)`)

// iconPage is one icon at full bleed, with no rounded corners, because iOS and
// Android mask the icon to their own shape. Its arguments are the size, the
// font file, the paper, the ink, the highlighter, the digit's size and line
// height, and the eye's path. The proportions are .badgeicon's: the eye 40 of
// 56 wide and 26 high, the iris 17, its digit 10.5.
const iconPage = `<!doctype html><html><head><style>
@font-face{font-family:Atkinson;src:url("file://%[2]s") format("woff2");font-weight:200 800}
body{margin:0}
.tile{position:relative;width:%[1]dpx;height:%[1]dpx;background:%[3]s;display:grid;place-items:center}
.tile svg{position:absolute;left:14.29%%;top:26.79%%;width:71.43%%;height:46.43%%}
.iris{position:relative;width:30.36%%;height:30.36%%;border-radius:50%%;background:%[5]s;color:%[4]s;font:750 %.2[6]fpx/%.2[7]fpx Atkinson,sans-serif;text-align:center}
</style></head><body><div class="tile"><svg xmlns="http://www.w3.org/2000/svg" fill="none" stroke="%[4]s" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" viewBox="0 0 48 30"><path d="%[8]s"/></svg><div class="iris">0</div></div>
</body></html>`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "appicon:", err)
		os.Exit(1)
	}
}

func run() error {
	css, err := os.ReadFile("docs/design/app.css")
	if err != nil {
		return err
	}
	// The icon's three colors are fixed on .badgeicon rather than read from a
	// theme, because a home screen icon does not change with the page's theme.
	colors := map[string]string{}
	names := []string{"--icon-paper", "--icon-ink", "--icon-hl"}
	for _, name := range names {
		if colors[name], err = token(css, name); err != nil {
			return err
		}
	}
	m := eyePath.FindSubmatch(css)
	if m == nil {
		return fmt.Errorf("app.css has no --eye-open path")
	}
	font, err := filepath.Abs("docs/design/fonts/atkinson-hyperlegible-next/latin-wght-normal.woff2")
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "appicon-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for _, n := range sizes {
		page := fmt.Sprintf(
			iconPage,
			n,
			font,
			colors["--icon-paper"],
			colors["--icon-ink"],
			colors["--icon-hl"],
			float64(n)*10.5/56,
			float64(n)*17/56,
			m[1],
		)
		src := filepath.Join(dir, fmt.Sprintf("icon-%d.html", n))
		if err := os.WriteFile(src, []byte(page), 0o600); err != nil {
			return err
		}
		out, err := filepath.Abs(filepath.Join("web", "static", "icons", fmt.Sprintf("icon-%d.png", n)))
		if err != nil {
			return err
		}
		// The time budget lets the font load before the screenshot, which
		// otherwise draws the digit in the fallback face.
		cmd := exec.Command(
			"chromium",
			"--headless=new",
			"--disable-gpu",
			"--hide-scrollbars",
			"--virtual-time-budget=3000",
			fmt.Sprintf("--window-size=%d,%d", n, n),
			"--screenshot="+out,
			"file://"+src,
		)
		if b, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("render %d: %w: %s", n, err, b)
		}
		fmt.Println(out)
	}
	return nil
}

// token reads a color token from app.css, its first definition.
func token(css []byte, name string) (string, error) {
	m := regexp.MustCompile(regexp.QuoteMeta(name) + `:\s*(#[0-9a-fA-F]{3,8})`).FindSubmatch(css)
	if m == nil {
		return "", fmt.Errorf("app.css has no %s color", name)
	}
	return string(m[1]), nil
}
