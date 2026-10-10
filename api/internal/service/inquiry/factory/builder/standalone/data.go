package standalone

import (
	"fmt"

	"github.com/clickvisual/clickvisual/api/internal/pkg/utils"
	"github.com/clickvisual/clickvisual/api/internal/service/inquiry/factory/builder/bumo"
	"github.com/clickvisual/clickvisual/api/internal/service/inquiry/factory/builder/common"
)

// DataBuilder stand-alone cluster version
// _time_ version string/float is the same sql, so we use the same data builder to finish the job.
type DataBuilder struct {
	QueryAssembly *bumo.QueryAssembly
}

func (b *DataBuilder) NewProject(params bumo.Params) {
	b.QueryAssembly = new(bumo.QueryAssembly)
	b.QueryAssembly.Params = params
}

func (b *DataBuilder) BuilderCreate() {
	b.QueryAssembly.Result += fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s\n", b.QueryAssembly.Params.Data.TableName)
}

func (b *DataBuilder) BuilderFields() {
	b.QueryAssembly.Result += common.BuilderFieldsData(b.QueryAssembly.Params.KafkaJsonMapping)
}

func (b *DataBuilder) BuilderWhere() {
}

func (b *DataBuilder) BuilderEngine() {
	b.QueryAssembly.Result += "ENGINE = MergeTree\nPARTITION BY toYYYYMMDD(_time_second_)\n"
}

func (b *DataBuilder) BuilderOrder() {
	b.QueryAssembly.Result += "ORDER BY _time_second_\n"
}

func (b *DataBuilder) BuilderTTL() {
	params := b.QueryAssembly.Params.Data
	ttl, err := utils.BuildTTLClause("_time_second_", params.ColdVolume, params.HotDays, params.Days)
	if err != nil {
		// Fall back to the historical single-layer form so existing tests and
		// callers that never opt into hot/cold keep producing identical SQL.
		ttl = fmt.Sprintf("TTL toDateTime(_time_second_) + INTERVAL %d DAY", params.Days)
	}
	b.QueryAssembly.Result += ttl + "\n"
}

func (b *DataBuilder) BuilderSetting() {
	s, err := utils.BuildSettingsClause(b.QueryAssembly.Params.Data.StoragePolicy)
	if err != nil {
		s = "SETTINGS index_granularity = 8192"
	}
	b.QueryAssembly.Result += s + "\n\n"
}

func (b *DataBuilder) GetResult() interface{} { return b.QueryAssembly }
