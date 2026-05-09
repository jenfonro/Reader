package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jenfonro/reader/internal/auth"
	"github.com/jenfonro/reader/internal/db"
)

func bootstrapHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		u := auth.CurrentUser(r)
		var user any
		if u != nil {
			user = u
		}
		siteName, err := database.SiteName()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "请求失败"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"siteName":      siteName,
			"version":       "v3.24.052012",
			"authenticated": u != nil,
			"user":          user,
		})
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, buildHomePayload())
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func loginHandler(authMw *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
			return
		}
		var input loginRequest
		contentType := strings.ToLower(r.Header.Get("Content-Type"))
		if strings.Contains(contentType, "application/json") {
			_ = json.NewDecoder(r.Body).Decode(&input)
		} else {
			_ = r.ParseForm()
			input.Username = r.FormValue("username")
			input.Password = r.FormValue("password")
		}
		u, status, msg := authMw.Login(w, input.Username, input.Password)
		if status != http.StatusOK {
			writeJSON(w, status, map[string]any{"success": false, "message": msg})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"user":    u,
		})
	}
}

type systemSettingsRequest struct {
	SiteName          *string `json:"siteName"`
	SearchConcurrency *int    `json:"searchConcurrency"`
}

type userSettingsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func usersHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(w, r) == nil {
			return
		}

		switch r.Method {
		case http.MethodGet:
			users, err := database.ListUsers()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "请求失败"})
				return
			}
			items := make([]any, 0, len(users))
			for _, row := range users {
				items = append(items, userPayload(row))
			}
			writeJSON(w, http.StatusOK, map[string]any{"users": items})
		case http.MethodPost:
			var input userSettingsRequest
			_ = json.NewDecoder(r.Body).Decode(&input)
			username := strings.TrimSpace(input.Username)
			if username == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"message": "用户名不能为空"})
				return
			}
			if strings.TrimSpace(input.Password) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"message": "密码不能为空"})
				return
			}
			row, err := database.CreateUser(username, input.Password, input.Role)
			if err != nil {
				writeUserError(w, err, "新增失败")
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"user": userPayload(row)})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		}
	}
}

func userHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := requireAdmin(w, r)
		if admin == nil {
			return
		}

		userID, ok := parseUserID(r.URL.Path)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "用户不存在"})
			return
		}

		switch r.Method {
		case http.MethodPut:
			var input userSettingsRequest
			_ = json.NewDecoder(r.Body).Decode(&input)
			if strings.TrimSpace(input.Username) == "" && strings.TrimSpace(input.Password) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请填写要修改的用户名或密码"})
				return
			}
			row, err := database.UpdateUser(userID, input.Username, input.Password)
			if err != nil {
				writeUserError(w, err, "修改失败")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"user": userPayload(row)})
		case http.MethodDelete:
			if admin.ID == userID {
				writeJSON(w, http.StatusBadRequest, map[string]string{"message": "不能删除当前登录用户"})
				return
			}
			if err := database.DeleteUser(userID); err != nil {
				writeUserError(w, err, "删除失败")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		}
	}
}

func systemSettingsHandler(database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if requireAdmin(w, r) == nil {
			return
		}

		switch r.Method {
		case http.MethodGet:
			writeSystemSettings(w, database)
		case http.MethodPut:
			var input systemSettingsRequest
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请求格式不正确"})
				return
			}
			if input.SiteName != nil {
				siteName := strings.TrimSpace(*input.SiteName)
				if siteName == "" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"message": "站点名称不能为空"})
					return
				}
				if err := database.SetSiteName(siteName); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "保存失败"})
					return
				}
			}
			if input.SearchConcurrency != nil {
				if *input.SearchConcurrency <= 0 {
					writeJSON(w, http.StatusBadRequest, map[string]string{"message": "搜索并发数必须大于 0"})
					return
				}
				if err := database.SetSearchConcurrency(*input.SearchConcurrency); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "保存失败"})
					return
				}
			}
			writeSystemSettings(w, database)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"message": "method not allowed"})
		}
	}
}

func writeSystemSettings(w http.ResponseWriter, database *db.DB) {
	siteName, err := database.SiteName()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "请求失败"})
		return
	}
	searchConcurrency, err := database.SearchConcurrency()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "请求失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"siteName":          siteName,
		"searchConcurrency": searchConcurrency,
	})
}

func requireAdmin(w http.ResponseWriter, r *http.Request) *auth.User {
	u := auth.CurrentUser(r)
	if u == nil || u.Role != "admin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "forbidden"})
		return nil
	}
	return u
}

func parseUserID(path string) (int64, bool) {
	value := strings.Trim(strings.TrimPrefix(path, "/api/settings/users/"), "/")
	if value == "" || strings.Contains(value, "/") {
		return 0, false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func userPayload(row db.UserRow) map[string]any {
	return map[string]any{
		"userId":    row.ID,
		"username":  row.Username,
		"role":      row.Role,
		"status":    row.Status,
		"createdAt": row.CreatedAt,
		"updatedAt": row.UpdatedAt,
	}
}

func writeUserError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, db.ErrUsernameExists):
		writeJSON(w, http.StatusConflict, map[string]string{"message": "用户名已存在"})
	case errors.Is(err, db.ErrNoUserUpdate):
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "请填写要修改的用户名或密码"})
	case errors.Is(err, db.ErrLastAdmin):
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "不能删除最后一个管理员"})
	case errors.Is(err, sql.ErrNoRows):
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "用户不存在"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": fallback})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
