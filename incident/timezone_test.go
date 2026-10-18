package incident

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rothskeller/packet/v4/cmd/packet/osdep"
)

// withTimeZone makes the named zone the local time zone for the duration of
// the test.
func withTimeZone(t *testing.T, name string) {
	t.Helper()
	if err := setLocal(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { setLocal("") })
}

// withDefaultsFile points the incident defaults at a scratch file for the
// duration of the test.
func withDefaultsFile(t *testing.T) string {
	t.Helper()
	saved := osdep.DefaultsFile
	osdep.DefaultsFile = filepath.Join(t.TempDir(), "packet.json")
	t.Cleanup(func() { osdep.DefaultsFile = saved })
	return osdep.DefaultsFile
}

func TestLoadTimeZone(t *testing.T) {
	for _, name := range []string{"", "Local", " local "} {
		if loc, err := LoadTimeZone(name); err != nil || loc != systemZone {
			t.Errorf("LoadTimeZone(%q) = %v, %v; want the system zone", name, loc, err)
		}
	}
	if loc, err := LoadTimeZone("America/Los_Angeles"); err != nil || loc.String() != "America/Los_Angeles" {
		t.Errorf("LoadTimeZone(America/Los_Angeles) = %v, %v", loc, err)
	}
	if _, err := LoadTimeZone("Nowhere/Special"); err == nil {
		t.Error("LoadTimeZone(Nowhere/Special): expected error")
	}
}

func TestSetTimeZone(t *testing.T) {
	withDefaultsFile(t)
	t.Cleanup(func() { setLocal("") })

	if err := SetTimeZone("Nowhere/Special"); err == nil {
		t.Error("SetTimeZone(Nowhere/Special): expected error")
	}
	if TimeZone() != "" || time.Local != systemZone {
		t.Fatalf("a bad zone changed the time zone to %q", TimeZone())
	}
	if err := SetTimeZone("Europe/London"); err != nil {
		t.Fatal(err)
	}
	if TimeZone() != "Europe/London" || time.Local.String() != "Europe/London" {
		t.Errorf("time zone is %q (time.Local %v), want Europe/London", TimeZone(), time.Local)
	}
	if got := GetIncDefaults().TimeZone; got != "Europe/London" {
		t.Errorf("saved time zone is %q, want Europe/London", got)
	}
	// It is restored at the next startup.
	setLocal("")
	ApplyTimeZone()
	if TimeZone() != "Europe/London" {
		t.Errorf("ApplyTimeZone: time zone is %q, want Europe/London", TimeZone())
	}
	// Back to the system time zone.
	if err := SetTimeZone(""); err != nil {
		t.Fatal(err)
	}
	if TimeZone() != "" || time.Local != systemZone || GetIncDefaults().TimeZone != "" {
		t.Errorf("SetTimeZone(\"\"): time zone is %q, saved %q", TimeZone(), GetIncDefaults().TimeZone)
	}
}

// TestSetTimeZoneKeepsDefaults checks that saving the time zone leaves the
// other incident defaults alone, and vice versa.
func TestSetTimeZoneKeepsDefaults(t *testing.T) {
	withDefaultsFile(t)
	t.Cleanup(func() { setLocal("") })

	inc := &Incident{Dir: t.TempDir(), Config: &Config{OpCall: "KC6RSC", OpName: "Test"}}
	inc.UpdateIncDefaults()
	if err := SetTimeZone("America/Chicago"); err != nil {
		t.Fatal(err)
	}
	inc.Config.OpName = "Changed"
	inc.UpdateIncDefaults()
	if d := GetIncDefaults(); d.OpCall != "KC6RSC" || d.OpName != "Changed" || d.TimeZone != "America/Chicago" {
		t.Errorf("defaults = %+v", d)
	}
}

// TestLogTimesInLocalZone checks that log entry times recorded in one time
// zone are shown in the configured one.
func TestLogTimesInLocalZone(t *testing.T) {
	withTimeZone(t, "America/New_York")
	fname := filepath.Join(t.TempDir(), jsonFileName)
	// 08:05 Pacific Daylight Time is 11:05 Eastern.
	if err := os.WriteFile(fname, []byte(`{"seq":1,"conf":{"opStart":"2026-10-16T08:05"},"bull":null,"log":[null,{"t":"2026-10-16T08:05:00-07:00"}]}`), 0666); err != nil {
		t.Fatal(err)
	}
	inc, err := readStateFile(fname)
	if err != nil {
		t.Fatal(err)
	}
	if got := inc.Log[1].Time.Format("15:04 MST"); got != "11:05 EDT" {
		t.Errorf("log entry time = %s, want 11:05 EDT", got)
	}
	if got := inc.Config.OpStart.Format("15:04 MST"); got != "08:05 EDT" {
		t.Errorf("OpStart = %s, want 08:05 EDT", got)
	}
}
