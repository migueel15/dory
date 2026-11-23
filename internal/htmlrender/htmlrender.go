// Package htmlrender holds what render_html and html_preview share: the
// limits of a page, how one is read, and the themed document a page is shown
// in, the same in the chat and in the preview capture.
package htmlrender

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxBytes    = 512_000
	MaxTitle    = 200
	MinHeight   = 80
	MaxHeight   = 2000
	ColumnWidth = 728
)

// Guide is the layout and theme advice both tools give the agent.
const Guide = "Self-contained: inline <style> and <script>; remote URLs load as-is. " +
	"Frame width is the reply width (728px desktop, about 360px phones), on the thread background. " +
	"Leave html, body and the outer element without background, border, banner title or side padding; Dory adds an 8px gutter. " +
	"A box that needs its own background gets 16px padding and var(--radius) corners. Give charts fixed pixel heights. " +
	"Let content set the height: no vh units or height:100% on html or body. " +
	"Use the theme variables, not fixed colors: --background, --foreground, --muted, --card, --border, --accent, --accent-foreground, --success, --warning, --destructive, --chart-1 to --chart-6, --radius, --font-sans, --font-mono. " +
	"The base sheet sets background, text and font, and follows light/dark mode."

// Read returns the page given inline or as an absolute .html path.
func Read(html, path string) (string, error) {
	switch {
	case html != "" && path != "":
		return "", errors.New("pass html or path, not both")
	case path != "":
		if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".html") {
			return "", errors.New("path must be an absolute .html file")
		}
		st, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if st.Size() > MaxBytes {
			return "", fmt.Errorf("%s is over %d bytes", path, MaxBytes)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		html = string(b)
	}
	if strings.TrimSpace(html) == "" {
		return "", errors.New("pass a non-empty html or path")
	}
	if len(html) > MaxBytes {
		return "", fmt.Errorf("html is over %d bytes", MaxBytes)
	}
	return html, nil
}

var palettes = map[bool]string{
	false: "--background:#ffffff;--foreground:#0f172a;--muted:#64748b;--card:#f8fafc;--border:#e2e8f0;--accent:#5e6ad2;--accent-foreground:#ffffff;--success:#059669;--warning:#d97706;--destructive:#dc2626;--chart-1:#5e6ad2;--chart-2:#0ea5e9;--chart-3:#10b981;--chart-4:#f59e0b;--chart-5:#ef4444;--chart-6:#a855f7",
	true:  "--background:#0f0f11;--foreground:#e6e6e8;--muted:#9898a2;--card:#17171a;--border:#2a2a30;--accent:#5e6ad2;--accent-foreground:#ffffff;--success:#34d399;--warning:#fbbf24;--destructive:#f87171;--chart-1:#7c85e0;--chart-2:#38bdf8;--chart-3:#34d399;--chart-4:#fbbf24;--chart-5:#f87171;--chart-6:#c084fc",
}

// Document wraps a page in Dory's theme and a script that reports its
// height to the parent frame, which fits the frame to it.
func Document(html string, dark bool) string {
	scheme := "light"
	if dark {
		scheme = "dark"
	}
	head := `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<style>:root{` + palettes[dark] + `;--radius:8px;--font-sans:"Adwaita Sans",Inter,system-ui,sans-serif;--font-mono:ui-monospace,SFMono-Regular,monospace;color-scheme:` + scheme + `}` +
		`html{padding:8px}html,body{margin:0;background:transparent;color:var(--foreground);font:14px/1.5 var(--font-sans)}</style>` +
		`<script>(()=>{const s=()=>parent!==window&&parent.postMessage({doryHtml:Math.ceil(document.documentElement.getBoundingClientRect().height)},"*");addEventListener("load",s);new ResizeObserver(s).observe(document.documentElement)})()</script>`
	return head + html
}
