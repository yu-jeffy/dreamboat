package dspostgres

import (
	"database/sql"
	"time"
)

type Datastore struct {
	Db *sql.DB
}

func (s *Datastore) Open(dbURL string, maxOpen, maxIdle int, maxIdleTime time.Duration) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return err
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxIdleTime(maxIdleTime)

	// test db connection
	if err = db.Ping(); err != nil {
		return err
	}

	s.Db = db
	return nil
}

func (s *Datastore) Close() error {
	return s.Db.Close()
}
