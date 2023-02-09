package manager

import (
	"github.com/duo/octopus/internal/db"
)

func init() {
	if _, err := db.DB.Exec(`BEGIN;
		CREATE TABLE IF NOT EXISTS link (
			id INTEGER PRIMARY KEY,
			master_limb TEXT NOT NULL,
			slave_limb TEXT NOT NULL,
			UNIQUE(master_limb, slave_limb)
		);
		COMMIT;`); err != nil {
		panic(err)
	}
}

type Link struct {
	ID         int64
	MasterLimb string
	SlaveLimb  string
	Title      string
}

func GetLinkList() ([]*Link, error) {
	links := []*Link{}

	rows, err := db.DB.Query(`SELECT
		l.id, l.master_limb, l.slave_limb, c.title 
		FROM link AS l LEFT JOIN chat AS c
		ON l.slave_limb = c.limb;`)
	if err != nil {
		return links, err
	}

	defer rows.Close()

	for rows.Next() {
		l := &Link{}
		if err := rows.Scan(&l.ID, &l.MasterLimb, &l.SlaveLimb, &l.Title); err != nil {
			return links, err
		}
		links = append(links, l)
	}
	if err = rows.Err(); err != nil {
		return links, err
	}

	return links, nil
}

func GetLinksByMaster(masterLimb string) ([]*Link, error) {
	links := []*Link{}

	rows, err := db.DB.Query(`SELECT
		l.id, l.master_limb, l.slave_limb, c.title 
		FROM link AS l LEFT JOIN chat AS c
		ON l.slave_limb = c.limb
		WHERE l.master_limb = ?;`,
		masterLimb,
	)
	if err != nil {
		return links, err
	}

	defer rows.Close()

	for rows.Next() {
		l := &Link{}
		if err := rows.Scan(&l.ID, &l.MasterLimb, &l.SlaveLimb, &l.Title); err != nil {
			return links, err
		}
		links = append(links, l)
	}
	if err = rows.Err(); err != nil {
		return links, err
	}

	return links, nil
}

func GetLinksBySlave(slaveLimb string) ([]*Link, error) {
	links := []*Link{}

	rows, err := db.DB.Query(`SELECT
		l.id, l.master_limb, l.slave_limb, c.title 
		FROM link AS l LEFT JOIN chat AS c
		ON l.slave_limb = c.limb
		WHERE l.slave_limb = ?;`,
		slaveLimb,
	)
	if err != nil {
		return links, err
	}

	defer rows.Close()

	for rows.Next() {
		l := &Link{}
		if err := rows.Scan(&l.ID, &l.MasterLimb, &l.SlaveLimb, &l.Title); err != nil {
			return links, err
		}
		links = append(links, l)
	}
	if err = rows.Err(); err != nil {
		return links, err
	}

	return links, nil
}

func AddLink(l *Link) error {
	_, err := db.DB.Exec(`INSERT INTO link (master_limb, slave_limb) VALUES (?, ?);`, l.MasterLimb, l.SlaveLimb)
	return err
}

func DelLinkById(id int64) error {
	_, err := db.DB.Exec(`DELETE FROM link WHERE id = ?;`, id)
	return err
}
