package todos

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
)

func newTestRepo(t *testing.T) *TodoRepo {
	t.Helper()
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.MustExec("PRAGMA foreign_keys=ON")
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewTodoRepo(db)
}

func TestTodoRepo_CreateAndGet(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, "write tests")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if created.Text != "write tests" {
		t.Errorf("text: got %q, want %q", created.Text, "write tests")
	}
	if created.Done {
		t.Error("new todo should not be done")
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID {
		t.Errorf("id: got %d, want %d", got.ID, created.ID)
	}
}

func TestTodoRepo_Get_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.Get(context.Background(), 999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTodoRepo_List(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for _, text := range []string{"first", "second", "third"} {
		repo.Create(ctx, text)
	}

	todos, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(todos) != 3 {
		t.Fatalf("got %d todos, want 3", len(todos))
	}
	if todos[0].Text != "first" {
		t.Errorf("todos[0]: got %q, want %q", todos[0].Text, "first")
	}
}

func TestTodoRepo_Update(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	todo, _ := repo.Create(ctx, "mark done")
	updated, err := repo.Update(ctx, todo.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Done {
		t.Error("expected done=true after update")
	}
}

func TestTodoRepo_Update_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	_, err := repo.Update(context.Background(), 999, true)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestTodoRepo_Delete(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	todo, _ := repo.Create(ctx, "delete me")
	if err := repo.Delete(ctx, todo.ID); err != nil {
		t.Fatal(err)
	}
	_, err := repo.Get(ctx, todo.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: expected ErrNotFound, got %v", err)
	}
}

func TestTodoRepo_DeleteDone(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for _, done := range []bool{true, false, true, true, false} {
		todo, _ := repo.Create(ctx, "todo")
		if done {
			repo.Update(ctx, todo.ID, true)
		}
	}

	deleted, err := repo.DeleteDone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 3 {
		t.Errorf("deleted: got %d, want 3", deleted)
	}

	remaining, _ := repo.List(ctx)
	if len(remaining) != 2 {
		t.Errorf("remaining: got %d, want 2", len(remaining))
	}
}
