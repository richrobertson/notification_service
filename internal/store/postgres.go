package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type Postgres struct {
	DB *sql.DB
}

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Postgres{DB: db}, nil
}

func (p *Postgres) Close() error {
	if p == nil || p.DB == nil {
		return nil
	}

	if err := p.DB.Close(); err != nil {
		return fmt.Errorf("close postgres connection: %w", err)
	}

	return nil
}

func (p *Postgres) Ping(ctx context.Context) error {
	if p == nil || p.DB == nil {
		return fmt.Errorf("postgres connection is not initialized")
	}

	if err := p.DB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	return nil
}
