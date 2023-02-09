package manager

import (
	"database/sql"

	"github.com/duo/octopus/internal/common"
	"github.com/duo/octopus/internal/db"
)

func init() {
	if _, err := db.DB.Exec(`BEGIN;
		CREATE TABLE IF NOT EXISTS message (
			id INTEGER PRIMARY KEY,
			master_limb TEXT NOT NULL,
			master_msg_id TEXT NOT NULL,
			master_msg_thread_id TEXT NOT NULL,
			slave_limb TEXT NOT NULL,
			slave_msg_id TEXT NOT NULL,
			slave_sender TEXT NOT NULL,
			content TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			created DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(master_limb, master_msg_id)
		);
		CREATE INDEX IF NOT EXISTS idx_slave_reply ON message (slave_limb, timestamp);
		CREATE INDEX IF NOT EXISTS idx_master_reply ON message (master_limb, master_msg_id);
		COMMIT;`); err != nil {
		panic(err)
	}
}

type Message struct {
	ID                string
	MasterLimb        string
	MasterMsgID       string
	MasterMsgThreadID string
	SlaveLimb         string
	SlaveMsgID        string
	SlaveSender       string
	Content           string
	Timestamp         int64
}

func AddMessage(m *Message) error {
	_, err := db.DB.Exec(`INSERT INTO message
		(master_limb, master_msg_id, master_msg_thread_id, slave_limb, slave_msg_id, slave_sender, content, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?);`,
		m.MasterLimb, m.MasterMsgID, m.MasterMsgThreadID, m.SlaveLimb, m.SlaveMsgID, m.SlaveSender, m.Content, m.Timestamp,
	)
	return err
}

func GetMessageByMasterMsgId(masterLimb, masterMsgId string) (*Message, error) {
	rows, err := db.DB.Query(`SELECT id, master_limb, master_msg_id, master_msg_thread_id, slave_limb, slave_msg_id, slave_sender, content, timestamp
		FROM message
		WHERE master_limb = ? AND master_msg_id = ?;`,
		masterLimb, masterMsgId)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	hasNext := rows.Next()
	if hasNext {
		m := &Message{}
		err = rows.Scan(&m.ID, &m.MasterLimb, &m.MasterMsgID, &m.MasterMsgThreadID, &m.SlaveLimb, &m.SlaveMsgID, &m.SlaveSender, &m.Content, &m.Timestamp)
		if err != nil {
			return nil, err
		}

		return m, err
	}

	return nil, nil
}

func GetMessagesBySlave(slaveLimb, slaveMsgId string) ([]*Message, error) {
	messages := []*Message{}

	rows, err := db.DB.Query(`SELECT id, master_limb, master_msg_id, master_msg_thread_id, slave_limb, slave_msg_id, content
		FROM message
		WHERE slave_limb = ? AND slave_msg_id = ?;`,
		slaveLimb, slaveMsgId)
	if err != nil {
		return messages, err
	}

	defer rows.Close()

	for rows.Next() {
		m := &Message{}
		err := rows.Scan(&m.ID, &m.MasterLimb, &m.MasterMsgID, &m.MasterMsgThreadID, &m.SlaveLimb, &m.SlaveMsgID, &m.Content)
		if err != nil {
			return messages, err
		}
		messages = append(messages, m)
	}
	if err = rows.Err(); err != nil {
		return messages, err
	}

	return messages, nil
}

func GetMessagesBySlaveReply(slaveLimb string, reply *common.ReplyInfo) ([]*Message, error) {
	messages := []*Message{}

	var rows *sql.Rows
	var err error
	if reply.Timestamp == 0 {
		rows, err = db.DB.Query(`SELECT id, master_limb, master_msg_id, master_msg_thread_id, slave_limb, slave_msg_id, content
		FROM message
		WHERE slave_limb = ? AND slave_msg_id = ?;`,
			slaveLimb, reply.ID)
	} else {
		// TODO: back search?
		rows, err = db.DB.Query(`SELECT id, master_limb, master_msg_id, master_msg_thread_id, slave_limb, slave_msg_id, content
		FROM message
		WHERE slave_limb = ? AND timestamp = ? AND slave_msg_id LIKE ?;`,
			slaveLimb, reply.Timestamp, reply.ID+"%")
	}
	if err != nil {
		return messages, err
	}

	defer rows.Close()

	for rows.Next() {
		m := &Message{}
		err := rows.Scan(&m.ID, &m.MasterLimb, &m.MasterMsgID, &m.MasterMsgThreadID, &m.SlaveLimb, &m.SlaveMsgID, &m.Content)
		if err != nil {
			return messages, err
		}
		messages = append(messages, m)
	}
	if err = rows.Err(); err != nil {
		return messages, err
	}

	return messages, nil
}
