package mongodb

import (
	"net/url"
	"testing"

	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"

	"github.com/whg517/sqlflow/internal/driver"
)

// TestExtractURIEscapesCredentials pins what a password may contain.
//
// The URI was assembled with fmt.Sprintf and no escaping, so a password holding
// any of the characters a connection string gives meaning to — /, @, :, ? — made
// the driver unparseable. The failure surfaced as "error parsing uri", which
// reads like a configuration typo rather than a password character, and the
// same repository already had a correctly escaping buildMongoURI that nothing
// called.
func TestExtractURIEscapesCredentials(t *testing.T) {
	passwords := []string{
		"p/ssw0rd",
		"p@ssw0rd",
		"pa:ssword",
		"pass?word",
		"pass word",
		"密码",
		"plain",
	}

	for _, password := range passwords {
		t.Run(password, func(t *testing.T) {
			uri := extractURI(&driver.Config{
				Host: "10.0.0.5", Port: 27017,
				Username: "ad/min", Password: password,
				Database: "app",
			})

			if _, err := connstring.ParseAndValidate(uri); err != nil {
				t.Fatalf("mongo cannot parse the URI this driver built: %v", err)
			}

			parsed, err := url.Parse(uri)
			if err != nil {
				t.Fatalf("url.Parse: %v", err)
			}
			gotUser := parsed.User.Username()
			gotPass, _ := parsed.User.Password()
			if gotUser != "ad/min" {
				t.Errorf("username round-tripped as %q, want %q", gotUser, "ad/min")
			}
			if gotPass != password {
				t.Errorf("password round-tripped as %q, want %q", gotPass, password)
			}
			if parsed.Host != "10.0.0.5:27017" {
				t.Errorf("host = %q, want 10.0.0.5:27017", parsed.Host)
			}
			if parsed.Path != "/app" {
				t.Errorf("path = %q, want /app", parsed.Path)
			}
		})
	}
}

// TestExtractURIWithoutCredentials keeps the anonymous case building a URI a
// cluster will accept rather than one with an empty userinfo section.
func TestExtractURIWithoutCredentials(t *testing.T) {
	uri := extractURI(&driver.Config{Host: "10.0.0.5", Port: 27017, Database: "app"})

	if _, err := connstring.ParseAndValidate(uri); err != nil {
		t.Fatalf("mongo cannot parse the credential-free URI: %v — got %q", err, uri)
	}
	if uri != "mongodb://10.0.0.5:27017/app" {
		t.Errorf("uri = %q, want mongodb://10.0.0.5:27017/app", uri)
	}
}
