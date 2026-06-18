package webexport

import (
	_ "embed"
	"text/template"
)

//go:embed assets/app.css
var appCSS []byte

//go:embed assets/app.js
var appJS []byte

//go:embed index.html.tmpl
var indexTmplSrc string

// indexTmpl renders index.html. text/template performs no HTML escaping, so the
// inlined JSON blob ({{.Data}}) is emitted verbatim — encoding/json already
// escapes <, > and & to \u00XX, which keeps the <script> payload safe.
var indexTmpl = template.Must(template.New("index").Parse(indexTmplSrc))

// pageData is the template context for index.html.
type pageData struct {
	Data        string // marshaled Export JSON, inlined into a <script> tag
	GeneratedAt string
}
