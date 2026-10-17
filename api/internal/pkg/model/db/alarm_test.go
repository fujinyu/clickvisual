package db

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MySQL 5.6 supports neither the JSON column type nor JSON_CONTAINS/JSON_EXTRACT, so the alarm
// queries must stay on plain text predicates.
func TestAlarmTidArrCond(t *testing.T) {
	cond, binds := alarmTidArrCond(nil)
	assert.Equal(t, "", cond)
	assert.Empty(t, binds)

	cond, binds = alarmTidArrCond([]int{1, 7})
	assert.Equal(t, 2, strings.Count(cond, "LIKE ?"))
	assert.NotContains(t, cond, "JSON_")
	assert.Equal(t, []interface{}{"%,1,%", "%,7,%"}, binds)
}

func TestAlarmDidTableJoinCond(t *testing.T) {
	join := alarmDidTableJoinCond()
	assert.NotContains(t, join, "JSON_")
	assert.Contains(t, join, "cv_base_table.id")
}

func TestModelTagsCarryNoJSONColumnType(t *testing.T) {
	for _, model := range []interface{}{&PmsRoleDetail{}, &PmsDefaultRole{}, &PmsCustomRole{}, &QueryFilterProfile{}, &Alarm{}} {
		typ := reflect.TypeOf(model).Elem()
		for i := 0; i < typ.NumField(); i++ {
			assert.NotContains(t, typ.Field(i).Tag.Get("gorm"), "type:json", "%s.%s", typ.Name(), typ.Field(i).Name)
		}
	}
}
