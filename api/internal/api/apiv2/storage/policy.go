package storage

import (
	"strconv"

	"github.com/spf13/cast"

	"github.com/clickvisual/clickvisual/api/internal/invoker"
	"github.com/clickvisual/clickvisual/api/internal/pkg/component/core"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/view"
	"github.com/clickvisual/clickvisual/api/internal/service"
	"github.com/clickvisual/clickvisual/api/internal/service/permission"
	"github.com/clickvisual/clickvisual/api/internal/service/permission/pmsplugin"
)

// StoragePolicies godoc
// @Summary	     List ClickHouse storage policies of an instance
// @Description  Read system.storage_policies from the target ClickHouse instance so the UI
// @Description  can offer hot/cold tiering dropdowns when creating a log library.
// @Tags         LOGSTORE
// @Produce      json
// @Param        iid path int true "instance id"
// @Success      200 {object} core.Res{}{data=[]view.RespStoragePolicy}
// @Router       /api/v2/instances/{iid}/storage-policies [get]
func StoragePolicies(c *core.Context) {
	iid := cast.ToInt(c.Param("iid"))
	if iid == 0 {
		c.JSONE(core.CodeErr, "param error: missing iid", nil)
		return
	}
	if _, err := db.InstanceInfo(invoker.Db, iid); err != nil {
		c.JSONE(core.CodeErr, "instance does not exist: "+err.Error(), nil)
		return
	}
	if err := permission.Manager.CheckNormalPermission(view.ReqPermission{
		UserId:      c.Uid(),
		ObjectType:  pmsplugin.PrefixInstance,
		ObjectIdx:   strconv.Itoa(iid),
		SubResource: pmsplugin.Log,
		Acts:        []string{pmsplugin.ActView},
	}); err != nil {
		c.JSONE(1, "permission verification failed", err)
		return
	}
	op, err := service.InstanceManager.Load(iid)
	if err != nil {
		c.JSONE(core.CodeErr, "failed to load instance: "+err.Error(), nil)
		return
	}
	policies, err := op.ListStoragePolicies()
	if err != nil {
		c.JSONE(core.CodeErr, err.Error(), nil)
		return
	}
	c.JSONOK(policies)
}
