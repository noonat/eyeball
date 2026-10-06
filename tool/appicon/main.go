// Command appicon renders the installed page's icons: the tearouts' badge icon,
// the letter e on the accent color, at the sizes the manifest and iOS ask for.
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
	vivid, err := token(css, "--accent-vivid")
	if err != nil {
		return err
	}
	deep, err := token(css, "--accent")
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "appicon-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for _, n := range sizes {
		// Full bleed, with no rounded corners: iOS and Android mask the
		// icon to their own shape.
		page := fmt.Sprintf(`<!doctype html><html><body style="margin:0">
<div style="width:%[1]dpx;height:%[1]dpx;display:grid;place-items:center;background:linear-gradient(160deg,%[2]s,%[3]s);color:#fff;font:700 %[4]dpx/1 system-ui,-apple-system,'Segoe UI',sans-serif;letter-spacing:-0.03em">e</div>
</body></html>`, n, vivid, deep, n*45/100)
		src := filepath.Join(dir, fmt.Sprintf("icon-%d.html", n))
		if err := os.WriteFile(src, []byte(page), 0o600); err != nil {
			return err
		}
		out, err := filepath.Abs(filepath.Join("web", "static", "icons", fmt.Sprintf("icon-%d.png", n)))
		if err != nil {
			return err
		}
		cmd := exec.Command(
			"chromium",
			"--headless=new",
			"--disable-gpu",
			"--hide-scrollbars",
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

// token reads a light-theme color token from app.css, the first definition,
// which is the light theme's.
func token(css []byte, name string) (string, error) {
	m := regexp.MustCompile(regexp.QuoteMeta(name) + `:\s*(#[0-9a-fA-F]{3,8})`).FindSubmatch(css)
	if m == nil {
		return "", fmt.Errorf("app.css has no %s color", name)
	}
	return string(m[1]), nil
}
