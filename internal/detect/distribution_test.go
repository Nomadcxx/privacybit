package detect

import "testing"

func TestFamilyFromOSRelease(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{`ID=arch`, "arch"},
		{`ID=linuxmint
ID_LIKE="ubuntu debian"`, "debian"},
		{`ID=opensuse-tumbleweed`, "suse"},
		{`ID=unknown
ID_LIKE="unknown fedora"`, "fedora"},
		{`ID=unknown`, ""},
	} {
		if got := FamilyFromOSRelease(tc.data); got != tc.want {
			t.Errorf("FamilyFromOSRelease(%q) = %q, want %q", tc.data, got, tc.want)
		}
	}
}
