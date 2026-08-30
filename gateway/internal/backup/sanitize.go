// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"database/sql"
	"strings"
)

var sanitizeStatements = []string{
	`DELETE FROM secrets`,
	`UPDATE connectors SET credential_ref = '' WHERE credential_ref IS NOT NULL AND credential_ref != ''`,
	`UPDATE webhooks SET hmac_enc = '', token_hash = '', token_prefix = ''`,
	`UPDATE webhook_jobs SET lease_token = '' WHERE lease_token IS NOT NULL AND lease_token != ''`,
	`UPDATE channel_pairing SET code_hash = '' WHERE code_hash IS NOT NULL AND code_hash != ''`,
	`UPDATE gateway_settings SET values_json = '{}'`,
}

// Sanitize strips credential material from a snapshot copy. Missing tables are ignored.
func Sanitize(path string) error {
	db, err := openSQLite(path, false)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, stmt := range sanitizeStatements {
		if _, err := db.Exec(stmt); err != nil && !missingTable(err) {
			return err
		}
	}
	return nil
}

func missingTable(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no such table") || strings.Contains(s, "does not exist")
}

func execIgnoreMissing(db *sql.DB, q string, args ...any) error {
	_, err := db.Exec(q, args...)
	if missingTable(err) {
		return nil
	}
	return err
}
