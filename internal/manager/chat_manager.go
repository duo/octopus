package manager

import (
	"database/sql"

	"github.com/duo/octopus/internal/db"
)

func init() {
	if _, err := db.DB.Exec(`BEGIN;
		CREATE TABLE IF NOT EXISTS chat (
			id INTEGER PRIMARY KEY,
			limb TEXT NOT NULL,
			chat_type TEXT NOT NULL,
			title TEXT NOT NULL,
			UNIQUE(limb)
		);
		CREATE INDEX IF NOT EXISTS idx_title ON chat (title);
		COMMIT;`); err != nil {
		panic(err)
	}
}

type Chat struct {
	ID       int64
	Limb     string
	ChatType string
	Title    string
}

func AddOrUpdateChat(c *Chat) error {
	rows, err := db.DB.Query(`SELECT * FROM chat WHERE limb = ?;`, c.Limb)
	if err != nil {
		return err
	}

	hasNext := rows.Next()
	rows.Close()
	if hasNext {
		if _, err := db.DB.Exec(`UPDATE chat SET title = ? WHERE limb = ?;`, c.Title, c.Limb); err != nil {
			return err
		}
	} else {
		if _, err := db.DB.Exec(`INSERT INTO chat (limb, chat_type, title) VALUES (?, ?, ?);`, c.Limb, c.ChatType, c.Title); err != nil {
			return err
		}
	}

	return nil
}

func GetChat(limb string) (*Chat, error) {
	rows, err := db.DB.Query(`SELECT * FROM chat WHERE limb = ?;`, limb)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	hasNext := rows.Next()
	if hasNext {
		c := &Chat{}
		err = rows.Scan(&c.ID, &c.Limb, &c.ChatType, &c.Title)
		if err != nil {
			return nil, err
		}

		return c, err
	}

	return nil, nil
}

func GetChatCount(query string) (int, error) {
	var rows *sql.Rows
	var err error
	if len(query) > 0 {
		rows, err = db.DB.Query(`SELECT count(*) FROM chat WHERE title LIKE ?;`, "%"+query+"%")
	} else {
		rows, err = db.DB.Query(`SELECT count(*) FROM chat;`)
	}
	if err != nil {
		return 0, err
	}

	defer rows.Close()

	hasNext := rows.Next()
	if hasNext {
		var count int
		err = rows.Scan(&count)
		if err != nil {
			return 0, err
		}

		return count, err
	}

	return 0, nil
}

func GetChatList(pageNum, pageSize int, query string) ([]*Chat, error) {
	chats := []*Chat{}

	offset := pageSize * (pageNum - 1)
	var rows *sql.Rows
	var err error
	if len(query) > 0 {
		rows, err = db.DB.Query(`SELECT * FROM chat
			WHERE title LIKE ?
			LIMIT ?,?;`,
			"%"+query+"%", offset, pageSize)
	} else {
		rows, err = db.DB.Query(`SELECT * FROM chat LIMIT ?,?;`, offset, pageSize)
	}
	if err != nil {
		return chats, err
	}

	defer rows.Close()

	for rows.Next() {
		c := &Chat{}
		if err := rows.Scan(&c.ID, &c.Limb, &c.ChatType, &c.Title); err != nil {
			return chats, err
		}
		chats = append(chats, c)
	}
	if err = rows.Err(); err != nil {
		return chats, err
	}

	return chats, nil
}
