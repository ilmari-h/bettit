package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tidwall/gjson"
)

var db *sql.DB

func InitDatabase() {
	openedDb, errOpen := sql.Open("sqlite3", "./bettit.db")
	db = openedDb
	if errOpen != nil {
		log.Fatalf("Error opening database: %s", errOpen.Error())
	}

	if statement, err := db.Prepare(`
		CREATE TABLE IF NOT EXISTS threads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			thread_id TEXT,
			title TEXT,
			content TEXT,
			author TEXT,
			UNIQUE(thread_id)
		);`,
	); err != nil {
		log.Fatalf("Error creating threads table: %s", err.Error())
	} else {
		statement.Exec()
	}

	if statement, err := db.Prepare(`CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT,
			author TEXT,
			thread_id INTEGER,
			parent_id INTEGER,
			FOREIGN KEY (thread_id) REFERENCES threads(id),
			FOREIGN KEY (parent_id) REFERENCES comments(id)
		);`,
	); err != nil {
		log.Fatalf("Error creating comments table: %s", err.Error())
	} else {
		statement.Exec()
	}

	if statement, err := db.Prepare(`
		CREATE INDEX IF NOT EXISTS threads_id_index ON threads(thread_id)
	`); err != nil {
		log.Fatalf("Error creating database index: %s", err.Error())
	} else {
		statement.Exec()
	}
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
	maxDepth int) {
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
		panic(
			fmt.Sprintf("Error creating a new comment: %s", err.Error()),
		)
	} else {
		result, exErr := statement.Exec(content, author, threadId, parentStr)
		if exErr != nil {
			panic(
				fmt.Sprintf("Error creating a new comment: %s", err.Error()),
			)
		}
		insertId, _ = result.LastInsertId()
	}
	replies := data.Get("data.replies.data.children")
	if replCount := replies.Get("#").Int(); replies.Exists() && replies.IsArray() && replCount > 0 {
		for i := 0; i < int(replCount); i++ {
			txPostComment(
				tx,
				replies.Get(fmt.Sprintf("%d", i)),
				threadId,
				int(insertId),
				currentDepth-1,
				maxDepth,
			)
		}
	}
}

// Post a new thread to the database.
//
func txPostThread(id string, sub string, data []byte) {

	// TODO defer with recover to revert tx and respond with error status

	if db == nil {
		panic("Database not open!")
	}
	tx, txErr := db.Begin()
	if txErr != nil {
		panic(
			fmt.Sprintf("Error starting transaction on a new thread: %s", txErr.Error()),
		)
	}

	// Add first post as thread.
	thrBody := gjson.GetBytes(data, "0.data.children.0.data.selftext_html").String()
	thrId := gjson.GetBytes(data, "0.data.children.0.data.id").String()
	thrTitle := gjson.GetBytes(data, "0.data.children.0.data.title").String()
	thrAuthor := gjson.GetBytes(data, "0.data.children.0.data.author").String()

	thrStmnt, stmntErr := tx.Prepare(`
		INSERT INTO threads ( thread_id, title, content, author )
		VALUES ( ?, ?, ?, ? );
	`)

	if stmntErr != nil {
		panic(
			fmt.Sprintf("Error preparing new thread insert query: %s", stmntErr.Error()),
		)
	}

	_, thrExcErr := thrStmnt.Exec(thrId, thrTitle, thrBody, thrAuthor)

	if thrExcErr != nil {
		panic(
			fmt.Sprintf("Error executing new thread insert query: %s", thrExcErr.Error()),
		)
	}

	log.Printf("Created a new thread. ID %s", thrId)

	comments := gjson.GetBytes(data, "1.data.children")
	for i := 0; i < int(comments.Get("#").Int()); i++ {
		txPostComment(tx, comments.Get(fmt.Sprintf("%d", i)), thrId, -1, 0, 100)
	}

	tx.Commit()
}
