package db

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

func init() {
	var err error
	DB, err = sql.Open("sqlite", "master.db?cache=shared&mode=rwc&_journal_mode=WAL&_busy_timeout=10000")
	if err != nil {
		panic(err)
	}
	DB.SetMaxOpenConns(1)
}
