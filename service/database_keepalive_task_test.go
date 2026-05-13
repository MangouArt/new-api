package service

import (
	"testing"
	"time"
)

func TestDatabaseKeepaliveIntervalFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want time.Duration
	}{
		{name: "empty uses default", env: "", want: databaseKeepaliveDefaultInterval},
		{name: "valid duration", env: "30m", want: 30 * time.Minute},
		{name: "trimmed duration", env: " 2h ", want: 2 * time.Hour},
		{name: "invalid duration uses default", env: "daily", want: databaseKeepaliveDefaultInterval},
		{name: "zero duration uses default", env: "0s", want: databaseKeepaliveDefaultInterval},
		{name: "negative duration uses default", env: "-1h", want: databaseKeepaliveDefaultInterval},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := databaseKeepaliveIntervalFromEnv(tt.env)
			if got != tt.want {
				t.Fatalf("databaseKeepaliveIntervalFromEnv(%q) = %s, want %s", tt.env, got, tt.want)
			}
		})
	}
}
