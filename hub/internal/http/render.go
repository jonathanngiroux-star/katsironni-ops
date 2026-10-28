package http

import (
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin/render"
)

// layoutRenderer gives every page file its OWN template set:
//   set[page] = base.html skeleton + that page's define blocks (title/content/scripts).
// gin's LoadHTMLGlob merges all files into one namespace where every page's
// "content" define collides and rendering a file by name yields an empty body.
// Rendering c.HTML(200, "landing.html", data) executes the base skeleton with
// that file's blocks — the classic layout pattern, correctly isolated.
type layoutRenderer struct {
	sets map[string]*template.Template
}

func newLayoutRenderer(funcs template.FuncMap) (*layoutRenderer, error) {
	sets := map[string]*template.Template{}
	files, err := filepath.Glob("templates/*.html")
	if err != nil {
		return nil, err
	}
	const base = "templates/base.html"
	for _, f := range files {
		if filepath.Base(f) == "base.html" {
			continue
		}
		t := template.New(filepath.Base(f)).Funcs(funcs)
		if _, err := t.ParseFiles(base, f); err != nil {
			return nil, err
		}
		// Pages that don't define optional blocks would fail at execute time —
		// register empty defaults for any missing ones base.html references.
		for _, block := range []string{"scripts", "title", "content"} {
			if t.Lookup(block) == nil {
				template.Must(t.Parse("{{define \"" + block + "\"}}{{end}}"))
			}
		}
		sets[filepath.Base(f)] = t
	}
	return &layoutRenderer{sets: sets}, nil
}

func (l *layoutRenderer) Instance(name string, data any) render.Render {
	t, ok := l.sets[name]
	if !ok {
		return &errorRender{msg: "template not found: " + name, code: http.StatusInternalServerError}
	}
	return &pageRender{t: t, name: "base.html", data: data}
}

type pageRender struct {
	t    *template.Template
	name string
	data any
}

func (p *pageRender) Render(w http.ResponseWriter) error {
	p.WriteContentType(w)
	if p.data == nil {
		p.data = map[string]any{}
	}
	return p.t.ExecuteTemplate(w, p.name, p.data)
}

func (p *pageRender) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if val := header["Content-Type"]; val == nil {
		header["Content-Type"] = []string{"text/html; charset=utf-8"}
	}
}

type errorRender struct {
	msg  string
	code int
}

func (e *errorRender) Render(w http.ResponseWriter) error {
	w.WriteHeader(e.code)
	_, _ = w.Write([]byte(e.msg))
	return nil
}

func (e *errorRender) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	if val := header["Content-Type"]; val == nil {
		header["Content-Type"] = []string{"text/plain; charset=utf-8"}
	}
}
