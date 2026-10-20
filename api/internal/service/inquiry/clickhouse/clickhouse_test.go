package clickhouse

import (
	"reflect"
	"strings"
	"testing"

	"github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
	"github.com/clickvisual/clickvisual/api/internal/service/inquiry/factory"
)

func Test_hashTransform(t *testing.T) {
	type args struct {
		query string
		index *db.BaseIndex
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "test-1",
			args: args{
				query: "application='xx-xxx' and url='123'",
				index: &db.BaseIndex{
					Field:   "application",
					HashTyp: 1,
				},
			},
			want: "_inner_siphash_application_=sipHash64('xx-xxx') and url='123'",
		},
		{
			name: "test-2",
			args: args{
				query: "url='123' and application='xx-xxx' and url='123'",
				index: &db.BaseIndex{
					Field:   "application",
					HashTyp: 2,
				},
			},
			want: "url='123' and _inner_urlhash_application_=URLHash('xx-xxx') and url='123'",
		},
		{
			name: "test-3",
			args: args{
				query: "body.tag='123'",
				index: &db.BaseIndex{
					Field:    "tag",
					HashTyp:  2,
					RootName: "body",
				},
			},
			want: "_inner_urlhash_body.tag_=URLHash('123')",
		},
		{
			name: "test-4",
			args: args{
				query: "upstream_proxy_host='fabio-crm-api'",
				index: &db.BaseIndex{
					Field:    "upstream_proxy_host",
					HashTyp:  1,
					RootName: "",
					Alias:    "upstream_proxy_host",
				},
			},
			want: "_inner_siphash_upstream_proxy_host_=sipHash64('fabio-crm-api')",
		},
		{
			name: "test-5",
			args: args{
				query: "1=1 and upstream_proxy_host='fabio-crm-api'",
				index: &db.BaseIndex{
					Field:   "host",
					HashTyp: 1,
				},
			},
			want: "1=1 and upstream_proxy_host='fabio-crm-api'",
		},
		{
			name: "test-6",
			args: args{
				query: "1=1 and upstream_proxy_host='fabio-crm-api'",
				index: &db.BaseIndex{
					Field:   "proxy_host",
					HashTyp: 1,
				},
			},
			want: "1=1 and upstream_proxy_host='fabio-crm-api'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hashTransform(tt.args.query, tt.args.index); got != tt.want {
				t.Errorf("hashTransform() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_adaSelectPart(t *testing.T) {
	type args struct {
		in string
	}
	tests := []struct {
		name    string
		args    args
		wantOut string
	}{
		{
			name: "test-1",
			args: args{
				in: `SELECT count(1)
from mogo_shimo_dev.shimodev_svc_front_tracker
WHERE ("_time_second_" >= toDateTime(NOW() - 3600)) AND ("_time_second_" < toDateTime(NOW()))   
limit 1`,
			},
			wantOut: `SELECT count(1)
,count(1) FROM mogo_shimo_dev.shimodev_svc_front_tracker
WHERE ("_time_second_" >= toDateTime(NOW() - 3600)) AND ("_time_second_" < toDateTime(NOW()))   
limit 1`,
		},
		{
			name: "test-2",
			args: args{
				in: `SELECT count(1),count(1)
from mogo_shimo_dev.shimodev_svc_front_tracker
WHERE ("_time_second_" >= toDateTime(NOW() - 3600)) AND ("_time_second_" < toDateTime(NOW()))   
limit 1`,
			},
			wantOut: `SELECT count(1),count(1)
from mogo_shimo_dev.shimodev_svc_front_tracker
WHERE ("_time_second_" >= toDateTime(NOW() - 3600)) AND ("_time_second_" < toDateTime(NOW()))   
limit 1`,
		},
		{
			name: "test-3",
			args: args{
				in: `SELECT sum(val) as val from (select count(*) as val from mogo_shimo_dev.shimodev_svc_front_tracker
WHERE ("_time_second_" >= toDateTime(NOW() - 3300)) AND ("_time_second_" < toDateTime(NOW()))   
limit 1)`,
			},
			wantOut: `SELECT sum(val) as val from (select count(*) as val from mogo_shimo_dev.shimodev_svc_front_tracker
WHERE ("_time_second_" >= toDateTime(NOW() - 3300)) AND ("_time_second_" < toDateTime(NOW()))   
limit 1)`,
		},
		{
			name: "test-4",
			args: args{
				in: `WITH(
	SELECT 
	       count(*) as p
	from pro_log.app_stdout_svc_front_tracker
	WHERE ("_time_second_" >= toDateTime(NOW() - 604800-600)) AND ("_time_second_" < toDateTime(NOW()-604800)) AND body.error_name like 'ERR_%'
) as p
SELECT 
	  p,
      count(*) as n,
      round((n-p)*100/n,2) as val
from pro_log.app_stdout_svc_front_tracker
WHERE ("_time_second_" >= toDateTime(NOW() -600)) AND ("_time_second_" < toDateTime(NOW())) AND body.error_name like 'ERR_%'`,
			},
			wantOut: `WITH(
	SELECT 
	       count(*) as p
	from pro_log.app_stdout_svc_front_tracker
	WHERE ("_time_second_" >= toDateTime(NOW() - 604800-600)) AND ("_time_second_" < toDateTime(NOW()-604800)) AND body.error_name like 'ERR_%'
) as p
SELECT 
	  p,
      count(*) as n,
      round((n-p)*100/n,2) as val
from pro_log.app_stdout_svc_front_tracker
WHERE ("_time_second_" >= toDateTime(NOW() -600)) AND ("_time_second_" < toDateTime(NOW())) AND body.error_name like 'ERR_%'`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotOut := adaSelectPart(tt.args.in); gotOut != tt.wantOut {
				t.Errorf("adaSelectPart() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func Test_adaSelectPart1(t *testing.T) {
	type args struct {
		in string
	}
	tests := []struct {
		name    string
		args    args
		wantOut string
	}{
		// TODO: Add test cases.
		{
			name: "test-1",
			args: args{
				in: `SELECT  
       round(sum( case when status > 499 then 1 else 0 end ) * 100.0 / count(1), 2) as "访问失败率", 
proxy_upstream_name  as service
from dev_0801.stdout_2
WHERE ("_time_second_" >= toDateTime(NOW() - 3600)) AND ("_time_second_" < toDateTime(NOW()))
group by service
ORDER by "访问失败率" desc    
limit 1`,
			},
			wantOut: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotOut := adaSelectPart(tt.args.in); gotOut != tt.wantOut {
				t.Errorf("adaSelectPart() = %v, want %v", gotOut, tt.wantOut)
			}
		})
	}
}

func Test_queryTransformLike(t *testing.T) {
	type args struct {
		createType  int
		rawLogField string
		query       string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test-1",
			args: args{
				createType:  1,
				rawLogField: "_raw_log_",
				query:       "tt",
			},
			want: "_raw_log_ LIKE '%tt%'",
		},
		{
			name: "test-2",
			args: args{
				createType:  1,
				rawLogField: "_raw_log_",
				query:       "_raw_log_ LIKE '%handleCreated%' AND _container_name_='svc-task'",
			},
			want: "_raw_log_ LIKE '%handleCreated%' AND _container_name_='svc-task'",
		},
		{
			name: "test-3",
			args: args{
				createType:  1,
				rawLogField: "_raw_log_",
				query:       "(`_container_name_`='app-uploader' or `_container_name_`='app-api') AND `_namespace_`='default' AND `code`>'499'",
			},
			want: "(`_container_name_`='app-uploader' or `_container_name_`='app-api') AND `_namespace_`='default' AND `code`>'499'",
		},
		{
			name: "test-5",
			args: args{
				createType:  1,
				rawLogField: "_raw_log_",
				query:       "_raw_log_ like '%handleCreated%' or _container_name_='svc-task'",
			},
			want: "_raw_log_ like '%handleCreated%' or _container_name_='svc-task'",
		},
		{
			name: "test-6",
			args: args{
				createType:  1,
				rawLogField: "_raw_log_",
				query:       "handleCreated and _container_name_='svc-task'",
			},
			want: "_raw_log_ LIKE '%handleCreated%' AND _container_name_='svc-task'",
		},
		{
			name: "test-7",
			args: args{
				createType:  1,
				rawLogField: "_raw_log_",
				query:       "测试 and _container_name_='svc-task'",
			},
			want: "_raw_log_ LIKE '%测试%' AND _container_name_='svc-task'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queryTransformLike(tt.args.createType, tt.args.rawLogField, tt.args.query); got != tt.want {
				t.Errorf("queryTransformLike() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_clickhouseVersionCompare(t *testing.T) {
	type args struct {
		current string
		stand   string
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		// TODO: Add test cases.
		{
			name: "test-1",
			args: args{
				current: "22.11.2.30",
				stand:   "22.3.7",
			},
			want: 1,
		},
		{
			name: "test-2",
			args: args{
				current: "22.11.2.30",
				stand:   "22.13.7",
			},
			want: -1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clickhouseVersionCompare(tt.args.current, tt.args.stand); got != tt.want {
				t.Errorf("clickhouseVersionCompare() = %v, want %v", got, tt.want)
			}
		})
	}
}

// 老采集未上报 time_ns 时的兼容性：float 时间模式必须带“秒×1e9”降级表达式，
// 不能把 _time_nanosecond_ 直接落 0(1970)；string 时间模式不依赖 time_ns。
func Test_timeParseJSONAsString_TimeNsFallback(t *testing.T) {
	c := &ClickHouseX{}
	got := c.timeParseJSONAsString(factory.TableTypeFloat, nil, "time", "", "_log")
	want := "fromUnixTimestamp64Nano(toInt64(if(JSONExtractInt(_log, 'time_ns') > 0, JSONExtractInt(_log, 'time_ns'), toInt64(JSONExtractFloat(_log, 'time'))*1000000000))) AS _time_nanosecond_"
	if !strings.Contains(got, want) {
		t.Fatalf("timeParseJSONAsString() = %s, want contains %s", got, want)
	}
	gotStr := c.timeParseJSONAsString(factory.TableTypeString, nil, "ts", "", "_log")
	if strings.Contains(gotStr, db.TimeNsField) {
		t.Fatalf("string time mode should not reference time_ns, got %s", gotStr)
	}
}

func Test_timeParseSQLV3_TimeNsFallback(t *testing.T) {
	c := &ClickHouseX{}
	got := c.timeParseSQLV3(factory.TableTypeFloat, nil, "time")
	if !strings.Contains(got, "if(JSONExtractFloat(body, 'time_ns') > 0") || !strings.Contains(got, "*1000000000") {
		t.Fatalf("timeParseSQLV3() missing time_ns fallback: %s", got)
	}
}

// planInt64Rebuild 必须按引擎区分处理：MergeTree 目标表走 MODIFY COLUMN 改列类型；
// Kafka 流表不支持 ALTER（会抛 code 48），只能整表 DROP+CREATE 重建，因此不能出现在
// alters 列表里，而应把 rebuildStream 置真。
func Test_planInt64Rebuild(t *testing.T) {
	tests := []struct {
		name        string
		rows        []map[string]interface{}
		wantAlters  []int64ColumnAlter
		wantRebuild bool
	}{
		{
			name: "mergetree target uses modify column with nullable detected",
			rows: []map[string]interface{}{
				{"table": "order_track", "name": "log_id", "type": "Float64", "engine": "MergeTree"},
				{"table": "order_track", "name": "time_ns", "type": "Nullable(Float64)", "engine": "MergeTree"},
			},
			wantAlters: []int64ColumnAlter{
				{table: "order_track", name: "log_id", newType: "Int64"},
				{table: "order_track", name: "time_ns", newType: "Nullable(Int64)"},
			},
			wantRebuild: false,
		},
		{
			name: "kafka stream triggers rebuild and is excluded from alters",
			rows: []map[string]interface{}{
				{"table": "ehub_track_info_stream", "name": "log_id", "type": "Float64", "engine": "Kafka"},
			},
			wantAlters:  nil,
			wantRebuild: true,
		},
		{
			name: "mixed target and stream",
			rows: []map[string]interface{}{
				{"table": "ehub_track_info", "name": "log_id", "type": "Float64", "engine": "MergeTree"},
				{"table": "ehub_track_info_stream", "name": "log_id", "type": "Float64", "engine": "Kafka"},
			},
			wantAlters: []int64ColumnAlter{
				{table: "ehub_track_info", name: "log_id", newType: "Int64"},
			},
			wantRebuild: true,
		},
		{
			name:        "empty rows produce nothing",
			rows:        nil,
			wantAlters:  nil,
			wantRebuild: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAlters, gotRebuild := planInt64Rebuild(tt.rows)
			if !reflect.DeepEqual(gotAlters, tt.wantAlters) {
				t.Errorf("planInt64Rebuild() alters = %v, want %v", gotAlters, tt.wantAlters)
			}
			if gotRebuild != tt.wantRebuild {
				t.Errorf("planInt64Rebuild() rebuildStream = %v, want %v", gotRebuild, tt.wantRebuild)
			}
		})
	}
}
