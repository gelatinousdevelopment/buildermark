// Temporary backfill script: removes noisy Codex messages and sets
// final_answer message_type on existing data.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := filepath.Join("..", "..", ".data", "local.db")
	if p := os.Getenv("BUILDERMARK_DB"); p != "" {
		dbPath = p
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Try from repo root.
		dbPath = filepath.Join(".data", "local.db")
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// Delete skippable codex noise messages.
	res, err := db.Exec(`DELETE FROM messages WHERE content LIKE '[response_item:reasoning]%'
		OR content LIKE '[event_msg:item_completed]%'
		OR content LIKE '[event_msg:token_count]%'`)
	if err != nil {
		log.Fatalf("delete noisy messages: %v", err)
	}
	deleted, _ := res.RowsAffected()
	fmt.Printf("Deleted %d noisy messages\n", deleted)

	// Update final_answer message types.
	res, err = db.Exec(`UPDATE messages SET message_type = 'final_answer'
		WHERE json_extract(raw_json, '$.payload.phase') = 'final_answer'
		AND message_type = 'log'`)
	if err != nil {
		log.Fatalf("update final_answer: %v", err)
	}
	updated, _ := res.RowsAffected()
	fmt.Printf("Updated %d messages to final_answer\n", updated)
}
