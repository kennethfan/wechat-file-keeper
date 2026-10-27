package format

import "testing"

func TestBytes(t *testing.T) {
	// 用运行时变量避免常量表达式在编译期求值导致的截断报错。
	gb := int64(1024 * 1024 * 1024)
	// 4.77 GB 是本机实测的账号库实占大小。
	fourPointSevenSevenGB := int64(4.77 * float64(gb))

	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{513 * 1024 * 1024, "513.0 MB"},
		{fourPointSevenSevenGB, "4.77 GB"},
		{-1, "0 B"},
	}
	for _, c := range cases {
		if got := Bytes(c.in); got != c.want {
			t.Errorf("Bytes(%d) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

func TestCount(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{14735, "14,735"},
	}
	for _, c := range cases {
		if got := Count(c.in); got != c.want {
			t.Errorf("Count(%d) = %q，期望 %q", c.in, got, c.want)
		}
	}
}
