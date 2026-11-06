package utils

import (
	"strings"
	"testing"
)

// RET-535-FU: ParseLIDParam must produce a JID that keeps Server "lid" so
// avatar fetches can address the LID identity directly instead of being
// silently resolved back to the phone identity by ValidateJidWithLogin.
func TestParseLIDParam(t *testing.T) {
	valid := map[string]string{
		"123456789012345":      "123456789012345",  // bare 15-digit LID
		"1234567890123456":     "1234567890123456", // bare 16-digit LID
		"123456789012345@lid":  "123456789012345",  // full @lid JID
		" 123456789012345@lid": "123456789012345",  // trimmed @lid JID
	}
	for input, wantUser := range valid {
		jid, err := ParseLIDParam(input)
		if err != nil {
			t.Fatalf("ParseLIDParam(%q) unexpected error: %v", input, err)
		}
		if jid.Server != "lid" {
			t.Fatalf("ParseLIDParam(%q) server = %q, want \"lid\"", input, jid.Server)
		}
		if jid.User != wantUser {
			t.Fatalf("ParseLIDParam(%q) user = %q, want %q", input, jid.User, wantUser)
		}
	}

	invalid := []string{
		"",                               // empty
		"   ",                            // whitespace only
		"+123456789012345",               // leading plus is a phone convention, not a LID
		"12345abcde12345",                // non-digit junk
		"12345",                          // too short
		"123456789012345678901234567890", // too long
		"123456789012345@s.whatsapp.net", // not a @lid JID
		"123456789012345@g.us",           // group JID is not a LID
		"@lid",                           // empty user
		"not-a-jid@lid",                  // non-digit user in @lid JID
	}
	for _, input := range invalid {
		jid, err := ParseLIDParam(input)
		if err == nil {
			t.Fatalf("ParseLIDParam(%q) = %v, want error", input, jid)
		}
		if !jid.IsEmpty() {
			t.Fatalf("ParseLIDParam(%q) returned non-empty JID alongside error: %v", input, jid)
		}
	}

	// The error message must never leak a full JID-shaped value beyond what the
	// caller supplied (it echoes the input by design; assert it stays on one line).
	if _, err := ParseLIDParam("123456789012345@s.whatsapp.net"); err == nil || strings.Count(err.Error(), "\n") != 0 {
		t.Fatalf("expected single-line error for non-lid JID input, got %v", err)
	}
}
