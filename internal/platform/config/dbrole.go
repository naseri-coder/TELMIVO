package config

import (
	"errors"
	"net/url"
	"strings"
)

// ExpectedDatabaseRole maps the executable role to a PostgreSQL login.
// Worker and tenant credentials must never be placed in the same container.
func ExpectedDatabaseRole(service string) (string, error) {
	switch service {
	case "api", "main-bot", "group-bot", "downloader-bot", "customer-bot-runtime":
		return "telmivo_api", nil
	case "worker", "media-worker":
		return "telmivo_worker", nil
	default:
		return "", errors.New("unsupported database service role")
	}
}

// ValidateDatabaseRoleDSN is a zero-I/O, fail-closed prerequisite, not proof
// of the server's current_user; database.OpenForExpectedRole attests every
// physical PostgreSQL connection independently after TLS negotiation.
func ValidateDatabaseRoleDSN(service, raw, environment string) error {
	expected, err := ExpectedDatabaseRole(service)
	if err != nil {
		return err
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") ||
		u.Opaque != "" || u.Hostname() == "" || u.Port() == "" ||
		u.User == nil || u.User.Username() != expected || u.Fragment != "" ||
		u.ForceQuery || u.RawPath != "" {
		return errors.New("database role split DSN is invalid")
	}
	if password, ok := u.User.Password(); !ok || password == "" {
		return errors.New("database role split DSN requires per-service secret")
	}
	if !strings.HasPrefix(u.Path, "/") || len(u.Path) < 2 {
		return errors.New("database role split DSN requires explicit database")
	}
	values, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return errors.New("database role split DSN parameters invalid")
	}
	for _, key := range []string{
		"host", "hostaddr", "port", "user", "password", "dbname",
		"service", "servicefile", "options", "role", "search_path",
	} {
		if _, present := values[key]; present {
			return errors.New("database role split DSN override forbidden")
		}
	}
	ssl := values["sslmode"]
	if len(ssl) != 1 {
		return errors.New("database role split requires unambiguous sslmode")
	}
	if environment == "production" {
		if ssl[0] != "require" && ssl[0] != "verify-ca" && ssl[0] != "verify-full" {
			return errors.New("database role split TLS required in production")
		}
	} else if ssl[0] != "disable" && ssl[0] != "require" &&
		ssl[0] != "verify-ca" && ssl[0] != "verify-full" {
		return errors.New("database role split sslmode invalid")
	}
	return nil
}
