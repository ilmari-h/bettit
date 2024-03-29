package main

import (
	"html/template"
	"io"
	"io/ioutil"
	"strings"

	"github.com/gin-gonic/gin"
)

const ITEMS_ON_PAGE = 100

//
// Types used in templates.
//

type IndexTmpl struct {
	TotalArchived int
	Latest        []ArchiveLinkTmpl // urls
}

type SubThreadsTmpl struct {
	page    int
	Threads []ArchiveLinkTmpl // urls
}

type ArchiveLinkTmpl struct {
	ArchiveTime int
	ThreadId    string
	ThreadTitle string
	Subreddit   string
}

type SubsListTmpl struct {
	page int
	Subs []string
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
	Route string
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
		Log("Error loading templates", err.Error()).Fatal()
	}
	for _, file := range files {
		filename := file.Name()
		if strings.HasSuffix(filename, ".tmpl") {
			allFiles = append(allFiles, "./templates/"+filename)
		}
	}
	templates, err = template.ParseFiles(allFiles...)
	if err != nil {
		Log("Error loading templates", err.Error()).Fatal()
	}
}

func RenderErrorPage(errStatus int, w gin.ResponseWriter) {
	tp := templates.Lookup("error.tmpl").Lookup("internal")
	w.WriteHeader(errStatus)
	switch errStatus {
	case 400:
		tp = templates.Lookup("error.tmpl").Lookup("invalidreq")
		break
	case 404:
		tp = templates.Lookup("error.tmpl").Lookup("notfound")
		break
	case 500:
		tp = templates.Lookup("error.tmpl").Lookup("internal")
		break
	default:
		tp = templates.Lookup("error.tmpl").Lookup("other")
		tp.Execute(w, struct{ Code int }{errStatus})
		return
	}
	tp.Execute(w, nil)
}

func RenderRedirectPage(route string, w io.Writer) {
	tp := templates.Lookup("redirect.tmpl").Lookup("page")
	tp.Execute(w, RedirectTmpl{
		Route: route,
	})
}

func RenderAlreadyExists(route string, w gin.ResponseWriter) {
	w.WriteHeader(409)
	tp := templates.Lookup("redirect.tmpl").Lookup("conflict")
	tp.Execute(w, RedirectTmpl{
		Route: route,
	})
}

func RenderSubsList(page int, w gin.ResponseWriter) int {
	if err, sTmpl := querySubsList(page, ITEMS_ON_PAGE); err != nil {
		return 500
	} else {
		tp := templates.Lookup("index.tmpl").Lookup("sublist")
		tp.Execute(w, sTmpl)
	}
	return 200
}

func RenderSubThreads(page int, w gin.ResponseWriter, sub string) int {
	if err, sTmpl := querySubArchives(page, ITEMS_ON_PAGE, sub); err != nil {
		return 500
	} else {
		tp := templates.Lookup("index.tmpl").Lookup("subthreads")
		tp.Execute(w, &SubThreadsTmpl{page, sTmpl})
	}
	return 200
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

func RenderAboutPage(w io.Writer) int {
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

	t := templates.Lookup("index.tmpl").Lookup("about")
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
