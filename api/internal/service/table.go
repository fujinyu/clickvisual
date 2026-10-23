package service

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/clickvisual/clickvisual/api/internal/invoker"
	"github.com/clickvisual/clickvisual/api/internal/pkg/constx"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/view"
	"github.com/clickvisual/clickvisual/api/internal/pkg/utils/mapping"
	"github.com/clickvisual/clickvisual/api/internal/service/inquiry/factory"
	"github.com/clickvisual/clickvisual/api/internal/service/permission/pmsplugin"
)

func TableViewIsPermission(uid, iid, did, tid int) bool {
	// in-memory summary, no db enumeration(see pmsplugin.GetUserViewSummary)
	return pmsplugin.GetUserViewSummary(uid).TableSubVisible(iid, did, tid)
}

func tableViewIsPermission(uid, iid, did, tid int, subResource string) bool {
	return pmsplugin.GetUserViewSummary(uid).TableSubResourceVisible(iid, did, tid, subResource)
}

// ErrTableSuspended is returned by the mutation paths which cannot run against a
// log store whose stream table is detached: DROP TABLE IF EXISTS would silently
// skip it and leave the metadata behind on disk.
var ErrTableSuspended = errors.New("ingestion of this log store is paused: resume it first")

// TableSuspendGuard rejects such a mutation. Callers pass tableInfo.SuspendStatus.
func TableSuspendGuard(suspendStatus int) error {
	if suspendStatus == db.SuspendStatusDetached {
		return ErrTableSuspended
	}
	return nil
}

// TableSuspend pauses ingestion by detaching the Kafka stream table of the log
// store. Nothing consumes the topic while it is detached, and the offsets are
// not committed, so the messages pile up and TableResume re-reads them.
func TableSuspend(tid int) error {
	tableInfo, err := loadSuspendableTable(tid)
	if err != nil {
		return err
	}
	suspender, err := streamSuspenderOf(tableInfo)
	if err != nil {
		return err
	}
	if err = suspender.SuspendStream(tableInfo); err != nil {
		return err
	}
	return db.TableUpdate(invoker.Db, tid, map[string]interface{}{
		"suspend_status": db.SuspendStatusDetached,
	})
}

// TableResume re-attaches the stream table, which resumes the consumer group
// "<database>_<table>" from its committed offset.
func TableResume(tid int) error {
	tableInfo, err := loadSuspendableTable(tid)
	if err != nil {
		return err
	}
	suspender, err := streamSuspenderOf(tableInfo)
	if err != nil {
		return err
	}
	if err = suspender.ResumeStream(tableInfo); err != nil {
		return err
	}
	return db.TableUpdate(invoker.Db, tid, map[string]interface{}{
		"suspend_status": db.SuspendStatusAttached,
	})
}

// loadSuspendableTable rejects the log stores which have no Kafka stream table
// to detach: self-built, trace calculation and buffer-null tables.
func loadSuspendableTable(tid int) (*db.BaseTable, error) {
	tableInfo, err := db.TableInfo(invoker.Db, tid)
	if err != nil {
		return nil, err
	}
	if tableInfo.ID == 0 {
		return nil, errors.Errorf("table not found: %d", tid)
	}
	if !constx.IsKafkaPipelineTable(tableInfo.CreateType) {
		return nil, errors.New("only log stores fed by a kafka stream table can be suspended")
	}
	return &tableInfo, nil
}

func streamSuspenderOf(tableInfo *db.BaseTable) (factory.StreamSuspender, error) {
	op, err := InstanceManager.Load(tableInfo.Database.Iid)
	if err != nil {
		return nil, err
	}
	suspender, ok := op.(factory.StreamSuspender)
	if !ok {
		return nil, errors.New("current datasource does not support suspending ingestion")
	}
	return suspender, nil
}

func IsCheckInner(createType int) bool {
	return createType == constx.TableCreateTypeJSONAsString
}

func StorageCreate(uid int, databaseInfo db.BaseDatabase, param view.ReqStorageCreate) (tableInfo db.BaseTable, err error) {
	param.SourceMapping, err = mapping.Handle(param.Source, IsCheckInner(param.CreateType))
	if err != nil {
		return
	}
	if err = json.Unmarshal([]byte(param.Source), &param.SourceMapping); err != nil {
		return
	}
	op, err := InstanceManager.Load(databaseInfo.Iid)
	if err != nil {
		return
	}
	var (
		s string
		d string
		v string
		a string
	)
	if param.CreateType == constx.TableCreateTypeJSONAsString {
		s, d, v, a, err = op.CreateStorageJSONAsString(databaseInfo, param)
	} else {
		s, d, v, a, err = op.CreateStorage(databaseInfo.ID, databaseInfo, param)
	}
	if err != nil {
		err = errors.Wrap(err, "storage create failed")
		return
	}
	tableInfo = db.BaseTable{
		Did:                     databaseInfo.ID,
		Name:                    param.TableName,
		TimeFieldKind:           param.Typ,
		Days:                    param.Days,
		Brokers:                 param.Brokers,
		Topic:                   param.Topics,
		Desc:                    param.Desc,
		ConsumerNum:             param.Consumers,
		SqlData:                 d,
		SqlStream:               s,
		SqlView:                 v,
		SqlDistributed:          a,
		CreateType:              param.CreateType,
		Uid:                     uid,
		RawLogField:             param.RawLogField,
		TimeField:               db.TimeFieldSecond,
		SelectFields:            param.SelectFields(),
		AnyJSON:                 param.JSON(),
		KafkaSkipBrokenMessages: param.KafkaSkipBrokenMessages,
		StoragePolicy:           param.StoragePolicy,
		ColdVolume:              param.ColdVolume,
		HotDays:                 param.HotDays,
	}
	tx := invoker.Db.Begin()
	err = db.TableCreate(tx, &tableInfo)
	if err != nil {
		tx.Rollback()
		err = errors.WithMessage(err, "TableCreateFailed")
		return
	}
	if param.CreateType == constx.TableCreateTypeJSONAsString || param.CreateType == constx.TableCreateTypeJSONEachRow {
		columns, errListColumn := op.ListColumn(databaseInfo.Name, param.TableName, false)
		if errListColumn != nil {
			tx.Rollback()
			err = errors.WithMessage(errListColumn, "ListColumn")
			return tableInfo, err
		}
		for _, col := range columns {
			if col.Type < 0 || col.Type == 3 {
				continue
			}
			if col.Name == "_raw_log_" {
				continue
			}
			err = db.IndexCreate(tx, &db.BaseIndex{
				Tid:      tableInfo.ID,
				Field:    col.Name,
				Typ:      col.Type,
				Alias:    "",
				RootName: "",
				Kind:     0,
			})
			if err != nil {
				tx.Rollback()
				err = errors.WithMessage(err, "IndexCreateFailed")
				return tableInfo, err
			}
		}
	}
	if err = tx.Commit().Error; err != nil {
		return tableInfo, err
	}
	return tableInfo, nil
}
