package safety

import (
	"testing"
)

func Test_Filter_IsAllowed_Cases(t *testing.T) {
	tests := []struct {
		name      string
		allowlist []string
		denylist  []string
		resource  string
		want      bool
	}{
		{
			name:      "empty lists allow everything",
			allowlist: []string{},
			denylist:  []string{},
			resource:  "anything",
			want:      true,
		},
		{
			name:      "nil lists allow everything",
			allowlist: nil,
			denylist:  nil,
			resource:  "anything",
			want:      true,
		},
		{
			name:      "in allowlist is allowed",
			allowlist: []string{"plex", "sonarr"},
			denylist:  []string{},
			resource:  "plex",
			want:      true,
		},
		{
			name:      "not in allowlist is denied",
			allowlist: []string{"plex", "sonarr"},
			denylist:  []string{},
			resource:  "radarr",
			want:      false,
		},
		{
			name:      "in denylist is denied",
			allowlist: []string{},
			denylist:  []string{"portainer"},
			resource:  "portainer",
			want:      false,
		},
		{
			name:      "denylist wins over allowlist",
			allowlist: []string{"plex", "portainer"},
			denylist:  []string{"portainer"},
			resource:  "portainer",
			want:      false,
		},
		{
			name:      "glob pattern in denylist matches",
			allowlist: []string{},
			denylist:  []string{"*backup*"},
			resource:  "nightly-backup-db",
			want:      false,
		},
		{
			name:      "glob pattern in allowlist matches",
			allowlist: []string{"plex*"},
			denylist:  []string{},
			resource:  "plex-media",
			want:      true,
		},
		{
			name:      "glob pattern no match in allowlist",
			allowlist: []string{"plex*"},
			denylist:  []string{},
			resource:  "sonarr",
			want:      false,
		},
		{
			name:      "glob denylist takes priority over glob allowlist",
			allowlist: []string{"*media*"},
			denylist:  []string{"*backup*"},
			resource:  "media-backup-service",
			want:      false,
		},
		{
			name:      "exact match in denylist with glob allowlist",
			allowlist: []string{"*"},
			denylist:  []string{"dangerous"},
			resource:  "dangerous",
			want:      false,
		},
		{
			name:      "wildcard allowlist allows non-denied",
			allowlist: []string{"*"},
			denylist:  []string{"dangerous"},
			resource:  "safe-service",
			want:      true,
		},
		{
			name:      "empty resource name with empty lists",
			allowlist: []string{},
			denylist:  []string{},
			resource:  "",
			want:      true,
		},
		{
			name:      "empty resource name not in allowlist",
			allowlist: []string{"plex"},
			denylist:  []string{},
			resource:  "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilter(tt.allowlist, tt.denylist)
			if f == nil {
				t.Fatal("NewFilter() returned nil")
			}

			got := f.IsAllowed(tt.resource)
			if got != tt.want {
				t.Errorf("IsAllowed(%q) = %v, want %v (allowlist=%v, denylist=%v)",
					tt.resource, got, tt.want, tt.allowlist, tt.denylist)
			}
		})
	}
}

func Test_NewFilter_ReturnsNonNil(t *testing.T) {
	tests := []struct {
		name      string
		allowlist []string
		denylist  []string
	}{
		{name: "both nil", allowlist: nil, denylist: nil},
		{name: "both empty", allowlist: []string{}, denylist: []string{}},
		{name: "populated", allowlist: []string{"a"}, denylist: []string{"b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFilter(tt.allowlist, tt.denylist)
			if f == nil {
				t.Error("NewFilter() should never return nil")
			}
		})
	}
}
