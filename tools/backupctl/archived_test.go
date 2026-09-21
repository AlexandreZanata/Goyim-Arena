package main

import "testing"

// TestValidArchivedName covers what PostgreSQL hands to archive_command and what
// the restore_command is therefore asked for later. The case that motivates the
// test is the .backup history file: a validator that accepted only the segments
// made the archive command fail on it, and because a checkpoint waits for the
// archive, the visible symptom was not a missing object but a base backup that
// never finished.
func TestValidArchivedName(t *testing.T) {
	testCases := []struct {
		name string
		want bool
	}{
		// The segment itself.
		{"000000010000000000000001", true},
		{"FFFFFFFF00000000FF0000FF", true},
		// The backup history file the server writes when a base backup ends.
		{"000000010000000000000003.00000028.backup", true},
		// The timeline history, without which a recovery cannot follow a switch.
		{"00000002.history", true},
		{"0000000A.history", true},
		// archive_mode=always archives partial segments too.
		{"000000010000000000000002.partial", true},

		// Shapes that must never be accepted: a name the archive cannot produce
		// is a name a caller invented, and a bad object name fails at the store
		// with a message about signing rather than about the caller's mistake.
		{"", false},
		{"00000001000000000000001", false},
		{"0000000100000000000000001", false},
		{"0000000g0000000000000001", false},
		{"00000001000000000000000a", false},
		{"000000010000000000000001.enc", false},
		{"000000010000000000000003.00000028.backup.enc", false},
		{"000000010000000000000003.backup", false},
		{"000000010000000000000003.0000002.backup", false},
		{"000000010000000000000003.00000028", false},
		{"0000001.history", false},
		{"000000001.history", false},
		{"00000002.history.enc", false},
		{"000000010000000000000002.partial.enc", false},
		{"000000010000000000000002.PARTIAL", false},
		{"../000000010000000000000001", false},
		{"wal/000000010000000000000001", false},
	}
	for _, testCase := range testCases {
		if got := validArchivedName(testCase.name); got != testCase.want {
			t.Errorf("validArchivedName(%q) = %t, want %t", testCase.name, got, testCase.want)
		}
	}
}

// TestValidSegmentNameStaysStrict is the other half: the retention floor compares
// segment names as strings, so the validator the floor relies on must keep
// refusing everything that is not a segment — including the history files the
// archive happily accepts.
func TestValidSegmentNameStaysStrict(t *testing.T) {
	for _, name := range []string{
		"000000010000000000000003.00000028.backup",
		"00000002.history",
		"000000010000000000000002.partial",
		"00000001000000000000000a",
	} {
		if validSegmentName(name) {
			t.Errorf("validSegmentName(%q) accepted a name the retention floor must not compare", name)
		}
	}
	if !validSegmentName("000000010000000000000001") {
		t.Fatal("validSegmentName rejected a WAL segment")
	}
}
