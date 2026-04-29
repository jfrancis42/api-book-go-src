package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS todos (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id),
    text       TEXT    NOT NULL,
    done       BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

func openDB(path string) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite write serialization
	if err := db.Ping(); err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// UserRepo handles user persistence.
type UserRepo struct{ db *sqlx.DB }

func NewUserRepo(db *sqlx.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) Create(ctx context.Context, username, hash string) (*User, error) {
	var u User
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO users (username, password_hash) VALUES (?, ?) RETURNING *`,
		username, hash,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return &u, nil
}

// TodoRepo handles todo persistence, scoped to a user.
type TodoRepo struct{ db *sqlx.DB }

func NewTodoRepo(db *sqlx.DB) *TodoRepo { return &TodoRepo{db: db} }

func (r *TodoRepo) List(ctx context.Context, userID int64, offset, limit int) ([]*Todo, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM todos WHERE user_id = ?`, userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	var todos []*Todo
	err = r.db.SelectContext(ctx, &todos,
		`SELECT * FROM todos WHERE user_id = ? ORDER BY id LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	return todos, total, nil
}

func (r *TodoRepo) Create(ctx context.Context, userID int64, text string) (*Todo, error) {
	var t Todo
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO todos (user_id, text) VALUES (?, ?) RETURNING *`,
		userID, text,
	).Scan(&t.ID, &t.UserID, &t.Text, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TodoRepo) Get(ctx context.Context, id int64) (*Todo, error) {
	var t Todo
	err := r.db.QueryRowContext(ctx,
		`SELECT * FROM todos WHERE id = ?`, id,
	).Scan(&t.ID, &t.UserID, &t.Text, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (r *TodoRepo) Update(ctx context.Context, id int64, text *string, done *bool) (*Todo, error) {
	existing, err := r.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	newText := existing.Text
	newDone := existing.Done
	if text != nil {
		newText = *text
	}
	if done != nil {
		newDone = *done
	}

	var t Todo
	err = r.db.QueryRowContext(ctx,
		`UPDATE todos SET text = ?, done = ?, updated_at = ? WHERE id = ? RETURNING *`,
		newText, newDone, time.Now(), id,
	).Scan(&t.ID, &t.UserID, &t.Text, &t.Done, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TodoRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM todos WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// errDBClosed is returned by sqlx when the DB is closed; we use it for health checks.
var errDBClosed = errors.New("sql: database is closed")
