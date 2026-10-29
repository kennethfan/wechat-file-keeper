package cli

import "testing"

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"512", 512},
		{"1KB", 1024},
		{"1kb", 1024},
		{"100MB", 100 * 1024 * 1024},
		{"100mb", 100 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"1.5GB", int64(1.5 * 1024 * 1024 * 1024)},
		{"2048B", 2048},
		{"0", 0},
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if err != nil {
			t.Errorf("parseSize(%q) 报错: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSize(%q) = %d，期望 %d", c.in, got, c.want)
		}
	}
}

func TestParseSize_RejectsInvalid(t *testing.T) {
	for _, in := range []string{"abc", "-1MB", "MB"} {
		if _, err := parseSize(in); err == nil {
			t.Errorf("parseSize(%q) 应报错，实际未报", in)
		}
	}
}
