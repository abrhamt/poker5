package poker

import "testing"

// Games saved before notifications gained a kind and a sequence hold a plain
// JSON array of strings. Those rows are already in the production database, so
// reading them has to keep working without a migration.
func TestDecodeLegacyNotifications(t *testing.T) {
	got := decodeNotifications(`["alice folded.","bob wins 120!"]`)

	if len(got) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(got))
	}
	if got[0].Text != "alice folded." || got[1].Text != "bob wins 120!" {
		t.Fatalf("text not carried over: %+v", got)
	}
	// Sequences must still be monotonic, or the client cannot tell new lines
	// from old ones after a restart.
	if got[0].Seq != 1 || got[1].Seq != 2 {
		t.Fatalf("expected sequences 1,2 got %d,%d", got[0].Seq, got[1].Seq)
	}
	if got[0].Kind != NoteSystem {
		t.Fatalf("legacy lines should decode as %q, got %q", NoteSystem, got[0].Kind)
	}
}

func TestDecodeStructuredNotifications(t *testing.T) {
	got := decodeNotifications(`[{"seq":7,"kind":"action","text":"carol raised to 80."}]`)

	if len(got) != 1 || got[0].Seq != 7 || got[0].Kind != NoteAction {
		t.Fatalf("structured decode failed: %+v", got)
	}
}

func TestDecodeNotificationsHandlesJunk(t *testing.T) {
	for _, raw := range []string{"", "null", "not json", "{}"} {
		if got := decodeNotifications(raw); got == nil {
			t.Fatalf("decode(%q) returned nil rather than an empty slice", raw)
		}
	}
}

// Sequences keep climbing as lines scroll out of the window, so a client that
// has seen seq N can always tell what is new.
func TestNotificationSequenceSurvivesTrimming(t *testing.T) {
	e := &GameEngine{Game: NewGame("t")}

	for i := 0; i < MaxNotifications+5; i++ {
		e.addNotification(NoteAction, "someone checked.")
	}

	notes := e.Game.Notifications
	if len(notes) != MaxNotifications {
		t.Fatalf("expected the window to cap at %d, got %d", MaxNotifications, len(notes))
	}
	if first, last := notes[0].Seq, notes[len(notes)-1].Seq; last-first != MaxNotifications-1 {
		t.Fatalf("sequences not contiguous across a trim: %d..%d", first, last)
	}
	if notes[len(notes)-1].Seq != MaxNotifications+5 {
		t.Fatalf("expected the newest line to be seq %d, got %d", MaxNotifications+5, notes[len(notes)-1].Seq)
	}
}
