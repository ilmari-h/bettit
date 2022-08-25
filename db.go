package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"html"
	"html/template"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tidwall/gjson"
)

var templates *template.Template
var dbReadOnly *sql.DB

type DbError struct {
	message string
	details string
}

func (err *DbError) Error() string {
	errDisplay := err.message
	errDisplay += fmt.Sprintf(": %s", err.details)
	return errDisplay
}

func InitDatabase() {
	db, errOpen := sql.Open("sqlite3", DBFILE)
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
			CONSTRAINT unq UNIQUE(thread_id, continuing_reply),
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
			comment_id TEXT,
			content TEXT,
			author TEXT,
			thread_key INTEGER,
			parent_id INTEGER,
			timestamp INTEGER,
			continues BOOLEAN,
			score INTEGER,
			FOREIGN KEY (thread_key) REFERENCES threads(id),
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

	// Create index for sub in threads. Need to be able to query by sub.
	if statement, err := db.Prepare(`
		CREATE INDEX IF NOT EXISTS threads_sub_index ON threads(sub)
	`); err != nil {
		Log("Error creating database index", err.Error()).Fatal()
	} else {
		statement.Exec()
	}

	dbReadOnly, _ = sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=rw&_busy_timeout=9999999", DBFILE))
}

func queryLatestArchives(limit int) (error, []ArchiveLinkTmpl) {
	results := []ArchiveLinkTmpl{}
	if rowsLatest, qErr := dbReadOnly.Query(`
		SELECT archive_timestamp, thread_id, title, sub
		FROM threads
		WHERE continuing_reply = ""
		ORDER BY archive_timestamp DESC
		LIMIT 10`,
	); qErr != nil {
		Log("Error in latest thread query", qErr.Error()).Error()
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

func querySubArchives(page, pageLen int, sub string) (error, []ArchiveLinkTmpl) {
	results := []ArchiveLinkTmpl{}
	if rowsLatest, qErr := dbReadOnly.Query(`
		SELECT archive_timestamp, thread_id, title, sub
		FROM threads
		WHERE continuing_reply = "" AND sub = ?
		ORDER BY archive_timestamp DESC
		LIMIT ?, ?`, sub, page*pageLen, pageLen,
	); qErr != nil {
		Log("Error in sub thread query", qErr.Error()).Error()
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

func querySubsList(page, pageLen int) (error, *SubsListTmpl) {
	subsOnPage := []string{}
	if rows, qErr := dbReadOnly.Query(`
		SELECT DISTINCT sub
		FROM threads
		ORDER BY sub
		LIMIT ?, ?`, page*pageLen, pageLen,
	); qErr != nil {
		Log("Error in list subs query", qErr.Error()).Error()
		return &DbError{"Error in list subs query", qErr.Error()}, nil
	} else {
		defer rows.Close()
		for rows.Next() {
			sub := ""
			rows.Scan(&sub)
			subsOnPage = append(subsOnPage, sub)
		}
	}
	return nil, &SubsListTmpl{page, subsOnPage}
}

// Post a new comment to the database and all replies to it.
// Recursive function.
func (dbtx *ThreadDbTx) txPostComment(
	data gjson.Result,
	threadId string,
	threadKey int64,
	sub string,
	parent int,
	currentDepth int,
	maxDepth int) *DbError {

	if currentDepth == maxDepth {
		return nil
	}

	content := data.Get("data.body_html").String()
	author := data.Get("data.author").String()
	timestamp := data.Get("data.created").Int()
	replies := data.Get("data.replies.data.children")
	loadMore := replies.Get("0.kind").String() == "more"
	commentId := data.Get("data.id").String()
	score := data.Get("data.score").Int()
	parentStr := "NULL"
	if parent > -1 {
		parentStr = fmt.Sprintf("%d", parent)
	}

	// For now, don't load links unless continuing a comment thread.
	isLink := author == "" && content == ""
	if isLink {
		return nil
	}

	var insertId int64 = -1
	if statement, err := dbtx.tx.Prepare(`
		REPLACE INTO comments (
			comment_id,
			content,
			author,
			thread_key,
			parent_id,
			timestamp,
			continues,
			score
		)
		VALUES ( ?, ?, ?, ?, ?, ?, ?, ?);
	`); err != nil {
		return &DbError{
			"Error creating a new comment", err.Error(),
		}
	} else {
		result, exErr := statement.Exec(commentId,
			content,
			author,
			threadKey,
			parentStr,
			timestamp,
			loadMore,
			score,
		)
		if exErr != nil {
			return &DbError{
				"Error creating a new comment", err.Error(),
			}
		}
		insertId, _ = result.LastInsertId()
	}

	// Continues in another thread / page for the same post.
	if loadMore {

		// Create a new thread from the comment as part of this transaction.
		if req, err := NewThreadRequest(sub, threadId, commentId); err != nil {
			Log("Error requesting comment thread", err.Error())
		} else {
			thrBytes, err := getThread(req)
			if err != nil {
				Log("Error requesting comment thread", err.Error())
			} else {
				dbtx.txPostThread(thrBytes, sub, commentId)
			}
		}

	} else if replCount := replies.Get("#").Int(); replies.Exists() && replies.IsArray() && replCount > 0 {
		for i := 0; i < int(replCount); i++ {
			bubbledError := dbtx.txPostComment(
				replies.Get(fmt.Sprintf("%d", i)),
				threadId,
				threadKey,
				sub,
				int(insertId),
				currentDepth+1,
				maxDepth,
			)
			if bubbledError != nil {
				return bubbledError
			}
		}
	}
	return nil
}

func (dbtx *ThreadDbTx) txPostThread(data []byte, sub string, fromReply string) error {

	// Add first post as thread.
	thrBody := gjson.GetBytes(data, "0.data.children.0.data.selftext_html").String()
	thrBodyLink := gjson.GetBytes(data, "0.data.children.0.data.url_overridden_by_dest").String()

	thrId := gjson.GetBytes(data, "0.data.children.0.data.id").String()
	thrTitle := gjson.GetBytes(data, "0.data.children.0.data.title").String()
	thrAuthor := gjson.GetBytes(data, "0.data.children.0.data.author").String()
	thrRepliesNum := gjson.GetBytes(data, "0.data.children.0.data.num_comments").Int()
	thrTimestamp := gjson.GetBytes(data, "0.data.children.0.data.created").Int()

	// If link is internal.
	if strings.HasPrefix(thrBodyLink, "/") {
		thrBodyLink = "https://reddit.com" + thrBodyLink
	}

	// TODO overwrite if previous older version exists.
	thrStmnt, stmntErr := dbtx.tx.Prepare(`
		REPLACE INTO threads (
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

	if stmntErr != nil || thrStmnt == nil {
		Log(
			"Error preparing new thread insert query", stmntErr.Error(),
		).Error()
		return &DbError{}
	}

	insertRes, thrExcErr := thrStmnt.Exec(
		thrId,
		fromReply,
		thrRepliesNum,
		thrTitle,
		thrBody,
		thrBodyLink,
		thrAuthor,
		sub,
		thrTimestamp,
		time.Now().Unix(),
	)
	if thrExcErr != nil || insertRes == nil {
		Log(
			"Error executing new thread insert query", thrExcErr.Error(),
		).Error()
		return &DbError{}
	}

	thrKey, _ := insertRes.LastInsertId()

	Log("Created a new thread.", fmt.Sprintf("ID %s", thrId)).Info()

	comments := gjson.GetBytes(data, "1.data.children")
	for i := 0; i < int(comments.Get("#").Int()); i++ {
		bubbledError := dbtx.txPostComment(
			comments.Get(fmt.Sprintf("%d", i)),
			thrId,
			thrKey,
			sub,
			-1,
			0,
			100,
		)
		if bubbledError != nil {
			Log(
				bubbledError.message, bubbledError.details,
			).Error()
			return &DbError{}
		}
	}

	return nil

}

type ThreadDbTx struct {
	dbConnection *sql.DB
	tx           *sql.Tx
	upsert       bool
}

func NewTransaction(upsert bool) (*ThreadDbTx, error) {
	db, oerr := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=rw&_busy_timeout=9999999", DBFILE))
	db.SetMaxOpenConns(1)
	if oerr != nil {
		return nil, oerr
	}
	tx, berr := db.Begin()
	if berr != nil {
		return nil, berr
	}
	return &ThreadDbTx{
		db,
		tx,
		upsert,
	}, nil
}

func (dbtx *ThreadDbTx) rollback() {
	dbtx.tx.Rollback()
	dbtx.dbConnection.Close()
}
func (dbtx *ThreadDbTx) done() {
	dbtx.tx.Commit()
	dbtx.dbConnection.Close()
}

func GetArchiveQuery(threadId string, replyId string) (*ArchiveTmpl, error) {

	//
	// Query thread, get highest level thread if no replyId specified, else start from reply.
	//

	rows, qerr := dbReadOnly.Query(`
		SELECT
		id, replies_num, sub, title, content, content_link, author, timestamp, archive_timestamp
		FROM threads
		WHERE thread_id = ? AND continuing_reply = ?
		ORDER BY archive_timestamp DESC
		LIMIT 1
		`,
		threadId,
		replyId,
	)
	if qerr != nil {
		return nil, LogE(&DbError{"Error with thread query", qerr.Error()})
	}

	rows.Next()

	thrNumId := 0
	thrRepliesC := 0
	thrTmpl := ThreadTmpl{}
	thrTimestamp := 0
	arcTimestamp := 0
	rows.Scan(
		&thrNumId,
		&thrRepliesC,
		&thrTmpl.Subreddit,
		&thrTmpl.ThreadTitle,
		&thrTmpl.ThreadContent,
		&thrTmpl.ThreadContentLink,
		&thrTmpl.Author,
		&thrTimestamp,
		&arcTimestamp,
	)

	rows.Close()

	// Nothing found.
	if arcTimestamp == 0 {
		return nil, nil
	}

	thrTmpl.Time = time.Unix(int64(thrTimestamp), 0).Format("02 Jan 2006")

	//
	// Query each comment and its replies starting from the original post.
	//

	queue := []struct {
		id string
		p  *CommentTmpl
	}{
		{"NULL", nil}}

	for len(queue) > 0 {

		currentParentId := queue[0].id
		currentParent := queue[0].p
		queue = queue[1:]

		rows, qerr = dbReadOnly.Query(`
			SELECT id, comment_id, content, author, timestamp, continues, score
			FROM comments
			WHERE thread_key = ? AND parent_id = ?
			ORDER BY score DESC
			`, thrNumId, currentParentId,
		)
		if qerr != nil {
			return nil, LogE(&DbError{"Error with comment query", qerr.Error()})
		}
		for rows.Next() {
			copmTmpl := &CommentTmpl{}
			rId := -1
			rTimestamp := 0
			rows.Scan(
				&rId,
				&copmTmpl.CommentId,
				&copmTmpl.CommentContent,
				&copmTmpl.Author,
				&rTimestamp,
				&copmTmpl.Continues,
				&copmTmpl.Score,
			)
			copmTmpl.Time = time.Unix(int64(rTimestamp), 0).Format("02 Jan 2006")
			copmTmpl.ThreadId = threadId
			queue = append(queue, struct {
				id string
				p  *CommentTmpl
			}{fmt.Sprintf("%d", rId), copmTmpl})
			if currentParent == nil {
				thrTmpl.Replies = append(thrTmpl.Replies, copmTmpl)
			} else {
				currentParent.Children = append(currentParent.Children, copmTmpl)
			}
		}
		rows.Close()
	}

	t := templates.Lookup("thread.tmpl").Lookup("thread")
	thrBuf := new(bytes.Buffer)
	t.Execute(thrBuf, thrTmpl)

	arcTs := ArchiveTmpl{
		time.Unix(int64(arcTimestamp), 0).Format("02 Jan 2006"),
		threadId,
		thrTmpl.ThreadTitle,
		replyId,
		thrTmpl.Subreddit,
		template.HTML(html.UnescapeString(thrBuf.String())),
	}

	return &arcTs, nil
}

func archiveThread(sub string, data []byte) error {

	thrId := gjson.GetBytes(data, "0.data.children.0.data.id").String()
	thrRepliesNum := gjson.GetBytes(data, "0.data.children.0.data.num_comments").Int()

	// Check that thread (with same or higher amount of replies) is not already archived.
	rows, _ := dbReadOnly.Query(`
		SELECT replies_num FROM threads
			WHERE thread_id = ? AND continuing_reply = ""
			LIMIT 1
		`,
		thrId,
		thrRepliesNum,
	)

	upsert := false
	if rows.Next() {
		existingReplies := 0
		rows.Scan(&existingReplies)
		if existingReplies >= int(thrRepliesNum) {
			rows.Close()
			return &DbError{"Thread is already archived", "Same thread with equal number or more replies exists"}
		} else {
			Log(
				"Updating existing thread.",
				fmt.Sprintf("New replies: %d - Previous replies: %d", thrRepliesNum, existingReplies),
			).Debug()
			upsert = true
		}
	}
	rows.Close()

	// Archive thread in another goroutine, return before for sending response.
	go func() {
		tx, txerr := NewTransaction(upsert)
		if txerr != nil {
			Log("Error starting transaction", txerr.Error()).Error()
		}
		if tx.txPostThread(data, sub, "") != nil {
			tx.rollback()
		} else {
			tx.done()
		}
	}()

	return nil
}
