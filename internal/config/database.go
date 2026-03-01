package config

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDB(dsn string) *pgxpool.Pool {
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatal("db open error:", err)
		return nil
	}

	if err := db.Ping(context.Background()); err != nil {
		log.Fatal("DB Ping error:- ", err)
		return nil
	}

	return db
}
