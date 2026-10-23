package factory

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/gotomicro/ego/core/econf"

	"github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/dto"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/view"
)

const (
	TableTypeString = 1
	TableTypeFloat  = 2
)

var (
	QueryOperatorArr = []string{"=", "!=", "<", "<=", ">", ">=", "like"}
)

type Operator interface {
	Conn() *sql.DB
	Chart(view.ReqQuery) ([]*view.HighChart, string, error)
	Count(view.ReqQuery) (uint64, error)
	GroupBy(view.ReqQuery) map[string]uint64
	DoSQL(string) (view.RespComplete, error)
	Prepare(view.ReqQuery, *db.BaseTable, bool) (view.ReqQuery, error)
	SyncView(db.BaseTable, *db.BaseView, []*db.BaseView, bool) (string, string, error)

	CreateDatabase(string, string) error
	CreateAlertView(string, string, string) error
	CreateKafkaTable(*db.BaseTable, view.ReqStorageUpdate) (string, error)
	CreateTraceJaegerDependencies(database, cluster, table string, ttl int) (err error)
	CreateTable(int, db.BaseDatabase, view.ReqTableCreate) (string, string, string, string, error)
	CreateStorageJSONAsString(db.BaseDatabase, view.ReqStorageCreate) (string, string, string, string, error)
	CreateStorage(int, db.BaseDatabase, view.ReqStorageCreate) (string, string, string, string, error)
	CreateMetricsSamples(string) error
	CreateBufferNullDataPipe(req db.ReqCreateBufferNullDataPipe) (names []string, sqls []string, err error)

	UpdateLogAnalysisFields(db.BaseDatabase, db.BaseTable, map[string]*db.BaseIndex, map[string]*db.BaseIndex, map[string]*db.BaseIndex) error
	UpdateMergeTreeTable(*db.BaseTable, view.ReqStorageUpdate) error

	GetLogs(view.ReqQuery, int) (view.RespQuery, error)
	GetCreateSQL(database, table string) (string, error)
	GetAlertViewSQL(*db.Alarm, db.BaseTable, int, *view.AlarmFilterItem) (string, string, error)
	GetTraceGraph(ctx context.Context) ([]view.RespJaegerDependencyDataModel, error)
	GetMetricsSamples() error
	ClusterInfo() (clusters map[string]dto.ClusterInfo, err error)

	ListSystemTable() []*view.SystemTables
	ListSystemCluster() ([]*view.SystemClusters, map[string]*view.SystemClusters, error)
	ListDatabase() ([]*view.RespDatabaseSelfBuilt, error)
	ListColumn(string, string, bool) ([]*view.RespColumn, error)
	// ListStoragePolicies returns the ClickHouse storage policies available on
	// the target instance (read from system.storage_policies). Data sources
	// without tiered-storage support return an error so the UI can hide the
	// dropdown instead of silently showing nothing.
	ListStoragePolicies() ([]*view.RespStoragePolicy, error)

	DeleteDatabase(string, string) error
	DeleteAlertView(string, string) error
	DeleteTable(string, string, string, int) error
	DeleteTableListByNames([]string, string) error
	DeleteTraceJaegerDependencies(database, cluster, table string) (err error)
	CalculateInterval(interval int64, timeField string) (string, int64)
}

// StorageRebuilder 可选接口：支持“按正确整数类型重建采集结构”的数据源（当前为 ClickHouse）才实现。
// 处理层通过类型断言判断当前数据源是否支持，不支持时给出提示。
type StorageRebuilder interface {
	// RebuildStorageInt64 将 log_id/time_ns 由 Float64 修正为 Int64（采集元数据+物理列），并重建全部物化视图；返回重建摘要。
	RebuildStorageInt64(tid int) (string, error)
}

// DDLExecutor 可选接口：通过 database/sql Exec 执行 DDL 语句。
// clickhouse-go 的 Query 路径（Operator.DoSQL）在读“成功”的 DDL 响应时会 EOF
// （ClickHouse 24.3 实测：SELECT 正常、DDL 报错能正常返回、DDL 成功必 EOF 且语句
// 实际已执行），因此 DDL 重放/回滚必须走 Exec，不能走 DoSQL。
type DDLExecutor interface {
	// ExecSQL 执行一条 DDL 语句（CREATE/DROP/ALTER 等），不返回结果集。
	ExecSQL(sql string) error
}

// StreamSuspender 可选接口：支持暂停/恢复 Kafka 流表入库的数据源（当前仅 ClickHouse）。
// 暂停 = DETACH 流表，消费者退出且不提交 offset，消息在 topic 堆积，恢复后补灌。
// 处理层通过类型断言判断，未实现的数据源给出“不支持”提示。
type StreamSuspender interface {
	SuspendStream(tableInfo *db.BaseTable) error
	ResumeStream(tableInfo *db.BaseTable) error
}

func TagsToString(alarm *db.Alarm, isMV bool, filterId int) string {
	tags := alarm.Tags
	if alarm.Tags == nil || len(alarm.Tags) == 0 {
		tags = make(map[string]string, 0)
	}
	tags["uuid"] = alarm.Uuid
	tags["alarmId"] = strconv.Itoa(alarm.ID)
	result := make([]string, 0)
	for k, v := range tags {
		result = ResultAppend(result, k, v, isMV)
	}
	if filterId != 0 {
		result = ResultAppend(result, "filterId", strconv.Itoa(filterId), isMV)
	}
	res := strings.Join(result, ",")
	if isMV && econf.GetString("prom2click.tags") != "" {
		res = fmt.Sprintf("%s,%s", res, econf.GetString("prom2click.tags"))
	}
	return res
}

func ResultAppend(input []string, k, v string, withQuote bool) []string {
	if withQuote {
		input = append(input, fmt.Sprintf("'%s=%s'", k, v))
	} else {
		input = append(input, fmt.Sprintf(`%s="%s"`, k, v))
	}
	return input
}

// RemoveEmptyValues 删除日志中的空值字段
func RemoveEmptyValues(logs []map[string]interface{}) {
	for _, logMap := range logs {
		for key, value := range logMap {
			if value == nil || value == "" {
				delete(logMap, key)
			}
		}
	}
}
