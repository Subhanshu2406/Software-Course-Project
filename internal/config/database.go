package config

import (
	"database/sql"
	"log"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func NewDB(dsn string) *sql.DB {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("db open error:", err)
		return nil
	}

	if err := db.Ping(); err != nil {
		log.Fatal("DB Ping error:- ", err)
		return nil
	}

	return db
}
