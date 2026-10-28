package main

import (
	"fmt"
	"html/template"
	"os"
)

func main() {
	t := template.New("check").Funcs(template.FuncMap{
		"cad":       func(v any) string { return "0.00" },
		"default":   func(def any, v any) any { if v == nil || v == "" { return def }; return v },
		"safeHTML":  func(s any) template.HTML { return template.HTML("ok") },
	})
	_, err := t.ParseGlob("templates/*")
	if err != nil {
		fmt.Println("PARSE ERROR:", err)
		os.Exit(1)
	}
	fmt.Println("ALL TEMPLATES PARSE OK")
}
