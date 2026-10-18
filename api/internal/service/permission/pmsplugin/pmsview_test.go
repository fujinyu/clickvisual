package pmsplugin

import (
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

// newTestEnforcer builds an in-memory enforcer with the real rbac model, so the
// summary is exercised against the very matcher production uses.
func newTestEnforcer(t *testing.T) *casbin.Enforcer {
	t.Helper()
	content, err := os.ReadFile("../../../../../config/rbac.conf")
	if err != nil {
		t.Skipf("rbac.conf not readable: %v", err)
	}
	m, err := model.NewModelFromString(string(content))
	if err != nil {
		t.Fatalf("NewModelFromString: %v", err)
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		t.Fatalf("NewEnforcer: %v", err)
	}
	return e
}

func addP(t *testing.T, e *casbin.Enforcer, iid int, subResource, act string) {
	t.Helper()
	addPRole(t, e, "7", iid, subResource, act)
}

func addPRole(t *testing.T, e *casbin.Enforcer, roleId string, iid int, subResource, act string) {
	t.Helper()
	role, _ := Assemble2CasbinStr(PrefixRole, roleId, PrefixInstance, strconv.Itoa(iid))
	obj, _ := Assemble2CasbinStr(PrefixInstance, strconv.Itoa(iid), PrefixSubRsrc, subResource)
	if _, err := e.AddPolicy(role, obj, act, anyDomain); err != nil {
		t.Fatalf("AddPolicy: %v", err)
	}
}

func addG3(t *testing.T, e *casbin.Enforcer, uid int, role string) {
	t.Helper()
	user, _ := Assemble2CasbinStr(PrefixUser, strconv.Itoa(uid))
	if _, err := e.AddNamedGroupingPolicy(RuleTypeG3, user, role); err != nil {
		t.Fatalf("AddNamedGroupingPolicy g3: %v", err)
	}
}

func addG(t *testing.T, e *casbin.Enforcer, uid int, role, dom string) {
	t.Helper()
	user, _ := Assemble2CasbinStr(PrefixUser, strconv.Itoa(uid))
	if _, err := e.AddNamedGroupingPolicy(RuleTypeG, user, role, dom); err != nil {
		t.Fatalf("AddNamedGroupingPolicy g: %v", err)
	}
}

// instanceWideRules: user 3 holds role 7 on instance 5 with Log(view)+Alarm(edit).
func instanceWideRules(t *testing.T) *casbin.Enforcer {
	e := newTestEnforcer(t)
	addP(t, e, 5, Log, ActView)
	addP(t, e, 5, Alarm, ActEdit)
	addG3(t, e, 3, "role__7__instance__5")
	return e
}

func TestUserViewSummaryInstanceWideGrant(t *testing.T) {
	e := instanceWideRules(t)
	s := buildUserViewSummary(e, 3)

	if !s.InstanceSubVisible(5, Log) || !s.InstanceSubVisible(5, Alarm) {
		t.Errorf("instance 5 Log/Alarm should be visible, got %v/%v",
			s.InstanceSubVisible(5, Log), s.InstanceSubVisible(5, Alarm))
	}
	if s.InstanceSubVisible(5, Pandas) {
		t.Error("instance 5 Pandas should NOT be visible(no p rule)")
	}
	if s.InstanceSubVisible(6, Log) {
		t.Error("instance 6 should NOT be visible(no grant)")
	}
	if !s.InstanceVisible(5) || s.InstanceVisible(6) {
		t.Errorf("InstanceVisible: inst5=%v inst6=%v, want true/false",
			s.InstanceVisible(5), s.InstanceVisible(6))
	}
	if !s.AnyInstanceSub(Log) || !s.AnyInstanceSub(Alarm) || s.AnyInstanceSub(Pandas) {
		t.Errorf("AnyInstanceSub: log=%v alarm=%v pandas=%v, want true/true/false",
			s.AnyInstanceSub(Log), s.AnyInstanceSub(Alarm), s.AnyInstanceSub(Pandas))
	}
	// an instance-wide grant subsumes every lower level(db/table) check
	if !s.DatabaseSubVisible(5, 7) || !s.TableSubVisible(5, 99) {
		t.Error("instance-wide grant must imply database/table visibility")
	}
	if ids := s.GrantedTableIds(5); len(ids) != 0 {
		t.Errorf("GrantedTableIds(5) = %v, want empty(no domain-scoped grant)", ids)
	}
	// other users see nothing
	other := buildUserViewSummary(e, 4)
	if other.InstanceVisible(5) || other.AnyInstanceSub(Log) {
		t.Error("user 4 has no grant and must not see instance 5")
	}
}

func TestUserViewSummaryDatabaseScopedGrant(t *testing.T) {
	e := newTestEnforcer(t)
	addP(t, e, 5, Log, ActView)
	addG(t, e, 3, "role__7__instance__5", "BaseDatabase__7")
	s := buildUserViewSummary(e, 3)

	// the instance itself becomes visible(the old cascade returned true here too,
	// because the db-level enforce of one of its databases passes)
	if !s.InstanceVisible(5) || !s.InstanceSubVisible(5, Log) {
		t.Error("a db-scoped grant must make the instance visible")
	}
	if !s.DatabaseSubVisible(5, 7) {
		t.Error("database 7 must be visible(granted)")
	}
	if s.DatabaseSubVisible(5, 8) {
		t.Error("database 8 must NOT be visible(not granted)")
	}
	if s.DatabaseSubVisible(6, 7) {
		t.Error("database 7 must NOT be visible on instance 6(grant belongs to instance 5)")
	}
	if s.TableSubVisible(5, 9) {
		t.Error("no table was granted, TableSubVisible must be false")
	}
	if !s.AnyInstanceSub(Log) {
		t.Error("AnyInstanceSub(Log) must be true")
	}
}

func TestUserViewSummaryTableScopedGrant(t *testing.T) {
	e := newTestEnforcer(t)
	addP(t, e, 5, Log, ActView)
	addG(t, e, 3, "role__7__instance__5", "BaseTable__9")
	s := buildUserViewSummary(e, 3)

	if !s.TableSubVisible(5, 9) {
		t.Error("table 9 must be visible(granted)")
	}
	if !s.TableSubResourceVisible(5, 9, Log) {
		t.Error("table 9 Log must be visible")
	}
	if s.TableSubVisible(5, 10) || s.TableSubVisible(6, 9) {
		t.Error("tables other than the granted one must NOT be visible")
	}
	// the database itself is not granted: only reachable through its tables,
	// which the caller resolves with GrantedTableIds
	if s.DatabaseSubVisible(5, 7) {
		t.Error("database 7 must NOT be visible by itself(only a table was granted)")
	}
	if ids := s.GrantedTableIds(5); !reflect.DeepEqual(ids, []int{9}) {
		t.Errorf("GrantedTableIds(5) = %v, want [9]", ids)
	}
	// ...but the instance shows up in menus/lists, exactly like the old cascade
	if !s.InstanceVisible(5) {
		t.Error("a table-scoped grant must make the instance visible")
	}
}

func TestUserViewSummaryNoRules(t *testing.T) {
	e := newTestEnforcer(t)
	addP(t, e, 5, Log, ActView) // the role exists, but user 3 holds no grant
	s := buildUserViewSummary(e, 3)

	if s.InstanceVisible(5) || s.InstanceSubVisible(5, Log) {
		t.Error("user without any grant must not see anything")
	}
	if s.AnyInstanceSub(Log) || s.AnyInstanceSub(Alarm) || s.AnyInstanceSub(Pandas) {
		t.Error("AnyInstanceSub must be false without grants")
	}
	if s.DatabaseSubVisible(5, 7) || s.TableSubVisible(5, 9) {
		t.Error("db/table visibility must be false without grants")
	}
	if ids := s.GrantedTableIds(5); len(ids) != 0 {
		t.Errorf("GrantedTableIds = %v, want empty", ids)
	}
}

func TestUserViewSummaryRootUser(t *testing.T) {
	e := newTestEnforcer(t)
	addP(t, e, 5, Log, ActView)
	addG3(t, e, 3, "role__7__instance__5")
	addG3(t, e, 1, "role__root") // root: no p rules needed
	s := buildUserViewSummary(e, 1)

	if !s.InstanceVisible(5) || !s.InstanceVisible(999) {
		t.Error("root must see every instance")
	}
	if !s.AnyInstanceSub(Log) || !s.AnyInstanceSub(Pandas) {
		t.Error("root must have every subResource flag")
	}
	if !s.DatabaseSubVisible(5, 7) || !s.TableSubVisible(5, 9) {
		t.Error("root must see every database/table")
	}
}

func TestUserViewSummaryActVariants(t *testing.T) {
	cases := []struct {
		name string
		act  string
		want bool
	}{
		{"view act grants view", ActView, true},
		{"edit act implies view", ActEdit, true},
		{"full act grants view", ActFull, true},
		{"view|edit act grants view", "view|edit", true},
		{"delete act does not grant view", ActDelete, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEnforcer(t)
			addP(t, e, 5, Log, c.act)
			addG3(t, e, 3, "role__7__instance__5")
			s := buildUserViewSummary(e, 3)
			if got := s.InstanceSubVisible(5, Log); got != c.want {
				t.Errorf("InstanceSubVisible(5, Log) = %v, want %v", got, c.want)
			}
		})
	}
}

func TestUserViewSummaryMultipleInstancesIsolation(t *testing.T) {
	e := newTestEnforcer(t)
	addP(t, e, 5, Log, ActView)
	addPRole(t, e, "8", 6, Log, ActView)
	addG(t, e, 3, "role__7__instance__5", "BaseDatabase__7")
	addG(t, e, 3, "role__8__instance__6", "BaseDatabase__8")
	s := buildUserViewSummary(e, 3)

	if !s.DatabaseSubVisible(5, 7) {
		t.Error("database 7 of instance 5 must be visible")
	}
	if s.DatabaseSubVisible(5, 8) {
		t.Error("database 8 belongs to instance 6 and must not be visible on instance 5")
	}
	if !s.DatabaseSubVisible(6, 8) || s.DatabaseSubVisible(6, 7) {
		t.Error("per-instance database isolation broken")
	}
	if !s.InstanceVisible(5) || !s.InstanceVisible(6) {
		t.Error("both granted instances must be visible")
	}
}

func TestUserViewSummaryStarDomainGrant(t *testing.T) {
	e := newTestEnforcer(t)
	addP(t, e, 5, Log, ActView)
	// a g rule whose domain is "*": casbin's g matches all three elements
	// exactly, so such a grant only satisfies the instance-level request
	addG(t, e, 3, "role__7__instance__5", anyDomain)
	s := buildUserViewSummary(e, 3)

	if !s.InstanceVisible(5) || !s.InstanceSubVisible(5, Log) {
		t.Error("a *-domain grant must make the instance visible")
	}
	if !s.AnyInstanceSub(Log) {
		t.Error("AnyInstanceSub(Log) must be true")
	}
	if s.DatabaseSubVisible(5, 7) || s.TableSubVisible(5, 9) {
		t.Error("a *-domain grant must NOT grant any database/table visibility")
	}
	if ids := s.GrantedTableIds(5); len(ids) != 0 {
		t.Errorf("GrantedTableIds(5) = %v, want empty", ids)
	}
}

// TestUserViewSummaryWildcardUserInstanceGrant reproduces the production
// incident: the default public instances are granted to the wildcard subject
// user__-1, and every CheckNormalPermission evaluates the request with that
// subject too(checkAsterisk). A user without any personal rule must still see
// them - otherwise menus/list returns [] and the UI shows a blank page.
func TestUserViewSummaryWildcardUserInstanceGrant(t *testing.T) {
	e := newTestEnforcer(t)
	addPRole(t, e, "9", 1, Log, ActView)
	addPRole(t, e, "9", 6, Log, ActView)
	addG3(t, e, -1, "role__9__instance__1") // user__-1 -> instance 1
	addG3(t, e, -1, "role__9__instance__6") // user__-1 -> instance 6

	s := buildUserViewSummary(e, 68) // a user with no personal rules at all

	if !s.InstanceSubVisible(1, Log) || !s.InstanceSubVisible(6, Log) {
		t.Error("instances granted to user__-1 must be visible for every user")
	}
	if !s.InstanceVisible(1) || !s.InstanceVisible(6) {
		t.Error("InstanceVisible must be true for wildcard-granted instances")
	}
	if !s.AnyInstanceSub(Log) {
		t.Error("AnyInstanceSub(Log) must be true(menu flags depend on it)")
	}
	if s.InstanceVisible(2) || s.InstanceSubVisible(2, Log) {
		t.Error("instance 2 has no wildcard grant and must stay invisible")
	}
	// an instance-wide grant subsumes every lower level check
	if !s.DatabaseSubVisible(1, 7) || !s.TableSubVisible(6, 9) {
		t.Error("wildcard instance-wide grant must imply db/table visibility")
	}
}

// TestUserViewSummaryWildcardUserDomainGrant covers a domain-scoped grant held
// by the wildcard subject: it must make exactly that database visible for
// every user, and nothing else.
func TestUserViewSummaryWildcardUserDomainGrant(t *testing.T) {
	e := newTestEnforcer(t)
	addPRole(t, e, "10", 2, Log, ActView)
	addG(t, e, -1, "role__10__instance__2", "BaseDatabase__7") // user__-1 -> db 7 of instance 2

	s := buildUserViewSummary(e, 68)

	if !s.InstanceVisible(2) || !s.InstanceSubVisible(2, Log) {
		t.Error("a wildcard db-scoped grant must make the instance visible")
	}
	if !s.DatabaseSubVisible(2, 7) || !s.DatabaseSubResourceVisible(2, 7, Log) {
		t.Error("database 7 must be visible through the wildcard grant")
	}
	if s.DatabaseSubVisible(2, 8) || s.DatabaseSubVisible(3, 7) {
		t.Error("only the granted database of the granted instance may be visible")
	}
	if s.TableSubVisible(2, 9) {
		t.Error("no table was granted, TableSubVisible must be false")
	}
	if ids := s.GrantedTableIds(2); len(ids) != 0 {
		t.Errorf("GrantedTableIds(2) = %v, want empty", ids)
	}
}

func TestInstanceIdFromRoleStr(t *testing.T) {
	cases := []struct {
		role string
		want int
	}{
		{"role__7__instance__5", 5},   // System B: role__<id>__instance__<refId>
		{"role__read__instance__12", 12}, // System A: role__<name>__instance__<iid>
		{"role__a__b__instance__3", 3},   // name containing the separator
		{"role__root", -1},               // root role is not resource-scoped
		{"", -1},
		{"role__x__instance__abc", -1},
	}
	for _, c := range cases {
		if got := instanceIdFromRoleStr(c.role); got != c.want {
			t.Errorf("instanceIdFromRoleStr(%q) = %d, want %d", c.role, got, c.want)
		}
	}
}

func TestParseDomainStr(t *testing.T) {
	cases := []struct {
		dom     string
		wantTyp string
		wantId  int
		wantErr bool
	}{
		{"BaseDatabase__7", PrefixDatabase, 7, false},
		{"BaseTable__9", PrefixTable, 9, false},
		{"*", "", 0, true},
		{"dom*", "", 0, true},
		{"BaseDatabase__abc", "", 0, true},
		{"SomethingElse__3", "", 0, true},
		{"BaseDatabase__7__8", "", 0, true},
	}
	for _, c := range cases {
		typ, id, err := parseDomainStr(c.dom)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseDomainStr(%q) should fail", c.dom)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDomainStr(%q) unexpected error: %v", c.dom, err)
			continue
		}
		if typ != c.wantTyp || id != c.wantId {
			t.Errorf("parseDomainStr(%q) = (%s, %d), want (%s, %d)", c.dom, typ, id, c.wantTyp, c.wantId)
		}
	}
}
