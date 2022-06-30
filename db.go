package main

import (
	"database/sql"
	"fmt"
	"html"
	"html/template"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tidwall/gjson"
)

var db *sql.DB
var templates *template.Template

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
			replies_num INTEGER,
			sub TEXT,
			title TEXT,
			content TEXT,
			author TEXT,
			CONSTRAINT unq UNIQUE(thread_id, replies_num)
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
			FOREIGN KEY (thread_id) REFERENCES threads(id),
			FOREIGN KEY (parent_id) REFERENCES comments(id)
		);`,
	); err != nil {
		Log("Error creating comments table", err.Error()).Fatal()
	} else {
		statement.Exec()
	}

	if statement, err := db.Prepare(`
		CREATE INDEX IF NOT EXISTS threads_id_index ON threads(thread_id)
	`); err != nil {
		Log("Error creating database index", err.Error()).Fatal()
	} else {
		statement.Exec()
	}

	LoadTemplates()
}

// Post a new comment to the database and all replies to it.
// Recursive function.
//
func txPostComment(
	tx *sql.Tx,
	data gjson.Result,
	threadId string,
	parent int,
	currentDepth int,
	maxDepth int) (*CommentTmpl, *DbError) {

	if currentDepth == maxDepth {
		return nil, nil
	}

	content := data.Get("data.body_html").String()
	author := data.Get("data.author").String()
	parentStr := "NULL"
	if parent > -1 {
		parentStr = fmt.Sprintf("%d", parent)
	}

	var insertId int64 = -1
	if statement, err := tx.Prepare(`
		INSERT INTO comments ( content, author, thread_id, parent_id )
		VALUES ( ?, ?, ?, ? );
	`); err != nil {
		return nil, &DbError{
			"Error creating a new comment", err.Error(),
		}
	} else {
		result, exErr := statement.Exec(content, author, threadId, parentStr)
		if exErr != nil {
			return nil, &DbError{
				"Error creating a new comment", err.Error(),
			}
		}
		insertId, _ = result.LastInsertId()
	}
	replies := data.Get("data.replies.data.children")
	repliesTmpl := []CommentTmpl{}
	if replCount := replies.Get("#").Int(); replies.Exists() && replies.IsArray() && replCount > 0 {
		for i := 0; i < int(replCount); i++ {
			reply, bubbledError := txPostComment(
				tx,
				replies.Get(fmt.Sprintf("%d", i)),
				threadId,
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
		fmt.Sprintf("reply-%d", insertId),
		template.HTML(html.UnescapeString(content)),
		repliesTmpl,
	}, nil
}

// Post a new thread to the database and create a corresponding HTML file.
// Writes the created HTML to disk.
//
func txPostThread(sub string, data []byte) error {

	if db == nil {
		panic("Database not open!")
	}
	// Add first post as thread.
	thrBody := gjson.GetBytes(data, "0.data.children.0.data.selftext_html").String()
	thrId := gjson.GetBytes(data, "0.data.children.0.data.id").String()
	thrTitle := gjson.GetBytes(data, "0.data.children.0.data.title").String()
	thrAuthor := gjson.GetBytes(data, "0.data.children.0.data.author").String()
	thrRepliesNum := gjson.GetBytes(data, "0.data.children.0.data.num_comments").Int()

	// Check that thread (with same or higher amount of posts) is not already archived.
	rows, _ := db.Query(
		`SELECT 1 FROM threads WHERE thread_id = ? AND replies_num >= ?`,
		thrId,
		thrRepliesNum,
	)
	if rows.Next() {
		return &DbError{"Thread is already archived", ""}
	}

	// Do heavy lifting in a separate goroutine.
	go func() {

		//
		// Do SQL transaction.
		//

		tx, txErr := db.Begin()
		if txErr != nil {
			Log(
				"Error starting transaction on a new thread", txErr.Error(),
			).Error()
			return
		}

		thrStmnt, stmntErr := tx.Prepare(`
			INSERT INTO threads ( thread_id, replies_num, title, content, author, sub )
			VALUES ( ?, ?, ?, ?, ?, ? );
		`)

		if stmntErr != nil {
			tx.Rollback()
			Log(
				"Error preparing new thread insert query", stmntErr.Error(),
			).Error()
			return
		}

		_, thrExcErr := thrStmnt.Exec(thrId, thrRepliesNum, thrTitle, thrBody, thrAuthor, sub)

		if thrExcErr != nil {
			tx.Rollback()
			Log(
				"Error executing new thread insert query", thrExcErr.Error(),
			).Error()
			return
		}

		Log("Created a new thread. ID %s", thrId).Info()
		replies := []CommentTmpl{}

		comments := gjson.GetBytes(data, "1.data.children")
		for i := 0; i < int(comments.Get("#").Int()); i++ {
			reply, bubbledError := txPostComment(tx, comments.Get(fmt.Sprintf("%d", i)), thrId, -1, 0, 100)
			if bubbledError != nil {
				tx.Rollback()
				Log(
					bubbledError.message, bubbledError.details,
				).Error()
				return
			}
			replies = append(replies, *reply)
		}

		tx.Commit()

		t := templates.Lookup("thread.tmpl").Lookup("thread")

		//
		// Write created html to file.
		//

		SavePage(
			fmt.Sprintf("%s.html", thrId),
			t,
			ThreadTmpl{thrTitle,
				template.HTML(html.UnescapeString(thrBody)),
				replies,
			},
		)
	}()

	return nil
}
