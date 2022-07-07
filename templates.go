package main

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

//
// Types used in templates.
//

type IndexTmpl struct {
	TotalArchived int
	Latest        []ArchiveLinkTmpl // urls
}

type ArchiveLinkTmpl struct {
	ArchiveTime int
	ThreadId    string
	ThreadTitle string
	Subreddit   string
}

type ArchiveTmpl struct {
	ArchiveTime string
	ThreadId    string
	ThreadTitle string
	ReplyId     string
	Subreddit   string
	ThreadHTML  template.HTML
}

type RedirectTmpl struct {
	Url string
}

type ThreadTmpl struct {
	ThreadTitle       string
	ThreadContent     template.HTML
	ThreadContentLink string
	Subreddit         string
	Replies           []*CommentTmpl
	Author            string
	Time              string
}

type CommentTmpl struct {
	CommentId      string
	ThreadId       string
	CommentContent template.HTML
	Children       []*CommentTmpl
	Author         string
	Time           string
	Continues      bool
	Score          string
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

func RenderErrorPage(errStatus int, w gin.ResponseWriter) {
	tp := templates.Lookup("redirect.tmpl").Lookup("other")
	w.WriteHeader(errStatus)
	switch errStatus {
	case 400:
		tp = templates.Lookup("redirect.tmpl").Lookup("invalidreq")
		break
	case 404:
		tp = templates.Lookup("redirect.tmpl").Lookup("notfound")
		break
	case 500:
		tp = templates.Lookup("redirect.tmpl").Lookup("internal")
		break
	default:
		tp = templates.Lookup("redirect.tmpl").Lookup("other")
		tp.Execute(w, struct{ Code int }{errStatus})
		return
	}
	tp.Execute(w, nil)
}

func RenderRedirectPage(url string, w io.Writer) {
	tp := templates.Lookup("redirect.tmpl").Lookup("page")
	tp.Execute(w, RedirectTmpl{
		Url: url,
	},
	)
}

func RenderAlreadyExists(url string, w gin.ResponseWriter) {
	w.WriteHeader(409)
	tp := templates.Lookup("redirect.tmpl").Lookup("conflict")
	tp.Execute(w, RedirectTmpl{
		Url: url,
	},
	)
}

func RenderIndexPage(w io.Writer) int {
	count := 0

	rows, qErr := dbReadOnly.Query(`SELECT COUNT(DISTINCT thread_id) from threads`)
	defer rows.Close()
	if qErr != nil {
		Log("Error with thread count query", qErr.Error()).Error()
		return 500
	}
	rows.Next()
	rows.Scan(&count)

	err, latestCreated := queryLatestArchives(10)
	if err != nil {
		return 500
	}

	t := templates.Lookup("index.tmpl").Lookup("index")
	t.Execute(w, IndexTmpl{count, latestCreated})
	return 200
}

func RenderThreadPage(fileId string, w gin.ResponseWriter) int {

	fnameParts := strings.Split(fileId, "-")
	threadId := fnameParts[0]
	continuingReply := ""

	// If thread is part of a longer comment thread the comment's ID will be the second element.
	if len(fnameParts) > 1 {
		continuingReply = fnameParts[1]
	}

	if arch, err := GetArchiveQuery(threadId, continuingReply); err != nil {
		Log("Error getting archive.", err.Error())
		return 500
	} else if arch == nil {
		return 404
	} else {
		t := templates.Lookup("thread.tmpl").Lookup("archive")
		t.Execute(w, arch)
		return 200
	}
}
