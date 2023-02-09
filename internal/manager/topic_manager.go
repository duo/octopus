package manager

import (
	"github.com/duo/octopus/internal/db"
)

func init() {
	if _, err := db.DB.Prepare(`BEGIN;
		CREATE TABLE IF NOT EXISTS topic (
			id INTEGER PRIMARY KEY,
			master_limb TEXT NOT NULL,
			slave_limb TEXT NOT NULL,
			topic_id INTEGER NOT NULL,
			UNIQUE(master_limb, slave_limb)
		);
		COMMIT;`); err != nil {
		panic(err)
	}
}

type Topic struct {
	ID         int64
	MasterLimb string
	SlaveLimb  string
	TopicID    string
}

func GetTopic(master_limb, slave_limb string) (*Topic, error) {
	rows, err := db.DB.Query(`SELECT * FROM topic WHERE master_limb = ? AND slave_limb = ?;`, master_limb, slave_limb)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	hasNext := rows.Next()
	if hasNext {
		t := &Topic{}
		err = rows.Scan(&t.ID, &t.MasterLimb, &t.SlaveLimb, &t.TopicID)
		if err != nil {
			return nil, err
		}

		return t, err
	}

	return nil, nil
}

func GetTopicByMaster(master_limb string, topic_id int64) (*Topic, error) {
	rows, err := db.DB.Query(`SELECT * FROM topic WHERE master_limb = ? AND topic_id = ?;`, master_limb, topic_id)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	hasNext := rows.Next()
	if hasNext {
		t := &Topic{}
		err = rows.Scan(&t.ID, &t.MasterLimb, &t.SlaveLimb, &t.TopicID)
		if err != nil {
			return nil, err
		}

		return t, err
	}

	return nil, nil
}

func AddTopic(t *Topic) error {
	_, err := db.DB.Exec(
		`INSERT INTO topic (master_limb, slave_limb, topic_id) VALUES (?, ?, ?);`,
		t.MasterLimb, t.SlaveLimb, t.TopicID,
	)
	return err
}

func DelTopic(master_limb, slave_limb string) error {
	_, err := db.DB.Exec(
		`DELETE FROM link WHERE master_limb = ? AND slave_limb = ?;`,
		master_limb, slave_limb,
	)
	return err
}
