package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/jenfonro/reader/internal/db"
)

const CookieName = "reader_auth"

var tokenTTL = 30 * 24 * time.Hour

type User struct {
	ID       int64  `json:"userId"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type Auth struct {
	db *db.DB
}

type ctxKey int

const userKey ctxKey = iota

func New(database *db.DB) *Auth {
	return &Auth{db: database}
}

func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(readCookie(r))
		if token == "" {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, (*User)(nil))))
			return
		}

		u, exp := a.resolveToken(token)
		if u == nil || exp.Before(time.Now()) || u.Status != "active" {
			a.deleteToken(token)
			clearCookie(w)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, (*User)(nil))))
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func CurrentUser(r *http.Request) *User {
	if r == nil {
		return nil
	}
	u, _ := r.Context().Value(userKey).(*User)
	return u
}

func (a *Auth) Login(w http.ResponseWriter, username, password string) (*User, int, string) {
	u := strings.TrimSpace(username)
	p := password
	if u == "" || p == "" {
		return nil, http.StatusBadRequest, "用户名与密码不能为空"
	}
	row, err := a.db.GetUserAuthByUsername(u)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, http.StatusUnauthorized, "用户名或密码错误"
		}
		return nil, http.StatusInternalServerError, "请求失败"
	}
	if strings.TrimSpace(row.Status) != "active" {
		return nil, http.StatusForbidden, "该账户已禁用"
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(p)); err != nil {
		return nil, http.StatusUnauthorized, "用户名或密码错误"
	}
	token, err := a.issueToken(row.ID)
	if err != nil || token == "" {
		return nil, http.StatusInternalServerError, "请求失败"
	}
	writeCookie(w, token)
	return &User{
		ID:       row.ID,
		Username: row.Username,
		Role:     row.Role,
		Status:   row.Status,
	}, http.StatusOK, ""
}

func (a *Auth) resolveToken(token string) (*User, time.Time) {
	row, err := a.db.ResolveToken(token)
	if err != nil {
		return nil, time.Time{}
	}
	return &User{
		ID:       row.UserID,
		Username: row.Username,
		Role:     row.Role,
		Status:   row.Status,
	}, row.ExpiresAt
}

func (a *Auth) issueToken(userID int64) (string, error) {
	if userID <= 0 {
		return "", errors.New("invalid user id")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	if err := a.db.InsertToken(token, userID, time.Now().Add(tokenTTL)); err != nil {
		return "", err
	}
	return token, nil
}

func (a *Auth) deleteToken(token string) {
	_ = a.db.DeleteToken(token)
}

func readCookie(r *http.Request) string {
	if r == nil {
		return ""
	}
	c, err := r.Cookie(CookieName)
	if err != nil || c == nil {
		return ""
	}
	return c.Value
}

func writeCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(tokenTTL.Seconds()),
	})
}

func clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
