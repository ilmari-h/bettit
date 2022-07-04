package main

import (
	"database/sql"
	"fmt"
	"html"
	"html/template"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tidwall/gjson"
)

var db *sql.DB
var templates *template.Template
var totalThreadsCreated = struct {
	LastUpdated int64
	Value       int
}{0, 0}

type DbError struct {
	message string
	details string
}

func (err *DbError) Error() string {
	errDisplay := err.message
	if gin.IsDebugging() {
		errDisplay += fmt.Sprintf(": %s", err.details)
	}
	return errDisplay
}

func InitDatabase() {
	openedDb, errOpen := sql.Open("sqlite3", "./bettit.db")
	db = openedDb
	if errOpen != nil {
		Log("Error opening database", errOpen.Error()).Fatal()
	}

	if statement, err := db.Prepare(`
		CREATE TABLE IF NOT EXISTS threads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			thread_id TEXT,
			continuing_reply TEXT,
			replies_num INTEGER,
			sub TEXT,
			title TEXT,
			content TEXT,
			content_link TEXT,
			author TEXT,
			timestamp INTEGER,
			archive_timestamp INTEGER,
			CONSTRAINT unq UNIQUE(thread_id, replies_num, continuing_reply),
			CONSTRAINT chk_id CHECK(LENGTH(thread_id) >= 6)
			CONSTRAINT chk_title CHECK(LENGTH(title) > 1)
			CONSTRAINT chk_sub CHECK(LENGTH(sub) > 1)
		);`,
	); err != nil {
		Log("Error creating threads table", err.Error()).Fatal()
	} else {
		statement.Exec()
	}

	if statement, err := db.Prepare(`
		CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT,
			author TEXT,
			thread_id INTEGER,
			parent_id INTEGER,
			timestamp INTEGER,
			FOREIGN KEY (thread_id) REFERENCES threads(id),
			FOREIGN KEY (parent_id) REFERENCES comments(id)
		);`,
	); err != nil {
		Log("Error creating comments table", err.Error()).Fatal()
	} else {
		statement.Exec()
	}

	// Create index for thread_id in threads.
	if statement, err := db.Prepare(`
		CREATE INDEX IF NOT EXISTS threads_id_index ON threads(thread_id)
	`); err != nil {
		Log("Error creating database index", err.Error()).Fatal()
	} else {
		statement.Exec()
	}

	// Create index for timestamp in threads.
	if statement, err := db.Prepare(`
		CREATE INDEX IF NOT EXISTS threads_timestamp_index ON threads(timestamp)
	`); err != nil {
		Log("Error creating database index", err.Error()).Fatal()
	} else {
		statement.Exec()
	}

	LoadTemplates()
}

func queryLatestArchives(limit int) (error, []ArchiveLinkTmpl) {
	results := []ArchiveLinkTmpl{}
	if rowsLatest, qErr := db.Query(`
		SELECT archive_timestamp, thread_id, title, sub
		FROM threads
		WHERE continuing_reply = ""
		ORDER BY archive_timestamp DESC
		LIMIT 10`,
	); qErr != nil {
		rowsLatest.Close()
		Log("Error latest thread query", qErr.Error()).Error()
		return &DbError{"Error with thread count query", qErr.Error()}, nil
	} else {
		defer rowsLatest.Close()
		for rowsLatest.Next() {
			nRes := ArchiveLinkTmpl{}
			rowsLatest.Scan(&nRes.ArchiveTime, &nRes.ThreadId, &nRes.ThreadTitle, &nRes.Subreddit)
			results = append(results, nRes)
		}
	}

	return nil, results
}

func queryReadThread(threadId string) *template.Template {
	return nil
}

// Post a new comment to the database and all replies to it.
// Recursive function.
//
func txPostComment(
	tx *sql.Tx,
	data gjson.Result,
	threadId string,
	sub string,
	parent int,
	currentDepth int,
	maxDepth int) (*CommentTmpl, *DbError) {

	if currentDepth == maxDepth {
		return nil, nil
	}

	content := data.Get("data.body_html").String()
	author := data.Get("data.author").String()
	timestamp := data.Get("data.created").Int()
	parentStr := "NULL"
	if parent > -1 {
		parentStr = fmt.Sprintf("%d", parent)
	}

	var insertId int64 = -1
	if statement, err := tx.Prepare(`
		INSERT INTO comments ( content, author, thread_id, parent_id, timestamp )
		VALUES ( ?, ?, ?, ?, ? );
	`); err != nil {
		return nil, &DbError{
			"Error creating a new comment", err.Error(),
		}
	} else {
		result, exErr := statement.Exec(content, author, threadId, parentStr, timestamp)
		if exErr != nil {
			return nil, &DbError{
				"Error creating a new comment", err.Error(),
			}
		}
		insertId, _ = result.LastInsertId()
	}
	replies := data.Get("data.replies.data.children")
	loadMore := replies.Get("0.kind").String() == "more"
	commentId := data.Get("data.id").String()

	// Continues in another thread / page for the same post.
	repliesTmpl := []CommentTmpl{}
	if loadMore {

		// Create a new thread from the comment as part of this transaction.
		if req, err := NewThreadRequest(sub, threadId, commentId); err != nil {
			Log("Error requesting comment thread", err.Error())
		} else {
			thrBytes, err := getThread(req)
			if err != nil {
				Log("Error requesting comment thread", err.Error())
			} else {
				txPostThread(tx, thrBytes, sub, commentId)
			}
		}

		repliesTmpl = append(repliesTmpl, CommentTmpl{
			fmt.Sprintf("reply-%d-more", insertId),
			template.HTML(fmt.Sprintf(`<a class="continue-thread" href="/%s-%s">Continue -></a>`, threadId, commentId)),
			[]CommentTmpl{},
			"",
			time.Unix(int64(timestamp), 0).Format("02 Jan 2006"),
		})

	} else if replCount := replies.Get("#").Int(); replies.Exists() && replies.IsArray() && replCount > 0 {
		for i := 0; i < int(replCount); i++ {
			reply, bubbledError := txPostComment(
				tx,
				replies.Get(fmt.Sprintf("%d", i)),
				threadId,
				sub,
				int(insertId),
				currentDepth+1,
				maxDepth,
			)
			if bubbledError != nil {
				return nil, bubbledError
			}
			repliesTmpl = append(repliesTmpl, *reply)
		}
	}
	return &CommentTmpl{
		fmt.Sprintf("reply-%s", commentId),
		template.HTML(html.UnescapeString(content)),
		repliesTmpl,
		author,
		time.Unix(int64(timestamp), 0).Format("02 Jan 2006"),
	}, nil
}

func txPostThread(tx *sql.Tx, data []byte, sub string, continueReply string) error {

	// Add first post as thread.
	thrBody := gjson.GetBytes(data, "0.data.children.0.data.selftext_html").String()
	thrBodyLink := gjson.GetBytes(data, "0.data.children.0.data.url_overridden_by_dest").String()

	thrId := gjson.GetBytes(data, "0.data.children.0.data.id").String()
	thrTitle := gjson.GetBytes(data, "0.data.children.0.data.title").String()
	thrAuthor := gjson.GetBytes(data, "0.data.children.0.data.author").String()
	thrRepliesNum := gjson.GetBytes(data, "0.data.children.0.data.num_comments").Int()
	thrTimestamp := gjson.GetBytes(data, "0.data.children.0.data.created").Int()

	// TODO overwrite if previous exists
	thrStmnt, stmntErr := tx.Prepare(`
		INSERT INTO threads (
			thread_id,
			continuing_reply,
			replies_num,
			title,
			content,
			content_link,
			author,
			sub,
			timestamp,
			archive_timestamp
		)
		VALUES ( ?, ?, ?, ?, ?, ?, ?, ?, ?, ? );
	`)

	Log("CURRENT OPEN CONNECTIONS", fmt.Sprintf("%d", db.Stats().InUse)).Info()

	if stmntErr != nil {
		Log(
			"Error preparing new thread insert query", stmntErr.Error(),
		).Error()
		return &DbError{}
	}

	_, thrExcErr := thrStmnt.Exec(
		thrId,
		continueReply,
		thrRepliesNum,
		thrTitle,
		thrBody,
		thrBodyLink,
		thrAuthor,
		sub,
		thrTimestamp,
		time.Now().Unix(),
	)

	if thrExcErr != nil {
		Log(
			"Error executing new thread insert query", thrExcErr.Error(),
		).Error()
		return &DbError{}
	}

	Log("Created a new thread.", fmt.Sprintf("ID %s", thrId)).Info()
	replies := []CommentTmpl{}

	comments := gjson.GetBytes(data, "1.data.children")
	for i := 0; i < int(comments.Get("#").Int()); i++ {
		reply, bubbledError := txPostComment(tx, comments.Get(fmt.Sprintf("%d", i)), thrId, sub, -1, 0, 100)
		if bubbledError != nil {
			Log(
				bubbledError.message, bubbledError.details,
			).Error()
			return &DbError{}
		}
		replies = append(replies, *reply)
	}

	go func() {

		t := templates.Lookup("thread.tmpl").Lookup("thread")

		//
		// Write created html to file.
		//

		fileName := thrId
		if continueReply != "" {
			fileName += "-" + continueReply
		}
		err := SavePage(
			fmt.Sprintf("%s.html", fileName),
			t,
			ThreadTmpl{thrTitle,
				template.HTML(html.UnescapeString(thrBody)),
				thrBodyLink,
				sub,
				replies,
				thrAuthor,
				time.Unix(int64(thrTimestamp), 0).Format("02 Jan 2006"),
			},
		)
		if err != nil {
			Log("Error saving page", err.Error()).Error()
		}
	}()

	return nil

}

// Post a new thread to the database and create a corresponding HTML file.
// Writes the created HTML to disk.
// If continueReply is not empty, the thread is a continuance of a comment thread,
// from a reply with that id
//
func archiveThread(sub string, data []byte) error {

	Log("CURRENT OPEN CONNECTIONS BEFORE", fmt.Sprintf("%d", db.Stats().InUse)).Info()
	if db == nil {
		panic("Database not open!")
	}

	thrId := gjson.GetBytes(data, "0.data.children.0.data.id").String()
	thrRepliesNum := gjson.GetBytes(data, "0.data.children.0.data.num_comments").Int()

	// Check that thread (with same or higher amount of posts) is not already archived.
	rows, _ := db.Query(
		`SELECT 1 FROM threads WHERE thread_id = ? AND replies_num >= ?`,
		thrId,
		thrRepliesNum,
	)
	if rows.Next() {
		rows.Close()
		return &DbError{"Thread is already archived", ""}
	}
	rows.Close()

	// Do heavy lifting in a separate goroutine.
	go func() {
		tx, txerr := db.Begin()
		if txerr != nil {
			Log("Error starting transaction", txerr.Error()).Error()
		}
		if txPostThread(tx, data, sub, "") != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	return nil
}
