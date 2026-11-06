package windowutil

import "testing"

func TestAccentColorHex(t *testing.T) {
	cases := []struct {
		name  string
		value uint32
		want  string
	}{
		{"windows default blue", 0xC4D47800, "#0078d4"},
		{"teal", 0x00C4D478, "#78d4c4"},
		{"black", 0x00000000, "#000000"},
		{"white with alpha", 0xFFFFFFFF, "#ffffff"},
		{"alpha ignored", 0x805A3C2B, "#2b3c5a"},
		{"bytes in RR GG BB order", 0x00112233, "#332211"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AccentColorHex(tc.value); got != tc.want {
				t.Fatalf("AccentColorHex(%#08x) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}
