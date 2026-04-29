package server

import "time"

// Todo is the core domain model.
type Todo struct {
	ID        int64     `db:"id"         json:"id"`
	UserID    int64     `db:"user_id"    json:"user_id"`
	Text      string    `db:"text"       json:"text"`
	Done      bool      `db:"done"       json:"done"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// User stores credentials and identity.
type User struct {
	ID           int64     `db:"id"            json:"id"`
	Username     string    `db:"username"       json:"username"`
	PasswordHash string    `db:"password_hash"  json:"-"`
	CreatedAt    time.Time `db:"created_at"     json:"created_at"`
}

// Page[T] is the paginated response envelope.
type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int   `json:"total"`
	Offset int   `json:"offset"`
	Limit  int   `json:"limit"`
}

// CreateTodoRequest is the request body for POST /todos.
type CreateTodoRequest struct {
	Text string `json:"text"`
}

// UpdateTodoRequest is the request body for PATCH /todos/{id}.
type UpdateTodoRequest struct {
	Text *string `json:"text"`
	Done *bool   `json:"done"`
}

// RegisterRequest is the body for POST /auth/register.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest is the body for POST /auth/login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TokenResponse carries a JWT after successful login/register.
type TokenResponse struct {
	Token string `json:"token"`
}
