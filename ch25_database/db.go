package todos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

const schema = `
CREATE TABLE IF NOT EXISTS todos (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    text      TEXT    NOT NULL,
    done      BOOLEAN NOT NULL DEFAULT 0,
    created   DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

func OpenDB(path string) (*sqlx.DB, error) {
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.MustExec("PRAGMA journal_mode=WAL")
	db.MustExec("PRAGMA foreign_keys=ON")
	db.SetMaxOpenConns(1)
	return db, nil
}

func Migrate(db *sqlx.DB) error {
	_, err := db.Exec(schema)
	return err
}

type Todo struct {
	ID      int64     `db:"id"      json:"id"`
	Text    string    `db:"text"    json:"text"`
	Done    bool      `db:"done"    json:"done"`
	Created time.Time `db:"created" json:"created"`
}

type TodoRepo struct {
	db *sqlx.DB
}

func NewTodoRepo(db *sqlx.DB) *TodoRepo {
	return &TodoRepo{db: db}
}

func (r *TodoRepo) List(ctx context.Context) ([]*Todo, error) {
	var todos []*Todo
	err := r.db.SelectContext(ctx, &todos, "SELECT * FROM todos ORDER BY id")
	return todos, err
}

func (r *TodoRepo) Create(ctx context.Context, text string) (*Todo, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO todos (text) VALUES (?)", text)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

func (r *TodoRepo) Get(ctx context.Context, id int64) (*Todo, error) {
	var todo Todo
	err := r.db.GetContext(ctx, &todo, "SELECT * FROM todos WHERE id = ?", id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("todo %d: %w", id, ErrNotFound)
	}
	return &todo, err
}

func (r *TodoRepo) Update(ctx context.Context, id int64, done bool) (*Todo, error) {
	result, err := r.db.ExecContext(ctx,
		"UPDATE todos SET done = ? WHERE id = ?", done, id)
	if err != nil {
		return nil, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("todo %d: %w", id, ErrNotFound)
	}
	return r.Get(ctx, id)
}

func (r *TodoRepo) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM todos WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("todo %d: %w", id, ErrNotFound)
	}
	return nil
}

func (r *TodoRepo) DeleteDone(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM todos WHERE done = 1")
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}
