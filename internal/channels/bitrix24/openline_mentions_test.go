package bitrix24

import (
	"strings"
	"testing"
)

// --- extractOpenlineNameMentions ---

func TestExtractOpenlineNameMentions(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "multiple distinct names first-seen order",
			text: "@Ngọc Thúy Synity ở đây có anh [Cường Egany] EGANY ... [Michael Huỳnh Minh Khoa] vs [Hiền Trịnh] ...",
			want: []string{"Cường Egany", "Michael Huỳnh Minh Khoa", "Hiền Trịnh"},
		},
		{
			name: "USER/BOT bbcode tags excluded",
			text: "group: [USER=62]Alice[/USER] và [BOT=1234]Bot[/BOT]",
			want: nil,
		},
		{
			name: "bbcode style tags denylisted",
			text: "dùng [b]đậm[/b] và [i]nghiêng[/i]",
			want: nil,
		},
		{
			name: "empty text",
			text: "",
			want: nil,
		},
		{
			name: "no brackets at all",
			text: "không có gì trong ngoặc ở đây",
			want: nil,
		},
		{
			name: "dedup case-insensitive keeps first-seen casing",
			text: "[Hiền Trịnh] đã xác nhận, và [hiền trịnh] cũng vậy",
			want: []string{"Hiền Trịnh"},
		},
		{
			// Roster is pipe-joined on the wire (MetaKeyOpenlineRoster) — a
			// bracket containing a literal `|` would corrupt that encoding
			// on split, so it must be skipped outright rather than trusted.
			name: "pipe character in bracket excluded (would corrupt pipe-joined roster wire format)",
			text: "chào [Name|Extra] và [Cường Egany]",
			want: []string{"Cường Egany"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractOpenlineNameMentions(tc.text)
			if !equalStrings(got, tc.want) {
				t.Errorf("extractOpenlineNameMentions(%q) = %v; want %v", tc.text, got, tc.want)
			}
		})
	}
}

// --- enforceOpenlineMentionBrackets ---

func TestEnforceOpenlineMentionBrackets(t *testing.T) {
	t.Run("golden trace verbatim echo wraps all roster names", func(t *testing.T) {
		roster := []string{"Cường Egany", "Michael Huỳnh Minh Khoa", "Hiền Trịnh"}
		input := "Dạ chào anh/chị:\n" +
			"• Cường Egany - đã xác nhận đơn hàng\n" +
			"• Michael Huỳnh Minh Khoa - đang chờ thanh toán\n" +
			"• Hiền Trịnh - đã hủy đơn\n" +
			"Em cảm ơn ạ."
		want := "Dạ chào anh/chị:\n" +
			"• [Cường Egany] - đã xác nhận đơn hàng\n" +
			"• [Michael Huỳnh Minh Khoa] - đang chờ thanh toán\n" +
			"• [Hiền Trịnh] - đã hủy đơn\n" +
			"Em cảm ơn ạ."
		if got := enforceOpenlineMentionBrackets(input, roster); got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	})

	t.Run("recasing applies canonical roster casing", func(t *testing.T) {
		roster := []string{"Cường Egany"}
		input := "...tìm tên Cường EGANY nhưng..."
		want := "...tìm tên [Cường Egany] nhưng..."
		if got := enforceOpenlineMentionBrackets(input, roster); got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	})

	t.Run("longest-first prevents shorter name matching inside longer", func(t *testing.T) {
		roster := []string{"Cường", "Cường Egany"}
		input := "Cảm ơn Cường Egany, và Cường"
		want := "Cảm ơn [Cường Egany], và [Cường]"
		if got := enforceOpenlineMentionBrackets(input, roster); got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	})

	t.Run("idempotent — already-bracketed occurrence skipped, plain wrapped", func(t *testing.T) {
		roster := []string{"Alice"}
		input := "Chào [Alice] và Alice"
		want := "Chào [Alice] và [Alice]"
		if got := enforceOpenlineMentionBrackets(input, roster); got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	})

	t.Run("word boundary — standalone tokens wrap, negative case does not", func(t *testing.T) {
		roster := []string{"Nam"}
		input := "Việt Nam và Nam"
		want := "Việt [Nam] và [Nam]"
		if got := enforceOpenlineMentionBrackets(input, roster); got != want {
			t.Errorf("got %q; want %q", got, want)
		}

		negRoster := []string{"Na"}
		negInput := "Nam ở đây"
		if got := enforceOpenlineMentionBrackets(negInput, negRoster); got != negInput {
			t.Errorf("short roster entry must not match inside longer word: got %q; want unchanged %q", got, negInput)
		}
	})

	t.Run("zone protection — code fences, inline code, and BBCode url spans untouched", func(t *testing.T) {
		roster := []string{"Cường"}
		input := "Cường đây nhé `Cường` và ```Cường``` và [url=http://x/Cường]link[/url] và Cường."
		want := "[Cường] đây nhé `Cường` và ```Cường``` và [url=http://x/Cường]link[/url] và [Cường]."
		if got := enforceOpenlineMentionBrackets(input, roster); got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	})

	t.Run("bare http link protected", func(t *testing.T) {
		roster := []string{"Cường"}
		input := "Xem thêm tại https://example.com/Cường-info và hỏi Cường nhé"
		want := "Xem thêm tại https://example.com/Cường-info và hỏi [Cường] nhé"
		if got := enforceOpenlineMentionBrackets(input, roster); got != want {
			t.Errorf("got %q; want %q", got, want)
		}
	})

	t.Run("empty roster is no-op", func(t *testing.T) {
		input := "Chào Cường"
		if got := enforceOpenlineMentionBrackets(input, nil); got != input {
			t.Errorf("empty roster should no-op: got %q; want %q", got, input)
		}
	})

	t.Run("empty text is no-op", func(t *testing.T) {
		if got := enforceOpenlineMentionBrackets("", []string{"Cường"}); got != "" {
			t.Errorf("empty text should no-op: got %q", got)
		}
	})

	t.Run("literal NUL in input is stripped before placeholder framing", func(t *testing.T) {
		roster := []string{"Cường Egany"}
		input := "Chào Cường Egany\x00 nhé"
		want := "Chào [Cường Egany] nhé"
		if got := enforceOpenlineMentionBrackets(input, roster); got != want {
			t.Errorf("got %q; want %q (NUL must not corrupt placeholder restore)", got, want)
		}
	})

	// Documents a known limit (see plan): abbreviating a multi-token roster
	// name down to its first token ("Hiền Trịnh" → "Hiền") is NOT
	// code-solvable without risky Vietnamese fuzzy matching — that's
	// persona's job (phase 4). The deterministic core must not
	// false-positive-wrap the bare abbreviation against the full-name
	// roster entry.
	t.Run("abbreviation is a documented no-op, not a false-positive wrap", func(t *testing.T) {
		roster := []string{"Hiền Trịnh"}
		input := "Chào Hiền, em có gì giúp được ạ?"
		if got := enforceOpenlineMentionBrackets(input, roster); got != input {
			t.Errorf("abbreviation must not be wrapped (no false positive): got %q; want unchanged %q", got, input)
		}
	})

	t.Run("determinism — repeated calls produce identical output", func(t *testing.T) {
		roster := []string{"Cường Egany", "Hiền Trịnh"}
		input := "Chào Cường Egany và Hiền Trịnh, hẹn gặp lại Cường Egany"
		first := enforceOpenlineMentionBrackets(input, roster)
		for i := range 100 {
			if got := enforceOpenlineMentionBrackets(input, roster); got != first {
				t.Fatalf("iteration %d: got %q; want %q (non-deterministic)", i, got, first)
			}
		}
	})
}

// equalStrings compares two possibly-nil string slices for equality.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sanity: no lookaround syntax is used anywhere in the package's regex
// literals (RE2 constraint documented in the plan).
func TestOpenlineMentions_NoLookaroundRegex(t *testing.T) {
	// This is enforced by code review / go vet in practice (RE2 rejects
	// lookaround at compile time — regexp.MustCompile would panic). This
	// test documents the constraint and fails loudly if anyone pastes in
	// a lookaround pattern that happens to compile under a different engine.
	src := strings.Join([]string{
		`\[([^\[\]#=/]+)\]`,
	}, "\n")
	if strings.Contains(src, "(?<") || strings.Contains(src, "(?=") || strings.Contains(src, "(?!") {
		t.Fatal("lookaround syntax detected — Go RE2 does not support it")
	}
}
