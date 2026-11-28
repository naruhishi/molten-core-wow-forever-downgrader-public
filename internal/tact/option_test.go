package tact

import "testing"

func TestSelectLeavesOptionalContentOut(t *testing.T) {
	mask := func(entries ...int) []byte {
		m := make([]byte, 1)
		for _, i := range entries {
			m[0] |= 0x80 >> i
		}
		return m
	}
	tags := []Tag{
		{Name: "Windows", Type: TagPlatform, mask: mask(0, 1, 2, 3)},
		{Name: "enUS", Type: TagLocale, mask: mask(0, 1, 2)},
		{Name: "HighRes", Type: TagOption, mask: mask(1)},
		{Name: "Alternate", Type: TagOption, mask: mask(2)},
	}
	for _, c := range []struct {
		wanted []string
		want   [4]bool
	}{
		{[]string{"Windows", "enUS"}, [4]bool{true, false, false, false}},
		{[]string{"Windows", "enUS", "highres"}, [4]bool{true, true, false, false}},
		{[]string{"Windows", "HighRes", "Alternate"}, [4]bool{true, true, true, true}},
	} {
		in, err := selectEntries(tags, 4, c.wanted)
		if err != nil || [4]bool(in) != c.want {
			t.Errorf("%v: %v %v, want %v", c.wanted, in, err, c.want)
		}
	}
}
