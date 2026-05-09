package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	db *sql.DB
}

type UserAuthRow struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	Status       string
	CreatedAt    int64
	UpdatedAt    int64
}

type UserRow struct {
	ID        int64
	Username  string
	Role      string
	Status    string
	CreatedAt int64
	UpdatedAt int64
}

var (
	ErrUsernameExists = errors.New("username exists")
	ErrNoUserUpdate   = errors.New("no user update")
	ErrLastAdmin      = errors.New("last admin")
)

type TokenRow struct {
	Token     string
	UserID    int64
	Username  string
	Role      string
	Status    string
	ExpiresAt time.Time
}

const (
	DefaultSiteName          = "开源阅读"
	DefaultSearchConcurrency = 24
)

func Open() (*DB, error) {
	filePath, err := resolveDBFile()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, err
	}
	raw, err := sql.Open("sqlite3", filePath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	if err := raw.Ping(); err != nil {
		_ = raw.Close()
		return nil, err
	}
	d := &DB{db: raw}
	if err := d.initSchema(); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return d, nil
}

func resolveDBFile() (string, error) {
	if v := strings.TrimSpace(os.Getenv("READER_DB_FILE")); v != "" {
		return filepath.Clean(v), nil
	}
	base := strings.TrimSpace(os.Getenv("READER_DATA_DIR"))
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		base = wd
	}
	return filepath.Join(base, "data.db"), nil
}

func (d *DB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

func (d *DB) initSchema() error {
	if d == nil || d.db == nil {
		return errors.New("db nil")
	}
	if _, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin','user')),
			status TEXT NOT NULL DEFAULT 'active',
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_users_role_status ON users(role, status);

		CREATE TABLE IF NOT EXISTS auth_tokens (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_auth_tokens_user_id ON auth_tokens(user_id);
		CREATE INDEX IF NOT EXISTS idx_auth_tokens_expires_at ON auth_tokens(expires_at);

		CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS book_sources (
			book_source_url TEXT PRIMARY KEY,
			book_source_name TEXT NOT NULL,
			book_source_group TEXT,
			book_source_type INTEGER NOT NULL DEFAULT 0,
			book_url_pattern TEXT,
			custom_order INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			enabled_explore INTEGER NOT NULL DEFAULT 1,
			js_lib TEXT,
			enabled_cookie_jar INTEGER DEFAULT 0,
			concurrent_rate TEXT,
			header TEXT,
			login_url TEXT,
			login_ui TEXT,
			login_check_js TEXT,
			cover_decode_js TEXT,
			book_source_comment TEXT,
			variable_comment TEXT,
			last_update_time INTEGER NOT NULL DEFAULT 0,
			respond_time INTEGER NOT NULL DEFAULT 180000,
			weight INTEGER NOT NULL DEFAULT 0,
			explore_url TEXT,
			explore_screen TEXT,
			search_url TEXT,
			rule_search TEXT,
			rule_explore TEXT,
			rule_book_info TEXT,
			rule_toc TEXT,
			rule_content TEXT,
			rule_review TEXT,
			raw_json TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_book_sources_group ON book_sources(book_source_group);
		CREATE INDEX IF NOT EXISTS idx_book_sources_enabled ON book_sources(enabled, enabled_explore);
		CREATE INDEX IF NOT EXISTS idx_book_sources_order ON book_sources(custom_order, book_source_name COLLATE NOCASE, book_source_url);
		CREATE INDEX IF NOT EXISTS idx_book_sources_updated ON book_sources(updated_at);
	`); err != nil {
		return err
	}
	if err := d.ensureDefaultSettings(); err != nil {
		return err
	}
	return d.ensureDefaultAdmin()
}

func (d *DB) ensureDefaultSettings() error {
	now := time.Now().Unix()
	_, err := d.db.Exec(`
		INSERT OR IGNORE INTO app_settings(key,value,created_at,updated_at) VALUES
			('site_name',?,?,?),
			('search_concurrency',?,?,?)
	`,
		DefaultSiteName,
		now,
		now,
		strconv.Itoa(DefaultSearchConcurrency),
		now,
		now,
	)
	return err
}

func (d *DB) ensureDefaultAdmin() error {
	var cnt int
	if err := d.db.QueryRow(`SELECT COUNT(1) FROM users WHERE role='admin'`).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte("admin"), 10)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	_, err = d.db.Exec(
		`INSERT INTO users(username,password,role,status,created_at,updated_at) VALUES(?,?, 'admin','active',?,?)`,
		"admin",
		string(hashed),
		now,
		now,
	)
	return err
}

func (d *DB) GetUserAuthByUsername(username string) (UserAuthRow, error) {
	if d == nil || d.db == nil {
		return UserAuthRow{}, errors.New("db nil")
	}
	u := strings.TrimSpace(username)
	if u == "" {
		return UserAuthRow{}, sql.ErrNoRows
	}
	var row UserAuthRow
	err := d.db.QueryRow(
		`SELECT id, username, password, role, status, created_at, updated_at FROM users WHERE username=? LIMIT 1`,
		u,
	).Scan(&row.ID, &row.Username, &row.PasswordHash, &row.Role, &row.Status, &row.CreatedAt, &row.UpdatedAt)
	return row, err
}

func (d *DB) ListUsers() ([]UserRow, error) {
	if d == nil || d.db == nil {
		return nil, errors.New("db nil")
	}
	rows, err := d.db.Query(`SELECT id, username, role, status, created_at, updated_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]UserRow, 0)
	for rows.Next() {
		var row UserRow
		if err := rows.Scan(&row.ID, &row.Username, &row.Role, &row.Status, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (d *DB) GetUserByID(userID int64) (UserRow, error) {
	if d == nil || d.db == nil {
		return UserRow{}, errors.New("db nil")
	}
	if userID <= 0 {
		return UserRow{}, sql.ErrNoRows
	}
	var row UserRow
	err := d.db.QueryRow(
		`SELECT id, username, role, status, created_at, updated_at FROM users WHERE id=? LIMIT 1`,
		userID,
	).Scan(&row.ID, &row.Username, &row.Role, &row.Status, &row.CreatedAt, &row.UpdatedAt)
	return row, err
}

func (d *DB) CreateUser(username, password, role string) (UserRow, error) {
	if d == nil || d.db == nil {
		return UserRow{}, errors.New("db nil")
	}
	u := strings.TrimSpace(username)
	if u == "" || strings.TrimSpace(password) == "" {
		return UserRow{}, errors.New("invalid user args")
	}
	r := strings.TrimSpace(role)
	if r != "admin" {
		r = "user"
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return UserRow{}, err
	}
	now := time.Now().Unix()
	result, err := d.db.Exec(
		`INSERT INTO users(username,password,role,status,created_at,updated_at) VALUES(?,?,?,'active',?,?)`,
		u,
		string(hashed),
		r,
		now,
		now,
	)
	if err != nil {
		if isUniqueConstraint(err) {
			return UserRow{}, ErrUsernameExists
		}
		return UserRow{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return UserRow{}, err
	}
	return d.GetUserByID(id)
}

func (d *DB) UpdateUser(userID int64, username, password string) (UserRow, error) {
	if d == nil || d.db == nil {
		return UserRow{}, errors.New("db nil")
	}
	if userID <= 0 {
		return UserRow{}, sql.ErrNoRows
	}
	u := strings.TrimSpace(username)
	passwordChanged := strings.TrimSpace(password) != ""
	if u == "" && !passwordChanged {
		return UserRow{}, ErrNoUserUpdate
	}

	now := time.Now().Unix()
	var err error
	if u != "" && passwordChanged {
		hashed, hashErr := bcrypt.GenerateFromPassword([]byte(password), 10)
		if hashErr != nil {
			return UserRow{}, hashErr
		}
		_, err = d.db.Exec(`UPDATE users SET username=?, password=?, updated_at=? WHERE id=?`, u, string(hashed), now, userID)
	} else if u != "" {
		_, err = d.db.Exec(`UPDATE users SET username=?, updated_at=? WHERE id=?`, u, now, userID)
	} else {
		hashed, hashErr := bcrypt.GenerateFromPassword([]byte(password), 10)
		if hashErr != nil {
			return UserRow{}, hashErr
		}
		_, err = d.db.Exec(`UPDATE users SET password=?, updated_at=? WHERE id=?`, string(hashed), now, userID)
	}
	if err != nil {
		if isUniqueConstraint(err) {
			return UserRow{}, ErrUsernameExists
		}
		return UserRow{}, err
	}
	return d.GetUserByID(userID)
}

func (d *DB) DeleteUser(userID int64) error {
	if d == nil || d.db == nil {
		return errors.New("db nil")
	}
	row, err := d.GetUserByID(userID)
	if err != nil {
		return err
	}
	if row.Role == "admin" {
		var count int
		if err := d.db.QueryRow(`SELECT COUNT(1) FROM users WHERE role='admin'`).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastAdmin
		}
	}
	result, err := d.db.Exec(`DELETE FROM users WHERE id=?`, userID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint failed")
}

func (d *DB) InsertToken(token string, userID int64, expiresAt time.Time) error {
	if d == nil || d.db == nil {
		return errors.New("db nil")
	}
	t := strings.TrimSpace(token)
	if t == "" || userID <= 0 || expiresAt.IsZero() {
		return errors.New("invalid token args")
	}
	now := time.Now().Unix()
	_, err := d.db.Exec(`INSERT INTO auth_tokens(token,user_id,created_at,expires_at) VALUES(?,?,?,?)`, t, userID, now, expiresAt.Unix())
	return err
}

func (d *DB) ResolveToken(token string) (TokenRow, error) {
	if d == nil || d.db == nil {
		return TokenRow{}, errors.New("db nil")
	}
	t := strings.TrimSpace(token)
	if t == "" {
		return TokenRow{}, sql.ErrNoRows
	}
	var row TokenRow
	var expires int64
	err := d.db.QueryRow(`
		SELECT a.token, a.user_id, u.username, u.role, u.status, a.expires_at
		FROM auth_tokens a
		JOIN users u ON u.id = a.user_id
		WHERE a.token = ?
		LIMIT 1
	`, t).Scan(&row.Token, &row.UserID, &row.Username, &row.Role, &row.Status, &expires)
	if err != nil {
		return TokenRow{}, err
	}
	row.ExpiresAt = time.Unix(expires, 0)
	return row, nil
}

func (d *DB) DeleteToken(token string) error {
	if d == nil || d.db == nil {
		return nil
	}
	t := strings.TrimSpace(token)
	if t == "" {
		return nil
	}
	_, err := d.db.Exec(`DELETE FROM auth_tokens WHERE token=?`, t)
	return err
}

func (d *DB) SiteName() (string, error) {
	if d == nil || d.db == nil {
		return DefaultSiteName, errors.New("db nil")
	}
	var value string
	err := d.db.QueryRow(`SELECT value FROM app_settings WHERE key='site_name' LIMIT 1`).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultSiteName, nil
		}
		return DefaultSiteName, err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultSiteName, nil
	}
	return value, nil
}

func (d *DB) SetSiteName(siteName string) error {
	if d == nil || d.db == nil {
		return errors.New("db nil")
	}
	value := strings.TrimSpace(siteName)
	if value == "" {
		value = DefaultSiteName
	}
	now := time.Now().Unix()
	_, err := d.db.Exec(`
		INSERT INTO app_settings(key,value,created_at,updated_at) VALUES('site_name',?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at
	`, value, now, now)
	return err
}

func (d *DB) SearchConcurrency() (int, error) {
	if d == nil || d.db == nil {
		return DefaultSearchConcurrency, errors.New("db nil")
	}
	var value string
	err := d.db.QueryRow(`SELECT value FROM app_settings WHERE key='search_concurrency' LIMIT 1`).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultSearchConcurrency, nil
		}
		return DefaultSearchConcurrency, err
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return DefaultSearchConcurrency, nil
	}
	return parsed, nil
}

func (d *DB) SetSearchConcurrency(value int) error {
	if d == nil || d.db == nil {
		return errors.New("db nil")
	}
	if value <= 0 {
		value = DefaultSearchConcurrency
	}
	now := time.Now().Unix()
	_, err := d.db.Exec(`
		INSERT INTO app_settings(key,value,created_at,updated_at) VALUES('search_concurrency',?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at
	`, strconv.Itoa(value), now, now)
	return err
}
