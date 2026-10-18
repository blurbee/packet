package htmlop

import (
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// AddTextImport adds an "Import…" link next to the <textarea> with the
// specified name, which loads the contents of a text file chosen by the user
// into it, so that commonly sent message text needn't be retyped.  The link
// goes at the end of the textarea's label (the element just before it), or just
// before the textarea if it has no label.  (It is a link, not a button,
// because the label may be a <label> element, which would take a button as its
// control.)  If the textarea is empty, the file's text fills it; otherwise the
// text replaces the selection, if the user had made one, or is added at the
// cursor, if the user had placed it, or else is added at the end.  It returns
// false if there is no such textarea.
func AddTextImport(doc *html.Node, name string) bool {
	ta := findTextarea(doc, name)
	if ta == nil {
		return false
	}
	link := &html.Node{Type: html.ElementNode, Data: "a", DataAtom: atom.A, Attr: []html.Attribute{
		{Key: "href", Val: "#"},
		{Key: "class", Val: "text-import"},
		{Key: "data-target", Val: name},
		{Key: "title", Val: "Load text from a file into this box"},
	}}
	link.AppendChild(&html.Node{Type: html.TextNode, Data: "Import…"})
	style := &html.Node{Type: html.ElementNode, Data: "style", DataAtom: atom.Style}
	style.AppendChild(&html.Node{Type: html.TextNode, Data: textImportCSS})
	script := &html.Node{Type: html.ElementNode, Data: "script", DataAtom: atom.Script}
	script.AppendChild(&html.Node{Type: html.TextNode, Data: textImportJS})
	if label := prevElement(ta); label != nil {
		label.AppendChild(&html.Node{Type: html.TextNode, Data: " "})
		label.AppendChild(link)
		label.AppendChild(style)
		label.AppendChild(script)
	} else {
		for _, n := range []*html.Node{link, style, script} {
			ta.Parent.InsertBefore(n, ta)
		}
	}
	return true
}

// findTextarea returns the <textarea> with the specified name, or nil.
func findTextarea(n *html.Node, name string) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == atom.Textarea && getAttr(n, "name") == name {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findTextarea(c, name); found != nil {
			return found
		}
	}
	return nil
}

// prevElement returns the element sibling just before n, or nil.
func prevElement(n *html.Node) *html.Node {
	for p := n.PrevSibling; p != nil; p = p.PrevSibling {
		if p.Type == html.ElementNode {
			return p
		}
	}
	return nil
}

const textImportCSS = `a.text-import{display:inline-block;margin-left:0.5em;padding:0 0.4em;border:1px solid #888;border-radius:3px;background:#f4f4f4;color:#000;font-weight:normal;font-size:smaller;line-height:1.4;text-decoration:none;white-space:nowrap}a.text-import:hover{background:#ddd}`

// textImportJS handles clicks on all Import links on the page.  It tracks
// where the user last left the cursor in each textarea, since clicking the
// link takes the focus away.
const textImportJS = `
if (!window.textImportReady) {
  window.textImportReady = true
  document.addEventListener('focusout', e => {
    if (e.target.tagName === 'TEXTAREA') e.target.textImportAt = [e.target.selectionStart, e.target.selectionEnd]
  })
  document.addEventListener('click', e => {
    const link = e.target.closest && e.target.closest('a.text-import')
    if (!link) return
    e.preventDefault()
    const ta = Array.from(document.querySelectorAll('textarea')).find(t => t.name === link.dataset.target)
    if (!ta) return
    const chooser = document.createElement('input')
    chooser.type = 'file'
    chooser.accept = '.txt,text/plain'
    chooser.addEventListener('change', async () => {
      if (!chooser.files.length) return
      let text = (await chooser.files[0].text()).replace(/\r\n?/g, '\n').replace(/\s+$/, '')
      if (ta.value.trim() === '') {
        ta.value = text
      } else if (ta.textImportAt) {
        ta.setRangeText(text, ta.textImportAt[0], ta.textImportAt[1], 'end')
      } else {
        ta.value = ta.value.replace(/\s+$/, '') + '\n' + text
      }
      ta.textImportAt = [ta.selectionStart, ta.selectionEnd]
      ta.dispatchEvent(new Event('input', { bubbles: true }))
      ta.dispatchEvent(new Event('change', { bubbles: true }))
      ta.focus()
    })
    chooser.click()
  })
}
`
