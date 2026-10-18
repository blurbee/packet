package cmd

import (
	"os"

	"github.com/rothskeller/packet/v4/cmd/packet/cio"
	"github.com/rothskeller/packet/v4/cmd/packet/osdep"
	"github.com/rothskeller/packet/v4/errors"
	"github.com/rothskeller/packet/v4/incident"
	"github.com/spf13/pflag"
)

const (
	archiveSlug = `Create and show the message archive for the incident`
	archiveHelp = `
usage: packet archive [--text]
  -t, --text  ⇥Show the text archive rather than the PDF one

The "packet archive" command shows the message archive for the incident: every sent and received message, and every hand-entered log entry, in log order.  By default it shows the PDF archive (all.pdf, with each message's PDF) in the system default PDF viewer; with --text, it shows the text archive (all.txt, with each message's text) in the system default text viewer.  The archive is generated if it doesn't already exist, and removed whenever the incident changes.
`
)

func cmdArchive(args []string) (err error) {
	var text bool

	flags := pflag.NewFlagSet("archive", pflag.ContinueOnError)
	flags.BoolVarP(&text, "text", "t", false, "Show the text archive")
	flags.Usage = func() {} // we do our own
	if err = flags.Parse(args); err == pflag.ErrHelp {
		return cmdHelp([]string{"archive"})
	} else if err != nil {
		cio.Open().Error(err)
		return usage(archiveHelp)
	}
	if flags.NArg() != 0 {
		return usage(archiveHelp)
	}
	file := incident.ArchivePDFFile
	if text {
		file = incident.ArchiveTextFile
	}
	if _, err = os.Stat(file); os.IsNotExist(err) {
		if err = incRead(func(i *incident.Incident) error {
			return i.GenerateArchive()
		}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	var showcmd = osdep.OpenFileCommand(file)
	if err := showcmd.Start(); err != nil {
		return errors.NewF("Unable to start viewer: %s", err)
	}
	go func() { showcmd.Wait() }()
	return nil
}
