package htmlop

import (
	"bytes"
	"strings"
	"testing"
)

func TestAddTextImport(t *testing.T) {
	for _, tc := range []struct{ name, in, field, want string }{
		{"form label", `<label class=pair><div class=label>Message:</div><textarea name=12. required></textarea></label>`, "12.",
			`<div class=label>Message: <a href=# class=text-import data-target=12.`},
		{"plain label", `<div class=pair><label for=body>Message</label><textarea id=body name=body></textarea></div>`, "body",
			`<label for=body>Message <a href=# class=text-import data-target=body`},
		{"no label", `<div><textarea name=body></textarea></div>`, "body",
			`<div><a href=# class=text-import data-target=body`},
	} {
		doc, err := Parse(strings.NewReader(tc.in))
		if err != nil {
			t.Fatal(err)
		}
		if !AddTextImport(doc, tc.field) {
			t.Errorf("%s: textarea not found", tc.name)
			continue
		}
		var out bytes.Buffer
		Minify(&out, doc)
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("%s: got %s\nwant it to contain %s", tc.name, out.String(), tc.want)
		}
		if strings.Count(out.String(), "<script>") != 1 {
			t.Errorf("%s: want one script, got %s", tc.name, out.String())
		}
	}
	doc, _ := Parse(strings.NewReader(`<textarea name=other></textarea>`))
	if AddTextImport(doc, "body") {
		t.Error("AddTextImport found a textarea that isn't there")
	}
}
