package cmd

import "testing"

func TestSentryBlueTagsFailClosed(t *testing.T) {
	tests := []struct {
		name    string
		primary string
		compat  string
		want    map[string]string
	}{
		{
			name:    "blue identity is tagged",
			primary: "gowa-blue",
			want:    map[string]string{"color": "blue", "service": "gowa-blue"},
		},
		{
			name:    "main stays untagged",
			primary: "gowa-main",
		},
		{
			name: "missing identity stays untagged",
		},
		{
			name:    "invalid identity stays untagged",
			primary: "gowa-canary",
		},
		{
			name:    "conflicting identity stays untagged",
			primary: "gowa-blue",
			compat:  "gowa-main",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOWA_INSTANCE_ID", tt.primary)
			t.Setenv("RETENA_GOWA_INSTANCE_ID", tt.compat)
			got := sentryBlueTags()
			if len(got) != len(tt.want) {
				t.Fatalf("tags = %#v, want %#v", got, tt.want)
			}
			for key, value := range tt.want {
				if got[key] != value {
					t.Fatalf("tags[%q] = %q, want %q", key, got[key], value)
				}
			}
		})
	}
}

func TestSentryServiceTagOnlyUsesBlueIdentity(t *testing.T) {
	t.Setenv("GOWA_INSTANCE_ID", "gowa-blue")
	t.Setenv("RETENA_GOWA_INSTANCE_ID", "")
	if got := sentryServiceTag(); got != "gowa-blue" {
		t.Fatalf("blue service tag = %q", got)
	}
	t.Setenv("GOWA_INSTANCE_ID", "gowa-main")
	if got := sentryServiceTag(); got != "go-whatsapp-multidevice" {
		t.Fatalf("main service tag = %q", got)
	}
}
