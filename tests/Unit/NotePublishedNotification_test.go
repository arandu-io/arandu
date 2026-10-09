// Example resource. Remove with the list under "The example resource" in README.md.

package unit_test

import (
	"strings"
	"testing"

	hnotifications "github.com/arandu-io/hesape/notifications"

	notifications "github.com/arandu-io/arandu/app/Notifications"
)

// TestTheNotePublishedNotificationCanBeDelivered builds the notification for a
// recipient and asks each channel's representation the question the channel
// asks before it sends: a key that can be stored, the channels Via promised,
// a payload that encodes, so a message that would be refused at
// midnight is refused here.
func TestTheNotePublishedNotificationCanBeDelivered(t *testing.T) {
	n := notifications.NotePublished{}
	to := hnotifications.Route(hnotifications.ChannelMail, "ada@example.com")

	if !n.Key().Valid() {
		t.Errorf("key %q cannot be stored: lowercase, dotted, no spaces", n.Key())
	}
	via := n.Via(to)
	want := []hnotifications.ChannelName{hnotifications.ChannelDatabase}
	if len(via) != len(want) {
		t.Fatalf("Via = %v, want %v", via, want)
	}
	for i := range want {
		if via[i] != want[i] {
			t.Errorf("Via = %v, want %v", via, want)
		}
	}
	if _, err := n.ToDatabase(to).JSON(); err != nil {
		t.Errorf("the stored payload does not encode: %v", err)
	}
}

// arandu:begin custom

// TestTheNotePublishedRowSaysWhichNote: the stored payload is what the bell
// menu draws from, so it carries the note's id and title.
func TestTheNotePublishedRowSaysWhichNote(t *testing.T) {
	n := notifications.NotePublished{NoteID: "note-1", Title: "Groceries"}
	payload, err := n.ToDatabase(hnotifications.Route(hnotifications.ChannelDatabase, "")).JSON()
	if err != nil {
		t.Fatalf("the stored payload does not encode: %v", err)
	}
	for _, want := range []string{`"note_id":"note-1"`, `"note_title":"Groceries"`} {
		if !strings.Contains(string(payload), want) {
			t.Errorf("payload %s does not carry %s", payload, want)
		}
	}
}

// arandu:end custom
