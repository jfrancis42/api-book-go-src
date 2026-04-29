package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

func handleRegister(users *UserRepo, jwtSecret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON"})
			return
		}

		var ve ValidationError
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			ve.Fields = append(ve.Fields, FieldError{Field: "username", Message: "required"})
		}
		if len(req.Password) < 8 {
			ve.Fields = append(ve.Fields, FieldError{Field: "password", Message: "must be at least 8 characters"})
		}
		if len(ve.Fields) > 0 {
			writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
				Error:  "validation failed",
				Code:   "VALIDATION_ERROR",
				Fields: ve.Fields,
			})
			return
		}

		hash, err := hashPassword(req.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
			return
		}

		user, err := users.Create(r.Context(), req.Username, hash)
		if err != nil {
			// SQLite UNIQUE constraint fails with "UNIQUE constraint failed"
			if strings.Contains(err.Error(), "UNIQUE") {
				writeJSON(w, http.StatusConflict, ErrorResponse{
					Error: "username already taken",
					Code:  "CONFLICT",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
			return
		}

		token, err := issueToken(jwtSecret, user.ID, user.Username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
			return
		}
		writeJSON(w, http.StatusCreated, TokenResponse{Token: token})
	}
}

func handleLogin(users *UserRepo, jwtSecret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON"})
			return
		}

		user, err := users.GetByUsername(r.Context(), req.Username)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{
				Error: "invalid credentials",
				Code:  "UNAUTHORIZED",
			})
			return
		}

		if err := checkPassword(user.PasswordHash, req.Password); err != nil {
			writeJSON(w, http.StatusUnauthorized, ErrorResponse{
				Error: "invalid credentials",
				Code:  "UNAUTHORIZED",
			})
			return
		}

		token, err := issueToken(jwtSecret, user.ID, user.Username)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
			return
		}
		writeJSON(w, http.StatusOK, TokenResponse{Token: token})
	}
}
