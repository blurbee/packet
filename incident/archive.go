package incident

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rothskeller/packet/v4/message"
	"github.com/rothskeller/pdf/v2"
)

// Names of the archive files in the incident directory.  (They contain no
// hyphen, so they can't conflict with a message number.)
const (
	ArchiveTextFile = "all.txt"
	ArchivePDFFile  = "all.pdf"
)

// ErrEmptyArchive is returned by GenerateArchive when the incident has no
// sent or received messages (or log entries) to archive.
var ErrEmptyArchive = errors.New("The incident has no sent or received messages to archive.")

// GenerateArchive creates the incident archive in the incident directory, in
// text (all.txt) and PDF (all.pdf) formats, unless they already exist.  (They
// are removed whenever the incident changes; see Write.)  The archive contains
// every sent and received message, and every hand-entered log entry, in log
// order; drafts are not included.  The text archive has each message's text
// (receipts included), and the PDF archive has each message's PDF (receipts
// have none).  It should be called with the incident read-locked.
func (i *Incident) GenerateArchive() (err error) {
	var entries []*LogEntry
	for _, le := range i.Log {
		switch le.Status {
		case StatusSent, StatusReceived, StatusHandEntered:
			entries = append(entries, le)
		}
	}
	if len(entries) == 0 {
		return ErrEmptyArchive
	}
	if _, err = os.Stat(filepath.Join(i.Dir, ArchiveTextFile)); err != nil {
		if err = i.writeArchiveText(entries); err != nil {
			return err
		}
	}
	if _, err = os.Stat(filepath.Join(i.Dir, ArchivePDFFile)); err != nil {
		if err = i.writeArchivePDF(entries); err != nil {
			return err
		}
	}
	return nil
}

// archiveRule separates the entries in the text archive.
var archiveRule = strings.Repeat("=", 72) + "\n"

// writeArchiveText writes all.txt.
func (i *Incident) writeArchiveText(entries []*LogEntry) (err error) {
	var sb strings.Builder

	sb.WriteString("PACKET MESSAGE ARCHIVE\n")
	if c := i.Config; c != nil {
		if name := strings.TrimSpace(c.IncidentName + " " + c.ActivationNum); name != "" {
			fmt.Fprintf(&sb, "Incident:  %s\n", name)
		}
		if c.TacCall != "" {
			fmt.Fprintf(&sb, "Station:   %s\n", strings.TrimSpace(c.TacCall+" "+c.TacName))
		}
		if c.OpCall != "" {
			fmt.Fprintf(&sb, "Operator:  %s\n", strings.TrimSpace(c.OpCall+" "+c.OpName))
		}
	}
	fmt.Fprintf(&sb, "Generated: %s\n", time.Now().Format("2006-01-02 15:04"))
	for _, le := range entries {
		sb.WriteString("\n" + archiveRule)
		sb.WriteString(archiveHeading(le) + "\n")
		sb.WriteString(archiveRule)
		fname := le.Filename()
		if fname == "" {
			if le.Subject != "" {
				sb.WriteString(le.Subject + "\n")
			}
			continue
		}
		text, err := os.ReadFile(filepath.Join(i.Dir, fname))
		if err != nil {
			// Don't lose the whole archive over one missing file;
			// note it there instead.
			slog.Error("archive: os.ReadFile", "f", fname, "err", err)
			fmt.Fprintf(&sb, "[The message file %s could not be read: %s]\n", fname, err)
			continue
		}
		text = bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
		sb.Write(text)
		if len(text) != 0 && text[len(text)-1] != '\n' {
			sb.WriteByte('\n')
		}
	}
	return writeFileAtomic(filepath.Join(i.Dir, ArchiveTextFile), []byte(sb.String()))
}

// archiveHeading returns the heading line for a log entry in the text archive,
// e.g., "2026-10-17 10:54  Sent NVY-100P to PKTMON (MON-19758P)".
func archiveHeading(le *LogEntry) string {
	var what string

	switch {
	case le.Status == StatusHandEntered:
		what = "Log entry"
		if le.Flags&FVoice != 0 {
			what = "Voice message"
		}
		what += idAndCall(le.LocalMsgID, "")
		if le.FromCall != "" || le.FromMsgID != "" {
			what += " from" + idAndCall(le.FromCall, le.FromMsgID)
		}
		if le.ToCall != "" || le.ToMsgID != "" {
			what += " to" + idAndCall(le.ToCall, le.ToMsgID)
		}
	case le.Flags&FIsReceipt != 0 && le.Status == StatusSent:
		what = "Sent receipt for " + le.LocalMsgID + " to" + idAndCall(le.ToCall, "")
	case le.Flags&FIsReceipt != 0:
		what = "Received receipt for " + le.LocalMsgID + " from" + idAndCall(le.FromCall, le.FromMsgID)
	case le.Status == StatusSent:
		what = "Sent " + le.LocalMsgID + " to" + idAndCall(le.ToCall, le.ToMsgID)
	default:
		what = "Received " + le.LocalMsgID + " from" + idAndCall(le.FromCall, le.FromMsgID)
	}
	return le.Time.Format("2006-01-02 15:04") + "  " + what
}

// idAndCall formats " CALL (ID)", leaving out whatever is empty.
func idAndCall(call, id string) (s string) {
	if call != "" {
		s = " " + call
	}
	if id != "" {
		if s == "" {
			s = " " + id
		} else {
			s += " (" + id + ")"
		}
	}
	return s
}

// writeArchivePDF writes all.pdf, combining the PDFs of the archived messages.
// A message whose PDF is missing gets one rendered for the purpose.
func (i *Incident) writeArchivePDF(entries []*LogEntry) (err error) {
	var (
		fname = filepath.Join(i.Dir, ArchivePDFFile)
		tmp   = fname + ".tmp"
		out   *os.File
		op    *pdf.PDF
		pages int
	)
	if out, err = os.Create(tmp); err != nil {
		slog.Error("os.Create", "f", tmp, "err", err)
		return err
	}
	defer func() {
		out.Close()
		if err != nil {
			os.Remove(tmp)
		}
	}()
	op = pdf.New(out)
	op.Info["Title"] = "Packet Message Archive"
	if i.Config != nil && i.Config.IncidentName != "" {
		op.Info["Title"] = "Packet Message Archive: " + i.Config.IncidentName
	}
	op.Info["Producer"] = "https://github.com/rothskeller/packet"
	for _, le := range entries {
		if le.Flags&FIsReceipt != 0 || le.Filename() == "" {
			continue // no PDF
		}
		var added int
		if added, err = i.appendMessagePDF(op, le, pages); err != nil {
			return err
		}
		pages += added
	}
	if pages != 0 {
		if err = stampArchivePages(op, pages); err != nil {
			return err
		}
	}
	if pages == 0 {
		// Only receipts and hand-entered log entries: there's nothing to
		// show in PDF form.  Leave a one-page note rather than an
		// invalid empty PDF.
		if err = op.AddPage(pdf.USLetterPortrait); err != nil {
			return err
		}
		if err = op.AddPageContent(1, "BT /F1 14 Tf 72 700 Td (No messages in this incident have a PDF form.) Tj ET"); err != nil {
			return err
		}
		if err = op.AddPageResource(1, "Font", "F1", pdf.Dict{"Type": pdf.Name("Font"), "Subtype": pdf.Name("Type1"), "BaseFont": pdf.Name("Helvetica")}); err != nil {
			return err
		}
	}
	if err = op.Write(); err != nil {
		slog.Error("pdf.Write", "f", tmp, "err", err)
		return err
	}
	if err = out.Close(); err != nil {
		slog.Error("os.Close", "f", tmp, "err", err)
		return err
	}
	if err = os.Rename(tmp, fname); err != nil {
		slog.Error("os.Rename", "f", tmp, "err", err)
		return err
	}
	return nil
}

// appendMessagePDF appends the pages of the PDF of the message for le to op,
// after its first pages pages, and returns the number of pages added.
func (i *Incident) appendMessagePDF(op *pdf.PDF, le *LogEntry, pages int) (added int, err error) {
	var (
		pdfname = filepath.Join(i.Dir, ToPDF(le.Filename()))
		fh      *os.File
		ip      *pdf.PDF
		imp     *pdf.Importer
		count   int
	)
	if fh, err = os.Open(pdfname); os.IsNotExist(err) {
		// The PDF is missing (e.g., removed by hand): render it into
		// a temporary file.
		var msg message.Message
		if msg, err = i.GetMessageFromLogEntry(le); msg == nil {
			slog.Error("archive: read message", "f", le.Filename(), "err", err)
			return 0, nil // the text archive notes the problem
		}
		tmp, err := os.CreateTemp("", "archive-*.pdf")
		if err != nil {
			return 0, err
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		switch err = msg.Type().RenderPDF(msg, tmp.Name(), ""); err.(type) {
		case nil, message.Warning:
		default:
			slog.Error("archive: RenderPDF", "f", le.Filename(), "err", err)
			return 0, nil
		}
		pdfname = tmp.Name()
		if fh, err = os.Open(pdfname); err != nil {
			return 0, err
		}
	} else if err != nil {
		slog.Error("os.Open", "f", pdfname, "err", err)
		return 0, err
	}
	defer fh.Close()
	if ip, err = pdf.Open(fh); err != nil {
		slog.Error("archive: pdf.Open", "f", pdfname, "err", err)
		return 0, fmt.Errorf("%s: %s", filepath.Base(pdfname), err)
	}
	if count, err = ip.NumPages(); err != nil {
		return 0, err
	}
	if imp, err = op.NewImporter(ip); err != nil {
		return 0, err
	}
	for p := 1; p <= count; p++ {
		if err = imp.ImportPage(p, pages+p); err != nil {
			slog.Error("archive: ImportPage", "f", pdfname, "page", p, "err", err)
			return 0, fmt.Errorf("%s page %d: %s", filepath.Base(pdfname), p, err)
		}
	}
	return count, nil
}

// archiveFont is the resource name of the font used for the archive page
// numbers.  (It must not clash with the imported pages' resource names.)
const archiveFont = "ArchivePageFont"

// archivePageFontSize and archivePageBaseline give the size of the archive
// page numbers and the height of their baseline above the bottom of the page,
// in points.  At the bottom center, they stay clear of the message number
// (bottom left) and the form's own "Page 1 of 1" (bottom right) that the
// message PDFs have, below them.
const (
	archivePageFontSize = 9.0
	archivePageBaseline = 20.0
)

// stampArchivePages adds "Archive page N of TOTAL" to the bottom center of
// each page of the archive PDF, since each message's own page numbers count
// only its own pages.
func stampArchivePages(op *pdf.PDF, total int) (err error) {
	font := pdf.Dict{"Type": pdf.Name("Font"), "Subtype": pdf.Name("Type1"), "BaseFont": pdf.Name("Helvetica"),
		"Encoding": pdf.Name("WinAnsiEncoding")}
	for p := 1; p <= total; p++ {
		var (
			path pdf.Path
			box  pdf.Rectangle
		)
		if path, err = op.PagePath(p); err != nil {
			return err
		}
		if box, err = op.GetRectangle(path.K("MediaBox")); err != nil {
			box = pdf.USLetterPortrait
		}
		text := fmt.Sprintf("Archive page %d of %d", p, total)
		x := box.LLX + (box.URX-box.LLX-helveticaWidth(text, archivePageFontSize))/2
		if err = op.AddPageResource(p, "Font", archiveFont, font); err != nil && !strings.Contains(err.Error(), "already exists") {
			return err
		}
		content := fmt.Sprintf("q BT /%s %g Tf 0 g %.2f %.2f Td (%s) Tj ET Q",
			archiveFont, archivePageFontSize, x, box.LLY+archivePageBaseline, text)
		if err = op.AddPageContent(p, content); err != nil {
			return err
		}
	}
	return nil
}

// helveticaWidth returns the width, in points, of text (letters, digits and
// spaces) set in Helvetica at size.
func helveticaWidth(text string, size float64) float64 {
	var units int
	for _, c := range text {
		switch {
		case c == ' ' || c == 'f' || c == 't':
			units += 278
		case c == 'r':
			units += 333
		case c == 'i' || c == 'l' || c == 'j':
			units += 222
		case strings.ContainsRune("cksvxyz", c):
			units += 500
		case c == 'm':
			units += 833
		case c == 'w':
			units += 722
		case c >= 'A' && c <= 'Z':
			units += 667
		default: // other lower case letters, and digits
			units += 556
		}
	}
	return float64(units) * size / 1000
}

// writeFileAtomic writes data to fname by way of a temporary file, so that a
// reader never sees a partial file.
func writeFileAtomic(fname string, data []byte) error {
	tmp := fname + ".tmp"
	if err := os.WriteFile(tmp, data, 0666); err != nil {
		slog.Error("os.WriteFile", "f", tmp, "err", err)
		return err
	}
	if err := os.Rename(tmp, fname); err != nil {
		os.Remove(tmp)
		slog.Error("os.Rename", "f", tmp, "err", err)
		return err
	}
	return nil
}
