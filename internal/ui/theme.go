package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Theme is a named set of colours applied to tview.Styles plus the
// dynamic-colour tags we sprinkle through our textual output.
//
// We bind the dynamic tags ([green]/[yellow]/[red]/[gray]/[aqua]/
// [yellow]) to tcell colour names at startup so a theme can remap them
// without touching every call site. This isn't a built-in tview feature
// — we substitute the tag set via a small registry.
type Theme struct {
	Name string

	Background tcell.Color
	Foreground tcell.Color
	Border     tcell.Color
	Title      tcell.Color

	// Semantic colours; mapped to the dynamic-colour tag set below.
	Accent  tcell.Color // "yellow" — keybinding callouts
	Good    tcell.Color // "green"  — success
	Bad     tcell.Color // "red"    — error / danger
	Warning tcell.Color // (yellow / orange) — kernel drift, etc.
	Muted   tcell.Color // "gray"   — placeholders, secondary info
	Header  tcell.Color // "aqua"   — table family names, headers
}

// AvailableThemes returns every theme by name. The first entry is the
// default. Used by --theme flag validation and by the help overlay.
func AvailableThemes() []*Theme {
	return []*Theme{themeDefault(), themeHighContrast(), themeMono()}
}

// LookupTheme returns the theme with the given name, or the default
// theme if name is empty / not recognised. The bool reports whether a
// match was found, so the caller can warn on an unknown name.
func LookupTheme(name string) (*Theme, bool) {
	for _, t := range AvailableThemes() {
		if strings.EqualFold(t.Name, name) {
			return t, true
		}
	}
	return AvailableThemes()[0], name == ""
}

// Apply installs the theme into tview.Styles. Call once at startup
// before any widget is constructed (Styles is global state read
// during widget construction). Returns a render helper that themes
// strings containing [yellow]/[green]/… tags.
func (t *Theme) Apply() {
	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    t.Background,
		ContrastBackgroundColor:     t.Border,
		MoreContrastBackgroundColor: t.Border,
		BorderColor:                 t.Border,
		TitleColor:                  t.Title,
		GraphicsColor:               t.Border,
		PrimaryTextColor:            t.Foreground,
		SecondaryTextColor:          t.Muted,
		TertiaryTextColor:           t.Muted,
		InverseTextColor:            t.Background,
		ContrastSecondaryTextColor:  t.Accent,
	}
}

func themeDefault() *Theme {
	return &Theme{
		Name:       "default",
		Background: tcell.ColorBlack,
		Foreground: tcell.ColorWhite,
		Border:     tcell.ColorDarkGray,
		Title:      tcell.ColorYellow,
		Accent:     tcell.ColorYellow,
		Good:       tcell.ColorGreen,
		Bad:        tcell.ColorRed,
		Warning:    tcell.ColorYellow,
		Muted:      tcell.ColorGray,
		Header:     tcell.ColorAqua,
	}
}

func themeHighContrast() *Theme {
	return &Theme{
		Name:       "high-contrast",
		Background: tcell.ColorBlack,
		Foreground: tcell.ColorWhite,
		Border:     tcell.ColorWhite,
		Title:      tcell.NewRGBColor(0xff, 0xff, 0x00), // pure yellow
		Accent:     tcell.NewRGBColor(0xff, 0xff, 0x00),
		Good:       tcell.NewRGBColor(0x00, 0xff, 0x00),
		Bad:        tcell.NewRGBColor(0xff, 0x40, 0x40),
		Warning:    tcell.NewRGBColor(0xff, 0xa5, 0x00), // orange
		Muted:      tcell.ColorSilver,
		Header:     tcell.NewRGBColor(0x00, 0xff, 0xff),
	}
}

func themeMono() *Theme {
	return &Theme{
		Name:       "mono",
		Background: tcell.ColorBlack,
		Foreground: tcell.ColorWhite,
		Border:     tcell.ColorGray,
		Title:      tcell.ColorWhite,
		Accent:     tcell.ColorWhite,
		Good:       tcell.ColorWhite,
		Bad:        tcell.ColorWhite,
		Warning:    tcell.ColorWhite,
		Muted:      tcell.ColorDarkGray,
		Header:     tcell.ColorWhite,
	}
}

// ThemeNames returns the comma-separated list of theme names. Used by
// the --theme flag help text.
func ThemeNames() string {
	names := make([]string, 0, len(AvailableThemes()))
	for _, t := range AvailableThemes() {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// ThemeError formats a useful error when an unknown theme name is
// supplied on the command line.
func ThemeError(name string) error {
	return fmt.Errorf("unknown theme %q — choose one of: %s",
		name, ThemeNames())
}
