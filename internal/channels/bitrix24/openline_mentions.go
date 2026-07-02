package bitrix24

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// --- Bitrix24 openline [Name] mention preservation ---
//
// Bitrix24 Open Channel relays customer messages containing `[Full Name]`
// plain brackets — the downstream connector app resolves `[Name]` → Zalo uid
// via EXACT string match. When the agent replies and references someone by
// name, it must echo the bracket form back verbatim (canonical casing) or
// the downstream resolution silently fails.
//
// This file is the deterministic floor: it never invents names, only
// re-wraps names the LLM already wrote — verbatim or recased — that match a
// roster built from `[Name]` mentions seen earlier in the same openline chat
// (see channel.go / factory.go for the roster cache; handle.go / send.go for
// wiring). Persona (SOUL.md) is the supplementary, probabilistic layer for
// abbreviation cases this code can't safely reach (see plan phase 4).
//
// RE2 constraint: Go's regexp package has no lookaround. Word-boundary and
// already-bracketed checks are done manually via rune inspection instead of
// `(?<=...)` / `(?=...)` assertions.

// bxOpenlineMentionRE matches a bracketed name candidate: no nested `[`/`]`,
// and no `#`/`=`/`/` in the inner text — that excludes Bitrix connector
// sender tags (`[Name] #uid`, the `#` sits OUTSIDE the brackets so it never
// reaches here) and BBCode attribute/closing forms (`[USER=62]`, `[/USER]`).
// `|` is also excluded: the roster is pipe-joined for the metadata wire
// format (see MetaKeyOpenlineRoster), so a bracket containing a literal `|`
// would corrupt the encoding on split — skip it outright rather than trust
// Bitrix content not to contain one.
var bxOpenlineMentionRE = regexp.MustCompile(`\[([^\[\]#=/|]+)\]`)

// bxBBCodeTagDenylist holds the bare BBCode opener keywords that must never
// be mistaken for a `[Name]` mention (e.g. `[b]`, `[code]`). Lowercased
// comparison — Bitrix BBCode tags are case-insensitive in practice.
var bxBBCodeTagDenylist = map[string]struct{}{
	"b": {}, "i": {}, "u": {}, "s": {},
	"code": {}, "quote": {}, "url": {}, "list": {},
	"table": {}, "tr": {}, "td": {}, "br": {}, "p": {},
	"img": {}, "color": {}, "size": {}, "font": {},
}

// isBBCodeTag reports whether inner (already trimmed) is a bare BBCode
// opener keyword rather than a person's name.
func isBBCodeTag(inner string) bool {
	_, ok := bxBBCodeTagDenylist[strings.ToLower(inner)]
	return ok
}

// extractOpenlineNameMentions returns the unique `[Name]` values found in
// text, in first-seen order, excluding BBCode tags and sender-tag remnants.
// Dedup is case-insensitive; the first-seen casing is kept (that's the
// casing the downstream connector already resolved successfully).
func extractOpenlineNameMentions(text string) []string {
	if text == "" {
		return nil
	}
	matches := bxOpenlineMentionRE.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		inner := strings.TrimSpace(m[1])
		if inner == "" || isBBCodeTag(inner) {
			continue
		}
		key := strings.ToLower(inner)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, inner)
	}
	return names
}

// mentionMatchMode picks how a roster name is matched against outbound text.
type mentionMatchMode int

const (
	// matchModeExact requires byte-for-byte (case-sensitive) equality.
	// Used for single-token names — Vietnamese single-token names collide
	// with common words ("Nam"/"nam", "Hiền"/"hiền") even at 4+ runes, so
	// length is not a safe discriminator; only token count is.
	matchModeExact mentionMatchMode = iota
	// matchModeCaseInsensitive tolerates any casing. Used for multi-token
	// names, which real openline `[Full Name]` renders always are — the
	// collision risk with common phrases is negligible at 2+ tokens.
	matchModeCaseInsensitive
)

// matchMode classifies name by token count: any whitespace → multi-token.
func matchMode(name string) mentionMatchMode {
	if strings.IndexFunc(name, unicode.IsSpace) >= 0 {
		return matchModeCaseInsensitive
	}
	return matchModeExact
}

// alreadyBracketed reports whether the match at byte range [s,e) in text is
// already wrapped in a single layer of `[...]` (byte-safe: `[` and `]` are
// single-byte ASCII regardless of the Unicode content between them).
func alreadyBracketed(text string, s, e int) bool {
	return s > 0 && text[s-1] == '[' && e < len(text) && text[e] == ']'
}

// wordBoundaryOK reports whether the match at byte range [s,e) in text is
// not glued to a letter/digit on either side — i.e. it's a standalone token,
// not a substring of a longer word. String start/end count as a boundary.
func wordBoundaryOK(text string, s, e int) bool {
	if s > 0 {
		r, _ := utf8.DecodeLastRuneInString(text[:s])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	if e < len(text) {
		r, _ := utf8.DecodeRuneInString(text[e:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// bxOpenlineURLBBCodeRE matches `[url]…[/url]` / `[url=…]…[/url]` spans so
// their contents (which may embed a roster name inside the URL itself, e.g.
// a slug) are never rewrapped.
var bxOpenlineURLBBCodeRE = regexp.MustCompile(`(?is)\[url(?:=[^\]]*)?\].*?\[/url\]`)

// bxOpenlineBareURLRE matches a bare http(s) link so a roster name appearing
// inside a URL path/query is never rewrapped.
var bxOpenlineBareURLRE = regexp.MustCompile(`https?://\S+`)

// bxOpenlineURLZones holds text with URL spans replaced by NUL placeholders,
// plus the original span contents for restoration (mirrors the
// bxExtractedBlocks pattern in format.go).
type bxOpenlineURLZones struct {
	text   string
	blocks []string
}

func bxExtractOpenlineURLZones(text string) bxOpenlineURLZones {
	var blocks []string
	stash := func(m string) string {
		blocks = append(blocks, m)
		return fmt.Sprintf("\x00UR%d\x00", len(blocks)-1)
	}
	text = bxOpenlineURLBBCodeRE.ReplaceAllStringFunc(text, stash)
	text = bxOpenlineBareURLRE.ReplaceAllStringFunc(text, stash)
	return bxOpenlineURLZones{text: text, blocks: blocks}
}

// wrapRosterName finds unwrapped, word-boundary-safe occurrences of name in
// text and replaces each with a fresh `\x00MNi\x00` placeholder, appending
// name (the roster's canonical casing) to placeholders for later restore.
// Occurrences already bracketed, or glued to a letter/digit, are left as-is.
func wrapRosterName(text, name string, mode mentionMatchMode, placeholders *[]string) string {
	var pat *regexp.Regexp
	if mode == matchModeCaseInsensitive {
		pat = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(name))
	} else {
		pat = regexp.MustCompile(regexp.QuoteMeta(name))
	}
	locs := pat.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		s, e := loc[0], loc[1]
		if s < last {
			continue // defensive: regexp FindAll never overlaps for one pattern
		}
		if alreadyBracketed(text, s, e) || !wordBoundaryOK(text, s, e) {
			continue
		}
		b.WriteString(text[last:s])
		idx := len(*placeholders)
		*placeholders = append(*placeholders, name)
		fmt.Fprintf(&b, "\x00MN%d\x00", idx)
		last = e
	}
	b.WriteString(text[last:])
	return b.String()
}

// enforceOpenlineMentionBrackets re-wraps every roster name occurrence in
// text with `[Name]` using the roster's canonical casing, unless the
// occurrence is already bracketed or sits inside a code/URL zone. Pure,
// deterministic, RE2-safe. No-op for empty text or empty roster.
func enforceOpenlineMentionBrackets(text string, roster []string) string {
	if text == "" || len(roster) == 0 {
		return text
	}

	// Sanitize NUL: this function frames placeholders in \x00...\x00 (same
	// scheme as format.go). A literal NUL in the input (mangled LLM output)
	// would collide and corrupt placeholder restoration below.
	if strings.ContainsRune(text, 0) {
		text = strings.ReplaceAll(text, "\x00", "")
	}

	// 1. Zone-protect code and URL spans so mention wrapping never touches
	// their contents. Order matches format.go's convention: fenced code
	// first, then inline code, so backticks inside a fence aren't
	// re-extracted as inline code.
	fenced := bxExtractFencedCode(text)
	text = fenced.text
	inline := bxExtractInlineCode(text)
	text = inline.text
	urls := bxExtractOpenlineURLZones(text)
	text = urls.text

	// 2. Longest-first so a longer name (e.g. "Cường Egany") is wrapped
	// before a shorter one ("Cường") can match inside it.
	sorted := append([]string(nil), roster...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return utf8.RuneCountInString(sorted[i]) > utf8.RuneCountInString(sorted[j])
	})

	// 3. Wrap each roster name's unwrapped occurrences.
	var mentionNames []string
	for _, name := range sorted {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		text = wrapRosterName(text, name, matchMode(name), &mentionNames)
	}

	// 4. Restore placeholders: mentions first, then zones in reverse
	// extraction order (URL, then inline code, then fenced code).
	for i, name := range mentionNames {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00MN%d\x00", i), "["+name+"]")
	}
	for i, u := range urls.blocks {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00UR%d\x00", i), u)
	}
	for i, code := range inline.codes {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00IC%d\x00", i), "`"+code+"`")
	}
	for i, code := range fenced.codes {
		text = strings.ReplaceAll(text, fmt.Sprintf("\x00CB%d\x00", i), "```"+code+"```")
	}

	return text
}

// --- Per-chat roster cache ---
//
// Cross-turn wrapping needs the roster of `[Name]` mentions seen earlier in
// the same openline chat, not just the current turn's text — a name
// introduced 5 turns ago must still be re-wrappable now. The cache lives on
// the Channel (one instance per bot/portal), keyed by chatID, with an
// opportunistic TTL prune and size caps so a long-lived process can't grow
// this unbounded.

const (
	// openlineRosterTTL bounds how long a chat's roster survives without a
	// new inbound message. Hardcoded (no config surface) — YAGNI.
	openlineRosterTTL = 24 * time.Hour
	// openlineRosterMaxChats caps distinct chats tracked per Channel.
	// Exceeding it evicts the oldest-by-updatedAt chat.
	openlineRosterMaxChats = 100
	// openlineRosterMaxNames caps names tracked per chat. Exceeding it
	// drops the oldest first-seen name(s) — defends against a sender
	// flooding brackets.
	openlineRosterMaxNames = 20
)

// rosterEntry is one chat's accumulated [Name] roster.
type rosterEntry struct {
	names     []string            // first-seen order, canonical casing
	seen      map[string]struct{} // lowercased dedup keys
	updatedAt time.Time
}

// mergeAndGetRoster merges newNames into chatID's roster (dedup
// case-insensitively, preserving first-seen casing and order) and returns a
// copy of the current roster for that chat. Safe to call with an empty
// newNames slice to just read the cached roster for the current turn.
// Concurrency-safe; the returned slice is a copy so callers can't mutate the
// cache.
func (c *Channel) mergeAndGetRoster(chatID string, newNames []string) []string {
	c.rosterMu.Lock()
	defer c.rosterMu.Unlock()

	now := c.now()
	if c.rosterCache == nil {
		c.rosterCache = make(map[string]*rosterEntry)
	}

	// Opportunistic prune of TTL-expired entries (including chatID itself,
	// which naturally resets it below via the !ok branch).
	for id, e := range c.rosterCache {
		if now.Sub(e.updatedAt) > openlineRosterTTL {
			delete(c.rosterCache, id)
		}
	}

	entry, ok := c.rosterCache[chatID]
	if !ok {
		if len(c.rosterCache) >= openlineRosterMaxChats {
			var oldestID string
			var oldestAt time.Time
			first := true
			for id, e := range c.rosterCache {
				if first || e.updatedAt.Before(oldestAt) {
					oldestID, oldestAt, first = id, e.updatedAt, false
				}
			}
			if oldestID != "" {
				delete(c.rosterCache, oldestID)
			}
		}
		entry = &rosterEntry{seen: make(map[string]struct{})}
		c.rosterCache[chatID] = entry
	}

	for _, name := range newNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := entry.seen[key]; dup {
			continue
		}
		entry.seen[key] = struct{}{}
		entry.names = append(entry.names, name)
		if len(entry.names) > openlineRosterMaxNames {
			oldest := entry.names[0]
			entry.names = entry.names[1:]
			delete(entry.seen, strings.ToLower(oldest))
		}
	}
	entry.updatedAt = now

	out := make([]string, len(entry.names))
	copy(out, entry.names)
	return out
}
