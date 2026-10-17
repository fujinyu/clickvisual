package base

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ego-component/egorm"
	"github.com/gotomicro/ego/core/elog"
	"github.com/pkg/errors"

	"github.com/clickvisual/clickvisual/api/internal/invoker"
	"github.com/clickvisual/clickvisual/api/internal/pkg/component/core"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
	"github.com/clickvisual/clickvisual/api/internal/service"
	"github.com/clickvisual/clickvisual/api/internal/service/event"
	"github.com/clickvisual/clickvisual/api/internal/service/inquiry/factory"
)

type logLibraryCopyRequest struct {
	SourceInstanceId int   `json:"sourceInstanceId"`
	SourceDatabaseId int   `json:"sourceDatabaseId"`
	TargetInstanceId int   `json:"targetInstanceId"`
	TargetDatabaseId int   `json:"targetDatabaseId"`
	TableIds         []int `json:"tableIds"`
}

type logLibraryCopyResult struct {
	Total       int      `json:"total"`
	Copied      int      `json:"copied"`
	Skipped     []string `json:"skipped"`
	Failed      []string `json:"failed"`
	FailedCount int      `json:"failedCount"`
}

// LogLibraryCopy recreates the selected log libraries of the source database
// under the target database. Log libraries whose name already exists in the
// target database are skipped; a failure on one log library does not abort the
// batch, the failed names are reported back so the UI can show them.
func LogLibraryCopy(c *core.Context) {
	var req logLibraryCopyRequest
	if err := c.Bind(&req); err != nil {
		c.JSONE(core.CodeErr, "invalid parameter: "+err.Error(), nil)
		return
	}
	if req.SourceInstanceId == 0 || req.SourceDatabaseId == 0 || req.TargetInstanceId == 0 || req.TargetDatabaseId == 0 {
		c.JSONE(core.CodeErr, "sourceInstanceId, sourceDatabaseId, targetInstanceId and targetDatabaseId are required", nil)
		return
	}
	if len(req.TableIds) == 0 {
		c.JSONE(core.CodeErr, "tableIds is required", nil)
		return
	}
	if req.SourceDatabaseId == req.TargetDatabaseId {
		c.JSONE(core.CodeErr, "source database and target database must be different", nil)
		return
	}
	sourceDatabase, err := db.DatabaseInfo(invoker.Db, req.SourceDatabaseId)
	if err != nil || sourceDatabase.ID == 0 {
		if err == nil {
			err = fmt.Errorf("source database does not exist")
		}
		c.JSONE(core.CodeErr, err.Error(), nil)
		return
	}
	if sourceDatabase.Iid != req.SourceInstanceId {
		c.JSONE(core.CodeErr, "source database does not belong to the source instance", nil)
		return
	}
	targetDatabase, err := db.DatabaseInfo(invoker.Db, req.TargetDatabaseId)
	if err != nil || targetDatabase.ID == 0 {
		if err == nil {
			err = fmt.Errorf("target database does not exist")
		}
		c.JSONE(core.CodeErr, err.Error(), nil)
		return
	}
	if targetDatabase.Iid != req.TargetInstanceId {
		c.JSONE(core.CodeErr, "target database does not belong to the target instance", nil)
		return
	}
	sourceTables, err := db.TableList(invoker.Db, egorm.Conds{"did": req.SourceDatabaseId})
	if err != nil {
		c.JSONE(core.CodeErr, "failed to load source log libraries: "+err.Error(), nil)
		return
	}
	sourceTableMap := make(map[int]*db.BaseTable, len(sourceTables))
	for _, item := range sourceTables {
		sourceTableMap[item.ID] = item
	}
	targetTables, err := db.TableList(invoker.Db, egorm.Conds{"did": req.TargetDatabaseId})
	if err != nil {
		c.JSONE(core.CodeErr, "failed to load target log libraries: "+err.Error(), nil)
		return
	}
	targetTableNames := make(map[string]bool, len(targetTables))
	for _, item := range targetTables {
		targetTableNames[item.Name] = true
	}
	sourceOp, err := service.InstanceManager.Load(req.SourceInstanceId)
	if err != nil {
		c.JSONE(core.CodeErr, "failed to load source instance: "+err.Error(), nil)
		return
	}
	targetOp, err := service.InstanceManager.Load(req.TargetInstanceId)
	if err != nil {
		c.JSONE(core.CodeErr, "failed to load target instance: "+err.Error(), nil)
		return
	}
	// The physical database must exist before the table DDLs can be replayed.
	// The statement is idempotent, so an already existing database is a no-op.
	if err = targetOp.CreateDatabase(targetDatabase.Name, targetDatabase.Cluster); err != nil {
		c.JSONE(core.CodeErr, "failed to prepare target database: "+err.Error(), nil)
		return
	}
	result := logLibraryCopyResult{
		Total:   len(req.TableIds),
		Skipped: make([]string, 0),
		Failed:  make([]string, 0),
	}
	for _, id := range req.TableIds {
		sourceTable, ok := sourceTableMap[id]
		if !ok {
			result.Failed = append(result.Failed, fmt.Sprintf("#%d", id))
			continue
		}
		if targetTableNames[sourceTable.Name] {
			result.Skipped = append(result.Skipped, sourceTable.Name)
			continue
		}
		if err = copyLogLibrary(c.Uid(), sourceTable, sourceDatabase, targetDatabase, sourceOp, targetOp); err != nil {
			elog.Error("copyLogLibrary", elog.String("table", sourceTable.Name), elog.FieldErr(err))
			result.Failed = append(result.Failed, sourceTable.Name)
			continue
		}
		targetTableNames[sourceTable.Name] = true
		result.Copied++
	}
	result.FailedCount = len(result.Failed)
	event.Event.InquiryCMDB(c.User(), db.OpnTablesCopy, map[string]interface{}{
		"sourceInstanceId": req.SourceInstanceId,
		"sourceDatabaseId": req.SourceDatabaseId,
		"targetInstanceId": req.TargetInstanceId,
		"targetDatabaseId": req.TargetDatabaseId,
		"result":           result,
	})
	c.JSONOK(result)
}

// copyLogLibrary recreates one log library under the target database: every
// physical table (local data, Kafka stream, materialized view, distributed) is
// re-created from its current DDL, and the metadata rows (log library, analysis
// fields, attach info) are inserted pointing at the target database.
func copyLogLibrary(uid int, sourceTable *db.BaseTable, sourceDatabase, targetDatabase db.BaseDatabase, sourceOp, targetOp factory.Operator) error {
	refs, err := logLibraryPhysicalTableRefs(*sourceTable, sourceDatabase.Name, sourceTable.Name)
	if err != nil {
		return errors.Wrap(err, "resolve physical tables")
	}
	if len(refs) == 0 {
		return errors.Errorf("no physical table resolved for %s", sourceTable.Name)
	}
	ddlByRole := make(map[string]string, len(refs))
	for _, ref := range refs {
		ddl, ddlErr := sourceOp.GetCreateSQL(ref.Database, ref.Table)
		if ddlErr != nil {
			return errors.Wrapf(ddlErr, "read ddl of %s", ref.Name)
		}
		ddl = rewriteLogLibraryCopyDDL(ddl, sourceDatabase, targetDatabase, sourceTable.Name)
		if _, execErr := targetOp.DoSQL(ddl); execErr != nil {
			return errors.Wrapf(execErr, "create %s", ref.Name)
		}
		ddlByRole[ref.Role] = ddl
	}
	tx := invoker.Db.Begin()
	targetTable := db.BaseTable{
		Did:                     targetDatabase.ID,
		Name:                    sourceTable.Name,
		Desc:                    sourceTable.Desc,
		TimeFieldKind:           sourceTable.TimeFieldKind,
		TimeFieldType:           sourceTable.TimeFieldType,
		CreateType:              sourceTable.CreateType,
		Days:                    sourceTable.Days,
		Topic:                   sourceTable.Topic,
		Brokers:                 sourceTable.Brokers,
		ConsumerNum:             sourceTable.ConsumerNum,
		TimeField:               sourceTable.TimeField,
		RawLogField:             sourceTable.RawLogField,
		KafkaSkipBrokenMessages: sourceTable.KafkaSkipBrokenMessages,
		StoragePolicy:           sourceTable.StoragePolicy,
		ColdVolume:              sourceTable.ColdVolume,
		HotDays:                 sourceTable.HotDays,
		IsKafkaTimestamp:        sourceTable.IsKafkaTimestamp,
		V3TableType:             sourceTable.V3TableType,
		SelectFields:            sourceTable.SelectFields,
		AnyJSON:                 sourceTable.AnyJSON,
		SqlData:                 ddlByRole["data"],
		SqlStream:               ddlByRole["stream"],
		SqlView:                 ddlByRole["view"],
		SqlDistributed:          ddlByRole["distributed"],
		Uid:                     uid,
	}
	if err = db.TableCreate(tx, &targetTable); err != nil {
		tx.Rollback()
		return errors.WithMessage(err, "TableCreateFailed")
	}
	indexes, err := db.IndexList(egorm.Conds{"tid": sourceTable.ID})
	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "load analysis fields")
	}
	for _, index := range indexes {
		if err = db.IndexCreate(tx, &db.BaseIndex{
			Tid:      targetTable.ID,
			Field:    index.Field,
			RootName: index.RootName,
			Typ:      index.Typ,
			HashTyp:  index.HashTyp,
			Alias:    index.Alias,
			Kind:     index.Kind,
		}); err != nil {
			tx.Rollback()
			return errors.WithMessage(err, "IndexCreateFailed")
		}
	}
	attach := db.BaseTableAttach{Tid: sourceTable.ID}
	if err = attach.Info(tx); err == nil {
		newAttach := db.BaseTableAttach{
			Tid:   targetTable.ID,
			Names: rewriteLogLibraryCopyNames(attach.Names, sourceDatabase.Name, targetDatabase.Name),
			SQLs:  rewriteLogLibraryCopySQLs(attach.SQLs, sourceDatabase, targetDatabase, sourceTable.Name),
		}
		if err = newAttach.Create(tx); err != nil {
			tx.Rollback()
			return errors.WithMessage(err, "AttachCreateFailed")
		}
	}
	return tx.Commit().Error
}

// rewriteLogLibraryCopyDDL re-points a CREATE TABLE / CREATE MATERIALIZED VIEW
// statement from the source database (and cluster) to the target ones.
func rewriteLogLibraryCopyDDL(ddl string, sourceDatabase, targetDatabase db.BaseDatabase, table string) string {
	ddl = rewriteLogLibraryCopyDatabase(ddl, sourceDatabase.Name, targetDatabase.Name)
	if sourceDatabase.Cluster != "" && sourceDatabase.Cluster != targetDatabase.Cluster {
		ddl = rewriteLogLibraryCopyCluster(ddl, sourceDatabase.Cluster, targetDatabase.Cluster)
	}
	// The Kafka consumer group defaults to "{database}_{table}"; re-point it at
	// the target database so the copy consumes the topic independently instead
	// of competing with the source table for messages.
	return rewriteLogLibraryCopyKafkaGroup(ddl, targetDatabase.Name, table)
}

func rewriteLogLibraryCopyKafkaGroup(ddl, targetDatabase, table string) string {
	return regexp.MustCompile(`(?i)(kafka_group_name\s*=\s*)'[^']*'`).
		ReplaceAllString(ddl, "${1}'"+targetDatabase+"_"+table+"'")
}

func rewriteLogLibraryCopyDatabase(ddl, sourceDatabase, targetDatabase string) string {
	if sourceDatabase == "" || sourceDatabase == targetDatabase {
		return ddl
	}
	source := regexp.QuoteMeta(sourceDatabase)
	// `srcDb`.`table`
	ddl = regexp.MustCompile("`"+source+"`\\.").ReplaceAllString(ddl, "`"+targetDatabase+"`.")
	// srcDb.table when the name is not part of a longer identifier
	ddl = regexp.MustCompile("(^|[^A-Za-z0-9_$])"+source+"\\.").ReplaceAllString(ddl, "${1}"+targetDatabase+".")
	// Distributed('cluster', 'srcDb', 'table', ...)
	ddl = regexp.MustCompile("(Distributed\\(\\s*'[^']*'\\s*,\\s*')"+source+"'").ReplaceAllString(ddl, "${1}"+targetDatabase+"'")
	return ddl
}

func rewriteLogLibraryCopyCluster(ddl, sourceCluster, targetCluster string) string {
	source := regexp.QuoteMeta(sourceCluster)
	if targetCluster == "" {
		// The target database is not clustered: drop the ON CLUSTER clause.
		return regexp.MustCompile("(?i)[ \\t]*ON\\s+CLUSTER\\s+'?"+source+"'?").ReplaceAllString(ddl, "")
	}
	// ON CLUSTER 'src' / ON CLUSTER src
	ddl = regexp.MustCompile("(?i)(ON\\s+CLUSTER\\s+)'?"+source+"'?").ReplaceAllString(ddl, "${1}'"+targetCluster+"'")
	// Distributed('src', 'db', 'table', ...)
	ddl = regexp.MustCompile("(Distributed\\(\\s*')"+source+"'").ReplaceAllString(ddl, "${1}"+targetCluster+"'")
	return ddl
}

func rewriteLogLibraryCopyNames(names []string, sourceDatabase, targetDatabase string) []string {
	if sourceDatabase == "" || sourceDatabase == targetDatabase || len(names) == 0 {
		return names
	}
	result := make([]string, 0, len(names))
	prefix := sourceDatabase + "."
	for _, name := range names {
		if strings.HasPrefix(name, prefix) {
			result = append(result, targetDatabase+"."+strings.TrimPrefix(name, prefix))
			continue
		}
		// Bare table names resolve against the owning database, which is the
		// target database after the copy, so they stay unchanged.
		result = append(result, name)
	}
	return result
}

func rewriteLogLibraryCopySQLs(sqls []string, sourceDatabase, targetDatabase db.BaseDatabase, table string) []string {
	if len(sqls) == 0 {
		return sqls
	}
	result := make([]string, 0, len(sqls))
	for _, sql := range sqls {
		result = append(result, rewriteLogLibraryCopyDDL(sql, sourceDatabase, targetDatabase, table))
	}
	return result
}
