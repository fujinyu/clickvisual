package permission

import (
	"reflect"
	"testing"
)

func TestParseUidFromSubjectStr(t *testing.T) {
	cases := []struct {
		name        string
		subjectStr  string
		expectedUid int
	}{
		{name: "normal user subject", subjectStr: "user__3", expectedUid: 3},
		{name: "legacy slice subject", subjectStr: "user__[3]", expectedUid: 3},
		{name: "legacy multi uid slice", subjectStr: "user__[3", expectedUid: 3},
		{name: "not a user subject", subjectStr: "role__read__instance__0", expectedUid: 0},
		{name: "empty", subjectStr: "", expectedUid: 0},
		{name: "non numeric uid", subjectStr: "user__abc", expectedUid: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseUidFromSubjectStr(c.subjectStr); got != c.expectedUid {
				t.Errorf("parseUidFromSubjectStr(%q) = %d, want %d", c.subjectStr, got, c.expectedUid)
			}
		})
	}
}

func TestDedupeInts(t *testing.T) {
	cases := []struct {
		name     string
		input    []int
		expected []int
	}{
		{name: "no duplication", input: []int{1, 2, 3}, expected: []int{1, 2, 3}},
		{name: "correct and legacy rule of same user", input: []int{3, 3}, expected: []int{3}},
		{name: "keep first-seen order", input: []int{5, 2, 5, 1, 2}, expected: []int{5, 2, 1}},
		{name: "empty", input: []int{}, expected: []int{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dedupeInts(c.input); !reflect.DeepEqual(got, c.expected) {
				t.Errorf("dedupeInts(%v) = %v, want %v", c.input, got, c.expected)
			}
		})
	}
}

func TestLegacyInstanceRoleStr(t *testing.T) {
	cases := []struct {
		name     string
		roleName string
		expected string
	}{
		{name: "normal role name", roleName: "read", expected: "role__read__instance__0"},
		{name: "role name with underscore", roleName: "log_read", expected: "role__log_read__instance__0"},
		{name: "empty role name", roleName: "", expected: "role____instance__0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := legacyInstanceRoleStr(c.roleName); got != c.expected {
				t.Errorf("legacyInstanceRoleStr(%q) = %q, want %q", c.roleName, got, c.expected)
			}
		})
	}
}
