package permission

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ego-component/egorm"
	"github.com/gotomicro/ego/core/elog"
	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/clickvisual/clickvisual/api/internal/invoker"
	db2 "github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
	"github.com/clickvisual/clickvisual/api/internal/pkg/model/view"
	"github.com/clickvisual/clickvisual/api/internal/service/permission/pmsplugin"
)

type pms struct{}

var Manager *pms

func InitManager() {
	pmsplugin.Invoker()
	Manager = &pms{}
}

func (p *pms) AddUsers2AppRoles(rolesWithUsers *[]view.AppRoleInfoItem) {
	if rolesWithUsers == nil {
		return
	}
	for _, roleItem := range *rolesWithUsers {
		if roleItem.BelongType != pmsplugin.PrefixInstance || roleItem.RoleName == "" || roleItem.ReferId == 0 || len(roleItem.Uids) == 0 {
			continue
		}
		if roleItem.DomainType != "" && roleItem.DomainId != 0 {
			// need dom, so using g rule
			domStr, err := pmsplugin.Assemble2CasbinStr(roleItem.DomainType, strconv.Itoa(roleItem.DomainId))
			if err != nil {
				elog.Warn("found invalid domStr, stop assigning users App role by g.",
					zap.String("domType", roleItem.DomainType), zap.Int("domId", roleItem.DomainId),
					zap.Int("appId", roleItem.ReferId), zap.String("roleName", roleItem.RoleName),
					zap.Error(err))
				continue
			}
			roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, roleItem.RoleName, roleItem.BelongType, strconv.Itoa(roleItem.ReferId))
			for _, uid := range roleItem.Uids {
				userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(uid))
				_, _ = pmsplugin.AddRule(pmsplugin.RuleTypeG, userStr, roleStr, domStr)
			}
		} else {
			// using g3 rule to ignore dom
			roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, roleItem.RoleName, roleItem.BelongType, strconv.Itoa(roleItem.ReferId))
			for _, uid := range roleItem.Uids {
				userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(uid))
				_, _ = pmsplugin.AddRule(pmsplugin.RuleTypeG3, userStr, roleStr)
			}
		}
	}
}

// GrantRoles2User overwrites the user roster of the target pmsRole: after a successful
// return, the submitted userIds are exactly the users holding the role.
//
// How a grant is stored(the pms_role / pms_role_ref / pms_role_ref_grant model):
//   - a pmsRole takes effect on an instance only through its Ref(pms_role_ref), because
//     the Ref is what materializes the role's p rules for that instance;
//   - granting the role to a user means, for every Ref of the role, one grant record
//     (cv_pms_role_ref_grant) plus the matching casbin g3 rule
//     "user__<uid> -> role__<roleId>__instance__<refId>".
//
// So this function covers ALL instances the role applies to(all current instances for a
// default role, the belonged instance for a custom role), creating the missing Refs first
// if needed. Domain-scoped(数据库/日志库) grants are per-instance concerns and are managed
// on the instance panel(系统设置 -> 实例管理 -> 角色授权), not here.
func (p *pms) GrantRoles2User(userId []int, roleId int) (err error) {
	roleInfo, err := db2.PmsRoleInfo(roleId)
	if err != nil || roleInfo == nil || roleInfo.ID == 0 {
		return fmt.Errorf("target pmsRole(id:%d) not found. ", roleId)
	}
	if roleInfo.BelongResource != pmsplugin.PrefixInstance {
		return fmt.Errorf("only support %s resource currently. ", pmsplugin.PrefixInstance)
	}
	if len(roleInfo.Details) <= 0 {
		return fmt.Errorf("target pmsRole(id:%d) has no detail, it cannot be granted. ", roleId)
	}
	pmsplugin.EnforcerLock()
	defer pmsplugin.EnforcerUnlock()

	// 1. make sure the role is activated(Ref + p rules) on every instance it applies to,
	//    otherwise the grant would match no p rule and silently do nothing.
	if err = p.ensurePmsRoleRefs(roleInfo); err != nil {
		return fmt.Errorf("ensure refs of pmsRole(id:%d) failed. %w", roleId, err)
	}
	refs, err := db2.GetPmsRoleRefList(egorm.Conds{"pms_role_id": roleId})
	if err != nil {
		return fmt.Errorf("get refs of pmsRole(id:%d) failed. %w", roleId, err)
	}
	if len(refs) == 0 {
		return fmt.Errorf("target pmsRole(id:%d) has not been activated on any instance. ", roleId)
	}
	refPkIds := make([]int, 0, len(refs))
	for _, ref := range refs {
		refPkIds = append(refPkIds, ref.ID)
	}
	// 2. diff the roster. The current roster is the union of the grant records(the
	//    authoritative source) and the legacy casbin rules(written by the old broken V1
	//    flow, which match no p rule but must still show up and get converted).
	currentRealUids, err := db2.GetPmsRoleGrantUids(roleId, pmsplugin.PrefixUser)
	if err != nil {
		return fmt.Errorf("get granted uids of pmsRole(id:%d) failed. %w", roleId, err)
	}
	legacyRoleStr := legacyInstanceRoleStr(roleInfo.Name)
	legacyUids := p.getLegacyRoleUserIds(legacyRoleStr)
	_, needAddUids, _ := pmsplugin.CmpUserIds2GetNewAndRmUserIds(dedupeInts(currentRealUids), userId)
	_, _, needRmUids := pmsplugin.CmpUserIds2GetNewAndRmUserIds(
		dedupeInts(append(append(make([]int, 0, len(currentRealUids)+len(legacyUids)), currentRealUids...), legacyUids...)), userId)

	tx := invoker.Db.Begin()
	// 3. overwrite semantics: remove the grants(record + casbin rule) of the users which
	//    are not in the submitted roster anymore.
	for _, rmUid := range needRmUids {
		if rmUid <= 0 {
			continue
		}
		userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(rmUid))
		userGrants, gErr := db2.GetPmsRoleRefGrantsByObj(refPkIds, pmsplugin.PrefixUser, rmUid)
		if gErr != nil {
			elog.Warn("GrantRoles2User: get grants of the user to be removed failed",
				zap.Int("uid", rmUid), zap.Error(gErr))
		}
		delEhRules := make([]pmsplugin.EnhancedCasbinRulesItem, 0)
		for _, ref := range refs {
			roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, strconv.Itoa(roleId),
				pmsplugin.PrefixInstance, strconv.Itoa(ref.RefId))
			// the format written by this function(and by the instance panel without domain)
			delEhRules = append(delEhRules, pmsplugin.EnhancedCasbinRulesItem{
				Ptype: pmsplugin.RuleTypeG3,
				Rules: [][]string{{userStr, roleStr}},
			})
			// domain-scoped grants made on the instance panel
			for _, grant := range userGrants {
				if grant.PmsRoleRefId != ref.ID || grant.DomainType == "" {
					continue
				}
				domStr, _ := pmsplugin.Assemble2CasbinStr(grant.DomainType, strconv.Itoa(grant.DomainId))
				delEhRules = append(delEhRules, pmsplugin.EnhancedCasbinRulesItem{
					Ptype: pmsplugin.RuleTypeG,
					Rules: [][]string{{userStr, roleStr, domStr}},
				})
			}
		}
		if err = pmsplugin.DelCasbinRulesFromDb(tx, delEhRules); err != nil {
			tx.Rollback()
			return fmt.Errorf("remove casbin rules of user(%d) failed. %w", rmUid, err)
		}
		if err = tx.Table(db2.TableNamePmsRoleRefGrant).
			Where("pms_role_ref_id IN ? AND object_type = ? AND object_id = ?",
				refPkIds, pmsplugin.PrefixUser, rmUid).
			Delete(&db2.PmsRoleRefGrant{}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("delete grant records of user(%d) failed. %w", rmUid, err)
		}
	}
	// 4. convert/clean the legacy rules of the broken V1 flow(name-based "role__<name>
	//    __instance__0" rules, which match no p rule and grant nothing): after this
	//    operation none of them should remain, no matter the user is added, kept or removed.
	if len(legacyUids) > 0 {
		legacyDelEhRules := make([]pmsplugin.EnhancedCasbinRulesItem, 0)
		for _, legacyUid := range legacyUids {
			if legacyUid <= 0 {
				continue
			}
			userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(legacyUid))
			// also try the legacy malformed subject(user__[3]) written before this was fixed
			legacyUserStr := fmt.Sprintf("%s%s[%d]", pmsplugin.PrefixUser, pmsplugin.SEP, legacyUid)
			for _, subject := range []string{userStr, legacyUserStr} {
				legacyDelEhRules = append(legacyDelEhRules,
					pmsplugin.EnhancedCasbinRulesItem{Ptype: pmsplugin.RuleTypeG3,
						Rules: [][]string{{subject, legacyRoleStr}}},
					pmsplugin.EnhancedCasbinRulesItem{Ptype: pmsplugin.RuleTypeG,
						Rules: [][]string{{subject, legacyRoleStr, ""}}},
				)
			}
		}
		if err = pmsplugin.DelCasbinRulesFromDb(tx, legacyDelEhRules); err != nil {
			tx.Rollback()
			return fmt.Errorf("remove legacy casbin rules of pmsRole(id:%d) failed. %w", roleId, err)
		}
	}
	// 5. add the grants(record + casbin g3 rule) of the new users, on every ref of the role
	for _, addUid := range needAddUids {
		if addUid <= 0 {
			continue
		}
		userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(addUid))
		existGrants, gErr := db2.GetPmsRoleRefGrantsByObj(refPkIds, pmsplugin.PrefixUser, addUid)
		if gErr != nil {
			elog.Warn("GrantRoles2User: get exist grants of the user failed",
				zap.Int("uid", addUid), zap.Error(gErr))
		}
		existRefIds := make(map[int]struct{}, len(existGrants))
		for _, grant := range existGrants {
			existRefIds[grant.PmsRoleRefId] = struct{}{}
		}
		addEhRules := make([]pmsplugin.EnhancedCasbinRulesItem, 0)
		for _, ref := range refs {
			roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, strconv.Itoa(roleId),
				pmsplugin.PrefixInstance, strconv.Itoa(ref.RefId))
			if _, exist := existRefIds[ref.ID]; !exist {
				grant := db2.PmsRoleRefGrant{
					PmsRoleRefId: ref.ID,
					Ptype:        pmsplugin.RuleTypeG3,
					ObjectType:   pmsplugin.PrefixUser,
					ObjectId:     addUid,
				}
				if err = tx.Table(db2.TableNamePmsRoleRefGrant).Create(&grant).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("create grant record of user(%d) on instance(%d) failed. %w",
						addUid, ref.RefId, err)
				}
			}
			// g3(no domain): the role page grants the role's full scope on the instance
			addEhRules = append(addEhRules, pmsplugin.EnhancedCasbinRulesItem{
				Ptype: pmsplugin.RuleTypeG3,
				Rules: [][]string{{userStr, roleStr}},
			})
		}
		if err = pmsplugin.AddCasbinRules2Db(tx, addEhRules); err != nil {
			tx.Rollback()
			return fmt.Errorf("add casbin rules of user(%d) failed. %w", addUid, err)
		}
	}
	if err = tx.Commit().Error; err != nil {
		return fmt.Errorf("commit grants of pmsRole(id:%d) failed. %w", roleId, err)
	}
	// 6. resync the enforcer with the db(the changes above only touched the db).
	pmsplugin.EnforcerLoadPolicy()
	return nil
}

// ensurePmsRoleRefs makes sure the role has been activated(a Ref, which materializes the
// role's p rules) on every instance it applies to: all current instances for a default
// role, the belonged instance for a custom role. Without a Ref the role has no p rule on
// that instance, so granting it would silently match nothing. Existing Refs are kept and
// missing ones are created here, which also picks up instances created after the last grant.
func (p *pms) ensurePmsRoleRefs(roleInfo *db2.PmsRole) (err error) {
	targetIids := make([]int, 0)
	if roleInfo.RoleType == db2.PmsRoleTypeCustom {
		if roleInfo.ResourceId <= 0 {
			return fmt.Errorf("custom pmsRole(id:%d) has no belonged resourceId. ", roleInfo.ID)
		}
		targetIids = append(targetIids, roleInfo.ResourceId)
	} else {
		instances, err := db2.InstanceList(egorm.Conds{})
		if err != nil {
			return fmt.Errorf("get instances failed. %w", err)
		}
		for _, ins := range instances {
			targetIids = append(targetIids, ins.ID)
		}
	}
	existedIids := make(map[int]struct{}, len(roleInfo.Refs))
	for _, ref := range roleInfo.Refs {
		existedIids[ref.RefId] = struct{}{}
	}
	newRefs := make([]*db2.PmsRoleRef, 0)
	for _, iid := range targetIids {
		if _, exist := existedIids[iid]; exist {
			continue
		}
		newRefs = append(newRefs, &db2.PmsRoleRef{PmsRoleId: roleInfo.ID, RefId: iid})
	}
	if len(newRefs) == 0 {
		return nil
	}
	tx := invoker.Db.Begin()
	if err = p.pmsRoleAddRef(tx, roleInfo, newRefs...); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// legacyInstanceRoleStr assembles the roleStr written by the old broken V1 grant flow:
// "role__<name>__instance__0". It is name-based(not role-id based) and points at a
// non-existent instance 0, so it matches no p rule and grants nothing. It is only used to
// find and clean up those legacy rules.
func legacyInstanceRoleStr(roleName string) string {
	return strings.Join([]string{pmsplugin.PrefixRole, roleName, pmsplugin.PrefixInstance, "0"},
		pmsplugin.SEP)
}

// getLegacyRoleUserIds reads the uids which only exist as casbin rules of the broken V1
// flow(i.e. name-based "role__<name>__instance__0" rules). They are returned so that the
// role page still displays those(historically)associated users, and the next save converts
// them into real grant records.
func (p *pms) getLegacyRoleUserIds(legacyRoleStr string) (uids []int) {
	uids = make([]int, 0)
	eRules := pmsplugin.GetRulesByRoleStrFromDb(legacyRoleStr, "")
	if eRules == nil {
		return uids
	}
	for _, eRule := range *eRules {
		for _, rule := range eRule.Rules {
			if uid := parseUidFromSubjectStr(rule[0]); uid > 0 {
				uids = append(uids, uid)
			}
		}
	}
	return dedupeInts(uids)
}

// AssignRoles2User - assign roles to specific user
func (p *pms) AssignRoles2User(userId string, reqRoles []view.RoleItem) (err error) {
	if userId == "" {
		return errors.New("userId cannot be empty")
	}
	userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, userId)
	for _, roleItem := range reqRoles {
		roleStr, err := pmsplugin.GetValidRoleStrByRoleItem(roleItem)
		if err != nil {
			elog.Warn("invalid roleString", zap.Error(err))
			continue
		}
		var gType = pmsplugin.RuleTypeG3
		var targetDom string
		if roleItem.DomainType != "" {
			targetDom, err = pmsplugin.Assemble2CasbinStr(roleItem.DomainType, strconv.Itoa(roleItem.DomainId))
			if err != nil {
				elog.Warn("invalid dom format", zap.Error(err))
				continue
			}
			gType = pmsplugin.RuleTypeG
		}
		if _, err := pmsplugin.AddRule(gType, userStr, roleStr, targetDom); err != nil {
			elog.Warn("assign role to user error", zap.String("role", roleStr), zap.Error(err))
		}
	}
	return nil
}

// AssignTableRolesUser:  re-assign users to all roles of app, based on the list of AppRoleUsersItem
func (p *pms) AssignTableRolesUser(appId int, reqAppRolesWithUsers []view.AppRoleInfoItem) {
	if appId <= 0 {
		return
	}
	var appRolesUser []view.AppRoleInfoItem
	for _, appRole := range reqAppRolesWithUsers {
		if appRole.BelongType != pmsplugin.PrefixInstance || appRole.RoleName == "" || len(appRole.Uids) == 0 {
			continue
		}
		appRolesUser = append(appRolesUser, appRole)
	}
	p.OverwriteAppRolesUser(appId, appRolesUser)

}

func (p *pms) AssignConfigResourceRolesUser(configResourceName string) {

}

func (p *pms) CheckNormalPermission(reqPms view.ReqPermission) error {
	checker := p.newUserPmsCheckStrategy(reqPms.ObjectType, reqPms.SubResource)
	return checker.Check(reqPms)
}

/*
CratedDefaultRolePms && DeleteDefaultRolePms:
	在default_role_table中创建record的同时, 需要在casbin给所有belongType类型的资源都添加p规则, 删除时同理需要都移除对应的p和相关的g g3
	同时 注意保持default_role_table 的唯一性; 另外也要兼顾custom_role_table中的record, 不能重名, 否则custom_role会不起作用.
CreateCustomRolePms && DeleteCustomRolePms:
	 注意: 在除了保证custom_role_table内的唯一性之前, 需要先去检查default_role_table中是否含有了欲添加role!!!!
	同理, 在对custom_role_table内增删record时, 需要在casbin中增p 和 删p,g, g3
*/

func (p *pms) CreateDefaultRolePms(newDefaultRole *view.DefaultRolePms) (err error) {
	if _, valid := pmsplugin.PermittedPrefixMap[newDefaultRole.BelongType]; !valid {
		return errors.New("the belongType of new default role is invalid")
	}
	if newDefaultRole.RoleName == "" {
		return errors.New("the roleName cannot be empty!")
	}

	// check the existence of current default roles, if already exist, then need to check the equality of subResource
	searchDefaultRes, err := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": newDefaultRole.BelongType,
		"role_name":   newDefaultRole.RoleName,
	})
	if err != nil {
		return errors.Wrap(err, "check existence of default role failed.")
	}
	if len(searchDefaultRes) > 0 {
		// check the subResource
		for _, dbDefaultRole := range searchDefaultRes {
			if pmsplugin.IsStringSliceEqual(dbDefaultRole.SubResources, newDefaultRole.SubResources) {
				// TODO: overwrite dbDefaultRole.Acts ?
				return errors.New("the role already exist.")
			}
		}
	}
	// 2. check if the customRole already has the role with same belongType and roleName, if exist, will return error
	searchCustomRes, err := db2.GetCustomRolePmsList(db2.Conds{
		"belong_type": newDefaultRole.BelongType,
		"role_name":   newDefaultRole.RoleName,
	})
	if err != nil {
		return errors.Wrap(err, "check existence of custom role failed.")
	}

	if len(searchCustomRes) > 0 {
		// TODO: support force overwrite?
		referIds := make([]int, 0)
		for _, custRes := range searchCustomRes {
			referIds = append(referIds, custRes.ReferId)
		}
		return errors.Errorf("the role(%s) already exist in customRole of %s_id(s) %v", newDefaultRole.RoleName,
			newDefaultRole.BelongType, referIds)
	}
	/*
		3. add p rules for all of current belong_type resources
			note that, defaultRolePms is associate with p rules in casbin.
	*/
	// 3.1 pre-process the p rules
	var pRulesField01NotDone [][]string
	var p0RoleTpl, _ = pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, newDefaultRole.RoleName, newDefaultRole.BelongType, "%s")
	var p2Act = pmsplugin.JointActs2RuleActStr(newDefaultRole.Acts...)
	var p3Dom = "*"
	for _, resrc := range newDefaultRole.SubResources {
		p1SubResrcTpl, _ := pmsplugin.Assemble2CasbinStr(newDefaultRole.BelongType, "%s", pmsplugin.PrefixSubRsrc, resrc)
		pRulesField01NotDone = append(pRulesField01NotDone, []string{p0RoleTpl, p1SubResrcTpl, p2Act, p3Dom})
	}
	// 3.2 find all items of belongType, and add p rules for item, current only support app
	var pRuleAdded bool
	switch newDefaultRole.BelongType {
	case pmsplugin.PrefixInstance:
		instances, err := db2.InstanceList(egorm.Conds{})
		if err != nil {
			return errors.Wrap(err, "get apps failed, stop adding p rules for app")
		}
		for _, ins := range instances {
			for _, pR := range pRulesField01NotDone {
				// just add p rules, if the p rule exist, will not be added.
				_, _ = pmsplugin.AddRule(pmsplugin.RuleTypeP, fmt.Sprintf(pR[0], strconv.Itoa(ins.ID)),
					fmt.Sprintf(pR[1], strconv.Itoa(ins.ID)), pR[2], pR[3])
			}
		}
		pRuleAdded = true
	default:
		return errors.Errorf("Not support add defaultRole for %s resource type currently", newDefaultRole.BelongType)
	}
	// 4. finally, if p rule(s) added in casbin then add this new default role in db.
	if pRuleAdded {
		if err = db2.PmsDefaultRoleCreate(&(newDefaultRole.PmsDefaultRole)); err != nil {
			return err
		}
	}
	return nil

}

func (*pms) CreateCustomRolePms(newCustomRole view.CustomRolePms) (err error) {
	if _, valid := pmsplugin.PermittedPrefixMap[newCustomRole.BelongType]; !valid {
		return errors.New("the belongType of new default role is invalid")
	}
	if newCustomRole.RoleName == "" {
		return errors.New("the roleName cannot be empty")
	}
	if newCustomRole.ReferId == 0 {
		return errors.New("the referId cannot be empty")
	}

	// check the existence of current default roles, if already exist, then return error
	searchDefaultRes, err := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": newCustomRole.BelongType,
		"role_name":   newCustomRole.RoleName,
	})
	if err != nil {
		return errors.Wrap(err, "check existence of default role failed.")
	}
	if len(searchDefaultRes) > 0 {
		// do not compare further. because different role
		return errors.New("same role already exist in default role.")
	}
	// 2. check the existence of customRole,
	searchCustomRes, err := db2.GetCustomRolePmsList(db2.Conds{
		"belong_type": newCustomRole.BelongType,
		"role_name":   newCustomRole.RoleName,
		"refer_id":    newCustomRole.ReferId,
	})
	if err != nil {
		return errors.Wrap(err, "check existence of custom role failed.")
	}
	// if exist
	if len(searchCustomRes) > 0 {
		// need check the equality further
		for _, dbCustomRole := range searchCustomRes {
			if pmsplugin.IsStringSliceEqual(dbCustomRole.SubResources, newCustomRole.SubResources) {
				return errors.Errorf("the customRole(%s) already exist", newCustomRole.RoleName)
			}
		}
	}
	/*
		3. add p rules for target refer belong_type resource
			note that, CustomRole is associate with p rules in casbin.
	*/
	// 3.1 pre-process the p rules
	var pRules = [][]string{}
	var p0Role, _ = pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, newCustomRole.RoleName,
		newCustomRole.BelongType, strconv.Itoa(newCustomRole.ReferId))
	var p2Act = pmsplugin.JointActs2RuleActStr(newCustomRole.Acts...)
	var p3Dom = "*"
	for _, resrc := range newCustomRole.SubResources {
		p1Resrc, _ := pmsplugin.Assemble2CasbinStr(newCustomRole.BelongType, strconv.Itoa(newCustomRole.ReferId),
			pmsplugin.PrefixSubRsrc, resrc)
		pRules = append(pRules, []string{p0Role, p1Resrc, p2Act, p3Dom})
	}
	// 3.2 check the existence of target belongType resource, then add p rules for it, current only support app
	var pRuleAdded bool
	switch newCustomRole.BelongType {
	case pmsplugin.PrefixInstance:
		instances, err := db2.InstanceList(egorm.Conds{"id": newCustomRole.ReferId})
		if err != nil {
			return errors.Wrap(err, "get target app failed, stop adding p rules for app")
		}
		if len(instances) == 0 {
			return errors.New("target app not exist, stop adding p rules of custom role for it.")
		}
		for _, pR := range pRules {
			// just add p rules, if the p rule exist, will not be added.
			_, _ = pmsplugin.AddRule(pmsplugin.RuleTypeP, pR[0], pR[1], pR[2], pR[3])
		}
		pRuleAdded = true
	default:
		return errors.Errorf("Not support add customRole for %s resource type currently", newCustomRole.BelongType)
	}
	// 4. finally, if p rule(s) added in casbin then add this new custom role in db.
	if pRuleAdded {
		if err := db2.PmsCustomRoleCreate(&(newCustomRole.PmsCustomRole)); err != nil {
			return err
		}
	}
	return nil

}

// delete DefaultRole function
func (p *pms) DeleteDefaultRolePms(delDefaultRole view.DefaultRolePms) (err error) {
	if _, valid := pmsplugin.PermittedPrefixMap[delDefaultRole.BelongType]; !valid {
		return errors.New("the belongType of default role is invalid, stop deleting")
	}
	if delDefaultRole.RoleName == "" {
		return errors.New("the roleName of default role cannot be empty, stop deleting")
	}

	// check the existence in current default roles, if not exist, then return error
	currentDefaultRoles, err := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": delDefaultRole.BelongType,
		"role_name":   delDefaultRole.RoleName,
	})
	if err != nil {
		return errors.Wrap(err, "check existence in current default roles failed.")
	}
	if len(currentDefaultRoles) == 0 {
		return errors.New("not found target default role in db, do not need to delete")
	}
	/*
		3. assemble not complete p rules based on default role, then remove all those p rules for belongType resources
	*/
	// 3.1 pre-process the p rules
	var pRulesField01NotDone = [][]string{}
	var p0RoleTpl, _ = pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, delDefaultRole.RoleName, delDefaultRole.BelongType, "%s")
	var p3Dom = "*"
	// Note using subResources of currentDefaultRoles(which got from db),
	// do not using the input param DelDefaultRole(which maybe a partial subResources, perhaps leave unused rules in casbin)
	for _, dbDefaultRole := range currentDefaultRoles {
		var p2Act = pmsplugin.JointActs2RuleActStr(dbDefaultRole.Acts...)
		for _, resrc := range dbDefaultRole.SubResources {
			p1SubResrcTpl, _ := pmsplugin.Assemble2CasbinStr(dbDefaultRole.BelongType, "%s", pmsplugin.PrefixSubRsrc, resrc)
			pRulesField01NotDone = append(pRulesField01NotDone, []string{p0RoleTpl, p1SubResrcTpl, p2Act, p3Dom})
		}
	}
	// 3.2 find all items of belongType, and remove p, g, g3 rules for item, current only support app
	var rulesDeleted bool
	switch delDefaultRole.BelongType {
	case pmsplugin.PrefixInstance:
		instances, err := db2.InstanceList(egorm.Conds{})
		if err != nil {
			return errors.Wrap(err, "get apps failed, stop removing p rules of default role for app")
		}
		for _, ins := range instances {
			currentAppRoleStr := fmt.Sprintf(p0RoleTpl, strconv.Itoa(ins.ID))
			// remove g and g3 rules first
			ehRules := pmsplugin.GetRulesByRole(currentAppRoleStr, "")
			if len(*ehRules) > 0 {
				for _, ehRule := range *ehRules {
					for _, rule := range ehRule.Rules {
						iRule := make([]interface{}, len(rule))
						for i, v := range rule {
							iRule[i] = v
						}
						_, _ = pmsplugin.DelRule(ehRule.Ptype, iRule...)
					}
				}
			}
			// remove p rules
			for _, pR := range pRulesField01NotDone {
				// just remove p rules, do not need to check the existence of p rule
				_, _ = pmsplugin.DelRule(pmsplugin.RuleTypeP, fmt.Sprintf(pR[0], strconv.Itoa(ins.ID)),
					fmt.Sprintf(pR[1], strconv.Itoa(ins.ID)), pR[2], pR[3])
			}
		}
		rulesDeleted = true

	default:
		return errors.Errorf("Not support remove defaultRole for %s resource type currently", delDefaultRole.BelongType)
	}

	if rulesDeleted {
		for _, dbDefaultRole := range currentDefaultRoles {
			if err := db2.PmsDefaultRoleDelete(int(dbDefaultRole.ID)); err != nil {
				elog.Error("delete defaultRole db record error", zap.Error(err))
			}
		}
	}
	return nil
}

// delete CustomRole function
func (p *pms) DeleteCustomRolePms(delCustomRole view.CustomRolePms) (err error) {
	if _, valid := pmsplugin.PermittedPrefixMap[delCustomRole.BelongType]; !valid {
		return errors.New("the belongType of custom role is invalid, stop deleting")
	}
	if delCustomRole.RoleName == "" {
		return errors.New("the roleName of custom role cannot be empty, stop deleting")
	}
	if delCustomRole.ReferId == 0 {
		return errors.New("the id of refer obj of custom role cannot be 0, stop deleting")
	}
	// check the existence in current custom roles, if not exist, then return error
	currentCustomRoles, err := db2.GetCustomRolePmsList(db2.Conds{
		"belong_type": delCustomRole.BelongType,
		"role_name":   delCustomRole.RoleName,
		"refer_id":    delCustomRole.ReferId,
	})
	if err != nil {
		return errors.Wrap(err, "check existence in current custom roles failed.")
	}
	if len(currentCustomRoles) == 0 {
		return errors.New("not found target custom role in db, do not need to delete")
	}
	/*
		3. assemble p rules based on searched custom role, then remove all those p rules for target resource
	*/
	// 3.1 pre-process the p rules
	var pRules = [][]string{}
	var p0Role, _ = pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, delCustomRole.RoleName, delCustomRole.BelongType, strconv.Itoa(delCustomRole.ReferId))
	var p3Dom = "*"
	// Note using subResources of currentCustomRoles(which got from db),
	// do not using the input param delCustomRole(which maybe a partial subResources, perhaps leave unused rules in casbin)
	for _, dbCustomRole := range currentCustomRoles {
		var p2Act = pmsplugin.JointActs2RuleActStr(dbCustomRole.Acts...)
		for _, resrc := range dbCustomRole.SubResources {
			p1SubResrc, _ := pmsplugin.Assemble2CasbinStr(dbCustomRole.BelongType, strconv.Itoa(dbCustomRole.ReferId),
				pmsplugin.PrefixSubRsrc, resrc)
			pRules = append(pRules, []string{p0Role, p1SubResrc, p2Act, p3Dom})
		}
	}
	// 3.2 do the remove process
	// 3.2.1 remove related g and g3 rules first
	ehRules := pmsplugin.GetRulesByRole(p0Role, "")
	if len(*ehRules) > 0 {
		for _, ehRule := range *ehRules {
			for _, rule := range ehRule.Rules {
				iRule := make([]interface{}, len(rule))
				for i, v := range rule {
					iRule[i] = v
				}
				_, _ = pmsplugin.DelRule(ehRule.Ptype, iRule...)
			}
		}
	}
	// 3.2.2 remove related p rules
	for _, pR := range pRules {
		// just remove p rules, do not need to check the existence of p rule
		_, _ = pmsplugin.DelRule(pmsplugin.RuleTypeP, pR[0], pR[1], pR[2], pR[3])
	}

	// 4. finally, delete custom role in db.
	for _, dbCustomRole := range currentCustomRoles {
		if err := db2.PmsCustomRoleDelete(int(dbCustomRole.ID)); err != nil {
			elog.Error("delete customRole db record error", zap.Error(err))
		}
	}
	return nil
}

func (p *pms) DelUsersFromAppRoles(rolesWithUsers *[]view.AppRoleInfoItem) {
	if rolesWithUsers == nil {
		return
	}
	for _, roleItem := range *rolesWithUsers {
		if roleItem.BelongType != pmsplugin.PrefixInstance || roleItem.RoleName == "" || roleItem.ReferId == 0 || len(roleItem.Uids) == 0 {
			continue
		}
		if roleItem.DomainType != "" && roleItem.DomainId != 0 {
			// need dom, so using g rule
			domStr, err := pmsplugin.Assemble2CasbinStr(roleItem.DomainType, strconv.Itoa(roleItem.DomainId))
			if err != nil {
				elog.Warn("found invalid domStr, stop remove users App role by g.",
					zap.String("domType", roleItem.DomainType), zap.Int("domId", roleItem.DomainId),
					zap.Int("appId", roleItem.ReferId), zap.String("roleName", roleItem.RoleName),
					zap.Error(err))
				continue
			}
			roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, roleItem.RoleName, roleItem.BelongType, strconv.Itoa(roleItem.ReferId))
			for _, uid := range roleItem.Uids {
				userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(uid))
				_, _ = pmsplugin.DelRule(pmsplugin.RuleTypeG, userStr, roleStr, domStr)
			}
		} else {
			// using g3 rule to ignore dom
			roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, roleItem.RoleName, roleItem.BelongType, strconv.Itoa(roleItem.ReferId))
			for _, uid := range roleItem.Uids {
				userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(uid))
				_, _ = pmsplugin.DelRule(pmsplugin.RuleTypeG3, userStr, roleStr)
			}
		}
	}
}

/*
EnsureResourceHasDefaultRoles:
Params:

	resourceType: current only support "app" or "configResource"
	resourceIdx: appId or configResourceName
*/
func (*pms) EnsureResourceHasDefaultRoles(resourceType string, resourceIdx string) (err error) {
	if resourceIdx == "" || resourceType == "" {
		return errors.New("resourceType or resourceIdx cannot be empty")
	}
	switch resourceType {
	case pmsplugin.PrefixInstance:
		// 1. ensure input param resourceIdx is a valid appId
		insId, err := strconv.Atoi(resourceIdx)
		if err != nil {
			return fmt.Errorf("resourceIdx is an invalid appId, error: %s", err.Error())
		}
		// 2. check the existence of target app based on resourceIdx(i.e. appId)
		targetInstanceList, err := db2.InstanceList(egorm.Conds{
			"id": insId,
		})
		if err != nil || len(targetInstanceList) <= 0 {
			return fmt.Errorf("not found target app based on resourceIdx(appId) %s", resourceIdx)
		}
		// 3. check the existence of "app" type defaultRole
		defaultRolePmsList, _ := db2.GetDefaultRolePmsList(db2.Conds{
			"belong_type": pmsplugin.PrefixInstance,
		})
		if len(defaultRolePmsList) <= 0 {
			return fmt.Errorf("not found any %s type roles currently", pmsplugin.PrefixInstance)
		}
		// 4. add new p rules for target app based on "app" type defaultRoles
		for _, appDefaultRolePms := range defaultRolePmsList {
			var pRules [][]string
			var p0Role, _ = pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, appDefaultRolePms.RoleName, pmsplugin.PrefixInstance, resourceIdx)
			var p2Act = pmsplugin.JointActs2RuleActStr(appDefaultRolePms.Acts...)
			var p3Dom = "*"
			for _, resrc := range appDefaultRolePms.SubResources {
				p1SubResrc, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixInstance, resourceIdx, pmsplugin.PrefixSubRsrc, resrc)
				pRules = append(pRules, []string{p0Role, p1SubResrc, p2Act, p3Dom})
			}
			for _, pR := range pRules {
				// just add p rules, if the p rule exist, will not be added.
				_, _ = pmsplugin.AddRule(pmsplugin.RuleTypeP, pR[0], pR[1], pR[2], pR[3])
			}
		}
		return nil

	default:
		return fmt.Errorf("not support %s type defaultRole currently", resourceType)
	}
}

/*
EnsureUsersHaveResourceDefaultRole
Params:

	reqFuzzyDefaultRole: see the comments of the struct definition
	uids: the id list of users which will be assign the defaultRole
*/
func (*pms) EnsureUsersHaveResourceDefaultRole(reqFuzzyDefaultRole view.ReqEnsureFuzzyDefaultRole, uids ...int) (err error) {
	if len(uids) <= 0 {
		return nil
	}
	// search fuzzyDefaultRole(s)
	fuzzyDefaultRoles, err := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": reqFuzzyDefaultRole.BelongType,
		"role_name": db2.Cond{
			Op:  "like",
			Val: reqFuzzyDefaultRole.RoleNameLike,
		},
	})
	if err != nil || len(fuzzyDefaultRoles) <= 0 {
		return fmt.Errorf("not found any %s type and name like %s defaultRoles",
			reqFuzzyDefaultRole.BelongType, reqFuzzyDefaultRole.RoleNameLike)
	}

	switch reqFuzzyDefaultRole.BelongType {
	case pmsplugin.PrefixInstance:
		// 1. ensure input param referIdx is a valid appId
		insId, err := strconv.Atoi(reqFuzzyDefaultRole.ReferIdx)
		if err != nil {
			return fmt.Errorf("referIdx is an invalid appId, error: %s", err.Error())
		}
		// 2. check the existence of target app based on referIdx(i.e. appId)
		targetInstanceList, err := db2.InstanceList(egorm.Conds{
			"id": insId,
		})
		if err != nil || len(targetInstanceList) <= 0 {
			return fmt.Errorf("not found target app based on ReferIdx(appId) %s", reqFuzzyDefaultRole.ReferIdx)
		}

		// 3. assign role to uids
		// 3.1 detect using g or g3 rule based on reqFuzzyDefaultRole's domainType and domainId
		var roleRuleType = pmsplugin.RuleTypeG3
		var domStr string
		if reqFuzzyDefaultRole.DomainType != "" {
			if _, valid := pmsplugin.PermittedDomPrefixMap[reqFuzzyDefaultRole.DomainType]; !valid {
				return fmt.Errorf("the DomainType of reqFuzzyDefaultRole is invalid")
			}
			if reqFuzzyDefaultRole.DomainId == 0 {
				return fmt.Errorf("reqFuzzyDefaultRole's DomainType is not empty, but DomainId is empty")
			}
			roleRuleType = pmsplugin.RuleTypeG
			domStr, _ = pmsplugin.Assemble2CasbinStr(reqFuzzyDefaultRole.DomainType, strconv.Itoa(reqFuzzyDefaultRole.DomainId))
		}
		roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, fuzzyDefaultRoles[0].RoleName,
			pmsplugin.PrefixInstance, reqFuzzyDefaultRole.ReferIdx)
		// 3.2 do assign role to user
		switch roleRuleType {
		case pmsplugin.RuleTypeG3:
			for _, uid := range uids {
				if uid <= 0 {
					continue
				}
				userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(uid))
				_, _ = pmsplugin.AddRule(pmsplugin.RuleTypeG3, userStr, roleStr)
			}
		case pmsplugin.RuleTypeG:
			for _, uid := range uids {
				if uid <= 0 {
					continue
				}
				userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(uid))
				_, _ = pmsplugin.AddRule(pmsplugin.RuleTypeG, userStr, roleStr, domStr)
			}
		}
		return nil
	default:
		return fmt.Errorf("not support %s type defaultRole currently", reqFuzzyDefaultRole.BelongType)
	}
}

// IsRootUser check target user(with param uid) is root or not.
// if returned error is nil, means target user can perform root user's action;
// if returned error != nil, please stop exec the rest codes from the invoked point of upper func and just throwout the error
// Note, this func check system lock status first, when system is locked will return error, even if target uid is root.
func (p *pms) IsRootUser(uid int) error {
	if uid <= 0 {
		return fmt.Errorf("invalid uid %d", uid)
	}
	if pmsplugin.IsRootWithoutCheckingSysLock(uid) {
		return nil
	}
	return errors.New(MsgNeedRoot)
}

func (p *pms) IsRootUserAndDomNotLock(uid int, reqDomains ...ReqDomainLockStatus) error {
	if err := p.IsRootUser(uid); err != nil {
		return err
	}
	return nil
}

func (*pms) GetRootUsersId() []int {
	rootUids := make([]int, 0)
	eRules := pmsplugin.GetRulesByRoleStrDirectly("role__root", "")
	if eRules != nil && len(*eRules) > 0 {
		for _, eR := range *eRules {
			for _, rule := range eR.Rules {
				uid := pmsplugin.GetUidBySubjectStr(rule[0])
				if uid > 0 {
					rootUids = append(rootUids, uid)
				}
			}
		}

	}
	return rootUids
}

// parseUidFromSubjectStr extracts the uid from a casbin subject like "user__3".
// grants stored before GrantRoles2User was fixed may look like "user__[3]",
// so surrounding brackets are tolerated here to keep old grants readable.
func parseUidFromSubjectStr(subjectStr string) int {
	if !strings.Contains(subjectStr, pmsplugin.PrefixUser+pmsplugin.SEP) {
		return 0
	}
	parts := strings.Split(subjectStr, pmsplugin.SEP)
	if len(parts) != 2 {
		return 0
	}
	uid, err := strconv.Atoi(strings.Trim(parts[1], "[] "))
	if err != nil {
		return 0
	}
	return uid
}

// GetRoleUsersId returns the uids of users who hold the target role.
//
// The authoritative source is the grant records(cv_pms_role_ref_grant) read straight from
// the db: the casbin enforcer memory is NOT reliable here, because rules with an empty
// domain are skipped by casbin's LoadPolicyArray and thus missing from the memory after
// every restart. Grants made on any instance(the role page) or with any domain(the
// instance panel) both count as "holding the role".
//
// Users which only exist as legacy casbin rules(written by the old broken V1 flow as
// name-based "role__<name>__instance__0" rules that match no p rule) are also returned,
// so that they still show up in the role page and get converted into real grant records
// on the next save.
func (p *pms) GetRoleUsersId(roleId int) (uids []int) {
	uids = make([]int, 0)
	roleInfo, err := db2.PmsRoleInfo(roleId)
	if err != nil || roleInfo == nil || roleInfo.ID == 0 {
		elog.Warn("GetRoleUsersId: get pmsRole failed", zap.Int("roleId", roleId), zap.Error(err))
		return uids
	}
	grantUids, err := db2.GetPmsRoleGrantUids(roleId, pmsplugin.PrefixUser)
	if err != nil {
		elog.Warn("GetRoleUsersId: get granted uids of pmsRole failed",
			zap.Int("roleId", roleId), zap.Error(err))
	}
	uids = append(uids, grantUids...)
	uids = append(uids, p.getLegacyRoleUserIds(legacyInstanceRoleStr(roleInfo.Name))...)
	// dedupe: a user may hold both a grant record and a legacy rule if the grant was
	// saved before the legacy rules were cleaned up.
	uids = dedupeInts(uids)
	sort.Ints(uids)
	return uids
}

// dedupeInts removes duplicated items and keeps the first-seen order.
func dedupeInts(uids []int) []int {
	seen := make(map[int]struct{}, len(uids))
	deduped := make([]int, 0, len(uids))
	for _, uid := range uids {
		if _, ok := seen[uid]; ok {
			continue
		}
		seen[uid] = struct{}{}
		deduped = append(deduped, uid)
	}
	return deduped
}

func (p *pms) GrantRootUsers(newRootUids []int) {
	currentRootUids := p.GetRootUsersId()
	isEqual, uidsNeed2Add, uidsNeed2Rm := pmsplugin.CmpUserIds2GetNewAndRmUserIds(currentRootUids, newRootUids)
	if isEqual {
		return
	}
	elog.Debug("pms", elog.Any("uidsNeed2Add", uidsNeed2Add), elog.Any("uidsNeed2Add", uidsNeed2Rm))
	for _, newUid := range uidsNeed2Add {
		userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(newUid))
		res, err := pmsplugin.AddRule(pmsplugin.RuleTypeG3, userStr, "role__root")
		if err != nil {
			elog.Error("pms", elog.Any("err", err.Error()), elog.Any("res", res))
		}
		elog.Debug("pms", elog.Any("res", res))
	}
	for _, rmUid := range uidsNeed2Rm {
		userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, strconv.Itoa(rmUid))
		_, _ = pmsplugin.DelRule(pmsplugin.RuleTypeG3, userStr, "role__root")
	}
	// do not forget to reload casbin policy, because we just modified records in db above.
	pmsplugin.EnforcerLoadPolicy()
}

// GetAllRoles would return all Rules of a user
func (*pms) GetAllRolesOfUser(uid int) (res []view.RoleItem, err error) {
	res = make([]view.RoleItem, 0)
	enhancedRules, err := pmsplugin.GetRulesByUserId(uid, pmsplugin.RuleTypeG, pmsplugin.RuleTypeG3)
	if err != nil {
		return
	}
	for _, eRule := range enhancedRules {
		for _, rule := range eRule.Rules {
			roleItem, err := pmsplugin.TransUserGxRule2RoleItemDetail(eRule.Ptype, rule...)
			if err != nil {
				elog.Warn("trans gx rule to roleItem error", zap.Error(err))
				continue
			}
			res = append(res, roleItem)
		}
	}
	return res, nil
}

func (p *pms) GetUsersIdByAppRoleInAllDomain(appId int, roleName string) view.DomainUids {
	result := make(view.DomainUids)
	// check custom role table
	customRolePmsList, _ := db2.GetCustomRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
		"refer_id":    appId,
		"role_name":   roleName,
	})
	if len(customRolePmsList) > 0 {
		validRoleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, customRolePmsList[0].RoleName, pmsplugin.PrefixInstance, strconv.Itoa(appId))
		eRules := pmsplugin.GetRulesByRole(validRoleStr, "")
		if eRules != nil && len(*eRules) > 0 {
			for _, eR := range *eRules {
				if eR.Ptype == pmsplugin.RuleTypeG3 {
					if _, exist := result["*"]; !exist {
						result["*"] = make([]int, 0)
					}
					for _, rule := range eR.Rules {
						uid := pmsplugin.GetUidBySubjectStr(rule[0])
						if uid > 0 {
							result["*"] = append(result["*"], uid)
						}
					}
				} else if eR.Ptype == pmsplugin.RuleTypeG {
					for _, rule := range eR.Rules {
						uid := pmsplugin.GetUidBySubjectStr(rule[0])
						if uid > 0 {
							dom := rule[2]
							if _, exist := result[dom]; !exist {
								result[dom] = make([]int, 0)
							}
							result[dom] = append(result[dom], uid)
						}
					}
				}
			}
		}
		return result
	}
	// default role table
	defaultRolePmsList, _ := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
		"role_name":   roleName,
	})
	if len(defaultRolePmsList) > 0 {
		roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, defaultRolePmsList[0].RoleName, pmsplugin.PrefixInstance, strconv.Itoa(appId))
		eRules := pmsplugin.GetRulesByRole(roleStr, "")
		if eRules != nil && len(*eRules) > 0 {
			for _, eR := range *eRules {
				if eR.Ptype == pmsplugin.RuleTypeG3 {
					if _, exist := result["*"]; !exist {
						result["*"] = make([]int, 0)
					}
					for _, rule := range eR.Rules {
						uid := pmsplugin.GetUidBySubjectStr(rule[0])
						if uid > 0 {
							result["*"] = append(result["*"], uid)
						}
					}
				} else if eR.Ptype == pmsplugin.RuleTypeG {
					for _, rule := range eR.Rules {
						uid := pmsplugin.GetUidBySubjectStr(rule[0])
						if uid > 0 {
							dom := rule[2]
							if _, exist := result[dom]; !exist {
								result[dom] = make([]int, 0)
							}
							result[dom] = append(result[dom], uid)
						}
					}
				}
			}
		}
		return result
	}
	return result
}

func (p *pms) GetUsersIdByAppRoleInDom(appId int, roleName string, reqDom string) []int {
	uids := make([]int, 0)
	// check custom role table
	customRolePmsList, _ := db2.GetCustomRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
		"refer_id":    appId,
		"role_name":   roleName,
	})
	if len(customRolePmsList) > 0 {
		validRoleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, customRolePmsList[0].RoleName, pmsplugin.PrefixInstance, strconv.Itoa(appId))
		eRules := pmsplugin.GetRulesByRole(validRoleStr, reqDom)
		if eRules != nil {
			for _, eR := range *eRules {
				for _, rule := range eR.Rules {
					uid := pmsplugin.GetUidBySubjectStr(rule[0])
					if uid > 0 {
						uids = append(uids, uid)
					}
				}
			}
		}
		return uids
	}
	// default role table
	defaultRolePmsList, _ := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
		"role_name":   roleName,
	})
	if len(defaultRolePmsList) > 0 {
		roleStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixRole, defaultRolePmsList[0].RoleName, pmsplugin.PrefixInstance, strconv.Itoa(appId))
		eRules := pmsplugin.GetRulesByRole(roleStr, reqDom)
		if eRules != nil {
			for _, eR := range *eRules {
				for _, rule := range eR.Rules {
					uid := pmsplugin.GetUidBySubjectStr(rule[0])
					if uid > 0 {
						uids = append(uids, uid)
					}
				}
			}
		}
		return uids
	}
	return uids
}

// GetTableAvailableRoles: get defaultRole(s) and customRole(s) of specific app
func (p *pms) GetTableAvailableRoles(appId int) *view.AppAvailableRoles {
	var appDefaultRoles = make([]view.RoleItem, 0)
	var appCustomRoles = make([]view.RoleItem, 0)
	// 1. process app default roles
	defaultRolePmsList, _ := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
	})
	defaultRoleMap := make(map[string][]db2.PmsDefaultRole)
	for _, defaultRole := range defaultRolePmsList {
		if _, exist := defaultRoleMap[defaultRole.RoleName]; !exist {
			defaultRoleMap[defaultRole.RoleName] = []db2.PmsDefaultRole{*defaultRole}
			continue
		}
		defaultRoleMap[defaultRole.RoleName] = append(defaultRoleMap[defaultRole.RoleName], *defaultRole)
	}
	for roleName, defaultRoles := range defaultRoleMap {
		desc := defaultRoles[0].Description
		details := make([]view.RolePmsDetail, 0)
		for _, defaultRole := range defaultRoles {
			details = append(details, view.RolePmsDetail{
				SubResources: defaultRole.SubResources,
				Acts:         defaultRole.Acts,
			})
		}
		appDefaultRoles = append(appDefaultRoles, view.RoleItem{
			BelongType: pmsplugin.PrefixInstance,
			ReferId:    0,
			ReferGroup: "",
			RoleName:   roleName,
			RoleDesc:   desc,
			PmsDetails: details,
			DomainType: "",
			DomainId:   0,
		})

	}
	// 2. process app custom roles
	customRolePmsList, _ := db2.GetCustomRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
		"refer_id":    appId,
	})
	customRoleMap := make(map[string][]db2.PmsCustomRole)
	for _, customRole := range customRolePmsList {
		if _, exist := customRoleMap[customRole.RoleName]; !exist {
			customRoleMap[customRole.RoleName] = []db2.PmsCustomRole{*customRole}
			continue
		}
		customRoleMap[customRole.RoleName] = append(customRoleMap[customRole.RoleName], *customRole)
	}
	for roleName, customRoles := range customRoleMap {
		desc := customRoles[0].Description
		details := make([]view.RolePmsDetail, 0)
		for _, customRole := range customRoles {
			details = append(details, view.RolePmsDetail{
				SubResources: customRole.SubResources,
				Acts:         customRole.Acts,
			})
		}
		appCustomRoles = append(appCustomRoles, view.RoleItem{
			BelongType: pmsplugin.PrefixInstance,
			ReferId:    appId,
			ReferGroup: "",
			RoleName:   roleName,
			RoleDesc:   desc,
			PmsDetails: details,
			DomainType: "",
			DomainId:   0,
		})

	}
	return &view.AppAvailableRoles{
		AppId:   appId,
		Default: appDefaultRoles,
		Custom:  appCustomRoles,
	}

}

func (p *pms) GetTableRolesAssignmentInfoInAllDom(appId int) (appRolesAssignRes view.TableRolesAssignmentInfo) {
	// 先查两个表, default_role & custom_role; 然后通过casbin api去获取userId 填充AppRolesAssignmentInfo
	customRolePmsList, _ := db2.GetCustomRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
		"refer_id":    appId,
	})
	defaultRolePmsList, _ := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
	})
	appRolesAssignRes.AppId = appId
	// first traverse app customRole item
	customRoleMap := make(map[string][]db2.PmsCustomRole)
	for _, customRole := range customRolePmsList {
		if _, exist := customRoleMap[customRole.RoleName]; !exist {
			customRoleMap[customRole.RoleName] = []db2.PmsCustomRole{*customRole}
			continue
		}
		customRoleMap[customRole.RoleName] = append(customRoleMap[customRole.RoleName], *customRole)
	}
	for roleName, customRoles := range customRoleMap {
		desc := customRoles[0].Description
		details := make([]view.RolePmsDetail, 0)
		for _, customRolePms := range customRoles {
			details = append(details, view.RolePmsDetail{
				SubResources: customRolePms.SubResources,
				Acts:         customRolePms.Acts,
			})
		}
		domUids := p.GetUsersIdByAppRoleInAllDomain(appId, roleName)
		if len(domUids) <= 0 {
			continue
		}
		for dom, uids := range domUids {
			domType, domId := pmsplugin.GetDomTypeAndId(dom)
			appRolesAssignRes.RolesInfo = append(appRolesAssignRes.RolesInfo, view.AppRoleInfoItem{
				RoleItem: view.RoleItem{
					BelongType: pmsplugin.PrefixInstance,
					ReferId:    appId,
					ReferGroup: "",
					RoleName:   roleName,
					RoleDesc:   desc,
					PmsDetails: details,
					DomainType: domType,
					DomainId:   domId,
				},
				Uids: uids,
			})
		}

	}
	// then traverse global defaultRole item of app
	defaultRoleMap := make(map[string][]db2.PmsDefaultRole)
	for _, defaultRole := range defaultRolePmsList {
		if _, exist := defaultRoleMap[defaultRole.RoleName]; !exist {
			defaultRoleMap[defaultRole.RoleName] = []db2.PmsDefaultRole{*defaultRole}
			continue
		}
		defaultRoleMap[defaultRole.RoleName] = append(defaultRoleMap[defaultRole.RoleName], *defaultRole)
	}
	for roleName, defaultRoles := range defaultRoleMap {
		roleDesc := defaultRoles[0].Description
		details := make([]view.RolePmsDetail, 0)
		for _, defaultRolePms := range defaultRoles {
			details = append(details, view.RolePmsDetail{
				SubResources: defaultRolePms.SubResources,
				Acts:         defaultRolePms.Acts,
			})
		}
		domUids := p.GetUsersIdByAppRoleInAllDomain(appId, roleName)
		if len(domUids) <= 0 {
			continue
		}
		for dom, uids := range domUids {
			domType, domId := pmsplugin.GetDomTypeAndId(dom)
			appRolesAssignRes.RolesInfo = append(appRolesAssignRes.RolesInfo, view.AppRoleInfoItem{
				RoleItem: view.RoleItem{
					BelongType: pmsplugin.PrefixInstance,
					ReferId:    0, // the referId of default role is 0
					ReferGroup: "",
					RoleName:   roleName,
					RoleDesc:   roleDesc,
					PmsDetails: details,
					DomainType: domType,
					DomainId:   domId,
				},
				Uids: uids,
			})
		}
	}
	return
}

func (p *pms) GetAppRolesAssignmentInfoInDom(appId int, reqDom string) (appRolesAssignRes view.TableRolesAssignmentInfo) {
	reqDomType, reqDomId := pmsplugin.GetDomTypeAndId(reqDom)
	// 先查两个表, default_role & custom_role; 然后通过casbin api去获取userId 填充AppRolesAssignmentInfo
	customRolePmsList, _ := db2.GetCustomRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
		"refer_id":    appId,
	})
	defaultRolePmsList, _ := db2.GetDefaultRolePmsList(db2.Conds{
		"belong_type": pmsplugin.PrefixInstance,
	})
	appRolesAssignRes.AppId = appId
	// first traverse app customRole item
	customRoleMap := make(map[string][]db2.PmsCustomRole)
	for _, customRole := range customRolePmsList {
		if _, exist := customRoleMap[customRole.RoleName]; !exist {
			customRoleMap[customRole.RoleName] = []db2.PmsCustomRole{*customRole}
			continue
		}
		customRoleMap[customRole.RoleName] = append(customRoleMap[customRole.RoleName], *customRole)
	}
	for roleName, customRoles := range customRoleMap {
		desc := customRoles[0].Description
		details := make([]view.RolePmsDetail, 0)
		for _, customRolePms := range customRoles {
			details = append(details, view.RolePmsDetail{
				SubResources: customRolePms.SubResources,
				Acts:         customRolePms.Acts,
			})
		}
		uids := p.GetUsersIdByAppRoleInDom(appId, roleName, reqDom)

		appRolesAssignRes.RolesInfo = append(appRolesAssignRes.RolesInfo, view.AppRoleInfoItem{
			RoleItem: view.RoleItem{
				BelongType: pmsplugin.PrefixInstance,
				ReferId:    appId,
				ReferGroup: "",
				RoleName:   roleName,
				RoleDesc:   desc,
				PmsDetails: details,
				DomainType: reqDomType,
				DomainId:   reqDomId,
			},
			Uids: uids,
		})
	}
	// then traverse global defaultRole item of app
	defaultRoleMap := make(map[string][]db2.PmsDefaultRole)
	for _, defaultRole := range defaultRolePmsList {
		if _, exist := defaultRoleMap[defaultRole.RoleName]; !exist {
			defaultRoleMap[defaultRole.RoleName] = []db2.PmsDefaultRole{*defaultRole}
			continue
		}
		defaultRoleMap[defaultRole.RoleName] = append(defaultRoleMap[defaultRole.RoleName], *defaultRole)
	}
	for roleName, defaultRoles := range defaultRoleMap {
		roleDesc := defaultRoles[0].Description
		details := make([]view.RolePmsDetail, 0)
		for _, defaultRolePms := range defaultRoles {
			details = append(details, view.RolePmsDetail{
				SubResources: defaultRolePms.SubResources,
				Acts:         defaultRolePms.Acts,
			})
		}
		uids := p.GetUsersIdByAppRoleInDom(appId, roleName, reqDom)
		appRolesAssignRes.RolesInfo = append(appRolesAssignRes.RolesInfo, view.AppRoleInfoItem{
			RoleItem: view.RoleItem{
				BelongType: pmsplugin.PrefixInstance,
				ReferId:    0, // the referId of default role is 0
				ReferGroup: "",
				RoleName:   roleName,
				RoleDesc:   roleDesc,
				PmsDetails: details,
				DomainType: reqDomType,
				DomainId:   reqDomId,
			},
			Uids: uids,
		})

	}
	return
}

// GetAllDefaultRoles:  get all current default roles
func (*pms) GetPmsDefaultRoles(conds db2.Conds) ([]*db2.PmsDefaultRole, error) {
	return db2.GetDefaultRolePmsList(conds)
}

// OverwriteAppRolesUser: overwrite the granted state of all roles of an app, based on the input param []AppRoleUsersItem
func (p *pms) OverwriteAppRolesUser(appId int, appNewRolesWithUsers []view.AppRoleInfoItem) {
	wait2Add := make([]view.AppRoleInfoItem, 0)
	wait2Del := make([]view.AppRoleInfoItem, 0)
	// appCurrentRolesAssignInfo := p.GetAppRolesAssignmentInfo(appId, "")
	appCurrentRolesAssignInfo := p.GetTableRolesAssignmentInfoInAllDom(appId)
	for k1, appNewRole := range appNewRolesWithUsers {
		if appNewRole.BelongType != pmsplugin.PrefixInstance || appNewRole.ReferId != appId {
			continue
		}
		for k2, appCurrentRole := range appCurrentRolesAssignInfo.RolesInfo {
			if appNewRole.RoleName == appCurrentRole.RoleName && appNewRole.DomainType == appCurrentRole.DomainType &&
				appNewRole.DomainId == appCurrentRole.DomainId {
				appNewRolesWithUsers[k1].RoleName = "-skip"
				appCurrentRolesAssignInfo.RolesInfo[k2].RoleName = "-skip"
				isEqual, needAdd, needRm := pmsplugin.CmpUserIds2GetNewAndRmUserIds(appCurrentRole.Uids, appNewRole.Uids)
				if isEqual {
					continue
				}
				if len(needAdd) > 0 {
					wait2Add = append(wait2Add, view.AppRoleInfoItem{
						RoleItem: appNewRole.RoleItem,
						Uids:     needAdd,
					})
				}
				if len(needRm) > 0 {
					wait2Del = append(wait2Del, view.AppRoleInfoItem{
						RoleItem: view.RoleItem{
							BelongType: pmsplugin.PrefixInstance,
							ReferId:    appId,
							ReferGroup: "",
							RoleName:   appCurrentRole.RoleName,
							DomainType: appCurrentRole.DomainType,
							DomainId:   appCurrentRole.DomainId,
						},
						Uids: needRm,
					})
				}
			}
		}
	}

	for _, newRole := range appNewRolesWithUsers {
		if newRole.RoleName == "-skip" || len(newRole.Uids) == 0 {
			continue
		}
		wait2Add = append(wait2Add, view.AppRoleInfoItem{
			RoleItem: newRole.RoleItem,
			Uids:     newRole.Uids,
		})
	}
	for _, rmRole := range appCurrentRolesAssignInfo.RolesInfo {
		if rmRole.RoleName == "-skip" || len(rmRole.Uids) == 0 {
			continue
		}
		wait2Del = append(wait2Del, view.AppRoleInfoItem{
			RoleItem: view.RoleItem{
				BelongType: pmsplugin.PrefixInstance,
				ReferId:    appId,
				ReferGroup: "",
				RoleName:   rmRole.RoleName,
				DomainType: rmRole.DomainType,
				DomainId:   rmRole.DomainId,
			},
			Uids: rmRole.Uids,
		})
	}

	if len(wait2Add) > 0 {
		p.AddUsers2AppRoles(&wait2Add)
	}
	if len(wait2Del) > 0 {
		p.DelUsersFromAppRoles(&wait2Del)
	}
}

// RemoveUserRoles - remove roles of specific user
func (p *pms) RemoveUserRoles(userId string, reqRoles []view.RoleItem) (err error) {
	if userId == "" {
		return errors.New("userId cannot be empty")
	}
	userStr, _ := pmsplugin.Assemble2CasbinStr(pmsplugin.PrefixUser, userId)
	for _, roleItem := range reqRoles {
		roleStr, err := pmsplugin.GetValidRoleStrByRoleItem(roleItem)
		if err != nil {
			continue
		}
		var gType = pmsplugin.RuleTypeG3
		var targetDom string
		if roleItem.DomainType != "" {
			targetDom, err = pmsplugin.Assemble2CasbinStr(roleItem.DomainType, strconv.Itoa(roleItem.DomainId))
			if err != nil {
				elog.Warn("invalid dom format", zap.Error(err))
				continue
			}
			gType = pmsplugin.RuleTypeG
		}
		if _, err := pmsplugin.DelRule(gType, userStr, roleStr, targetDom); err != nil {
			elog.Warn("remove user role error", zap.String("role", roleStr), zap.Error(err))
		}
	}
	return nil
}
