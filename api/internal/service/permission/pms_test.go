package permission

import "testing"

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
