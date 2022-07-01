package main

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Thread type used in templates.
//
type ArchiveTmpl struct {
	ArchiveTime string
	ThreadId    string
	ThreadHTML  template.HTML
}

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
	Author        string
	Time          string
}

// Comment type used in templates.
//
type CommentTmpl struct {
	CommentId      string
	CommentContent template.HTML
	Children       []CommentTmpl
	Author         string
	Time           string
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

// Retrieve a public file path safely.
func getRenderFilePath(fpath string) (string, error) {

	fname := filepath.Clean(fpath)
	renderDir, perr := filepath.Abs("./archive")
	if perr != nil {
		return "", &TemplateError{}
	}

	if envRenderDir := os.Getenv("BETTIT_PUBLIC_HTML_DIR"); envRenderDir != "" {
		renderDir, perr = filepath.Abs(envRenderDir)
		if perr != nil {
			return "", &TemplateError{}
		}
	}

	fullPath := filepath.Join(renderDir, fname)

	//canonicalPath, cerr := filepath.EvalSymlinks(fullPath)
	//if cerr != nil {
	//	return "", &TemplateError{}
	//}

	// Make sure file ends up in `renderDir`.
	if filepath.Dir(fullPath) != renderDir {
		return "", &TemplateError{}
	}

	return fullPath, nil
}

func SavePage(fname string, tmpl *template.Template, data any) error {

	newFilePath, err := getRenderFilePath(fname)
	if err != nil {
		return err
	}

	file, ferr := os.Create(newFilePath)
	if ferr != nil {
		return &TemplateError{}
	}

	tmpl.Execute(file, data)
	return nil
}

func RenderThreadPage(threadId string, w io.Writer) error {
	fname := threadId + ".html"
	fpath, err := getRenderFilePath(fname)
	if err != nil {
		Log("Error getting render file path", err.Error()).Error()
		return err
	}

	rows, qErr := db.Query(
		`SELECT archive_timestamp FROM threads WHERE thread_id = ?`,
		threadId,
	)

	archiveTimestamp := 0
	if qErr != nil {
		Log("Error with thread query", qErr.Error()).Error()
		return &DbError{"Error with thread query", qErr.Error()}
	} else {
		rows.Next()
		rows.Scan(&archiveTimestamp)
	}

	if data, ferr := os.ReadFile(fpath); ferr != nil {
		Log("Error reading file for page render", ferr.Error()).Error()
		return &TemplateError{}
	} else {
		t := templates.Lookup("thread.tmpl").Lookup("archive")
		newPage := ArchiveTmpl{
			time.Unix(int64(archiveTimestamp), 0).Format("02 Jan 2006"),
			threadId,
			template.HTML(html.UnescapeString(string(data))),
		}
		t.Execute(w, newPage)
		return nil
	}
}
