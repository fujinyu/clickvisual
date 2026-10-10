package utils

import (
	"fmt"
	"strings"
)

// defaultTTLTimeField is the fallback column name used inside TTL expressions
// when the caller does not provide one. Matches the ClickVisual built-in
// log tables that always store `_time_second_`.
const defaultTTLTimeField = "_time_second_"

// BuildTTLClause builds the "TTL ..." clause of a ClickHouse MergeTree CREATE
// statement. It returns "TTL " + BuildTTLExpression(...); callers that need to
// emit an ALTER TABLE ... MODIFY TTL form should call BuildTTLExpression
// directly (the leading keyword is added by the ALTER itself).
//
// Rules identical to BuildTTLExpression; see below.
func BuildTTLClause(timeField, coldVolume string, hotDays, totalDays int) (string, error) {
	expr, err := BuildTTLExpression(timeField, coldVolume, hotDays, totalDays)
	if err != nil {
		return "", err
	}
	return "TTL " + expr, nil
}

// BuildTTLExpression builds the TTL expression (without the leading "TTL "
// keyword) that can be used in both CREATE TABLE ... TTL <expr> and ALTER
// TABLE ... MODIFY TTL <expr> statements.
//
// Rules:
//   - totalDays must be > 0; hotDays must be >= 0.
//   - hotDays == 0 or hotDays >= totalDays  -> single-layer expression (no
//     TO VOLUME), preserving the historical behavior.
//   - 0 < hotDays < totalDays               -> two-layer expression: move to
//     coldVolume after hotDays, delete after totalDays. coldVolume is required
//     in this case.
func BuildTTLExpression(timeField, coldVolume string, hotDays, totalDays int) (string, error) {
	if totalDays <= 0 {
		return "", fmt.Errorf("totalDays must be greater than 0, got %d", totalDays)
	}
	if hotDays < 0 {
		return "", fmt.Errorf("hotDays must not be negative, got %d", hotDays)
	}
	if timeField == "" {
		timeField = defaultTTLTimeField
	}
	if hotDays == 0 || hotDays >= totalDays {
		return fmt.Sprintf("toDateTime(%s) + INTERVAL %d DAY", timeField, totalDays), nil
	}
	if coldVolume == "" {
		return "", fmt.Errorf("coldVolume is required when 0 < hotDays < totalDays")
	}
	return fmt.Sprintf(
		"toDateTime(%s) + INTERVAL %d DAY TO VOLUME '%s', toDateTime(%s) + INTERVAL %d DAY",
		timeField, hotDays, escapeSQLSingleQuote(coldVolume),
		timeField, totalDays,
	), nil
}

// BuildSettingsClause returns the "SETTINGS ..." tail of a CREATE TABLE
// statement. When storagePolicy is empty, it returns the historical default
// (index_granularity only), keeping backward compatibility for tables that
// never opted into hot/cold tiering.
func BuildSettingsClause(storagePolicy string) (string, error) {
	if storagePolicy == "" {
		return "SETTINGS index_granularity = 8192", nil
	}
	return fmt.Sprintf(
		"SETTINGS storage_policy = '%s', index_granularity = 8192",
		escapeSQLSingleQuote(storagePolicy),
	), nil
}

// escapeSQLSingleQuote doubles any single quote inside a ClickHouse identifier
// or string literal so that it is safe to embed within a single-quoted literal.
func escapeSQLSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
