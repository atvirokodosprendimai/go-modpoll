package messaging

import "testing"

func TestExtractDevice(t *testing.T) {
	cases := []struct {
		pattern string
		subject string
		want    string
	}{
		{"modpoll.*.set", "modpoll.dev01.set", "dev01"},
		{"modpoll.*.set", "modpoll.dev-2.set", "dev-2"},
		{"modpoll.*.set", "other.dev01.set", ""},
		{"modpoll.*.set", "modpoll.dev01", ""},
		{"x.*.y.*", "x.a.y.b", "a"},
	}
	for _, c := range cases {
		got := extractDevice(c.pattern, c.subject)
		if got != c.want {
			t.Errorf("extractDevice(%q, %q) = %q, want %q", c.pattern, c.subject, got, c.want)
		}
	}
}

func TestSanitizeSubjectToken(t *testing.T) {
	if got := sanitizeSubjectToken("temp.outside"); got != "temp_outside" {
		t.Errorf("got %q want temp_outside", got)
	}
	if got := sanitizeSubjectToken("hello world"); got != "hello_world" {
		t.Errorf("got %q want hello_world", got)
	}
}
