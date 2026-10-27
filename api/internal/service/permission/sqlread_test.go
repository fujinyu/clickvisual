package permission

import (
	"strings"
	"testing"
)

func TestExtractSQLTableRefs(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		want    []string
		wantErr string
	}{
		{
			name: "statistical table tab query",
			sql: "SELECT\n*\nFROM\n`prod_fc`.`order_no_track_report_v1`\nWHERE\n" +
				"_time_second_ >= toDateTime(1790480019)\nORDER BY _time_nanosecond_ desc\nLIMIT 50",
			want: []string{"prod_fc.order_no_track_report_v1"},
		},
		{
			name: "join two tables",
			sql:  "SELECT * FROM db.a JOIN db.b ON a.id = b.id",
			want: []string{"db.a", "db.b"},
		},
		{
			name: "subquery in from",
			sql:  "SELECT * FROM (SELECT c FROM db.t1) x",
			want: []string{"db.t1"},
		},
		{
			name: "scalar subquery in select list",
			sql:  "SELECT (SELECT count() FROM db.secret) AS x FROM db.t1",
			want: []string{"db.secret", "db.t1"},
		},
		{
			name: "in subquery",
			sql:  "SELECT * FROM db.t1 WHERE id IN (SELECT id FROM db.secret)",
			want: []string{"db.t1", "db.secret"},
		},
		{
			name: "exists subquery",
			sql:  "SELECT * FROM db.t1 WHERE EXISTS (SELECT 1 FROM db.secret)",
			want: []string{"db.t1", "db.secret"},
		},
		{
			name: "union all",
			sql:  "SELECT * FROM db.a UNION ALL SELECT * FROM db.b",
			want: []string{"db.a", "db.b"},
		},
		{
			name: "cte reference is not a table",
			sql:  "WITH c AS (SELECT * FROM db.t1) SELECT * FROM c",
			want: []string{"db.t1"},
		},
		{
			name: "cte name is case insensitive",
			sql:  "WITH c AS (SELECT * FROM db.t1) SELECT * FROM C",
			want: []string{"db.t1"},
		},
		{
			name: "multiple ctes",
			sql:  "WITH c AS (SELECT * FROM db.t1), d AS (SELECT * FROM db.t2) SELECT * FROM c JOIN d ON 1 = 1",
			want: []string{"db.t1", "db.t2"},
		},
		{
			name: "cte inside a subquery",
			sql:  "SELECT * FROM (WITH c AS (SELECT * FROM db.t1) SELECT * FROM c) z",
			want: []string{"db.t1"},
		},
		{
			name: "same table twice is reported once",
			sql:  "SELECT * FROM db.t1 a JOIN db.t1 b ON a.id = b.id",
			want: []string{"db.t1"},
		},
		{
			name: "table name in a string literal is not a table",
			sql:  "SELECT 'db.fake' AS s FROM db.t1",
			want: []string{"db.t1"},
		},
		{
			name:    "table function",
			sql:     "SELECT * FROM numbers(10)",
			wantErr: "无法鉴权的对象",
		},
		{
			name:    "cluster table function hides its argument",
			sql:     "SELECT * FROM clusterAllReplicas('c', db.t1)",
			wantErr: "无法鉴权的对象",
		},
		{
			name:    "merge table function",
			sql:     "SELECT * FROM merge('db', 't.*')",
			wantErr: "无法鉴权的对象",
		},
		{
			name:    "table function in a join",
			sql:     "SELECT * FROM db.t1 JOIN numbers(3) ON 1 = 1",
			wantErr: "无法鉴权的对象",
		},
		{
			name:    "unqualified table",
			sql:     "SELECT * FROM t1",
			wantErr: "未限定的表名",
		},
		{
			name:    "dotted name inside backticks is a single identifier",
			sql:     "SELECT * FROM `db.tbl`",
			wantErr: "未限定的表名",
		},
		{
			name:    "two statements",
			sql:     "SELECT 1; SELECT * FROM db.t2",
			wantErr: "一条语句",
		},
		{
			name:    "insert is not a select",
			sql:     "INSERT INTO db.t1 VALUES (1)",
			wantErr: "只允许执行 SELECT",
		},
		{
			name:    "describe is not a select",
			sql:     "DESCRIBE TABLE db.t1",
			wantErr: "只允许执行 SELECT",
		},
		{
			name:    "query without any table",
			sql:     "SELECT 1",
			wantErr: "未引用任何数据表",
		},
		{
			name:    "unparsable",
			sql:     "SELEC * FRM",
			wantErr: "无法解析",
		},
		{
			name:    "empty",
			sql:     "",
			wantErr: "一条语句",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			refs, err := ExtractSQLTableRefs(c.sql)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("ExtractSQLTableRefs(%q) = %v, want error containing %q", c.sql, refs, c.wantErr)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Errorf("ExtractSQLTableRefs(%q) error = %q, want it to contain %q", c.sql, err.Error(), c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractSQLTableRefs(%q) unexpected error: %v", c.sql, err)
			}
			got := make([]string, 0, len(refs))
			for _, ref := range refs {
				got = append(got, ref.Database+"."+ref.Table)
			}
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("ExtractSQLTableRefs(%q) = %v, want %v", c.sql, got, c.want)
			}
		})
	}
}
