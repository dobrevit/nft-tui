package ui

import "testing"

func TestLookupTheme(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantOk   bool
	}{
		{"default", "default", true},
		{"DEFAULT", "default", true}, // case-insensitive
		{"high-contrast", "high-contrast", true},
		{"mono", "mono", true},
		{"", "default", true}, // empty: silent default
		{"chartreuse", "default", false},
	}
	for _, c := range cases {
		got, ok := LookupTheme(c.in)
		if got.Name != c.wantName || ok != c.wantOk {
			t.Errorf("LookupTheme(%q) = (%q, %v), want (%q, %v)",
				c.in, got.Name, ok, c.wantName, c.wantOk)
		}
	}
}

func TestAvailableThemesAllNamed(t *testing.T) {
	for _, th := range AvailableThemes() {
		if th.Name == "" {
			t.Errorf("theme has empty Name: %+v", th)
		}
	}
}

func TestThemeNamesContainsAll(t *testing.T) {
	got := ThemeNames()
	for _, th := range AvailableThemes() {
		if !contains(got, th.Name) {
			t.Errorf("ThemeNames() = %q missing %q", got, th.Name)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
