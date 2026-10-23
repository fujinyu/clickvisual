package service

import (
	"sort"

	"github.com/ego-component/egorm"

	"github.com/clickvisual/clickvisual/api/internal/invoker"
	db2 "github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
	view2 "github.com/clickvisual/clickvisual/api/internal/pkg/model/view"
	"github.com/clickvisual/clickvisual/api/internal/service/permission/pmsplugin"
)

func DatabaseListFilterPms(uid int) (res []view2.RespDatabaseSimple, err error) {
	res = make([]view2.RespDatabaseSimple, 0)
	dMap := make(map[int]view2.RespDatabaseSimple)
	// all database rows, so a database which is visible only through one of its
	// tables(the old per-table cascade) can still be materialized below
	allDbMap := make(map[int]*db2.BaseDatabase)
	// in-memory permission summary: no per-database sql any more
	summary := pmsplugin.GetUserViewSummary(uid)
	// Fill in all database information and verify related permissions
	ds, _ := db2.DatabaseList(invoker.Db, egorm.Conds{})
	for _, d := range ds {
		allDbMap[d.ID] = d
		if !summary.DatabaseSubVisible(d.Iid, d.ID) {
			continue
		}
		dMap[d.ID] = view2.RespDatabaseSimple{
			Id:           d.ID,
			Iid:          d.Iid,
			DatabaseName: d.Name,
			IsCreateByCV: d.IsCreateByCV,
			Desc:         d.Desc,
			Cluster:      d.Cluster,
			Tables:       make([]view2.RespTableSimple, 0),
		}
	}
	ts, err := db2.TableList(invoker.Db, egorm.Conds{})
	if err != nil {
		return
	}
	for _, row := range ts {
		if row.Database == nil {
			continue
		}
		if !summary.TableSubVisible(row.Database.Iid, row.Did, row.ID) {
			continue
		}
		item, ok := dMap[row.Database.ID]
		if !ok {
			d, exists := allDbMap[row.Database.ID]
			if !exists {
				continue
			}
			item = view2.RespDatabaseSimple{
				Id:           d.ID,
				Iid:          d.Iid,
				DatabaseName: d.Name,
				IsCreateByCV: d.IsCreateByCV,
				Desc:         d.Desc,
				Cluster:      d.Cluster,
				Tables:       make([]view2.RespTableSimple, 0),
			}
			dMap[d.ID] = item
		}
		item.Tables = append(item.Tables, view2.RespTableSimple{
			Id:              row.ID,
			Did:             row.Database.ID,
			TableName:       row.Name,
			CreateType:      row.CreateType,
			Desc:            row.Desc,
			V3TableType:     row.V3TableType,
			RelTraceTableId: row.TraceTableId,
		})
		dMap[row.Database.ID] = item
	}
	for _, v := range dMap {
		res = append(res, v)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].DatabaseName < res[j].DatabaseName
	})
	return
}

// DatabaseViewIsPermission reports whether the user may view database did(of
// instance iid): through the instance itself, through a database-scoped grant,
// or through a grant on one of the database's tables. Only the tables the user
// actually holds grants on are looked up(one query at most), instead of listing
// every table of the database once per subResource like the old cascade did.
func DatabaseViewIsPermission(uid, iid, did int) bool {
	summary := pmsplugin.GetUserViewSummary(uid)
	if summary.DatabaseSubVisible(iid, did) {
		return true
	}
	tids := summary.GrantedTableIds(iid)
	if len(tids) == 0 {
		return false
	}
	tables, err := db2.TableList(invoker.Db, egorm.Conds{
		"id": egorm.Cond{Op: "in", Val: tids},
	})
	if err != nil {
		return false
	}
	for _, t := range tables {
		if t.Did == did && summary.TableSubVisible(iid, did, t.ID) {
			return true
		}
	}
	return false
}
