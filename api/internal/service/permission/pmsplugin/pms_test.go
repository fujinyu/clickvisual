package pmsplugin

import (
	"reflect"
	"testing"

	db2 "github.com/clickvisual/clickvisual/api/internal/pkg/model/db"
)

func TestGroupDbCasbinRules(t *testing.T) {
	cases := []struct {
		name     string
		dbRules  []db2.PmsCasbinRule
		reqDom   string
		expected []EnhancedCasbinRulesItem
	}{
		{
			name:     "empty input",
			dbRules:  nil,
			expected: []EnhancedCasbinRulesItem{},
		},
		{
			name: "g3 rules are grouped and keep 2 tokens",
			dbRules: []db2.PmsCasbinRule{
				{Ptype: RuleTypeG3, V0: "user__3", V1: "role__read__instance__0"},
				{Ptype: RuleTypeG3, V0: "user__4", V1: "role__read__instance__0"},
			},
			expected: []EnhancedCasbinRulesItem{
				{Ptype: RuleTypeG3, Rules: [][]string{
					{"user__3", "role__read__instance__0"},
					{"user__4", "role__read__instance__0"},
				}},
			},
		},
		{
			name: "g rules are grouped and keep 3 tokens",
			dbRules: []db2.PmsCasbinRule{
				{Ptype: RuleTypeG, V0: "user__3", V1: "role__read__instance__5", V2: "BaseDatabase__7"},
			},
			expected: []EnhancedCasbinRulesItem{
				{Ptype: RuleTypeG, Rules: [][]string{
					{"user__3", "role__read__instance__5", "BaseDatabase__7"},
				}},
			},
		},
		{
			name: "g and g3 are separated, g comes first",
			dbRules: []db2.PmsCasbinRule{
				{Ptype: RuleTypeG3, V0: "user__4", V1: "role__read__instance__0"},
				{Ptype: RuleTypeG, V0: "user__3", V1: "role__read__instance__5", V2: "BaseDatabase__7"},
			},
			expected: []EnhancedCasbinRulesItem{
				{Ptype: RuleTypeG, Rules: [][]string{
					{"user__3", "role__read__instance__5", "BaseDatabase__7"},
				}},
				{Ptype: RuleTypeG3, Rules: [][]string{
					{"user__4", "role__read__instance__0"},
				}},
			},
		},
		{
			name: "legacy empty-domain g rule is still visible(kept for cleanup)",
			dbRules: []db2.PmsCasbinRule{
				{Ptype: RuleTypeG, V0: "user__[3]", V1: "role__read__instance__0", V2: ""},
			},
			expected: []EnhancedCasbinRulesItem{
				{Ptype: RuleTypeG, Rules: [][]string{
					{"user__[3]", "role__read__instance__0", ""},
				}},
			},
		},
		{
			name: "reqDom filters out unmatched g rules, g3 is never filtered",
			dbRules: []db2.PmsCasbinRule{
				{Ptype: RuleTypeG, V0: "user__3", V1: "role__read__instance__5", V2: "BaseDatabase__7"},
				{Ptype: RuleTypeG, V0: "user__4", V1: "role__read__instance__5", V2: "BaseDatabase__8"},
				{Ptype: RuleTypeG3, V0: "user__5", V1: "role__read__instance__0"},
			},
			reqDom: "BaseDatabase__7",
			expected: []EnhancedCasbinRulesItem{
				{Ptype: RuleTypeG, Rules: [][]string{
					{"user__3", "role__read__instance__5", "BaseDatabase__7"},
				}},
				{Ptype: RuleTypeG3, Rules: [][]string{
					{"user__5", "role__read__instance__0"},
				}},
			},
		},
		{
			name: "reqDom matching all domains via wildcard",
			dbRules: []db2.PmsCasbinRule{
				{Ptype: RuleTypeG, V0: "user__3", V1: "role__read__instance__5", V2: "*"},
			},
			reqDom: "BaseDatabase__7",
			expected: []EnhancedCasbinRulesItem{
				{Ptype: RuleTypeG, Rules: [][]string{
					{"user__3", "role__read__instance__5", "*"},
				}},
			},
		},
		{
			name: "other ptypes are ignored",
			dbRules: []db2.PmsCasbinRule{
				{Ptype: RuleTypeP, V0: "role__read__instance__5", V1: "instance__5__subResource__*", V2: "view", V3: "*"},
			},
			expected: []EnhancedCasbinRulesItem{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := groupDbCasbinRules(c.dbRules, c.reqDom)
			if !reflect.DeepEqual(got, c.expected) {
				t.Errorf("groupDbCasbinRules() = %v, want %v", got, c.expected)
			}
		})
	}
}

func TestIsEmptyDomParam(t *testing.T) {
	cases := []struct {
		name     string
		dom      interface{}
		expected bool
	}{
		{name: "empty string", dom: "", expected: true},
		{name: "whitespace only", dom: "   ", expected: true},
		{name: "non-empty domain", dom: "BaseDatabase__7", expected: false},
		{name: "wildcard domain is not empty", dom: "*", expected: false},
		{name: "non-string param", dom: 0, expected: false},
		{name: "nil param", dom: nil, expected: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isEmptyDomParam(c.dom); got != c.expected {
				t.Errorf("isEmptyDomParam(%v) = %v, want %v", c.dom, got, c.expected)
			}
		})
	}
}

// TestCmpUserIds2GetNewAndRmUserIds covers the diff which the role-page grant
// (GrantRoles2User) relies on: needAdd = future \ current, needRm = current \ future.
func TestCmpUserIds2GetNewAndRmUserIds(t *testing.T) {
	cases := []struct {
		name        string
		current     []int
		future      []int
		wantEqual   bool
		wantAdd     []int
		wantRm      []int
	}{
		{
			name: "both empty", current: []int{}, future: []int{},
			wantEqual: true, wantAdd: []int{}, wantRm: []int{},
		},
		{
			name: "same roster", current: []int{1, 2}, future: []int{1, 2},
			wantEqual: true, wantAdd: []int{}, wantRm: []int{},
		},
		{
			name: "add one user", current: []int{1}, future: []int{1, 3},
			wantEqual: false, wantAdd: []int{3}, wantRm: []int{},
		},
		{
			name: "remove one user", current: []int{1, 2}, future: []int{1},
			wantEqual: false, wantAdd: []int{}, wantRm: []int{2},
		},
		{
			name: "replace all users", current: []int{1, 2}, future: []int{3, 4},
			wantEqual: false, wantAdd: []int{3, 4}, wantRm: []int{1, 2},
		},
		{
			name: "clear the roster", current: []int{1, 2}, future: []int{},
			wantEqual: false, wantAdd: []int{}, wantRm: []int{1, 2},
		},
		{
			name: "grant on an empty roster", current: []int{}, future: []int{1, 2},
			wantEqual: false, wantAdd: []int{1, 2}, wantRm: []int{},
		},
		{
			name: "duplicated future uids", current: []int{1}, future: []int{1, 1},
			wantEqual: true, wantAdd: []int{}, wantRm: []int{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isEqual, needAdd, needRm := CmpUserIds2GetNewAndRmUserIds(c.current, c.future)
			if isEqual != c.wantEqual {
				t.Errorf("isEqual = %v, want %v", isEqual, c.wantEqual)
			}
			if !reflect.DeepEqual(needAdd, c.wantAdd) {
				t.Errorf("needAdd = %v, want %v", needAdd, c.wantAdd)
			}
			// needRm is collected from a map, so its order is not deterministic
			if len(needRm) != len(c.wantRm) {
				t.Errorf("needRm = %v, want %v", needRm, c.wantRm)
				return
			}
			rmMap := make(map[int]struct{}, len(needRm))
			for _, uid := range needRm {
				rmMap[uid] = struct{}{}
			}
			for _, uid := range c.wantRm {
				if _, ok := rmMap[uid]; !ok {
					t.Errorf("needRm = %v, want %v", needRm, c.wantRm)
					return
				}
			}
		})
	}
}
