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
	ThreadHTML  template.HTML
}

type RedirectPageTmpl struct {
	IsErr   bool
	Content RedirectTmpl
}
type RedirectTmpl struct {
	Url string
}

type ThreadTmpl struct {
	ThreadTitle       string
	ThreadContent     template.HTML
	ThreadContentLink string
	Subreddit         string
	Replies           []CommentTmpl
	Author            string
	Time              string
}

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

func RenderRedirectPage(err bool, url string, w io.Writer) {
	tp := templates.Lookup("redirect.tmpl").Lookup("page")
	tp.Execute(w, RedirectPageTmpl{
		err,
		RedirectTmpl{
			Url: url,
		},
	})
}

func RenderIndexPage(w io.Writer) error {
	count := 0

	// Do this query at most once a minute
	if time.Now().Unix() > 60+totalThreadsCreated.LastUpdated {
		rows, qErr := dbReadOnly.Query(`SELECT COUNT(DISTINCT thread_id) from threads`)
		defer rows.Close()
		if qErr != nil {
			Log("Error with thread count query", qErr.Error()).Error()
			return &DbError{"Error with thread count query", qErr.Error()}
		}
		rows.Next()
		rows.Scan(&count)
		totalThreadsCreated.LastUpdated = time.Now().Unix()
		totalThreadsCreated.Value = count
	} else {
		count = totalThreadsCreated.Value
	}

	err, latestCreated := queryLatestArchives(10)
	if err != nil {
		return err
	}

	t := templates.Lookup("index.tmpl").Lookup("index")
	t.Execute(w, IndexTmpl{count, latestCreated})
	return nil
}

func RenderThreadPage(fileId string, w io.Writer) error {
	fname := fileId + ".html"
	fpath, err := getRenderFilePath(fname)
	if err != nil {
		Log("Error getting render file path", err.Error()).Error()
		return err
	}
	fnameParts := strings.Split(fileId, "-")
	threadId := fnameParts[0]
	continuingReply := ""

	// If thread is part of a longer comment thread the comment's ID will be the second element.
	if len(fnameParts) > 1 {
		continuingReply = fnameParts[1]

	}
	rows, qErr := dbReadOnly.Query(`
		SELECT archive_timestamp, title FROM threads
		WHERE thread_id = ? AND continuing_reply = ?
		ORDER BY archive_timestamp DESC
		LIMIT 1`,
		threadId,
		continuingReply,
	)
	defer rows.Close()

	archiveTimestamp := 0
	threadTitle := ""
	if qErr != nil {
		Log("Error with thread query", qErr.Error()).Error()
		return &DbError{"Error with thread query", qErr.Error()}
	} else {
		rows.Next()
		rows.Scan(&archiveTimestamp, &threadTitle)
	}

	if data, ferr := os.ReadFile(fpath); ferr != nil {
		Log("Error reading file for page render", ferr.Error()).Error()
		return &TemplateError{}
	} else {
		t := templates.Lookup("thread.tmpl").Lookup("archive")
		newPage := ArchiveTmpl{
			time.Unix(int64(archiveTimestamp), 0).Format("02 Jan 2006"),
			threadId,
			threadTitle,
			template.HTML(html.UnescapeString(string(data))),
		}
		t.Execute(w, newPage)
		return nil
	}
}
