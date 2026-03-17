package database

import (
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	_ "github.com/denisenkom/go-mssqldb"
)

func EnsureDatabase(baseConnString, dbName string) error {
	normalized := normalizeConnString(baseConnString)
	masterConn := withDatabase(normalized, "master")
	db, err := sql.Open("sqlserver", masterConn)
	if err != nil {
		return wrapLocalDBError(err, baseConnString)
	}
	defer db.Close()

	query := fmt.Sprintf("IF DB_ID('%s') IS NULL CREATE DATABASE [%s];", dbName, dbName)
	_, err = db.Exec(query)
	return wrapLocalDBError(err, baseConnString)
}

func Connect(baseConnString, dbName string) (*sql.DB, error) {
	normalized := normalizeConnString(baseConnString)
	conn := withDatabase(normalized, dbName)
	db, err := sql.Open("sqlserver", conn)
	if err != nil {
		return nil, wrapLocalDBError(err, baseConnString)
	}
	if err := db.Ping(); err != nil {
		return nil, wrapLocalDBError(err, baseConnString)
	}
	return db, nil
}

func EnsureTables(db *sql.DB) error {
	statements := []string{
		`IF OBJECT_ID('dbo.users', 'U') IS NULL
      CREATE TABLE dbo.users (
        id INT IDENTITY PRIMARY KEY,
        email NVARCHAR(255) UNIQUE NOT NULL,
        password_hash NVARCHAR(255) NOT NULL,
        created_at DATETIME DEFAULT GETDATE()
      );`,
		`IF OBJECT_ID('dbo.projects', 'U') IS NULL
      CREATE TABLE dbo.projects (
        id INT IDENTITY PRIMARY KEY,
        user_id INT NOT NULL,
        name NVARCHAR(255) NOT NULL,
        created_at DATETIME DEFAULT GETDATE()
      );`,
		`IF OBJECT_ID('dbo.domains', 'U') IS NULL
      CREATE TABLE dbo.domains (
        id INT IDENTITY PRIMARY KEY,
        project_id INT NOT NULL,
        domain NVARCHAR(255) NOT NULL,
        port INT DEFAULT 443,
        created_at DATETIME DEFAULT GETDATE()
      );`,
		`IF OBJECT_ID('dbo.scan_results', 'U') IS NULL
      CREATE TABLE dbo.scan_results (
        id INT IDENTITY PRIMARY KEY,
        domain_id INT NOT NULL,
        ssl_expiry DATETIME NULL,
        domain_expiry DATETIME NULL,
        tls_version NVARCHAR(50) NULL,
        issuer NVARCHAR(255) NULL,
        issuer_dn NVARCHAR(500) NULL,
        ip_address NVARCHAR(100) NULL,
        status NVARCHAR(50) NULL,
        nameservers NVARCHAR(1000) NULL,
        checked_at DATETIME DEFAULT GETDATE()
      );`,
		`IF OBJECT_ID('dbo.alerts', 'U') IS NULL
      CREATE TABLE dbo.alerts (
        id INT IDENTITY PRIMARY KEY,
        domain_id INT NOT NULL,
        type NVARCHAR(100) NULL,
        severity NVARCHAR(50) NULL,
        message NVARCHAR(500) NULL,
        created_at DATETIME DEFAULT GETDATE(),
        resolved BIT DEFAULT 0
      );`,
		`IF OBJECT_ID('dbo.user_notification_settings', 'U') IS NULL
      CREATE TABLE dbo.user_notification_settings (
        user_id INT NOT NULL PRIMARY KEY,
        email_enabled BIT NOT NULL DEFAULT 1,
        email_recipients NVARCHAR(MAX) NOT NULL DEFAULT '[]',
        notify_days NVARCHAR(MAX) NOT NULL DEFAULT '["30","14","7","3"]',
        schedule_time NVARCHAR(5) NOT NULL DEFAULT '03:00',
        last_scanned_at DATETIME NULL,
        updated_at DATETIME DEFAULT GETDATE()
      );`,
		`IF OBJECT_ID('dbo.logs', 'U') IS NULL
      CREATE TABLE dbo.logs (
        id INT IDENTITY PRIMARY KEY,
        user_id INT NOT NULL,
        domain_id INT NULL,
        level NVARCHAR(20) NOT NULL,
        message NVARCHAR(1000) NOT NULL,
        created_at DATETIME DEFAULT GETDATE()
      );`,
		`IF OBJECT_ID('dbo.fk_projects_users', 'F') IS NULL
      ALTER TABLE dbo.projects
      ADD CONSTRAINT fk_projects_users FOREIGN KEY (user_id) REFERENCES dbo.users(id);
    `,
		`IF OBJECT_ID('dbo.fk_domains_projects', 'F') IS NULL
      ALTER TABLE dbo.domains
      ADD CONSTRAINT fk_domains_projects FOREIGN KEY (project_id) REFERENCES dbo.projects(id);
    `,
		`IF OBJECT_ID('dbo.fk_scanresults_domains', 'F') IS NULL
      ALTER TABLE dbo.scan_results
      ADD CONSTRAINT fk_scanresults_domains FOREIGN KEY (domain_id) REFERENCES dbo.domains(id);
    `,
		`IF COL_LENGTH('dbo.scan_results', 'nameservers') IS NULL
      ALTER TABLE dbo.scan_results
      ADD nameservers NVARCHAR(1000) NULL;
    `,
		`IF COL_LENGTH('dbo.scan_results', 'issuer_dn') IS NULL
      ALTER TABLE dbo.scan_results
      ADD issuer_dn NVARCHAR(500) NULL;
    `,
		`IF OBJECT_ID('dbo.fk_alerts_domains', 'F') IS NULL
      ALTER TABLE dbo.alerts
      ADD CONSTRAINT fk_alerts_domains FOREIGN KEY (domain_id) REFERENCES dbo.domains(id);
    `,
		`IF OBJECT_ID('dbo.fk_notification_users', 'F') IS NULL
      ALTER TABLE dbo.user_notification_settings
      ADD CONSTRAINT fk_notification_users FOREIGN KEY (user_id) REFERENCES dbo.users(id);
    `,
		`IF OBJECT_ID('dbo.fk_logs_users', 'F') IS NULL
      ALTER TABLE dbo.logs
      ADD CONSTRAINT fk_logs_users FOREIGN KEY (user_id) REFERENCES dbo.users(id);
    `,
		`IF OBJECT_ID('dbo.fk_logs_domains', 'F') IS NULL
      ALTER TABLE dbo.logs
      ADD CONSTRAINT fk_logs_domains FOREIGN KEY (domain_id) REFERENCES dbo.domains(id);
    `,
		`IF COL_LENGTH('dbo.user_notification_settings', 'email_enabled') IS NULL
      ALTER TABLE dbo.user_notification_settings
      ADD email_enabled BIT NOT NULL DEFAULT 1;
    `,
		`IF COL_LENGTH('dbo.user_notification_settings', 'email_recipients') IS NULL
      ALTER TABLE dbo.user_notification_settings
      ADD email_recipients NVARCHAR(MAX) NOT NULL DEFAULT '[]';
    `,
		`IF COL_LENGTH('dbo.user_notification_settings', 'notify_days') IS NULL
      ALTER TABLE dbo.user_notification_settings
      ADD notify_days NVARCHAR(MAX) NOT NULL DEFAULT '["30","14","7","3"]';
    `,
		`IF COL_LENGTH('dbo.user_notification_settings', 'schedule_time') IS NULL
      ALTER TABLE dbo.user_notification_settings
      ADD schedule_time NVARCHAR(5) NOT NULL DEFAULT '03:00';
    `,
		`IF COL_LENGTH('dbo.user_notification_settings', 'last_scanned_at') IS NULL
      ALTER TABLE dbo.user_notification_settings
      ADD last_scanned_at DATETIME NULL;
    `,
		`IF COL_LENGTH('dbo.user_notification_settings', 'updated_at') IS NULL
      ALTER TABLE dbo.user_notification_settings
      ADD updated_at DATETIME DEFAULT GETDATE();
    `,
		`IF COL_LENGTH('dbo.logs', 'domain_id') IS NULL
      ALTER TABLE dbo.logs
      ADD domain_id INT NULL;
    `,
		`IF COL_LENGTH('dbo.logs', 'level') IS NULL
      ALTER TABLE dbo.logs
      ADD level NVARCHAR(20) NOT NULL DEFAULT 'INFO';
    `,
		`IF COL_LENGTH('dbo.logs', 'message') IS NULL
      ALTER TABLE dbo.logs
      ADD message NVARCHAR(1000) NOT NULL DEFAULT '';
    `,
		`IF COL_LENGTH('dbo.logs', 'created_at') IS NULL
      ALTER TABLE dbo.logs
      ADD created_at DATETIME DEFAULT GETDATE();
    `,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func EnsureDefaultUser(db *sql.DB) (int, error) {
	var userID int
	err := db.QueryRow("SELECT TOP 1 id FROM users ORDER BY id").Scan(&userID)
	if err == nil && userID > 0 {
		if err := ensureNotificationSettings(db, userID); err != nil {
			return userID, err
		}
		return userID, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	err = db.QueryRow(
		"INSERT INTO users (email, password_hash) OUTPUT INSERTED.id VALUES (@p1, @p2)",
		"local@localhost", "disabled",
	).Scan(&userID)
	if err != nil {
		return 0, err
	}
	if err := ensureNotificationSettings(db, userID); err != nil {
		return userID, err
	}
	return userID, nil
}

func ensureNotificationSettings(db *sql.DB, userID int) error {
	if userID <= 0 {
		return nil
	}
	_, err := db.Exec(
		`IF NOT EXISTS (SELECT 1 FROM dbo.user_notification_settings WHERE user_id=@p1)
		 INSERT INTO dbo.user_notification_settings (user_id, email_enabled, email_recipients, notify_days, schedule_time)
		 VALUES (@p1, 1, '[]', '["30","14","7","3"]', '03:00')`,
		userID,
	)
	return err
}

func withDatabase(connString, dbName string) string {
	parts := strings.Split(connString, ";")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "database=") || strings.HasPrefix(lower, "initial catalog=") {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	cleaned = append(cleaned, "database="+dbName)
	return strings.Join(cleaned, ";")
}

func normalizeConnString(connString string) string {
	parts := strings.Split(connString, ";")
	cleaned := make([]string, 0, len(parts))
	var localDBInstance string

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			cleaned = append(cleaned, trimmed)
			continue
		}

		key := strings.TrimSpace(trimmed[:eq])
		value := strings.TrimSpace(trimmed[eq+1:])
		lower := strings.ToLower(key)

		switch lower {
		case "data source", "server", "addr", "address", "network address":
			key = "server"
			if strings.HasPrefix(strings.ToLower(value), "(localdb)\\") {
				localDBInstance = value[len("(localdb)\\"):]
				value = "localhost\\" + localDBInstance
			} else if strings.HasPrefix(strings.ToLower(value), "localhost\\") {
				localDBInstance = value[len("localhost\\"):]
			} else if strings.HasPrefix(strings.ToLower(value), ".\\") {
				localDBInstance = value[len(".\\"):]
			}
		case "application name":
			key = "app name"
		case "integrated security":
			key = "integrated security"
			if strings.EqualFold(value, "true") || strings.EqualFold(value, "sspi") {
				value = "sspi"
			}
		case "persist security info", "pooling", "multipleactiveresultsets", "command timeout":
			continue
		}

		cleaned = append(cleaned, key+"="+value)
	}

	if localDBInstance != "" {
		if pipe, err := resolveLocalDBPipe(localDBInstance); err == nil && pipe != "" {
			server := pipeToServer(pipe)
			cleaned = replaceKey(cleaned, "server", "server="+server)
		}
	}

	return strings.Join(cleaned, ";")
}

func resolveLocalDBPipe(instance string) (string, error) {
	output, err := exec.Command("sqllocaldb", "info", instance).CombinedOutput()
	if err == nil {
		if pipe, parseErr := parseLocalDBPipe(string(output)); parseErr == nil {
			return pipe, nil
		}
	}

	if runtime.GOOS == "windows" {
		if pipe, regErr := readLocalDBPipeFromRegistry(instance); regErr == nil {
			return pipe, nil
		}
	}

	return "", errors.New("pipe name not found")
}

func pipeToServer(pipe string) string {
	trimmed := strings.TrimSpace(pipe)
	if strings.HasPrefix(strings.ToLower(trimmed), "np:") {
		return trimmed
	}
	if strings.HasPrefix(trimmed, `\\.\pipe\`) {
		return "np:" + trimmed
	}
	if strings.HasPrefix(trimmed, `\\`) {
		return "np:" + trimmed
	}
	return "np:\\\\.\\pipe\\" + trimmed
}

func parseLocalDBPipe(output string) (string, error) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "instance pipe name:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				break
			}
			pipe := strings.TrimSpace(parts[1])
			if pipe == "" {
				return "", errors.New("empty pipe name")
			}
			return pipe, nil
		}
	}
	return "", errors.New("pipe name not found")
}

func readLocalDBPipeFromRegistry(instance string) (string, error) {
	key := `HKCU\SOFTWARE\Microsoft\Microsoft SQL Server\LocalDB\Instances\` + instance
	output, err := exec.Command("reg", "query", key, "/v", "InstancePipeName").CombinedOutput()
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "instancepipename") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				pipe := fields[len(fields)-1]
				if pipe != "" {
					return pipe, nil
				}
			}
		}
	}
	return "", errors.New("pipe name not found")
}

func replaceKey(parts []string, key, replacement string) []string {
	for i, part := range parts {
		lower := strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(lower, key+"=") {
			parts[i] = replacement
			return parts
		}
	}
	return append(parts, replacement)
}

func wrapLocalDBError(err error, connString string) error {
	if err == nil {
		return nil
	}
	lowerConn := strings.ToLower(connString)
	if strings.Contains(lowerConn, "localdb") {
		lowerErr := strings.ToLower(err.Error())
		if strings.Contains(lowerErr, "no instance matching") || strings.Contains(lowerErr, "localdb") {
			return fmt.Errorf("localdb instance not available. Install SQL Server LocalDB or set DB_CONN_STRING to a valid SQL Server instance (e.g. server=localhost\\\\SQLEXPRESS;integrated security=sspi;encrypt=true;trustservercertificate=false;app name=DomainGuard). original error: %w", err)
		}
	}
	return err
}
