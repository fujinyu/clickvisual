package base

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/clickvisual/clickvisual/api/internal/pkg/component/core"
	"github.com/clickvisual/clickvisual/api/internal/pkg/constx"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
)

func TestLogLibraryCopyInvalidParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/log-library-management/databases/copy", core.Handle(LogLibraryCopy))
	req := httptest.NewRequest("POST", "/log-library-management/databases/copy", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != 200 {
		t.Fatalf("expected HTTP 200, got %d", resp.Code)
	}
	if got := resp.Body.String(); !strings.Contains(got, `"code":1`) {
		t.Fatalf("expected business error code, got %q", got)
	}
}

func TestRewriteLogLibraryCopyDDLRewritesDatabaseReferences(t *testing.T) {
	ddl := "CREATE TABLE IF NOT EXISTS `cv_logs`.`test_local` ON CLUSTER 'logs'\n" +
		"(\n`_time_second_` DateTime\n)\n" +
		"ENGINE = ReplicatedMergeTree('/clickhouse/tables/test_local/{shard}', '{replica}')\n" +
		"ORDER BY _time_second_"
	got := rewriteLogLibraryCopyDDL(ddl, db.BaseDatabase{Name: "cv_logs", Cluster: "logs"}, db.BaseDatabase{Name: "cv_logs_bak", Cluster: "logs"}, "test_local")
	if !strings.Contains(got, "`cv_logs_bak`.`test_local`") {
		t.Fatalf("expected the table to be re-pointed at the target database, got %q", got)
	}
	if !strings.Contains(got, "ON CLUSTER 'logs'") {
		t.Fatalf("expected the cluster clause to be kept for the same cluster, got %q", got)
	}
	// The ReplicatedMergeTree path is keyed by the table name only and must not
	// be rewritten.
	if !strings.Contains(got, "'/clickhouse/tables/test_local/{shard}'") {
		t.Fatalf("expected the replica path to stay unchanged, got %q", got)
	}
}

func TestRewriteLogLibraryCopyDDLRewritesMaterializedViewAndDistributed(t *testing.T) {
	ddl := "CREATE MATERIALIZED VIEW `cv_logs`.`test_view` TO `cv_logs`.`test_local`\n" +
		"AS SELECT _raw_log_ FROM `cv_logs`.`test_stream`"
	got := rewriteLogLibraryCopyDDL(ddl, db.BaseDatabase{Name: "cv_logs"}, db.BaseDatabase{Name: "cv_logs_bak"}, "test_view")
	if !strings.Contains(got, "TO `cv_logs_bak`.`test_local`") || !strings.Contains(got, "FROM `cv_logs_bak`.`test_stream`") {
		t.Fatalf("expected all references to be rewritten, got %q", got)
	}

	distributed := "CREATE TABLE cv_logs.test ON CLUSTER logs AS cv_logs.test_local ENGINE = Distributed('logs', 'cv_logs', 'test_local', rand())"
	got = rewriteLogLibraryCopyDDL(distributed, db.BaseDatabase{Name: "cv_logs", Cluster: "logs"}, db.BaseDatabase{Name: "cv_logs_bak", Cluster: "logs_bak"}, "test")
	if !strings.Contains(got, "Distributed('logs_bak', 'cv_logs_bak', 'test_local', rand())") {
		t.Fatalf("expected the distributed engine to be rewritten, got %q", got)
	}
	if !strings.Contains(got, "ON CLUSTER 'logs_bak'") {
		t.Fatalf("expected the cluster to be rewritten, got %q", got)
	}
}

func TestRewriteLogLibraryCopyDDLDropsClusterClauseForStandaloneTarget(t *testing.T) {
	ddl := "CREATE TABLE cv_logs.test ON CLUSTER 'logs' (`x` String) ENGINE = MergeTree ORDER BY x"
	got := rewriteLogLibraryCopyDDL(ddl, db.BaseDatabase{Name: "cv_logs", Cluster: "logs"}, db.BaseDatabase{Name: "cv_logs_bak"}, "test")
	if strings.Contains(got, "ON CLUSTER") {
		t.Fatalf("expected the ON CLUSTER clause to be dropped, got %q", got)
	}
	if !strings.Contains(got, "CREATE TABLE IF NOT EXISTS cv_logs_bak.test (`x` String)") {
		t.Fatalf("expected the database to be rewritten and the ddl to be idempotent, got %q", got)
	}
}

func TestRewriteLogLibraryCopyDDLRewritesKafkaConsumerGroup(t *testing.T) {
	ddl := "CREATE TABLE cv_logs.test_stream (`_raw_log_` String)\n" +
		"ENGINE = Kafka SETTINGS kafka_broker_list = 'kafka:9092', kafka_topic_list = 'topic', kafka_group_name = 'cv_logs_test', kafka_format = 'JSONEachRow'"
	got := rewriteLogLibraryCopyDDL(ddl, db.BaseDatabase{Name: "cv_logs"}, db.BaseDatabase{Name: "cv_logs_bak"}, "test")
	if !strings.Contains(got, "kafka_group_name = 'cv_logs_bak_test'") {
		t.Fatalf("expected the kafka group to be re-pointed at the target database, got %q", got)
	}
	if !strings.Contains(got, "kafka_topic_list = 'topic'") {
		t.Fatalf("expected the kafka topic to stay unchanged, got %q", got)
	}
}

func TestRewriteLogLibraryCopyDDLIsIdempotent(t *testing.T) {
	// SHOW CREATE TABLE never emits IF NOT EXISTS, but the datasource retries
	// a statement once when the response read fails, so the replayed DDL must
	// not fail the retry with "table already exists".
	cases := []struct {
		name     string
		ddl      string
		expected string
	}{
		{
			name:     "create table",
			ddl:      "CREATE TABLE cv_logs.test (`x` String) ENGINE = MergeTree ORDER BY x",
			expected: "CREATE TABLE IF NOT EXISTS cv_logs.test (`x` String) ENGINE = MergeTree ORDER BY x",
		},
		{
			name:     "create materialized view",
			ddl:      "CREATE MATERIALIZED VIEW `cv_logs`.`test_view` TO `cv_logs`.`test_local` AS SELECT 1",
			expected: "CREATE MATERIALIZED VIEW IF NOT EXISTS `cv_logs`.`test_view` TO `cv_logs`.`test_local` AS SELECT 1",
		},
		{
			name:     "already idempotent create table",
			ddl:      "CREATE TABLE IF NOT EXISTS cv_logs.test (`x` String) ENGINE = MergeTree ORDER BY x",
			expected: "CREATE TABLE IF NOT EXISTS cv_logs.test (`x` String) ENGINE = MergeTree ORDER BY x",
		},
		{
			name:     "already idempotent create view",
			ddl:      "CREATE MATERIALIZED VIEW  IF NOT EXISTS `cv_logs`.`test_view` TO `cv_logs`.`test_local` AS SELECT 1",
			expected: "CREATE MATERIALIZED VIEW  IF NOT EXISTS `cv_logs`.`test_view` TO `cv_logs`.`test_local` AS SELECT 1",
		},
		{
			name:     "lower case keywords",
			ddl:      "create table cv_logs.test (`x` String) ENGINE = MergeTree ORDER BY x",
			expected: "create table IF NOT EXISTS cv_logs.test (`x` String) ENGINE = MergeTree ORDER BY x",
		},
		{
			name:     "not a create statement",
			ddl:      "ALTER TABLE cv_logs.test ADD COLUMN y String",
			expected: "ALTER TABLE cv_logs.test ADD COLUMN y String",
		},
	}
	for _, c := range cases {
		if got := makeDDLIdempotent(c.ddl); got != c.expected {
			t.Fatalf("%s: makeDDLIdempotent() = %q, want %q", c.name, got, c.expected)
		}
	}
}

func TestRewriteLogLibraryCopyDatabaseKeepsLongerIdentifiers(t *testing.T) {
	ddl := "CREATE TABLE cv_logs.test (`cv_logs_tag` String) ENGINE = MergeTree ORDER BY _time_second_"
	got := rewriteLogLibraryCopyDatabase(ddl, "cv_logs", "cv_logs_bak")
	if !strings.Contains(got, "`cv_logs_tag`") {
		t.Fatalf("expected the longer column name to stay unchanged, got %q", got)
	}
	if !strings.Contains(got, "CREATE TABLE cv_logs_bak.test") {
		t.Fatalf("expected the table reference to be rewritten, got %q", got)
	}
}

func TestRewriteLogLibraryCopyNames(t *testing.T) {
	got := rewriteLogLibraryCopyNames([]string{"cv_logs.test_local", "test_view"}, "cv_logs", "cv_logs_bak")
	if len(got) != 2 || got[0] != "cv_logs_bak.test_local" || got[1] != "test_view" {
		t.Fatalf("unexpected rewritten names: %#v", got)
	}
}

func TestLogLibraryIsKafkaPipeline(t *testing.T) {
	pipeline := []int{constx.TableCreateTypeCV, constx.TableCreateTypeJSONEachRow, constx.TableCreateTypeUBW, constx.TableCreateTypeJSONAsString}
	for _, createType := range pipeline {
		if !logLibraryIsKafkaPipeline(&db.BaseTable{CreateType: createType}) {
			t.Fatalf("expected create type %d to be a kafka pipeline table", createType)
		}
	}
	for _, createType := range []int{constx.TableCreateTypeExist, constx.TableCreateTypeBufferNullDataPipe, constx.TableCreateTypeTraceCalculation} {
		if logLibraryIsKafkaPipeline(&db.BaseTable{CreateType: createType}) {
			t.Fatalf("expected create type %d not to be a kafka pipeline table", createType)
		}
	}
}

func TestLogLibraryMetadataNeedsInt64Fix(t *testing.T) {
	cases := []struct {
		name     string
		anyJSON  string
		expected bool
	}{
		{
			name:     "legacy float64 log_id",
			anyJSON:  `{"sourceMapping":{"data":[{"key":"log_id","value":"Float64"},{"key":"msg","value":"String"}]}}`,
			expected: true,
		},
		{
			name:     "legacy nullable float64 time_ns",
			anyJSON:  `{"sourceMapping":{"data":[{"key":"time_ns","value":"Nullable(Float64)"}]}}`,
			expected: true,
		},
		{
			name:     "already fixed to int64",
			anyJSON:  `{"sourceMapping":{"data":[{"key":"log_id","value":"Int64"},{"key":"time_ns","value":"Int64"}]}}`,
			expected: false,
		},
		{
			name:     "pipeline without log fields",
			anyJSON:  `{"sourceMapping":{"data":[{"key":"msg","value":"String"}]}}`,
			expected: false,
		},
		{
			name:     "empty metadata",
			anyJSON:  "",
			expected: false,
		},
	}
	for _, c := range cases {
		got := logLibraryMetadataNeedsInt64Fix(&db.BaseTable{AnyJSON: c.anyJSON})
		if got != c.expected {
			t.Fatalf("%s: logLibraryMetadataNeedsInt64Fix() = %v, want %v", c.name, got, c.expected)
		}
	}
}
