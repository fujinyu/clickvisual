package pmsplugin

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/casbin/casbin/v2"
)

// viewSubResources are the subResources whose "view" permission decides whether
// an instance/database/table is listed for a non-root user. Keep in sync with
// service.InstanceViewIsPermission / service.DatabaseViewIsPermission.
var viewSubResources = []string{Log, Alarm, Pandas}

// anyDomain is the domain a CheckNormalPermission derives when the request
// carries no DomainType(see getCasbinItemsFromReqPermission).
const anyDomain = "*"

// rootRoleStr is the role every root user is linked to by a g3 rule, see
// IsRootWithoutCheckingSysLock and GrantRootUsers.
const rootRoleStr = PrefixRole + SEP + "root"

// wildcardUserSubject mirrors checkAsterisk in the permission package: every
// CheckNormalPermission also evaluates the same rule with subject user__-1.
const wildcardUserSubject = PrefixUser + SEP + "-1"

// UserViewSummary is a per-user, in-memory summary of "view" permissions.
//
// Why it exists: the old visibility checks(service.InstanceViewPmsWithSubResource
// etc.) walked the whole resource tree with sql whenever the instance-level check
// failed - one DatabaseList per instance plus one TableList per database, times
// three subResources. On a deployment with hundreds of databases that is
// thousands of queries per request, which made /api/v1/menus/list and the
// instance list take seconds.
//
// The summary replaces that tree walk with pure casbin memory reads. The key
// observation(verified against the rbac.conf matcher, see pmsview_test.go):
//   - a domain-scoped grant is stored as a g rule "user, role__X__instance__I, dom",
//     and the role string itself encodes the instance I, while the p rules of that
//     role always carry obj "instance__I__subResource__S". So a g rule of another
//     instance can never match another instance's obj, and enumerating the
//     databases/tables of an instance is unnecessary: it is enough to evaluate
//     the domains which the user actually holds g rules for.
//   - an instance-wide grant(g3) passes the enforce for every domain, so if the
//     instance-level check fails, only the user's own g-rule domains can still
//     make a lower-level check pass.
//   - a g rule with dom "*" only matches the instance-level request(casbin's g
//     matches all three elements exactly), so it is tracked separately: it makes
//     the instance visible but no database or table.
//
// The result is exactly what the old cascade computed, without touching the db.
//
// The grants of the wildcard subject user__-1 are merged into every summary:
// every CheckNormalPermission evaluates the request with that subject as well
// (see checkAsterisk in the permission package), so those grants apply to
// every non-root user - e.g. the default public instances are granted to
// user__-1 and must stay visible for everybody.
type UserViewSummary struct {
	root bool
	// instG3Sub[iid][subResource]: instance-wide grant(g3) of the user. Such a
	// grant passes every lower-level(db/table) check as well.
	instG3Sub map[int]map[string]bool
	// instStarSub[iid][subResource]: grant through a g rule whose domain is
	// "*". Visible at instance level only.
	instStarSub map[int]map[string]bool
	// instDomSub[iid][subResource]: grant through a g rule with a typed
	// domain(BaseDatabase__id / BaseTable__id) whose role belongs to instance
	// iid. Makes the instance visible; the specific db/table is tracked in
	// dbSub/tblSub.
	instDomSub map[int]map[string]bool
	// dbSub[did][iid][subResource]: view granted on database did(of instance
	// iid) through a domain-scoped g rule.
	dbSub map[int]map[int]map[string]bool
	// tblSub[tid][iid][subResource]: view granted on table tid(of instance
	// iid) through a domain-scoped g rule.
	tblSub map[int]map[int]map[string]bool
}

var (
	summaryCacheMu   sync.RWMutex
	summaryCacheVer  uint64
	summaryCache     = map[int]*UserViewSummary{}
	viewRulesVersion uint64
)

// bumpViewRulesVersion invalidates every cached UserViewSummary. It must be
// called whenever the enforcer's rules change, otherwise a cached summary could
// keep granting(or hiding) visibility after a grant was added or removed.
func bumpViewRulesVersion() {
	atomic.AddUint64(&viewRulesVersion, 1)
}

// GetUserViewSummary returns the(cached) view summary of the target user.
// The cache is invalidated by bumpViewRulesVersion, which every rule mutation
// path calls(AddRule, DelRule, EnforcerLoadPolicy and the redis watcher).
func GetUserViewSummary(uid int) *UserViewSummary {
	if enforcer == nil {
		return buildUserViewSummary(nil, uid)
	}
	ver := atomic.LoadUint64(&viewRulesVersion)
	summaryCacheMu.RLock()
	cached, hit := summaryCache[uid]
	cachedVer := summaryCacheVer
	summaryCacheMu.RUnlock()
	if hit && cachedVer == ver {
		return cached
	}
	built := buildUserViewSummary(enforcer, uid)
	summaryCacheMu.Lock()
	if summaryCacheVer != ver {
		// rules changed while building: drop the stale generation
		summaryCache = map[int]*UserViewSummary{}
		summaryCacheVer = ver
	}
	summaryCache[uid] = built
	summaryCacheMu.Unlock()
	return built
}

func buildUserViewSummary(e *casbin.Enforcer, uid int) *UserViewSummary {
	s := &UserViewSummary{
		instG3Sub:   make(map[int]map[string]bool),
		instStarSub: make(map[int]map[string]bool),
		instDomSub:  make(map[int]map[string]bool),
		dbSub:       make(map[int]map[int]map[string]bool),
		tblSub:      make(map[int]map[int]map[string]bool),
	}
	if uid <= 0 || e == nil {
		return s
	}
	userStr, err := Assemble2CasbinStr(PrefixUser, strconv.Itoa(uid))
	if err != nil {
		return s
	}
	// Read the user's rules and evaluate the enforces under the lock: the
	// enforcer is shared with every other permission check.
	enforcerLock.Lock()
	defer enforcerLock.Unlock()
	isRoot := len(e.GetFilteredNamedGroupingPolicy(RuleTypeG3, 0, userStr, rootRoleStr)) > 0
	// The wildcard subject user__-1 is evaluated by every CheckNormalPermission
	// (checkAsterisk), so its grants apply to every non-root user: merge its
	// rules into the enumeration below. Without this, a user whose only
	// visibility comes from a user__-1 grant(e.g. the default public
	// instances) would get an empty summary and see nothing.
	g3Rules := copyStringRules(e.GetFilteredNamedGroupingPolicy(RuleTypeG3, 0, userStr))
	g3Rules = append(g3Rules, copyStringRules(e.GetFilteredNamedGroupingPolicy(RuleTypeG3, 0, wildcardUserSubject))...)
	gRules := copyStringRules(e.GetFilteredNamedGroupingPolicy(RuleTypeG, 0, userStr))
	gRules = append(gRules, copyStringRules(e.GetFilteredNamedGroupingPolicy(RuleTypeG, 0, wildcardUserSubject))...)
	if isRoot {
		s.root = true
		return s
	}

	// instances which hold an instance-wide(g3) grant of the user
	instCandidates := make(map[int]bool)
	for _, rule := range g3Rules {
		if iid := instanceIdFromRoleStr(rule[1]); iid >= 0 {
			instCandidates[iid] = true
		}
	}
	for _, subResource := range viewSubResources {
		for iid := range instCandidates {
			if enforceViewFor(e, userStr, iid, subResource, anyDomain) {
				setSubResource(s.instG3Sub, iid, subResource)
			}
		}
		// domain-scoped grants: the g rule's domain tells which db/table(if any)
		// is granted
		for _, rule := range gRules {
			iid := instanceIdFromRoleStr(rule[1])
			if iid < 0 {
				continue
			}
			if rule[2] == anyDomain {
				// a "*" domain only matches the instance-level request
				if enforceViewFor(e, userStr, iid, subResource, anyDomain) {
					setSubResource(s.instStarSub, iid, subResource)
				}
				continue
			}
			domType, domId, err := parseDomainStr(rule[2])
			if err != nil {
				continue
			}
			if !enforceViewFor(e, userStr, iid, subResource, rule[2]) {
				continue
			}
			setSubResource(s.instDomSub, iid, subResource)
			switch domType {
			case PrefixDatabase:
				setSubResource2(s.dbSub, domId, iid, subResource)
			case PrefixTable:
				setSubResource2(s.tblSub, domId, iid, subResource)
			}
		}
	}
	return s
}

// enforceViewFor reports whether userStr may view subResource of instance iid
// under the given domain, evaluating the very same request a
// CheckNormalPermission would build(plus its user__-1 asterisk rule).
func enforceViewFor(e *casbin.Enforcer, userStr string, iid int, subResource, dom string) bool {
	obj, err := Assemble2CasbinStr(PrefixInstance, strconv.Itoa(iid), PrefixSubRsrc, subResource)
	if err != nil {
		return false
	}
	act := JointActs2RuleActStr(ActView)
	rules := [][]interface{}{
		{userStr, obj, act, dom},
		{wildcardUserSubject, obj, act, dom},
	}
	results, err := e.BatchEnforce(rules)
	if err != nil {
		return false
	}
	for _, passed := range results {
		if passed {
			return true
		}
	}
	return false
}

// instanceIdFromRoleStr extracts the instance id encoded at the tail of a role
// string. Both grant systems put it last: System B writes
// role__<roleID>__instance__<refId>(the ref id is the instance id) and System A
// writes role__<name>__instance__<iid>, so the last "__"-separated field is the
// instance id whenever the role is resource-scoped.
func instanceIdFromRoleStr(roleStr string) int {
	if roleStr == "" {
		return -1
	}
	parts := partsOf(roleStr)
	iid, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return -1
	}
	return iid
}

// parseDomainStr splits a domain like "BaseDatabase__7"/"BaseTable__9" into its
// type and numeric id. Domains of any other shape(the "dom*" placeholder of the
// cascader selector, ...) are rejected: they carry no resource id and can never
// be produced by a real grant.
func parseDomainStr(dom string) (domType string, domId int, err error) {
	parts := partsOf(dom)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("domain(%s) is not a <type>__<id> pair", dom)
	}
	domId, err = strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("domain(%s) has a non-numeric id", dom)
	}
	switch parts[0] {
	case PrefixDatabase, PrefixTable:
		return parts[0], domId, nil
	}
	return "", 0, fmt.Errorf("domain(%s) has an unsupported type", dom)
}

func partsOf(s string) []string { return strings.Split(s, SEP) }

func copyStringRules(rules [][]string) [][]string {
	out := make([][]string, len(rules))
	for i, r := range rules {
		out[i] = append([]string(nil), r...)
	}
	return out
}

func setSubResource(m map[int]map[string]bool, id int, subResource string) {
	subs, ok := m[id]
	if !ok {
		subs = make(map[string]bool)
		m[id] = subs
	}
	subs[subResource] = true
}

func setSubResource2(m map[int]map[int]map[string]bool, domId, iid int, subResource string) {
	byInst, ok := m[domId]
	if !ok {
		byInst = make(map[int]map[string]bool)
		m[domId] = byInst
	}
	setSubResource(byInst, iid, subResource)
}

func anySubVisible(m map[int]map[string]bool, id int) bool {
	subs, ok := m[id]
	if !ok {
		return false
	}
	for _, sub := range viewSubResources {
		if subs[sub] {
			return true
		}
	}
	return false
}

// InstanceSubVisible reports whether the user may view subResource of instance
// iid, either instance-wide or through any domain-scoped grant of it(mirrors
// service.InstanceViewPmsWithSubResource).
func (s *UserViewSummary) InstanceSubVisible(iid int, subResource string) bool {
	if s.root {
		return true
	}
	for _, m := range []map[int]map[string]bool{s.instG3Sub, s.instStarSub, s.instDomSub} {
		if subs, ok := m[iid]; ok && subs[subResource] {
			return true
		}
	}
	return false
}

// InstanceVisible reports whether the user may view any subResource of instance
// iid(mirrors service.InstanceViewIsPermission).
func (s *UserViewSummary) InstanceVisible(iid int) bool {
	if s.root {
		return true
	}
	for _, m := range []map[int]map[string]bool{s.instG3Sub, s.instStarSub, s.instDomSub} {
		if anySubVisible(m, iid) {
			return true
		}
	}
	return false
}

// AnyInstanceSub reports whether the user may view subResource on at least one
// instance(mirrors the menu flags of permission.MenuList).
func (s *UserViewSummary) AnyInstanceSub(subResource string) bool {
	if s.root {
		return true
	}
	for _, m := range []map[int]map[string]bool{s.instG3Sub, s.instStarSub, s.instDomSub} {
		for _, subs := range m {
			if subs[subResource] {
				return true
			}
		}
	}
	return false
}

// DatabaseSubResourceVisible reports whether the user may view subResource of
// database did(of instance iid) through the database itself. Only an
// instance-wide(g3) grant or a grant scoped to this very database counts:
// a grant on another database of the same instance does NOT, mirroring the old
// databaseViewIsPermission before its table cascade.
func (s *UserViewSummary) DatabaseSubResourceVisible(iid, did int, subResource string) bool {
	if s.root {
		return true
	}
	if subs, ok := s.instG3Sub[iid]; ok && subs[subResource] {
		return true
	}
	byInst, ok := s.dbSub[did]
	if !ok {
		return false
	}
	subs, ok := byInst[iid]
	return ok && subs[subResource]
}

// DatabaseSubVisible is DatabaseSubResourceVisible for any view subResource.
// Note: table-scoped grants of this database are NOT included, the caller
// resolves those through GrantedTableIds(mirrors the old DatabaseViewIsPermission
// without its per-table cascade).
func (s *UserViewSummary) DatabaseSubVisible(iid, did int) bool {
	if s.root {
		return true
	}
	if anySubVisible(s.instG3Sub, iid) {
		return true
	}
	byInst, ok := s.dbSub[did]
	if !ok {
		return false
	}
	return anySubVisible(byInst, iid)
}

// TableSubResourceVisible reports whether the user may view subResource of
// table tid(of instance iid), either instance-wide or through a grant scoped to
// this very table(mirrors the old tableViewIsPermission).
func (s *UserViewSummary) TableSubResourceVisible(iid, tid int, subResource string) bool {
	if s.root {
		return true
	}
	if subs, ok := s.instG3Sub[iid]; ok && subs[subResource] {
		return true
	}
	byInst, ok := s.tblSub[tid]
	if !ok {
		return false
	}
	subs, ok := byInst[iid]
	return ok && subs[subResource]
}

// TableSubVisible is TableSubResourceVisible for any view subResource(mirrors
// service.TableViewIsPermission).
func (s *UserViewSummary) TableSubVisible(iid, tid int) bool {
	if s.root {
		return true
	}
	if anySubVisible(s.instG3Sub, iid) {
		return true
	}
	byInst, ok := s.tblSub[tid]
	if !ok {
		return false
	}
	return anySubVisible(byInst, iid)
}

// GrantedTableIds returns the table ids on which the user holds a table-scoped
// grant for instance iid. Callers use it to run the(now single) table lookup
// which decides whether a database is visible through one of its tables.
func (s *UserViewSummary) GrantedTableIds(iid int) (tids []int) {
	for tid, byInst := range s.tblSub {
		if anySubVisible(byInst, iid) {
			tids = append(tids, tid)
		}
	}
	return tids
}
