package db

import (
	"strings"

	"github.com/ego-component/egorm"
	"github.com/gotomicro/ego/core/elog"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	TimeFieldTypeDT     = 0 // DateTime
	TimeFieldTypeSecond = 1
	TimeFieldTypeTsMs   = 2 // unix ms
	TimeFieldTypeDT3    = 3 // DataTime64(3)
	TimeFieldTypeDT6    = 4 // DataTime64(6)
	TimeFieldTypeDT9    = 5 // DataTime64(9)
)

// suspend_status values: whether the Kafka stream table of the log store is
// attached in ClickHouse. See scripts/migration/logstore_suspend/.
const (
	SuspendStatusAttached int = 0
	SuspendStatusDetached int = 1
)

type BaseTable struct {
	BaseModel
	Uid          int    `gorm:"column:uid;type:int(11)" json:"uid"`                                           // operator uid
	Did          int    `gorm:"column:did;type:int(11);index:uix_did_name,unique" json:"did"`                 // database id
	Name         string `gorm:"column:name;type:varchar(128);NOT NULL;index:uix_did_name,unique" json:"name"` // table
	Desc         string `gorm:"column:desc;type:varchar(255)" json:"desc"`
	TraceTableId int    `gorm:"column:trace_table_id;type:int(11)" json:"traceTableId"` // associated trace table id

	// base attribute
	TimeFieldKind int `gorm:"column:typ;type:int(11)" json:"typ"`                                          // time field kind: 1 string 2 float
	TimeFieldType int `gorm:"column:time_field_type;type:int(11);default:0;NOT NULL" json:"timeFieldType"` // time field type: 0 DateTime 1 second 2 ms 3 DataTime64(3)

	CreateType int `gorm:"column:create_type;type:tinyint(1)" json:"createType"` // operation type, 0 means create clickvisual fresh table, 1 means use exists table

	// ClickHouse setting
	Days                    int    `gorm:"column:days;type:int(11)" json:"days"`                     // data expire days
	Topic                   string `gorm:"column:topic;type:varchar(128);NOT NULL" json:"topic"`     // kafka topic
	Brokers                 string `gorm:"column:brokers;type:varchar(255);NOT NULL" json:"brokers"` // kafka broker
	ConsumerNum             int    `gorm:"column:consumer_num;type:int(11)" json:"consumerNum"`      // kafka consumer number
	TimeField               string `gorm:"column:time_field;type:varchar(128);NOT NULL" json:"timeField"`
	RawLogField             string `gorm:"column:raw_log_field;type:varchar(255)" json:"rawLogField"`
	KafkaSkipBrokenMessages int    `gorm:"column:kafka_skip_broken_messages;type:int(11)" json:"kafkaSkipBrokenMessages"`

	// ClickHouse hot/cold tiering (all zero/empty values keep the legacy single-layer behavior)
	StoragePolicy string `gorm:"column:storage_policy;type:varchar(64);NOT NULL;default:''" json:"storagePolicy"` // storage_policy name, empty means disabled
	ColdVolume    string `gorm:"column:cold_volume;type:varchar(64);NOT NULL;default:''" json:"coldVolume"`       // TTL TO VOLUME target, empty means disabled
	HotDays       int    `gorm:"column:hot_days;type:int(11);default:0" json:"hotDays"`                            // days on hot volume before moving to cold

	// Ingestion pause state. 1 means the Kafka stream table of this log store is
	// DETACHed in ClickHouse, so nothing is consumed or inserted; messages pile up
	// in the topic until it is resumed. Requires scripts/migration/
	// logstore_suspend/01_alter_cv_base_table.mysql.sql.
	SuspendStatus int `gorm:"column:suspend_status;type:tinyint(1);NOT NULL;default:0" json:"suspendStatus"`

	// Deprecated: use CreateType instead
	IsKafkaTimestamp int `gorm:"column:is_kafka_timestamp;type:tinyint(1)" json:"isKafkaTimestamp"`
	// Deprecated: use CreateType instead
	V3TableType int `gorm:"column:v3_table_type;type:int(11)" json:"v3TableType"` // 0 default 1 jaegerJson
	// Deprecated: use base_table_attach instead
	SelectFields string `gorm:"column:select_fields;type:text" json:"selectFields"` // sql_distributed
	// Deprecated: use base_table_attach instead
	AnyJSON string `gorm:"column:any_json;type:text" json:"anyJSON"`
	// Deprecated: use base_table_attach instead
	SqlData string `gorm:"column:sql_data;type:text" json:"sqlData"` // sql_data
	// Deprecated: use base_table_attach instead
	SqlStream string `gorm:"column:sql_stream;type:text" json:"sqlStream"` // sql_stream
	// Deprecated: use base_table_attach instead
	SqlView string `gorm:"column:sql_view;type:text" json:"sqlView"` // sql_view
	// Deprecated: use base_table_attach instead
	SqlDistributed string `gorm:"column:sql_distributed;type:text" json:"sqlDistributed"` // sql_distributed

	Database *BaseDatabase `json:"database,omitempty" gorm:"foreignKey:Did;references:ID"`
}

type ReqCreateBufferNullDataPipe struct {
	Cluster  string
	Database string
	Table    string
	TTL      int
}

func (b *BaseTable) TableName() string {
	return TableNameBaseTable
}

func (b *BaseTable) GetTimeField() string {
	if b.TimeField == "" {
		return TimeFieldSecond
	}
	return b.TimeField
}

// TableCreate ...
func TableCreate(db *gorm.DB, data *BaseTable) (err error) {
	if err = db.Model(BaseTable{}).Create(data).Error; err != nil {
		return errors.Wrapf(err, "data is %v", data)
	}
	return
}

// TableDelete Soft delete
func TableDelete(db *gorm.DB, id int) (err error) {
	if err = db.Model(BaseTable{}).Unscoped().Delete(&BaseTable{}, id).Error; err != nil {
		elog.Error("delete error", zap.Error(err))
		return
	}
	return
}

// TableInfoX Info extension method to query a single record according to Cond
func TableInfoX(db *gorm.DB, conds map[string]interface{}) (resp BaseTable, err error) {
	sql, binds := egorm.BuildQuery(conds)
	if err = db.Table(TableNameBaseTable).Where(sql, binds...).First(&resp).Error; err != nil && err != gorm.ErrRecordNotFound {
		elog.Error("infoX error", zap.Error(err))
		return
	}
	return
}

func TableInfo(db *gorm.DB, paramId int) (resp BaseTable, err error) {
	var sql = "`id`= ?"
	var binds = []interface{}{paramId}
	if err = db.Table(TableNameBaseTable).Preload("Database").Where(sql, binds...).First(&resp).Error; err != nil && err != gorm.ErrRecordNotFound {
		err = errors.Wrapf(err, "table id: %d", paramId)
		return
	}
	return
}

// TableUpdate ...
func TableUpdate(db *gorm.DB, paramId int, ups map[string]interface{}) (err error) {
	var sql = "`id`=?"
	var binds = []interface{}{paramId}
	if err = db.Table(TableNameBaseTable).Where(sql, binds...).Updates(ups).Error; err != nil {
		elog.Error("update error", zap.Error(err))
		return
	}
	return
}

// TableList Get all currently undeleted clusters. Mainly used for front end
func TableList(db *gorm.DB, conds egorm.Conds) (resp []*BaseTable, err error) {
	sql, binds := egorm.BuildQuery(conds)
	// Fetch record with Rancher Info....
	if err = db.Table(TableNameBaseTable).Preload("Database").Where(sql, binds...).Order("id asc").Find(&resp).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, errors.Wrapf(err, "conds %v:", conds)
	}
	return
}

func TableListByInstanceId(db *gorm.DB, iid int) (resp []*BaseTable, err error) {
	conds := egorm.Conds{}
	conds["iid"] = iid
	databases, _ := DatabaseList(db, conds)
	for _, database := range databases {
		condsTb := egorm.Conds{}
		condsTb["did"] = database.ID
		tables, errTableList := TableList(db, condsTb)
		if errTableList != nil {
			continue
		}
		resp = append(resp, tables...)
	}
	return
}

// LogLibraryTableRow is one log library flattened together with the names of the
// database and instance owning it, which is the shape the log management list
// needs. Kept separate from BaseTable because the list never touches the
// ClickHouse settings carried by the entity.
type LogLibraryTableRow struct {
	Id            int    `gorm:"column:id"`
	Did           int    `gorm:"column:did"`
	Iid           int    `gorm:"column:iid"`
	InstanceName  string `gorm:"column:instance_name"`
	DatabaseName  string `gorm:"column:database_name"`
	TableName     string `gorm:"column:table_name"`
	Desc          string `gorm:"column:table_desc"`
	CreateType    int    `gorm:"column:create_type"`
	SuspendStatus int    `gorm:"column:suspend_status"`
}

// ReqLogLibraryTableList holds the list filters. Every one of them is pushed
// down as a bound parameter so the browser never receives rows it filtered out.
type ReqLogLibraryTableList struct {
	Iid int
	Did int
	Kw  string
	// CreateTypes restricts the rows to a create-type set, empty meaning any.
	CreateTypes []int
	// SuspendStatus is nil for any state, otherwise an exact match.
	SuspendStatus *int
	Current       int
	PageSize      int
}

// TableListPageForLogLibrary returns one page of log libraries across all
// instances. The instance name is not on cv_base_table, so the owning database
// and instance are joined in rather than looked up per row; both joins are
// many-to-one, which keeps the count equal to the number of table rows.
func TableListPageForLogLibrary(invoker *gorm.DB, req ReqLogLibraryTableList) (total int64, resp []*LogLibraryTableRow, err error) {
	resp = make([]*LogLibraryTableRow, 0)
	query := invoker.Table(TableNameBaseTable + " AS t").
		Select("t.id, t.did, t.name AS table_name, t.`desc` AS table_desc, t.create_type, t.suspend_status, " +
			"d.iid, d.name AS database_name, i.name AS instance_name").
		Joins("LEFT JOIN " + TableNameBaseDatabase + " AS d ON d.id = t.did").
		Joins("LEFT JOIN " + TableNameBaseInstance + " AS i ON i.id = d.iid")
	where := make([]string, 0, 5)
	binds := make([]interface{}, 0, 8)
	if req.Iid > 0 {
		where = append(where, "d.iid = ?")
		binds = append(binds, req.Iid)
	}
	if req.Did > 0 {
		where = append(where, "t.did = ?")
		binds = append(binds, req.Did)
	}
	if kw := strings.TrimSpace(req.Kw); kw != "" {
		like := "%" + kw + "%"
		where = append(where, "(t.name LIKE ? OR t.`desc` LIKE ? OR d.name LIKE ? OR i.name LIKE ?)")
		binds = append(binds, like, like, like, like)
	}
	if len(req.CreateTypes) > 0 {
		holders := make([]string, 0, len(req.CreateTypes))
		for _, createType := range req.CreateTypes {
			holders = append(holders, "?")
			binds = append(binds, createType)
		}
		where = append(where, "t.create_type IN ("+strings.Join(holders, ",")+")")
	}
	if req.SuspendStatus != nil {
		where = append(where, "t.suspend_status = ?")
		binds = append(binds, *req.SuspendStatus)
	}
	if len(where) > 0 {
		query = query.Where(strings.Join(where, " AND "), binds...)
	}
	if err = query.Count(&total).Error; err != nil {
		return 0, resp, errors.Wrap(err, "count log libraries failed")
	}
	if err = query.Order("t.id asc").
		Offset((req.Current - 1) * req.PageSize).Limit(req.PageSize).
		Find(&resp).Error; err != nil {
		return total, resp, errors.Wrap(err, "list log libraries failed")
	}
	return
}
