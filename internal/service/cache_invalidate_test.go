package service

import "testing"

type fakeCache struct {
	deleted []string
	items   map[string][]string
}

func (c *fakeCache) Get(path string) ([]string, bool) { v, ok := c.items[path]; return v, ok }
func (c *fakeCache) Set(path string, values []string) { c.items[path] = values }
func (c *fakeCache) Delete(path string)               { c.deleted = append(c.deleted, path) }

func TestInvalidateSectionsDeletesColorPath(t *testing.T) {
	d := &fakeDisk{
		subdirs: map[string][]string{
			"disk:/Root":                    {"F05P01"},
			"disk:/Root/F05P01":             {"F05P01-ЖЁЛТ"},
			"disk:/Root/F05P01/F05P01-ЖЁЛТ": {"Титульники"},
		},
	}
	c := &fakeCache{items: map[string][]string{}}
	f := NewFlow(d, "disk:/Root", nil, WithTreeCache(c))

	f.InvalidateSections("F05P01", "ЖЁЛТ")

	want := "disk:/Root/F05P01/F05P01-ЖЁЛТ"
	found := false
	for _, p := range c.deleted {
		if p == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Delete(%q), got: %#v", want, c.deleted)
	}
}
