package ui

import "testing"

func TestCommonPrefixOfStrings(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"alone"}, "alone"},
		{[]string{"abc", "abd"}, "ab"},
		{[]string{"/etc/passwd", "/etc/shadow"}, "/etc/"},
		{[]string{"/x/", "/y/"}, "/"},
		{[]string{"same", "same", "same"}, "same"},
		{[]string{"a", "b"}, ""},
		// One empty entry pins the prefix to empty.
		{[]string{"abc", "", "abd"}, ""},
	}
	for _, c := range cases {
		if got := commonPrefixOfStrings(c.in); got != c.want {
			t.Errorf("commonPrefixOfStrings(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
