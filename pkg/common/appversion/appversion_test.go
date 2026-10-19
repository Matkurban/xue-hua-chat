package appversion

import (
	"strconv"
	"strings"
	"testing"

	admindb "chat/pkg/common/db/table/admin"
)

func rel(arch, ver string, build int64, mods ...func(*admindb.Application)) *admindb.Application {
	a := &admindb.Application{Platform: "android", Arch: arch, Version: ver, Build: build, Status: StatusPublished,
		Url: "https://x/y.apk", Size: 1, Sha256: strings.Repeat("a", 64)}
	for _, m := range mods {
		m(a)
	}
	return a
}

func force(a *admindb.Application) { a.Force = true }
func minV(v string) func(*admindb.Application) {
	return func(a *admindb.Application) { a.MinVersion = v }
}
func revoked(a *admindb.Application) { a.Status = StatusRevoked }

func TestParseVersion(t *testing.T) {
	good := map[string]Version{"1.2.3": {1, 2, 3}, "v2.0.10": {2, 0, 10}, "2.0.3+3014": {2, 0, 3}, "10.0.0": {10, 0, 0}}
	for in, want := range good {
		got, err := ParseVersion(in)
		if err != nil || got != want {
			t.Errorf("ParseVersion(%q)=%v,%v want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "1.2", "1.2.3.4", "a.b.c", "1.2.3-beta", "1..3"} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) should fail", in)
		}
	}
}

func TestCompareNumeric(t *testing.T) {
	a, _ := ParseVersion("1.10.0")
	b, _ := ParseVersion("1.9.0")
	if Compare(a, 0, b, 99) != 1 {
		t.Fatal("1.10.0 must be newer than 1.9.0 regardless of build (numeric, not lexicographic)")
	}
	if Compare(a, 5, a, 7) != -1 || Compare(a, 7, a, 7) != 0 || Compare(a, 8, a, 7) != 1 {
		t.Fatal("build must break ties")
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name       string
		rels       []*admindb.Application
		ver        string
		build      int64
		arch       string
		wantUpdate bool
		wantForce  bool
		wantLatest string // version+build of Latest, "" when nil
	}{
		{"no records", nil, "1.0.0", 1, "arm64", false, false, ""},
		{"up to date", []*admindb.Application{rel("universal", "1.0.0", 1)}, "1.0.0", 1, "arm64", false, false, "1.0.0+1"},
		{"client ahead of server", []*admindb.Application{rel("universal", "1.0.0", 1)}, "1.1.0", 1, "arm64", false, false, "1.0.0+1"},
		{"newer version", []*admindb.Application{rel("universal", "1.1.0", 1)}, "1.0.0", 9, "arm64", true, false, "1.1.0+1"},
		{"numeric not lexicographic", []*admindb.Application{rel("universal", "1.10.0", 1)}, "1.9.0", 1, "arm64", true, false, "1.10.0+1"},
		{"newer build same version", []*admindb.Application{rel("universal", "1.0.0", 5)}, "1.0.0", 4, "x64", true, false, "1.0.0+5"},
		{"older build same version", []*admindb.Application{rel("universal", "1.0.0", 3)}, "1.0.0", 4, "x64", false, false, "1.0.0+3"},
		{"picks highest", []*admindb.Application{rel("universal", "1.1.0", 1), rel("universal", "1.3.0", 1), rel("universal", "1.2.0", 9)}, "1.0.0", 1, "x64", true, false, "1.3.0+1"},
		{"revoked ignored", []*admindb.Application{rel("universal", "1.1.0", 1), rel("universal", "2.0.0", 1, revoked)}, "1.0.0", 1, "x64", true, false, "1.1.0+1"},
		{"only revoked", []*admindb.Application{rel("universal", "2.0.0", 1, revoked)}, "1.0.0", 1, "x64", false, false, ""},
		{"exact arch only", []*admindb.Application{rel("arm64", "1.1.0", 1)}, "1.0.0", 1, "arm64", true, false, "1.1.0+1"},
		{"other arch not served", []*admindb.Application{rel("arm64", "1.1.0", 1)}, "1.0.0", 1, "x64", false, false, ""},
		{"universal fallback", []*admindb.Application{rel("universal", "1.1.0", 1)}, "1.0.0", 1, "armeabi-v7a", true, false, "1.1.0+1"},
		{"exact beats universal on tie", []*admindb.Application{rel("universal", "1.1.0", 1), rel("arm64", "1.1.0", 1, func(a *admindb.Application) { a.Url = "https://x/arm64.apk" })}, "1.0.0", 1, "arm64", true, false, "1.1.0+1"},
		{"newer universal beats older exact", []*admindb.Application{rel("arm64", "1.1.0", 1), rel("universal", "1.2.0", 1)}, "1.0.0", 1, "arm64", true, false, "1.2.0+1"},
		{"record force", []*admindb.Application{rel("universal", "1.1.0", 1, force)}, "1.0.0", 1, "x64", true, true, "1.1.0+1"},
		{"force ignored when up to date", []*admindb.Application{rel("universal", "1.1.0", 1, force)}, "1.1.0", 1, "x64", false, false, "1.1.0+1"},
		{"below min_version forced", []*admindb.Application{rel("universal", "2.0.0", 1, minV("1.5.0"))}, "1.4.9", 1, "x64", true, true, "2.0.0+1"},
		{"at min_version not forced", []*admindb.Application{rel("universal", "2.0.0", 1, minV("1.5.0"))}, "1.5.0", 1, "x64", true, false, "2.0.0+1"},
		{"above min_version not forced", []*admindb.Application{rel("universal", "2.0.0", 1, minV("1.5.0"))}, "1.9.0", 1, "x64", true, false, "2.0.0+1"},
		{"min_version numeric compare", []*admindb.Application{rel("universal", "2.0.0", 1, minV("1.10.0"))}, "1.9.0", 1, "x64", true, true, "2.0.0+1"},
		{"force+min both", []*admindb.Application{rel("universal", "2.0.0", 1, force, minV("1.0.0"))}, "1.9.0", 1, "x64", true, true, "2.0.0+1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Check(tc.rels, tc.ver, tc.build, tc.arch)
			if err != nil {
				t.Fatal(err)
			}
			if got.HasUpdate != tc.wantUpdate || got.Force != tc.wantForce {
				t.Fatalf("has_update=%v force=%v, want %v %v", got.HasUpdate, got.Force, tc.wantUpdate, tc.wantForce)
			}
			l := ""
			if got.Latest != nil {
				l = got.Latest.Version + "+" + itoa(got.Latest.Build)
			}
			if l != tc.wantLatest {
				t.Fatalf("latest=%q want %q", l, tc.wantLatest)
			}
		})
	}
}

func TestCheckExactArchWinsOnTie(t *testing.T) {
	got, _ := Check([]*admindb.Application{rel("universal", "1.1.0", 1), rel("arm64", "1.1.0", 1)}, "1.0.0", 1, "arm64")
	if got.Latest.Arch != "arm64" {
		t.Fatalf("want exact arch, got %s", got.Latest.Arch)
	}
	got, _ = Check([]*admindb.Application{rel("arm64", "1.1.0", 1), rel("universal", "1.1.0", 1)}, "1.0.0", 1, "arm64")
	if got.Latest.Arch != "arm64" {
		t.Fatalf("order must not matter, got %s", got.Latest.Arch)
	}
}

func TestCheckBadClientVersion(t *testing.T) {
	if _, err := Check(nil, "abc", 1, "x64"); err == nil {
		t.Fatal("want error")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestValidate(t *testing.T) {
	ok := func() *admindb.Application { return rel("arm64", "1.2.3", 1) }
	cases := []struct {
		name string
		mod  func(*admindb.Application)
		ok   bool
	}{
		{"valid", func(a *admindb.Application) {}, true},
		{"empty url allowed", func(a *admindb.Application) { a.Url = "" }, true},
		{"ios without url/sha", func(a *admindb.Application) { a.Platform, a.Url, a.Sha256, a.Size = "ios", "", "", 0 }, true},
		{"empty arch defaults to universal", func(a *admindb.Application) { a.Arch = "" }, true},
		{"canonical armv7", func(a *admindb.Application) { a.Arch = "armv7" }, true},
		{"alias armeabi-v7a accepted", func(a *admindb.Application) { a.Arch = "armeabi-v7a" }, true},
		{"canonical x86", func(a *admindb.Application) { a.Arch = "x86" }, true},
		{"bad platform", func(a *admindb.Application) { a.Platform = "symbian" }, false},
		{"bad arch", func(a *admindb.Application) { a.Arch = "mips" }, false},
		{"bad version", func(a *admindb.Application) { a.Version = "1.2" }, false},
		{"v prefix rejected when stored", func(a *admindb.Application) { a.Version = "v1.2.3" }, false},
		{"negative build", func(a *admindb.Application) { a.Build = -1 }, false},
		{"ftp url", func(a *admindb.Application) { a.Url = "ftp://x/a.apk" }, false},
		{"javascript url", func(a *admindb.Application) { a.Url = "javascript:alert(1)" }, false},
		{"file url", func(a *admindb.Application) { a.Url = "file:///etc/passwd" }, false},
		{"bad sha", func(a *admindb.Application) { a.Sha256 = "zz" }, false},
		{"upper sha normalised", func(a *admindb.Application) { a.Sha256 = strings.Repeat("A", 64) }, true},
		{"bad package type", func(a *admindb.Application) { a.PackageType = "msi" }, false},
		{"good package type", func(a *admindb.Application) { a.PackageType = "tar.gz" }, true},
		{"bad status", func(a *admindb.Application) { a.Status = "draft" }, false},
		{"min_version above version", func(a *admindb.Application) { a.MinVersion = "2.0.0" }, false},
		{"bad min_version", func(a *admindb.Application) { a.MinVersion = "x" }, false},
		{"negative size", func(a *admindb.Application) { a.Size = -1 }, false},
	}
	for _, tc := range cases {
		a := ok()
		tc.mod(a)
		err := Validate(a)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v want ok=%v", tc.name, err, tc.ok)
		}
	}
	a := ok()
	a.Arch, a.Status = "", ""
	_ = Validate(a)
	if a.Arch != ArchUniversal || a.Status != StatusPublished {
		t.Errorf("defaults not applied: %s %s", a.Arch, a.Status)
	}
}

func TestNormalizeArch(t *testing.T) {
	cases := map[string]string{
		"": "universal", " Universal ": "universal",
		"arm64": "arm64", "arm64-v8a": "arm64", "AARCH64": "arm64",
		"armv7": "armv7", "armeabi-v7a": "armv7", "armeabi": "armv7", "armv7l": "armv7", "armhf": "armv7", "arm": "armv7",
		"x64": "x64", "amd64": "x64", "x86_64": "x64", "x86-64": "x64",
		"x86": "x86", "i386": "x86", "i686": "x86", "ia32": "x86",
		"mips": "mips",
	}
	for in, want := range cases {
		if got := NormalizeArch(in); got != want {
			t.Errorf("NormalizeArch(%q)=%q want %q", in, got, want)
		}
	}
}

func TestValidateNormalizesAlias(t *testing.T) {
	a := rel("amd64", "1.0.0", 1)
	a.Platform = "windows"
	if err := Validate(a); err != nil || a.Arch != "x64" {
		t.Fatalf("want x64, got %q err=%v", a.Arch, err)
	}
}

func TestCheckArchAliases(t *testing.T) {
	// legacy (unmigrated) record spelled armeabi-v7a must be served to a client reporting armv7, and vice versa
	got, err := Check([]*admindb.Application{rel("armeabi-v7a", "1.1.0", 1)}, "1.0.0", 1, "armv7")
	if err != nil || !got.HasUpdate {
		t.Fatalf("armv7 client should match armeabi-v7a record: %+v %v", got, err)
	}
	got, err = Check([]*admindb.Application{rel("armv7", "1.1.0", 1)}, "1.0.0", 1, "armeabi-v7a")
	if err != nil || !got.HasUpdate {
		t.Fatalf("armeabi-v7a client should match armv7 record: %+v %v", got, err)
	}
	got, _ = Check([]*admindb.Application{rel("x64", "1.1.0", 1)}, "1.0.0", 1, "x86_64")
	if !got.HasUpdate {
		t.Fatal("x86_64 client should match x64 record")
	}
	got, _ = Check([]*admindb.Application{rel("x64", "1.1.0", 1)}, "1.0.0", 1, "x86")
	if got.HasUpdate || got.Latest != nil {
		t.Fatal("x86 client must not get x64 record")
	}
	// exact (aliased) arch still wins over universal on a tie
	got, _ = Check([]*admindb.Application{rel("universal", "1.1.0", 1), rel("amd64", "1.1.0", 1)}, "1.0.0", 1, "x64")
	if got.Latest.Arch != "amd64" {
		t.Fatalf("exact arch should win, got %s", got.Latest.Arch)
	}
}

func TestCheckSkippedForceNotInherited(t *testing.T) {
	// 2.0.5 is force, latest 2.0.6 is not: client 2.0.4 is not forced (spec section 3); min_version expresses it
	r5 := rel("universal", "2.0.5", 0, func(a *admindb.Application) { a.Force = true })
	r6 := rel("universal", "2.0.6", 0)
	got, _ := Check([]*admindb.Application{r5, r6}, "2.0.4", 0, "arm64")
	if !got.HasUpdate || got.Force || got.Latest.Version != "2.0.6" {
		t.Fatalf("force must not be inherited: %+v", got)
	}
	r6.MinVersion = "2.0.5"
	got, _ = Check([]*admindb.Application{r5, r6}, "2.0.4", 0, "arm64")
	if !got.Force {
		t.Fatal("min_version on latest must force older clients")
	}
	// revoked latest is fully excluded, including its force
	r7 := rel("universal", "2.0.7", 0, func(a *admindb.Application) { a.Force = true; a.Status = StatusRevoked })
	got, _ = Check([]*admindb.Application{r6, r7}, "2.0.6", 0, "arm64")
	if got.HasUpdate || got.Force || got.Latest.Version != "2.0.6" {
		t.Fatalf("revoked must be excluded: %+v", got)
	}
}
