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
