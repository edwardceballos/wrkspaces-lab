// Package theme centralizes Lip Gloss palettes selected by parameters.theme.
// See ADR-0007. Start with gruvbox; add themes by adding a constructor.
package theme

import (
	"fmt"
	"strconv"
	"strings"
)

type Theme struct {
	Name string

	Bg    string // base background
	Bg1   string // raised surface
	Fg    string // primary text
	FgDim string // secondary text
	Gray  string // borders / muted

	Red    string
	Green  string
	Yellow string
	Blue   string
	Purple string
	Aqua   string
	Orange string

	// Stops define the gradient used by the splash sweep.
	Stops []string
}

// Gruvbox (dark) palette.
func Gruvbox() Theme {
	return Theme{
		Name:   "gruvbox",
		Bg:     "#282828",
		Bg1:    "#3c3836",
		Fg:     "#ebdbb2",
		FgDim:  "#a89984",
		Gray:   "#928374",
		Red:    "#fb4934",
		Green:  "#b8bb26",
		Yellow: "#fabd2f",
		Blue:   "#83a598",
		Purple: "#d3869b",
		Aqua:   "#8ec07c",
		Orange: "#fe8019",
		Stops:  []string{"#fb4934", "#fe8019", "#fabd2f", "#b8bb26", "#8ec07c", "#83a598", "#d3869b"},
	}
}

// Load maps parameters.theme -> Theme, with a safe default.
func Load(name string) Theme {
	switch strings.ToLower(name) {
	case "gruvbox", "":
		return Gruvbox()
	default:
		return Gruvbox() // fallback; add more themes here
	}
}

// Ramp returns `steps` hex colors interpolated across the theme stops.
func (t Theme) Ramp(steps int) []string {
	if steps < 2 || len(t.Stops) < 2 {
		return t.Stops
	}
	out := make([]string, steps)
	segs := len(t.Stops) - 1
	for i := 0; i < steps; i++ {
		pos := float64(i) / float64(steps-1) * float64(segs)
		seg := int(pos)
		if seg >= segs {
			seg = segs - 1
		}
		frac := pos - float64(seg)
		out[i] = LerpHex(t.Stops[seg], t.Stops[seg+1], frac)
	}
	return out
}

// LerpHex linearly interpolates two #rrggbb colors (t in [0,1]).
func LerpHex(a, b string, t float64) string {
	ar, ag, ab := hexRGB(a)
	br, bg, bb := hexRGB(b)
	r := int(float64(ar) + (float64(br)-float64(ar))*t)
	g := int(float64(ag) + (float64(bg)-float64(ag))*t)
	bl := int(float64(ab) + (float64(bb)-float64(ab))*t)
	return fmt.Sprintf("#%02x%02x%02x", clamp(r), clamp(g), clamp(bl))
}

func hexRGB(h string) (int, int, int) {
	h = strings.TrimPrefix(h, "#")
	if len(h) != 6 {
		return 235, 219, 178 // fg fallback
	}
	r, _ := strconv.ParseInt(h[0:2], 16, 0)
	g, _ := strconv.ParseInt(h[2:4], 16, 0)
	b, _ := strconv.ParseInt(h[4:6], 16, 0)
	return int(r), int(g), int(b)
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
