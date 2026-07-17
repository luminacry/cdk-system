package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: migrate-sqlite-to-postgres <sqlite.db> <postgres-url>")
		os.Exit(1)
	}

	sqlitePath := os.Args[1]
	postgresURL := os.Args[2]

	ctx := context.Background()

	sqliteDB, err := sql.Open("sqlite3", sqlitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open sqlite: %v\n", err)
		os.Exit(1)
	}
	defer sqliteDB.Close()

	pool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect postgres: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	sqliteTx, err := sqliteDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "begin sqlite snapshot: %v\n", err)
		os.Exit(1)
	}
	defer sqliteTx.Rollback()

	postgresTx, err := pool.Begin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "begin postgres transaction: %v\n", err)
		os.Exit(1)
	}
	defer postgresTx.Rollback(ctx)

	if err := migrateBatches(ctx, sqliteTx, postgresTx); err != nil {
		fmt.Fprintf(os.Stderr, "migrate batches: %v\n", err)
		os.Exit(1)
	}
	if err := migrateCodes(ctx, sqliteTx, postgresTx); err != nil {
		fmt.Fprintf(os.Stderr, "migrate codes: %v\n", err)
		os.Exit(1)
	}
	if err := migrateFAQ(ctx, sqliteTx, postgresTx); err != nil {
		fmt.Fprintf(os.Stderr, "migrate faq: %v\n", err)
		os.Exit(1)
	}
	if err := migrateRedemptions(ctx, sqliteTx, postgresTx); err != nil {
		fmt.Fprintf(os.Stderr, "migrate redemptions: %v\n", err)
		os.Exit(1)
	}
	if err := backfillUserBatchUsage(ctx, postgresTx); err != nil {
		fmt.Fprintf(os.Stderr, "backfill user usage: %v\n", err)
		os.Exit(1)
	}
	if err := resetImportedSequences(ctx, postgresTx); err != nil {
		fmt.Fprintf(os.Stderr, "reset sequences: %v\n", err)
		os.Exit(1)
	}
	if err := postgresTx.Commit(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "commit postgres transaction: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("migration completed successfully")
}

func migrateBatches(ctx context.Context, sqliteTx *sql.Tx, postgresTx pgx.Tx) error {
	rows, err := sqliteTx.QueryContext(ctx, `SELECT id, name, description, payload_json, prefix, code_length, expires_at,
		max_uses_per_code, max_redeems_per_user, webhook_url, webhook_secret, status, created_at FROM batches`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var name, description, payloadJSON, prefix, webhookURL, webhookSecret, status string
		var codeLength, maxUses, maxRedeems int
		var expiresAt, createdAt sql.NullString
		if err := rows.Scan(&id, &name, &description, &payloadJSON, &prefix, &codeLength, &expiresAt,
			&maxUses, &maxRedeems, &webhookURL, &webhookSecret, &status, &createdAt); err != nil {
			return err
		}

		_, err := postgresTx.Exec(ctx, `
			INSERT INTO batches (id, name, description, payload_json, prefix, code_length, expires_at,
				max_uses_per_code, max_redeems_per_user, webhook_url, webhook_secret, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
			ON CONFLICT (id) DO UPDATE SET
				name = EXCLUDED.name, description = EXCLUDED.description, payload_json = EXCLUDED.payload_json,
				prefix = EXCLUDED.prefix, code_length = EXCLUDED.code_length, expires_at = EXCLUDED.expires_at,
				max_uses_per_code = EXCLUDED.max_uses_per_code,
				max_redeems_per_user = EXCLUDED.max_redeems_per_user,
				webhook_url = EXCLUDED.webhook_url, webhook_secret = EXCLUDED.webhook_secret,
				status = EXCLUDED.status, created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at`,
			id, name, description, normalizeJSON(payloadJSON), strings.ToUpper(prefix), codeLength,
			parseTimePG(expiresAt.String), maxUses, maxRedeems, webhookURL, webhookSecret, status,
			parseTime(createdAt.String),
		)
		if err != nil {
			return fmt.Errorf("insert batch %d: %w", id, err)
		}
	}
	return rows.Err()
}

func migrateCodes(ctx context.Context, sqliteTx *sql.Tx, postgresTx pgx.Tx) error {
	rows, err := sqliteTx.QueryContext(ctx, `SELECT id, batch_id, code, status, use_count, created_at FROM codes`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, batchID int64
		var code, status string
		var useCount int
		var createdAt string
		if err := rows.Scan(&id, &batchID, &code, &status, &useCount, &createdAt); err != nil {
			return err
		}

		_, err := postgresTx.Exec(ctx, `
			INSERT INTO codes (id, batch_id, code, status, use_count, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO UPDATE SET batch_id = EXCLUDED.batch_id, code = EXCLUDED.code,
				status = EXCLUDED.status, use_count = EXCLUDED.use_count, created_at = EXCLUDED.created_at`,
			id, batchID, strings.ToUpper(code), status, useCount, parseTime(createdAt),
		)
		if err != nil {
			return fmt.Errorf("insert code %d: %w", id, err)
		}
	}
	return rows.Err()
}

func migrateFAQ(ctx context.Context, sqliteTx *sql.Tx, postgresTx pgx.Tx) error {
	rows, err := sqliteTx.QueryContext(ctx, `SELECT id, sort_order, title, content, created_at FROM faq_items`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var sortOrder int
		var title, content string
		var createdAt string
		if err := rows.Scan(&id, &sortOrder, &title, &content, &createdAt); err != nil {
			return err
		}

		_, err := postgresTx.Exec(ctx, `
			INSERT INTO faq_items (id, sort_order, title, content, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5)
			ON CONFLICT (id) DO UPDATE SET sort_order = EXCLUDED.sort_order, title = EXCLUDED.title,
				content = EXCLUDED.content, created_at = EXCLUDED.created_at, updated_at = EXCLUDED.updated_at`,
			id, sortOrder, title, content, parseTime(createdAt),
		)
		if err != nil {
			return fmt.Errorf("insert faq %d: %w", id, err)
		}
	}
	return rows.Err()
}

func migrateRedemptions(ctx context.Context, sqliteTx *sql.Tx, postgresTx pgx.Tx) error {
	rows, err := sqliteTx.QueryContext(ctx, `SELECT id, code_id, batch_id, code_text, user_id, payload_snapshot,
		result, message, webhook_status, webhook_response, webhook_attempt, ip, created_at FROM redemptions`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var codeID, batchID sql.NullInt64
		var codeText, userID, payloadSnapshot, result, message, webhookStatus, webhookResponse, ip, createdAt string
		var webhookAttempt int
		if err := rows.Scan(&id, &codeID, &batchID, &codeText, &userID, &payloadSnapshot,
			&result, &message, &webhookStatus, &webhookResponse, &webhookAttempt, &ip, &createdAt); err != nil {
			return err
		}

		userKey := normalizeUserKey(userID)
		_, err := postgresTx.Exec(ctx, `
			INSERT INTO redemptions (id, code_id, batch_id, code_text, user_id, user_key, payload_snapshot,
				result, message, webhook_status, webhook_response, webhook_attempt, ip, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (id) DO UPDATE SET
				code_id = EXCLUDED.code_id, batch_id = EXCLUDED.batch_id, code_text = EXCLUDED.code_text,
				user_id = EXCLUDED.user_id, user_key = EXCLUDED.user_key,
				payload_snapshot = EXCLUDED.payload_snapshot, result = EXCLUDED.result,
				message = EXCLUDED.message, webhook_status = EXCLUDED.webhook_status,
				webhook_response = EXCLUDED.webhook_response, webhook_attempt = EXCLUDED.webhook_attempt,
				ip = EXCLUDED.ip, created_at = EXCLUDED.created_at`,
			id, nullableInt64(codeID), nullableInt64(batchID), strings.ToUpper(codeText), userID, userKey,
			normalizeJSON(payloadSnapshot), result, message, webhookStatus, webhookResponse, webhookAttempt,
			normalizeIP(ip), parseTime(createdAt),
		)
		if err != nil {
			return fmt.Errorf("insert redemption %d: %w", id, err)
		}
	}
	return rows.Err()
}

func backfillUserBatchUsage(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO user_batch_usage (batch_id, user_key, used_count)
		SELECT batch_id, user_key, COUNT(*)::integer
		FROM redemptions
		WHERE result = 'success' AND batch_id IS NOT NULL
		GROUP BY batch_id, user_key
		ON CONFLICT (batch_id, user_key) DO UPDATE
		SET used_count = EXCLUDED.used_count`)
	return err
}

func resetImportedSequences(ctx context.Context, tx pgx.Tx) error {
	for _, table := range []string{"batches", "codes", "faq_items", "redemptions"} {
		query := fmt.Sprintf(
			"SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM %s",
			table, table,
		)
		if _, err := tx.Exec(ctx, query); err != nil {
			return fmt.Errorf("reset %s sequence: %w", table, err)
		}
	}
	return nil
}

func normalizeJSON(raw string) json.RawMessage {
	value := []byte(strings.TrimSpace(raw))
	if len(value) == 0 || !json.Valid(value) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(value)
}

func normalizeIP(raw string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return "0.0.0.0"
	}
	return addr.String()
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

func parseTimePG(s string) pgtype.Timestamptz {
	if s == "" {
		return pgtype.Timestamptz{Valid: false}
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

func nullableInt64(ns sql.NullInt64) *int64 {
	if ns.Valid {
		return &ns.Int64
	}
	return nil
}

func normalizeUserKey(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ToLower(s)
	return strings.TrimFunc(s, func(r rune) bool {
		return !unicode.IsPrint(r) || unicode.IsSpace(r)
	})
}
