package pgconn

import "testing"

// QuoteString must keep backslashes literal (relies on
// standard_conforming_strings=on) so JSON escapes like \n survive. Using E''
// would collapse \n into a newline and corrupt JSON values.
func TestQuoteString(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`plain`, `'plain'`},
		{`it's`, `'it''s'`},
		{`{"body":"a\nb"}`, `'{"body":"a\nb"}'`},
		{`back\slash`, `'back\slash'`},
	}
	for _, c := range cases {
		got := QuoteString(c.in)
		if got != c.want {
			t.Errorf("QuoteString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
