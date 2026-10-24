package normalize

import "testing"

func TestValid(t *testing.T) {
	for _, name := range Filters {
		if !Valid(name) {
			t.Errorf("Valid(%q) = false, want true (listed in Filters)", name)
		}
	}
	if Valid("timestamp") {
		t.Error(`Valid("timestamp") = true, want false (singular is not built in)`)
	}
	if Valid("") {
		t.Error(`Valid("") = true, want false`)
	}
}

func TestLine_Timestamps(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"started at 2026-03-18T10:20:30Z", "started at <timestamp>"},
		{"at 2026-03-18 10:20:30.123+02:00 ok", "at <timestamp> ok"},
		{"date 2026-03-18 done", "date <timestamp> done"},
		{"elapsed 10:20:30", "elapsed <timestamp>"},
		{"no time here", "no time here"},
	}
	for _, tt := range tests {
		if got := Line(tt.in, []string{"timestamps"}); got != tt.want {
			t.Errorf("Line(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLine_UUIDs(t *testing.T) {
	in := "id=550e8400-e29b-41d4-a716-446655440000 status=ok"
	want := "id=<uuid> status=ok"
	if got := Line(in, []string{"uuids"}); got != want {
		t.Errorf("Line(%q) = %q, want %q", in, got, want)
	}
}

func TestLine_Paths(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"reading /usr/local/bin/tool now", "reading <path> now"},
		{"log dir /var/log/app/", "log dir <path>"},
		{"relative path docs/readme.md", "relative path <path>"},
	}
	for _, tt := range tests {
		if got := Line(tt.in, []string{"paths"}); got != tt.want {
			t.Errorf("Line(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLine_Whitespace(t *testing.T) {
	in := "  hello   world  \tmore  "
	want := "hello world more"
	if got := Line(in, []string{"whitespace"}); got != want {
		t.Errorf("Line(%q) = %q, want %q", in, got, want)
	}
}

func TestLine_Composed(t *testing.T) {
	in := "2026-03-18T10:20:30Z req 550e8400-e29b-41d4-a716-446655440000 from /var/log/app"
	want := "<timestamp> req <uuid> from <path>"
	got := Line(in, []string{"timestamps", "uuids", "paths"})
	if got != want {
		t.Errorf("Line composed = %q, want %q", got, want)
	}
}

func TestLine_UnknownFilterIgnored(t *testing.T) {
	in := "unchanged 2026-03-18"
	if got := Line(in, []string{"nonsense"}); got != in {
		t.Errorf("unknown filter changed the line: %q", got)
	}
}

func TestText_PerLine(t *testing.T) {
	in := "2026-03-18 one\nplain\n2027-01-02 two\n"
	want := "<timestamp> one\nplain\n<timestamp> two\n"
	if got := Text(in, []string{"timestamps"}); got != want {
		t.Errorf("Text = %q, want %q", got, want)
	}
	if got := Text(in, nil); got != in {
		t.Errorf("Text with no filters = %q, want unchanged", got)
	}
}
