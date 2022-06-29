package main

import (
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// Redirect page used in templates.
//
type RedirectTmpl struct {
	CreatedUrl string
}

// Thread type used in templates.
//
type ThreadTmpl struct {
	ThreadTitle   string
	ThreadContent template.HTML
	Replies       []CommentTmpl
}

// Comment type used in templates.
//
type CommentTmpl struct {
	CommentId      string
	CommentContent template.HTML
	Children       []CommentTmpl
}

type TemplateError struct {
}

func (err *TemplateError) Error() string {
	return "Templating error."
}

func LoadTemplates() {
	var allFiles []string
	files, err := ioutil.ReadDir("./templates")
	if err != nil {
		fmt.Println(err)
	}
	for _, file := range files {
		filename := file.Name()
		if strings.HasSuffix(filename, ".tmpl") {
			allFiles = append(allFiles, "./templates/"+filename)
		}
	}
	templates, err = template.ParseFiles(allFiles...)
}

func SavePage(fname string, tmpl *template.Template, data any) error {

	renderDir, perr := filepath.Abs("./public")
	if perr != nil {
		return &TemplateError{}
	}

	if envRenderDir := os.Getenv("BETTIT_PUBLIC_HTML_DIR"); envRenderDir != "" {
		renderDir, perr = filepath.Abs(envRenderDir)
		if perr != nil {
			return &TemplateError{}
		}
	}

	// Security check to make sure file ends up in `renderDir`.
	newFilePath := filepath.Join(renderDir, fname)
	if filepath.Dir(newFilePath) != renderDir {
		return &TemplateError{}
	}

	file, ferr := os.Create(newFilePath)
	if ferr != nil {
		return &TemplateError{}
	}

	tmpl.Execute(file, data)
	return nil
}
