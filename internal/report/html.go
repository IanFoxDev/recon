package report

import (
	"bytes"
	_ "embed"
	"html/template"
	"strings"

	"github.com/ianfoxdev/recon/internal/match"
)

//go:embed report.html.tmpl
var htmlTemplate string

var page = template.Must(template.New("report").Funcs(template.FuncMap{
	"label": label,
	"join":  strings.Join,
	"of": func(ds []Difference, c match.Category) []Difference {
		var out []Difference
		for _, d := range ds {
			if d.Category == c {
				out = append(out, d)
			}
		}
		return out
	},
}).Parse(htmlTemplate))

// HTML is the report as one file: no scripts, no external styles.
func (r Report) HTML() ([]byte, error) {
	var buf bytes.Buffer
	if err := page.Execute(&buf, r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
