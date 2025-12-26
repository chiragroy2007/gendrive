package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type AuthHandler struct {
	DB       *sql.DB
	Sessions map[string]string // Token -> UserID
	mu       sync.Mutex
}

func NewAuthHandler(db *sql.DB) *AuthHandler {
	return &AuthHandler{
		DB:       db,
		Sessions: make(map[string]string),
	}
}

type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Signup
func (a *AuthHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid credentials", http.StatusBadRequest)
		return
	}

	id := uuid.New().String()
	// Storing plaintext password for prototype speed.
	// Production: Use bcrypt (`golang.org/x/crypto/bcrypt`)
	_, err := a.DB.Exec("INSERT INTO users (id, email, password, created_at) VALUES (?, ?, ?, ?)",
		id, creds.Email, creds.Password, time.Now().Format(time.RFC3339))

	if err != nil {
		http.Error(w, "User already exists or DB error", http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// Login
func (a *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	var id string
	err := a.DB.QueryRow("SELECT id FROM users WHERE email = ? AND password = ?", creds.Email, creds.Password).Scan(&id)
	if err != nil {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	// Create Session
	token := generateToken()
	a.mu.Lock()
	a.Sessions[token] = id
	a.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:  "session_token",
		Value: token,
		Path:  "/",
	})

	json.NewEncoder(w).Encode(map[string]string{"user_id": id})
}

// Middleware
func (a *AuthHandler) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session_token")
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		a.mu.Lock()
		userID, ok := a.Sessions[c.Value]
		a.mu.Unlock()

		if !ok {
			http.Error(w, "Invalid session", http.StatusUnauthorized)
			return
		}

		// Inject UserID into Header for handlers to access
		r.Header.Set("X-User-ID", userID)
		next(w, r)
	}
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
