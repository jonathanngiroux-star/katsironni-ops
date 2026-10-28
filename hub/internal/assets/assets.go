package assets

import (
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var Templates *template.Template
var StaticFS fs.FS

func LoadTemplates() error {
	tmpl := template.New("").Funcs(template.FuncMap{
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	})

	err := filepath.WalkDir("templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".html" {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			name := strings.TrimPrefix(path, "templates/")
			tmpl.New(name).Parse(string(content))
		}
		return nil
	})
	if err != nil {
		return err
	}

	Templates = tmpl
	StaticFS = os.DirFS("static")
	return nil
}

func ExecuteTemplate(name string, data any) (string, error) {
	if Templates == nil {
		return "", nil
	}
	var buf strings.Builder
	err := Templates.ExecuteTemplate(&buf, name, data)
	return buf.String(), err
}