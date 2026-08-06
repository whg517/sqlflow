package mysql

import (
	"strings"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/whg517/sqlflow/internal/driver"
)

// TestBuildDSNAppliesSSLMode pins a setting the API accepted and never applied.
//
// The DSN was assembled with fmt.Sprintf and carried only timeout and
// parseTime; cfg.SSLMode appeared nowhere in this driver. PostgreSQL used it
// and Elasticsearch used it, so a user setting sslmode on a MySQL datasource
// saved a security preference the server would silently decline to honor.
func TestBuildDSNAppliesSSLMode(t *testing.T) {
	tests := []struct {
		sslMode string
		wantTLS string
	}{
		{"", "false"},
		{"disable", "false"},
		{"prefer", "preferred"},
		{"require", "skip-verify"},
		{"verify-ca", "true"},
		{"verify-full", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.sslMode, func(t *testing.T) {
			dsn, err := buildDSN(&driver.Config{
				Host: "db.example.com", Port: 3306,
				Username: "root", Password: "pw", Database: "app",
				SSLMode: tt.sslMode,
			})
			if err != nil {
				t.Fatalf("buildDSN: %v", err)
			}
			cfg, err := gomysql.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("the driver cannot parse its own DSN: %v", err)
			}
			if cfg.TLSConfig != tt.wantTLS {
				t.Errorf("sslmode %q produced tls=%q, want %q", tt.sslMode, cfg.TLSConfig, tt.wantTLS)
			}
		})
	}
}

// TestBuildDSNEscapesCredentials covers the other half of hand-built DSNs.
//
// A password is arbitrary bytes; the DSN grammar gives meaning to @ : / and ?.
// Concatenating one into the string made the driver read a different user, host
// or database than the one configured — or refuse the DSN outright.
func TestBuildDSNEscapesCredentials(t *testing.T) {
	for _, password := range []string{"p@ssw0rd", "pa:ss", "p/w", "pass?w", "密码", "plain"} {
		t.Run(password, func(t *testing.T) {
			dsn, err := buildDSN(&driver.Config{
				Host: "db.example.com", Port: 3306,
				Username: "ro@ot", Password: password, Database: "app",
			})
			if err != nil {
				t.Fatalf("buildDSN: %v", err)
			}
			cfg, err := gomysql.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("the driver cannot parse its own DSN: %v", err)
			}
			if cfg.User != "ro@ot" {
				t.Errorf("user = %q, want ro@ot", cfg.User)
			}
			if cfg.Passwd != password {
				t.Errorf("password = %q, want %q", cfg.Passwd, password)
			}
			if cfg.Addr != "db.example.com:3306" {
				t.Errorf("addr = %q, want db.example.com:3306", cfg.Addr)
			}
			if cfg.DBName != "app" {
				t.Errorf("dbname = %q, want app", cfg.DBName)
			}
		})
	}
}

// TestBuildDSNRejectsUnknownSSLMode keeps a typo from quietly meaning "off".
func TestBuildDSNRejectsUnknownSSLMode(t *testing.T) {
	_, err := buildDSN(&driver.Config{
		Host: "db.example.com", Port: 3306, Username: "root", SSLMode: "verify_full",
	})
	if err == nil {
		t.Fatal("an unrecognized sslmode was accepted")
	}
	if !strings.Contains(err.Error(), "verify_full") {
		t.Errorf("the error does not name the offending value: %v", err)
	}
}
