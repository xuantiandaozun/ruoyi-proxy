// Package database 提供项目数据库发现、连接档案和受控 SQL 执行能力。
package database

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

const profilesFile = "configs/database_profiles.json"
const secretsFile = "configs/database_secrets.json"
const maxRows = 200

var mysqlURLPattern = regexp.MustCompile(`(?i)(?:jdbc:)?mysql://([^:/?#\s"'\\]+)(?::(\d+))?/([^?\s"'\\]+)`)

// Profile 数据库连接档案。密码不会写入档案，运行时从环境变量或来源配置重新读取。
type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ProjectName string `json:"project_name,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	Driver      string `json:"driver"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	PasswordEnv string `json:"password_env,omitempty"`
	SourcePath  string `json:"source_path,omitempty"`
	Environment string `json:"environment,omitempty"`
	Remark      string `json:"remark,omitempty"`
	HasPassword bool   `json:"has_password,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Password    string `json:"-"`
}

// ConnectionInput 手动创建或更新远程数据库连接时使用的输入。
type ConnectionInput struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	ProjectName string `json:"project_name"`
	Environment string `json:"environment,omitempty"`
	Host        string `json:"host"`
	Port        int    `json:"port,omitempty"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	Remark      string `json:"remark,omitempty"`
}

// QueryResult SQL 执行结果。
type QueryResult struct {
	Columns      []string        `json:"columns,omitempty"`
	Rows         [][]interface{} `json:"rows,omitempty"`
	RowsAffected int64           `json:"rows_affected,omitempty"`
	LastInsertID int64           `json:"last_insert_id,omitempty"`
	Truncated    bool            `json:"truncated,omitempty"`
}

// Discover 扫描项目常见配置文件并发现 MySQL 连接。
func Discover(projectPath string) ([]Profile, error) {
	root, err := filepath.Abs(projectPath)
	if err != nil {
		return nil, fmt.Errorf("解析项目路径失败: %v", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("项目路径不可用: %s", root)
	}
	var found []Profile
	seen := map[string]bool{}
	scanned := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if scanned >= 300 || !configFile(entry.Name()) {
			return nil
		}
		stat, statErr := entry.Info()
		if statErr != nil || stat.Size() > 2<<20 {
			return nil
		}
		scanned++
		profiles, parseErr := discoverFile(path, root)
		if parseErr != nil {
			return nil
		}
		for _, profile := range profiles {
			key := strings.ToLower(fmt.Sprintf("%s:%d/%s@%s", profile.Host, profile.Port, profile.Database, profile.Username))
			if !seen[key] {
				seen[key] = true
				found = append(found, profile)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("扫描项目配置失败: %v", err)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

func skipDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".idea", ".vscode", "node_modules", "vendor", "target", "bin", "dist", "build", ".cache":
		return true
	default:
		return false
	}
}

func configFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, ".env") || strings.HasSuffix(lower, ".properties") || strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".toml")
}

func discoverFile(path, projectPath string) ([]Profile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	values := parseKeyValues(text)
	username := firstValue(values, "spring.datasource.username", "datasource.username", "db_username", "db_user", "mysql_user", "username")
	passwordRaw := firstValue(values, "spring.datasource.password", "datasource.password", "db_password", "mysql_password", "password")
	passwordEnv := referencedEnv(passwordRaw)
	password := resolveValue(passwordRaw)
	urls := []string{firstValue(values, "spring.datasource.url", "datasource.url", "database_url", "db_url", "mysql_url")}
	urls = append(urls, mysqlURLPattern.FindAllString(text, -1)...)
	var profiles []Profile
	for _, rawURL := range urls {
		match := mysqlURLPattern.FindStringSubmatch(resolveValue(rawURL))
		if len(match) == 0 {
			continue
		}
		port := 3306
		if match[2] != "" {
			port, _ = strconv.Atoi(match[2])
		}
		profile := newProfile(projectPath, path, match[1], port, strings.TrimSpace(match[3]), resolveValue(username), passwordEnv)
		profile.Password = password
		profiles = append(profiles, profile)
	}
	if len(profiles) == 0 {
		host := firstValue(values, "db_host", "mysql_host", "database_host")
		dbName := firstValue(values, "db_name", "mysql_database", "database_name")
		if host != "" && dbName != "" {
			port, _ := strconv.Atoi(firstValue(values, "db_port", "mysql_port", "database_port"))
			if port == 0 {
				port = 3306
			}
			profile := newProfile(projectPath, path, resolveValue(host), port, resolveValue(dbName), resolveValue(username), passwordEnv)
			profile.Password = password
			profiles = append(profiles, profile)
		}
	}
	return profiles, nil
}

func parseKeyValues(text string) map[string]string {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(text))
	var parents []string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		separator := strings.IndexAny(trimmed, "=:")
		if separator <= 0 {
			continue
		}
		key := strings.Trim(strings.TrimSpace(trimmed[:separator]), `"'`)
		value := strings.Trim(strings.TrimSpace(trimmed[separator+1:]), `"',`)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		level := indent / 2
		if value == "" && strings.Contains(trimmed, ":") {
			if level < len(parents) {
				parents = parents[:level]
			}
			parents = append(parents, strings.ToLower(key))
			continue
		}
		fullKey := strings.ToLower(key)
		if indent > 0 && len(parents) > 0 && !strings.Contains(key, ".") {
			prefix := parents
			if level < len(prefix) {
				prefix = prefix[:level]
			}
			fullKey = strings.Join(append(prefix, strings.ToLower(key)), ".")
		}
		values[fullKey] = value
	}
	return values
}

func firstValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}
func referencedEnv(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		return strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}"), ":", 2)[0]
	}
	return ""
}
func resolveValue(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		parts := strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(value, "${"), "}"), ":", 2)
		if envValue := os.Getenv(parts[0]); envValue != "" {
			return envValue
		}
		if len(parts) == 2 {
			return parts[1]
		}
		return ""
	}
	return value
}

func newProfile(projectPath, sourcePath, host string, port int, dbName, username, passwordEnv string) Profile {
	sum := sha256.Sum256([]byte(strings.ToLower(sourcePath + "|" + host + "|" + dbName + "|" + username)))
	projectName := filepath.Base(projectPath)
	return Profile{ID: hex.EncodeToString(sum[:6]), Name: projectName + "/" + dbName, ProjectName: projectName, ProjectPath: projectPath, Driver: "mysql", Host: host, Port: port, Database: dbName, Username: username, PasswordEnv: passwordEnv, SourcePath: sourcePath}
}

// LoadProfiles 读取已保存连接档案。
func LoadProfiles() ([]Profile, error) {
	raw, err := os.ReadFile(profilesFile)
	if os.IsNotExist(err) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取数据库档案失败: %v", err)
	}
	var profiles []Profile
	if err := json.Unmarshal(raw, &profiles); err != nil {
		return nil, fmt.Errorf("解析数据库档案失败: %v", err)
	}
	for i := range profiles {
		normalizeProfile(&profiles[i])
		if !profiles[i].HasPassword {
			profiles[i].HasPassword = hasStoredSecret(profiles[i].ID)
		}
	}
	return profiles, nil
}

// SaveProfile 新增或更新连接档案。
func SaveProfile(profile Profile) error {
	normalizeProfile(&profile)
	if profile.Password != "" {
		if err := saveSecret(profile.ID, profile.Password); err != nil {
			return err
		}
		profile.HasPassword = true
	}
	profile.Password = ""
	now := time.Now().Format(time.RFC3339)
	if profile.CreatedAt == "" {
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	profiles, err := LoadProfiles()
	if err != nil {
		return err
	}
	replaced := false
	for i := range profiles {
		if profiles[i].ID == profile.ID {
			profiles[i] = profile
			replaced = true
			break
		}
	}
	if !replaced {
		profiles = append(profiles, profile)
	}
	if err := os.MkdirAll(filepath.Dir(profilesFile), 0700); err != nil {
		return fmt.Errorf("创建数据库配置目录失败: %v", err)
	}
	raw, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化数据库档案失败: %v", err)
	}
	if err := os.WriteFile(profilesFile, raw, 0600); err != nil {
		return fmt.Errorf("保存数据库档案失败: %v", err)
	}
	return nil
}

// SaveConnection 保存控制台或 AI 提供的远程 MySQL 连接。
func SaveConnection(input ConnectionInput) (Profile, error) {
	input.ProjectName = strings.TrimSpace(input.ProjectName)
	input.Host = strings.TrimSpace(input.Host)
	input.Database = strings.TrimSpace(input.Database)
	input.Username = strings.TrimSpace(input.Username)
	if input.ProjectName == "" {
		return Profile{}, fmt.Errorf("项目名称不能为空")
	}
	if input.Host == "" || input.Database == "" || input.Username == "" {
		return Profile{}, fmt.Errorf("MySQL 地址、数据库名和用户名不能为空")
	}
	if input.Port == 0 {
		input.Port = 3306
	}
	if input.Port < 1 || input.Port > 65535 {
		return Profile{}, fmt.Errorf("MySQL 端口无效: %d", input.Port)
	}
	id := strings.TrimSpace(input.ID)
	createdAt := ""
	if id != "" {
		if existing, err := GetProfile(id); err == nil {
			createdAt = existing.CreatedAt
		}
	} else {
		sum := sha256.Sum256([]byte(strings.ToLower(input.ProjectName + "|" + input.Environment + "|" + input.Host + "|" + strconv.Itoa(input.Port) + "|" + input.Database + "|" + input.Username)))
		id = hex.EncodeToString(sum[:6])
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = input.ProjectName
		if input.Environment != "" {
			name += "-" + input.Environment
		}
		name += "/" + input.Database
	}
	profile := Profile{ID: id, Name: name, ProjectName: input.ProjectName, Driver: "mysql", Host: input.Host, Port: input.Port, Database: input.Database, Username: input.Username, Environment: strings.TrimSpace(input.Environment), Remark: strings.TrimSpace(input.Remark), Password: input.Password, CreatedAt: createdAt}
	if input.Password == "" {
		profile.HasPassword = hasStoredSecret(id)
	}
	if err := SaveProfile(profile); err != nil {
		return Profile{}, err
	}
	profile.Password = ""
	profile.HasPassword = profile.HasPassword || hasStoredSecret(id)
	return profile, nil
}

func normalizeProfile(profile *Profile) {
	if profile.Driver == "" {
		profile.Driver = "mysql"
	}
	if profile.Port == 0 {
		profile.Port = 3306
	}
	if profile.ProjectName == "" {
		if profile.ProjectPath != "" {
			profile.ProjectName = filepath.Base(profile.ProjectPath)
		}
		if profile.ProjectName == "" && strings.Contains(profile.Name, "/") {
			profile.ProjectName = strings.SplitN(profile.Name, "/", 2)[0]
		}
	}
}

func loadSecrets() (map[string]string, error) {
	raw, err := os.ReadFile(secretsFile)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取数据库密钥失败: %v", err)
	}
	secrets := map[string]string{}
	if err := json.Unmarshal(raw, &secrets); err != nil {
		return nil, fmt.Errorf("解析数据库密钥失败: %v", err)
	}
	return secrets, nil
}

func saveSecret(id, password string) error {
	secrets, err := loadSecrets()
	if err != nil {
		return err
	}
	secrets[id] = password
	if err := os.MkdirAll(filepath.Dir(secretsFile), 0700); err != nil {
		return fmt.Errorf("创建数据库密钥目录失败: %v", err)
	}
	raw, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化数据库密钥失败: %v", err)
	}
	if err := os.WriteFile(secretsFile, raw, 0600); err != nil {
		return fmt.Errorf("保存数据库密钥失败: %v", err)
	}
	if err := os.Chmod(secretsFile, 0600); err != nil {
		return fmt.Errorf("设置数据库密钥权限失败: %v", err)
	}
	return nil
}

func hasStoredSecret(id string) bool {
	secrets, err := loadSecrets()
	if err != nil {
		return false
	}
	return secrets[id] != ""
}

// GetProfile 按 ID 或名称获取连接档案。
func GetProfile(ref string) (Profile, error) {
	profiles, err := LoadProfiles()
	if err != nil {
		return Profile{}, err
	}
	var projectMatches []Profile
	for _, profile := range profiles {
		if profile.ID == ref || strings.EqualFold(profile.Name, ref) {
			return profile, nil
		}
		if strings.EqualFold(profile.ProjectName, ref) {
			projectMatches = append(projectMatches, profile)
		}
	}
	if len(projectMatches) == 1 {
		return projectMatches[0], nil
	}
	if len(projectMatches) > 1 {
		ids := make([]string, 0, len(projectMatches))
		for _, profile := range projectMatches {
			ids = append(ids, profile.ID+"("+profile.Environment+"/"+profile.Database+")")
		}
		return Profile{}, fmt.Errorf("项目 %s 有多个数据库连接，请指定连接 ID: %s", ref, strings.Join(ids, ", "))
	}
	return Profile{}, fmt.Errorf("未找到数据库档案: %s", ref)
}

func resolvePassword(profile Profile) (string, error) {
	if profile.Password != "" {
		return profile.Password, nil
	}
	if profile.PasswordEnv != "" {
		if value := os.Getenv(profile.PasswordEnv); value != "" {
			return value, nil
		}
	}
	if secrets, err := loadSecrets(); err == nil && secrets[profile.ID] != "" {
		return secrets[profile.ID], nil
	}
	if profile.SourcePath != "" {
		candidates, err := discoverFile(profile.SourcePath, profile.ProjectPath)
		if err == nil {
			for _, candidate := range candidates {
				if candidate.Host == profile.Host && candidate.Database == profile.Database && candidate.Username == profile.Username && candidate.Password != "" {
					return candidate.Password, nil
				}
			}
		}
	}
	return "", fmt.Errorf("未找到数据库密码，请设置密码环境变量或检查来源配置")
}

func open(ctx context.Context, profile Profile) (*sql.DB, error) {
	if profile.Driver != "" && profile.Driver != "mysql" {
		return nil, fmt.Errorf("暂不支持数据库驱动: %s", profile.Driver)
	}
	password, err := resolvePassword(profile)
	if err != nil {
		return nil, err
	}
	cfg := mysql.Config{User: profile.Username, Passwd: password, Net: "tcp", Addr: net.JoinHostPort(profile.Host, strconv.Itoa(profile.Port)), DBName: profile.Database, ParseTime: true, Timeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, AllowNativePasswords: true}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("创建数据库连接失败: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接数据库失败: %v", err)
	}
	return db, nil
}

// Test 测试数据库连接。
func Test(ctx context.Context, profile Profile) error {
	db, err := open(ctx, profile)
	if err != nil {
		return err
	}
	return db.Close()
}

// IsReadOnlySQL 判断 SQL 是否属于允许免确认的只读语句。
func IsReadOnlySQL(statement string) bool {
	statement = strings.TrimSpace(stripComments(statement))
	if statement == "" || strings.Contains(strings.TrimSuffix(statement, ";"), ";") {
		return false
	}
	upper := strings.ToUpper(statement)
	for _, dangerous := range []string{" INTO OUTFILE", " INTO DUMPFILE", " FOR UPDATE", " LOCK IN SHARE MODE"} {
		if strings.Contains(upper, dangerous) {
			return false
		}
	}
	first := strings.ToUpper(strings.Fields(statement)[0])
	switch first {
	case "SELECT", "SHOW", "DESCRIBE", "DESC", "EXPLAIN":
		return true
	default:
		return false
	}
}

func stripComments(statement string) string {
	for {
		statement = strings.TrimSpace(statement)
		if strings.HasPrefix(statement, "--") || strings.HasPrefix(statement, "#") {
			if i := strings.IndexByte(statement, '\n'); i >= 0 {
				statement = statement[i+1:]
				continue
			}
		}
		if strings.HasPrefix(statement, "/*") {
			if i := strings.Index(statement, "*/"); i >= 0 {
				statement = statement[i+2:]
				continue
			}
		}
		return statement
	}
}

// Execute 执行 SQL；查询最多返回 200 行。
func Execute(ctx context.Context, profile Profile, statement string) (QueryResult, error) {
	db, err := open(ctx, profile)
	if err != nil {
		return QueryResult{}, err
	}
	defer db.Close()
	if IsReadOnlySQL(statement) {
		rows, err := db.QueryContext(ctx, statement)
		if err != nil {
			return QueryResult{}, fmt.Errorf("执行查询失败: %v", err)
		}
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			return QueryResult{}, fmt.Errorf("读取查询列失败: %v", err)
		}
		result := QueryResult{Columns: columns}
		for rows.Next() {
			if len(result.Rows) >= maxRows {
				result.Truncated = true
				break
			}
			values := make([]interface{}, len(columns))
			pointers := make([]interface{}, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				return QueryResult{}, fmt.Errorf("读取查询结果失败: %v", err)
			}
			for i, value := range values {
				if bytes, ok := value.([]byte); ok {
					values[i] = string(bytes)
				}
			}
			result.Rows = append(result.Rows, values)
		}
		if err := rows.Err(); err != nil {
			return QueryResult{}, fmt.Errorf("遍历查询结果失败: %v", err)
		}
		return result, nil
	}
	execResult, err := db.ExecContext(ctx, statement)
	if err != nil {
		return QueryResult{}, fmt.Errorf("执行 SQL 失败: %v", err)
	}
	affected, _ := execResult.RowsAffected()
	lastID, _ := execResult.LastInsertId()
	return QueryResult{RowsAffected: affected, LastInsertID: lastID}, nil
}

// Schema 返回当前库的表和字段结构。
func Schema(ctx context.Context, profile Profile) (QueryResult, error) {
	return Execute(ctx, profile, `SELECT table_name, column_name, column_type, is_nullable, column_key FROM information_schema.columns WHERE table_schema = DATABASE() ORDER BY table_name, ordinal_position`)
}
