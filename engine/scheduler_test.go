package engine

import (
	"testing"

	"learning-runtime/db"
)

func TestIsSafeWebhookURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"https discord.com", "https://discord.com/api/webhooks/123/abc", true},
		{"https ptb.discord.com", "https://ptb.discord.com/api/webhooks/123/abc", true},
		{"https discordapp.com", "https://discordapp.com/api/webhooks/123/abc", true},
		{"http discord.com rejected", "http://discord.com/api/webhooks/123/abc", false},
		{"https evil.com rejected", "https://evil.com/api/webhooks/123/abc", false},
		{"https IMDS rejected", "https://169.254.169.254/latest/meta-data/", false},
		{"empty rejected", "", false},
		{"suffix spoof rejected", "https://evildiscord.com/x", false},
		{"double dot rejected", "https://discord..com/x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := db.IsSafeWebhookURL(tc.url); got != tc.want {
				t.Errorf("IsSafeWebhookURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}
