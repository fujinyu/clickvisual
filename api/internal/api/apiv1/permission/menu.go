package permission

import (
	"github.com/clickvisual/clickvisual/api/internal/pkg/component/core"
	"github.com/clickvisual/clickvisual/api/internal/service"
	"github.com/clickvisual/clickvisual/api/internal/service/permission"
	"github.com/clickvisual/clickvisual/api/internal/service/permission/pmsplugin"
)

// @Tags         PREMISSION
func MenuList(c *core.Context) {
	if err := permission.Manager.IsRootUser(c.Uid()); err == nil {
		c.JSONOK(service.Permission.AdminMenuList())
		return
	}
	// The menu only needs to know whether the user can view each subResource on
	// at least one instance. That used to be decided by walking every instance
	// with a db-enumerating permission check(one DatabaseList plus one TableList
	// per database, per subResource), which took seconds on deployments with
	// many databases. The in-memory summary answers it without any sql.
	summary := pmsplugin.GetUserViewSummary(c.Uid())
	logFlag := summary.AnyInstanceSub(pmsplugin.Log)
	alarmFlag := summary.AnyInstanceSub(pmsplugin.Alarm)
	pandasFlag := summary.AnyInstanceSub(pmsplugin.Pandas)
	res := make([]permission.MenuTreeItem, 0)
	for _, p := range service.Permission.AdminMenuList() {
		if logFlag && p.Name == "log" {
			res = append(res, p)
		}
		if alarmFlag && p.Name == "alarm" {
			res = append(res, p)
		}
		if pandasFlag && p.Name == "bigdata" {
			res = append(res, p)
		}
	}
	c.JSONOK(res)
}
