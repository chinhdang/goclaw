package bitrix24

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

func newRosterTestChannel() *Channel {
	return &Channel{}
}

func containsString(list []string, s string) bool {
	return slices.Contains(list, s)
}

func TestMergeAndGetRoster_CrossTurn(t *testing.T) {
	ch := newRosterTestChannel()
	got := ch.mergeAndGetRoster("chat1", []string{"Hiền Trịnh"})
	if !equalStrings(got, []string{"Hiền Trịnh"}) {
		t.Fatalf("first merge: got %v", got)
	}
	// Turn with no new brackets still surfaces the cached roster.
	got = ch.mergeAndGetRoster("chat1", nil)
	if !equalStrings(got, []string{"Hiền Trịnh"}) {
		t.Errorf("cross-turn: got %v; want cached roster to persist", got)
	}
}

func TestMergeAndGetRoster_DedupFirstSeenCasing(t *testing.T) {
	ch := newRosterTestChannel()
	ch.mergeAndGetRoster("chat1", []string{"Cường Egany"})
	got := ch.mergeAndGetRoster("chat1", []string{"cường egany"})
	if !equalStrings(got, []string{"Cường Egany"}) {
		t.Errorf("got %v; want first-seen casing preserved, no dup", got)
	}
}

func TestMergeAndGetRoster_ChatIsolation(t *testing.T) {
	ch := newRosterTestChannel()
	ch.mergeAndGetRoster("chat1", []string{"Alice"})
	got := ch.mergeAndGetRoster("chat2", nil)
	if len(got) != 0 {
		t.Errorf("chat2 must not see chat1's roster, got %v", got)
	}
}

func TestMergeAndGetRoster_TTLExpiry(t *testing.T) {
	ch := newRosterTestChannel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ch.nowFn = func() time.Time { return now }

	ch.mergeAndGetRoster("chat1", []string{"Old Name"})
	now = now.Add(25 * time.Hour) // past openlineRosterTTL (24h)
	got := ch.mergeAndGetRoster("chat1", []string{"New Name"})
	if !equalStrings(got, []string{"New Name"}) {
		t.Errorf("expired entry should reset: got %v", got)
	}
}

func TestMergeAndGetRoster_MaxNamesCap(t *testing.T) {
	ch := newRosterTestChannel()
	names := make([]string, 25)
	for i := range names {
		names[i] = fmt.Sprintf("Name%02d", i)
	}
	got := ch.mergeAndGetRoster("chat1", names)
	if len(got) != openlineRosterMaxNames {
		t.Fatalf("len = %d; want %d", len(got), openlineRosterMaxNames)
	}
	for i := range 5 {
		if containsString(got, names[i]) {
			t.Errorf("oldest name %q should have been dropped, got %v", names[i], got)
		}
	}
	for i := 5; i < 25; i++ {
		if !containsString(got, names[i]) {
			t.Errorf("recent name %q should remain, got %v", names[i], got)
		}
	}
}

func TestMergeAndGetRoster_MaxChatsEviction(t *testing.T) {
	ch := newRosterTestChannel()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range openlineRosterMaxChats {
		ts := base.Add(time.Duration(i) * time.Minute)
		ch.nowFn = func() time.Time { return ts }
		ch.mergeAndGetRoster(fmt.Sprintf("chat%d", i), []string{"N"})
	}
	// One more chat past capacity — oldest-by-updatedAt (chat0) must go.
	latest := base.Add(time.Duration(openlineRosterMaxChats) * time.Minute)
	ch.nowFn = func() time.Time { return latest }
	ch.mergeAndGetRoster("chatNEW", []string{"N"})

	ch.rosterMu.Lock()
	_, chat0Exists := ch.rosterCache["chat0"]
	size := len(ch.rosterCache)
	ch.rosterMu.Unlock()
	if chat0Exists {
		t.Error("oldest chat (chat0) should have been evicted")
	}
	if size != openlineRosterMaxChats {
		t.Errorf("cache size = %d; want capped at %d", size, openlineRosterMaxChats)
	}
}

func TestMergeAndGetRoster_ConcurrencySafe(t *testing.T) {
	ch := newRosterTestChannel()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			chatID := fmt.Sprintf("chat%d", i%5)
			ch.mergeAndGetRoster(chatID, []string{fmt.Sprintf("Name%d", i)})
		}(i)
	}
	wg.Wait()
}

func TestMergeAndGetRoster_ReturnIsCopy(t *testing.T) {
	ch := newRosterTestChannel()
	got := ch.mergeAndGetRoster("chat1", []string{"Alice"})
	got[0] = "Mutated"
	got2 := ch.mergeAndGetRoster("chat1", nil)
	if got2[0] != "Alice" {
		t.Errorf("cache was mutated via returned slice: got %v", got2)
	}
}
