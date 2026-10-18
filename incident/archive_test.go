package incident

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rothskeller/packet/v4/message"
	"github.com/rothskeller/pdf/v2"
)

// receiveTestMessage receives a plain-text message into the incident in dir,
// as a BBS exchange does (queuing its delivery receipt).
func receiveTestMessage(t *testing.T, dir string, n int) {
	t.Helper()
	raw := fmt.Sprintf("Received: FROM w1xsc.scc-ares-races.org BY pktmsg.local FOR kc6rsc;\n\tSat, 17 Oct 2026 11:%02d:00 -0700\nDate: Sat, 17 Oct 2026 11:%02d:00 -0700\nFrom: pktmon@w2xsc.scc-ares-races.org\nTo: kc6rsc@w1xsc.scc-ares-races.org\nSubject: MON-%03dP_R_Test message %d\n\nThis is test message %d.\n", n, n, n, n, n)
	msg, err := message.NewJustReceivedMessage(raw, "W1XSC", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = Write(dir, func(i *Incident) error {
		dr, _, err := i.ReceiveMessage(msg)
		if err != nil {
			return err
		}
		if dr != nil {
			_, err = i.AddDraftMessage(dr)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateArchive(t *testing.T) {
	dir := newTestIncident(t)
	// Nothing to archive yet.
	if err := Read(dir, func(i *Incident) error { return i.GenerateArchive() }); err != ErrEmptyArchive {
		t.Fatalf("empty incident: got %v, want ErrEmptyArchive", err)
	}
	receiveTestMessage(t, dir, 1)
	receiveTestMessage(t, dir, 2)
	if err := Read(dir, func(i *Incident) error { return i.GenerateArchive() }); err != nil {
		t.Fatal(err)
	}

	// The text archive has both messages, in log order, under headings,
	// and leaves out the unsent delivery receipts.
	text, err := os.ReadFile(filepath.Join(dir, ArchiveTextFile))
	if err != nil {
		t.Fatal(err)
	}
	s := string(text)
	for _, want := range []string{"PACKET MESSAGE ARCHIVE\n", "Operator:  KC6RSC Test Operator\n",
		"Received XSC-100M from PKTMON (MON-001P)", "This is test message 1.",
		"Received XSC-101M from PKTMON (MON-002P)", "This is test message 2."} {
		if !strings.Contains(s, want) {
			t.Errorf("text archive lacks %q:\n%s", want, s)
		}
	}
	if strings.Index(s, "test message 1.") > strings.Index(s, "test message 2.") {
		t.Error("text archive is not in log order")
	}
	if strings.Contains(s, "DELIVERED:") {
		t.Errorf("text archive includes unsent receipts:\n%s", s)
	}

	// The PDF archive has the pages of both messages' PDFs.
	var want int
	for _, name := range []string{"XSC-100M.pdf", "XSC-101M.pdf"} {
		want += pdfPages(t, filepath.Join(dir, name))
	}
	if got := pdfPages(t, filepath.Join(dir, ArchivePDFFile)); got != want || got == 0 {
		t.Errorf("PDF archive has %d pages, want %d", got, want)
	}

	// Each page is stamped with its archive page number.
	raw, err := os.ReadFile(filepath.Join(dir, ArchivePDFFile))
	if err != nil {
		t.Fatal(err)
	}
	for p := 1; p <= want; p++ {
		if stamp := fmt.Sprintf("(Archive page %d of %d)", p, want); !strings.Contains(string(raw), stamp) {
			t.Errorf("PDF archive lacks %q", stamp)
		}
	}

	// A missing message PDF is rendered for the archive.
	os.Remove(filepath.Join(dir, "XSC-101M.pdf"))
	os.Remove(filepath.Join(dir, ArchivePDFFile))
	if err := Read(dir, func(i *Incident) error { return i.GenerateArchive() }); err != nil {
		t.Fatal(err)
	}
	if got := pdfPages(t, filepath.Join(dir, ArchivePDFFile)); got != want {
		t.Errorf("PDF archive with a message PDF missing has %d pages, want %d", got, want)
	}

	// Any change to the incident removes the archive.
	receiveTestMessage(t, dir, 3)
	for _, name := range []string{ArchiveTextFile, ArchivePDFFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s not removed when the incident changed", name)
		}
	}
}

func pdfPages(t *testing.T, fname string) int {
	t.Helper()
	fh, err := os.Open(fname)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	p, err := pdf.Open(fh)
	if err != nil {
		t.Fatalf("%s: %v", fname, err)
	}
	n, err := p.NumPages()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestArchiveHeading(t *testing.T) {
	for _, tc := range []struct {
		le   LogEntry
		want string
	}{
		{LogEntry{Status: StatusSent, LocalMsgID: "NVY-100P", ToCall: "PKTMON", ToMsgID: "MON-19758P"}, "Sent NVY-100P to PKTMON (MON-19758P)"},
		{LogEntry{Status: StatusSent, LocalMsgID: "NVY-100P", ToCall: "PKTMON"}, "Sent NVY-100P to PKTMON"},
		{LogEntry{Status: StatusReceived, LocalMsgID: "NVY-200M", FromCall: "PKTMON", FromMsgID: "MON-1P"}, "Received NVY-200M from PKTMON (MON-1P)"},
		{LogEntry{Status: StatusReceived, Flags: FIsReceipt, LocalMsgID: "NVY-100P", FromCall: "PKTMON", FromMsgID: "MON-1P"}, "Received receipt for NVY-100P from PKTMON (MON-1P)"},
		{LogEntry{Status: StatusHandEntered, Flags: FVoice, FromCall: "W6ABC", ToCall: "KC6RSC"}, "Voice message from W6ABC to KC6RSC"},
	} {
		if got := archiveHeading(&tc.le); !strings.HasSuffix(got, "  "+tc.want) {
			t.Errorf("archiveHeading = %q, want ... %q", got, tc.want)
		}
	}
}
