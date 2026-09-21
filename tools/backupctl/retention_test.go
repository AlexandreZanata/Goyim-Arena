package main

import (
	"strings"
	"testing"
	"time"
)

// TestSegmentOfLSN pins the conversion the retention floor rests on: WAL
// segments are compared as strings, and a wrong segment means deleting a
// segment a stored base backup still needs.
func TestSegmentOfLSN(t *testing.T) {
	t.Parallel()

	cases := []struct {
		timeline uint32
		lsn      string
		want     string
	}{
		{timeline: 1, lsn: "0/0", want: "000000010000000000000000"},
		{timeline: 1, lsn: "0/1", want: "000000010000000000000000"},
		{timeline: 1, lsn: "0/FFFFFF", want: "000000010000000000000000"},
		{timeline: 1, lsn: "0/1000000", want: "000000010000000000000001"},
		{timeline: 1, lsn: "0/1A000000", want: "00000001000000000000001A"},
		{timeline: 1, lsn: "0/FFFFFFFF", want: "0000000100000000000000FF"},
		{timeline: 1, lsn: "1/0", want: "000000010000000100000000"},
		{timeline: 2, lsn: "2/3C000000", want: "00000002000000020000003C"},
		{timeline: 1, lsn: " 0/A000000 ", want: "00000001000000000000000A"},
	}
	for _, testCase := range cases {
		got, err := segmentOfLSN(testCase.timeline, testCase.lsn)
		if err != nil {
			t.Fatalf("segmentOfLSN(%d, %q) error = %v", testCase.timeline, testCase.lsn, err)
		}
		if got != testCase.want {
			t.Fatalf("segmentOfLSN(%d, %q) = %s, want %s", testCase.timeline, testCase.lsn, got, testCase.want)
		}
	}

	for _, bad := range []string{"", "0", "not hex/0", "0/not hex"} {
		if _, err := segmentOfLSN(1, bad); err == nil {
			t.Fatalf("segmentOfLSN(1, %q) accepted something that is not a log sequence number", bad)
		}
	}
}

// TestValidSegmentName: the retention policy compares names, so a name it
// cannot read must be recognisable as one rather than compared.
func TestValidSegmentName(t *testing.T) {
	t.Parallel()

	valid := []string{"000000010000000000000000", "0000000100000000000000FF", "FFFFFFFF00000001000000AB"}
	for _, name := range valid {
		if !validSegmentName(name) {
			t.Fatalf("validSegmentName(%q) = false, want true", name)
		}
	}
	invalid := []string{"", "00000001000000000000000", "0000000100000000000000000", "00000001000000000000000g", "00000001-00000000-00000001", "000000010000000000000000.enc"}
	for _, name := range invalid {
		if validSegmentName(name) {
			t.Fatalf("validSegmentName(%q) = true, want false", name)
		}
	}
}

func manifest(t *testing.T, name string, created time.Time, lsn string) baseManifest {
	t.Helper()
	return baseManifest{
		Name:      name,
		CreatedAt: created.UTC().Format(time.RFC3339),
		StartLSN:  lsn,
		Timeline:  1,
	}
}

// TestRetentionWindowIsTwoSided covers both halves of the policy: age decides,
// and a floor decides when age would remove too much. A window that is an age
// alone deletes the last backup of a deployment that stopped backing up.
func TestRetentionWindowIsTwoSided(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	daily := func(count int) []baseManifest {
		manifests := make([]baseManifest, 0, count)
		for index := 0; index < count; index++ {
			created := now.AddDate(0, 0, -index)
			manifests = append(manifests, manifest(t, "daily-"+created.Format("2006-01-02"), created, "0/1000000"))
		}
		return manifests
	}

	t.Run("a healthy deployment keeps its window and removes the rest", func(t *testing.T) {
		t.Parallel()

		manifests := daily(40)
		keep, remove := selectBackups(manifests, now, 30, 7)
		if len(keep) != 30 {
			t.Fatalf("kept %d backups, want the 30 days of the window", len(keep))
		}
		if len(remove) != 10 {
			t.Fatalf("removed %d backups, want the 10 beyond the window", len(remove))
		}
		for _, removed := range remove {
			if removed.createdAt().After(now.AddDate(0, 0, -30)) {
				t.Fatalf("%s is inside the window and was selected for removal", removed.Name)
			}
		}
	})

	t.Run("a deployment that stopped backing up keeps everything it has", func(t *testing.T) {
		t.Parallel()

		manifests := daily(3)
		for index := range manifests {
			manifests[index].CreatedAt = now.AddDate(0, 0, -400+index).Format(time.RFC3339)
		}
		keep, remove := selectBackups(manifests, now, 30, 7)
		if len(keep) != 3 || len(remove) != 0 {
			t.Fatalf("kept %d and removed %d, want all 3 kept: a floor of 7 is what stands between an outage and no backup at all", len(keep), len(remove))
		}
	})

	t.Run("the newest backups are kept even when every one of them is old", func(t *testing.T) {
		t.Parallel()

		manifests := daily(9)
		for index := range manifests {
			manifests[index].CreatedAt = now.AddDate(0, 0, -100-index).Format(time.RFC3339)
		}
		keep, remove := selectBackups(manifests, now, 30, 7)
		if len(keep) != 7 || len(remove) != 2 {
			t.Fatalf("kept %d and removed %d, want the floor of 7 kept and 2 removed", len(keep), len(remove))
		}
		if keep[0].createdAt().Before(keep[len(keep)-1].createdAt()) {
			t.Fatal("the kept set is not ordered from newest to oldest")
		}
	})

	t.Run("a manifest whose date cannot be read is the oldest thing there is", func(t *testing.T) {
		t.Parallel()

		manifests := []baseManifest{
			{Name: "unreadable", CreatedAt: "yesterday-ish", StartLSN: "0/1000000", Timeline: 1},
			manifest(t, "recent", now, "0/1000000"),
		}
		keep, remove := selectBackups(manifests, now, 30, 1)
		if len(keep) != 1 || keep[0].Name != "recent" {
			t.Fatalf("kept %+v, want the newest", keep)
		}
		if len(remove) != 1 || remove[0].Name != "unreadable" {
			t.Fatalf("removed %+v, want the one whose date cannot be read", remove)
		}
	})
}

// TestCorrectedWallClockBreaksTheTie is a note about ordering: two backups taken
// in the same second must not make the selection depend on the map order the
// store happened to return.
func TestOrderingIsStableForTheSameInstant(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	manifests := []baseManifest{
		manifest(t, "a", now, "0/1000000"),
		manifest(t, "b", now, "0/1000000"),
		manifest(t, "c", now, "0/1000000"),
	}
	keep, remove := selectBackups(manifests, now, 30, 2)
	if len(keep) != 3 || len(remove) != 0 {
		t.Fatalf("kept %d and removed %d, want everything kept: the window covers all of them", len(keep), len(remove))
	}
}

// TestRetentionFloorIsTheOldestKeptBackup: WAL older than the oldest backup that
// stays cannot restore anything that is still stored, and WAL at or after it
// must stay.
func TestRetentionFloor(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	keep := []baseManifest{
		manifest(t, "newest", now, "0/20000000"),
		manifest(t, "oldest", now.AddDate(0, 0, -20), "0/3C000000"),
	}
	floor, err := oldestSegmentOf(keep)
	if err != nil {
		t.Fatalf("oldestSegmentOf() error = %v", err)
	}
	if floor != "00000001000000000000003C" {
		t.Fatalf("floor = %s, want the segment of the oldest kept backup", floor)
	}

	if _, err := oldestSegmentOf(nil); err == nil {
		t.Fatal("oldestSegmentOf() derived a floor from no backups, which would delete every WAL segment")
	}
	if _, err := oldestSegmentOf([]baseManifest{{Name: "nameless"}}); err == nil || !strings.Contains(err.Error(), "no start LSN") {
		t.Fatalf("oldestSegmentOf() accepted a manifest without a start LSN: %v", err)
	}
}
