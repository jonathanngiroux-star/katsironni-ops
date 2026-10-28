package main

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
)

func main() {
	funcs := template.FuncMap{
		"cad":      func(v any) string { return "0.00" },
		"default":  func(def any, v any) any { return v },
		"safeHTML": func(s any) template.HTML { return template.HTML("x") },
	}
	files, _ := filepath.Glob("templates/*.html")
	for _, f := range files {
		if filepath.Base(f) == "base.html" {
			continue
		}
		t := template.New(filepath.Base(f)).Funcs(funcs)
		if _, err := t.ParseFiles("templates/base.html", f); err != nil {
			fmt.Printf("%-24s PARSE ERR: %v\n", filepath.Base(f), err)
			continue
		}
		for _, block := range []string{"scripts", "title", "content"} {
			if t.Lookup(block) == nil {
				template.Must(t.Parse("{{define \"" + block + "\"}}{{end}}"))
			}
		}
		var buf bytes.Buffer
		if err := t.ExecuteTemplate(&buf, "base.html", map[string]any{}); err != nil {
			fmt.Printf("%-24s EXEC ERR: %v\n", filepath.Base(f), err)
			continue
		}
		status := "OK"
		if buf.Len() < 100 {
			status = "<-- SUSPICIOUSLY SMALL"
		}
		fmt.Printf("%-24s %5d bytes %s\n", filepath.Base(f), buf.Len(), status)
	}
}
