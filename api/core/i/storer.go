package i

import (
	"database/sql"
)

type Storer interface {
	// Description read model
	Description() string

	Create() (tables []string, sqls []string, err error)
	Delete() error
	Detach() error
	Attach() error
}

type StorerParams struct {
	// common
	CreateType int

	IsShard   bool    // isShard Does it include shard
	IsReplica bool    // isReplica Does it include replica
	Cluster   string  // cluster name
	Database  string  // database name
	Table     string  // table name
	Conn      *sql.DB // clickhouse

	// storer
	Fields string
	TTL    int // ttl Data expiration time, unit is the day

	// hot/cold tiering (ClickHouse only, all zero/empty values keep the legacy behavior)
	StoragePolicy string // ClickHouse storage_policy name, empty means disabled
	ColdVolume    string // TTL "TO VOLUME" target name, empty means disabled
	HotDays       int    // days data stays on hot volume before moving to cold; 0 or >=TTL means single-layer

	// switcher

}
