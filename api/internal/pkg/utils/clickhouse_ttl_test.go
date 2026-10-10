package utils

import "testing"

func TestBuildTTLClause(t *testing.T) {
	tests := []struct {
		name       string
		timeField  string
		coldVolume string
		hotDays    int
		totalDays  int
		want       string
		wantErr    bool
	}{
		{
			name:      "not enabled: hot=0 falls back to single-layer TTL",
			timeField: "",
			hotDays:   0,
			totalDays: 30,
			want:      "TTL toDateTime(_time_second_) + INTERVAL 30 DAY",
		},
		{
			name:       "hot==total treated as single-layer (user rule: same value means hot-only)",
			timeField:  "",
			coldVolume: "cold",
			hotDays:    30,
			totalDays:  30,
			want:       "TTL toDateTime(_time_second_) + INTERVAL 30 DAY",
		},
		{
			name:       "hot>total treated as single-layer",
			coldVolume: "cold",
			hotDays:    40,
			totalDays:  30,
			want:       "TTL toDateTime(_time_second_) + INTERVAL 30 DAY",
		},
		{
			name:       "two-layer hot_to_cold exact match with user example",
			timeField:  "_time_second_",
			coldVolume: "cold",
			hotDays:    15,
			totalDays:  29,
			want: "TTL toDateTime(_time_second_) + INTERVAL 15 DAY TO VOLUME 'cold', " +
				"toDateTime(_time_second_) + INTERVAL 29 DAY",
		},
		{
			name:       "custom time field",
			timeField:  "ts",
			coldVolume: "cold",
			hotDays:    7,
			totalDays:  30,
			want: "TTL toDateTime(ts) + INTERVAL 7 DAY TO VOLUME 'cold', " +
				"toDateTime(ts) + INTERVAL 30 DAY",
		},
		{
			name:      "coldVolume required when 0<hot<total",
			timeField: "_time_second_",
			hotDays:   15,
			totalDays: 30,
			wantErr:   true,
		},
		{
			name:      "total<=0 invalid",
			totalDays: 0,
			wantErr:   true,
		},
		{
			name:      "hot<0 invalid",
			hotDays:   -1,
			totalDays: 30,
			wantErr:   true,
		},
		{
			name:       "coldVolume single quote must be escaped",
			coldVolume: "cold's",
			hotDays:    10,
			totalDays:  30,
			want: "TTL toDateTime(_time_second_) + INTERVAL 10 DAY TO VOLUME 'cold''s', " +
				"toDateTime(_time_second_) + INTERVAL 30 DAY",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildTTLClause(tt.timeField, tt.coldVolume, tt.hotDays, tt.totalDays)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BuildTTLClause() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("BuildTTLClause()\n got = %q\nwant = %q", got, tt.want)
			}
		})
	}
}

func TestBuildSettingsClause(t *testing.T) {
	tests := []struct {
		name          string
		storagePolicy string
		want          string
		wantErr       bool
	}{
		{
			name:          "empty policy falls back to defaults",
			storagePolicy: "",
			want:          "SETTINGS index_granularity = 8192",
		},
		{
			name:          "hot_to_cold policy prepended",
			storagePolicy: "hot_to_cold",
			want:          "SETTINGS storage_policy = 'hot_to_cold', index_granularity = 8192",
		},
		{
			name:          "policy with single quote must be escaped",
			storagePolicy: "a'b",
			want:          "SETTINGS storage_policy = 'a''b', index_granularity = 8192",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildSettingsClause(tt.storagePolicy)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BuildSettingsClause() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("BuildSettingsClause()\n got = %q\nwant = %q", got, tt.want)
			}
		})
	}
}
