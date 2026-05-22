package ui

import "testing"

func TestSplitCommentFromNFT(t *testing.T) {
	cases := []struct {
		in, body, comment string
	}{
		{
			in:      `tcp dport 22 counter accept comment "ssh"`,
			body:    `tcp dport 22 counter accept`,
			comment: `ssh`,
		},
		{
			in:      `ip saddr 10.0.0.0/24 accept`, // no comment
			body:    `ip saddr 10.0.0.0/24 accept`,
			comment: ``,
		},
		{
			in:      `accept comment "with spaces and \"quotes\""`,
			body:    `accept`,
			comment: `with spaces and \"quotes\"`,
		},
		{
			in:      ``, // empty body
			body:    ``,
			comment: ``,
		},
	}
	for _, c := range cases {
		body, comment := splitCommentFromNFT(c.in)
		if body != c.body || comment != c.comment {
			t.Errorf("splitCommentFromNFT(%q)\n  got:  body=%q comment=%q\n  want: body=%q comment=%q",
				c.in, body, comment, c.body, c.comment)
		}
	}
}
