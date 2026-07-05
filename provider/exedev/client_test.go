package exedev

import "testing"

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"":             "''",
		"simple":       "simple",
		"ubuntu:22.04": "ubuntu:22.04",
		"a=b,c=d":      "a=b,c=d",
		"has space":    "'has space'",
		"it's":         `'it'\''s'`,
		"20GB":         "20GB",
		"--flag":       "--flag",
		"semi;colon":   "'semi;colon'",
		"new\nline":    "'new\nline'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCmdFlag(t *testing.T) {
	c := newCmd("new")
	c.flag("name", "my-vm")
	c.flag("comment", "staging copy")
	c.flag("cpu", "4")
	c.raw("--json")
	got := c.String()
	want := "new --name=my-vm --comment='staging copy' --cpu=4 --json"
	if got != want {
		t.Errorf("cmd = %q, want %q", got, want)
	}
}

func TestSizeParsing(t *testing.T) {
	gb := map[string]int{"4": 4, "4G": 4, "4GB": 4, "20gb": 20, " 50 G ": 50}
	for in, want := range gb {
		got, err := SizeToGB(in)
		if err != nil || got != want {
			t.Errorf("SizeToGB(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "4TB", "4MB", "-3"} {
		if _, err := SizeToGB(bad); err == nil {
			t.Errorf("SizeToGB(%q) expected error", bad)
		}
	}
	if n, _ := NormalizeSize("8G"); n != "8GB" {
		t.Errorf("NormalizeSize(8G) = %q, want 8GB", n)
	}
}

func TestParseVM(t *testing.T) {
	// {"vms":[...]} shape
	if vm, err := parseVM([]byte(`{"vms":[{"vm_name":"a","status":"running"}]}`)); err != nil || vm.Name != "a" {
		t.Errorf("vms-array shape: %+v, %v", vm, err)
	}
	// bare object shape
	if vm, err := parseVM([]byte(`{"vm_name":"b","https_url":"https://b.exe.xyz"}`)); err != nil || vm.Name != "b" {
		t.Errorf("bare shape: %+v, %v", vm, err)
	}
	// {"vm":{...}} shape
	if vm, err := parseVM([]byte(`{"vm":{"vm_name":"c"}}`)); err != nil || vm.Name != "c" {
		t.Errorf("vm-object shape: %+v, %v", vm, err)
	}
	if _, err := parseVM([]byte(`{"unexpected":true}`)); err == nil {
		t.Error("expected error for unparseable response")
	}
}

func TestDiffTags(t *testing.T) {
	add, remove := diffTags([]string{"a", "b"}, []string{"b", "c"})
	if !setEq(add, []string{"c"}) || !setEq(remove, []string{"a"}) {
		t.Errorf("diffTags add=%v remove=%v", add, remove)
	}
}

func TestParseDomains(t *testing.T) {
	// {"domains":[..]} with the hostname under "domain"
	ds, err := parseDomains([]byte(`{"domains":[{"domain":"api.example.com","verified":true}]}`))
	if err != nil || len(ds) != 1 || ds[0].name() != "api.example.com" || !ds[0].Verified {
		t.Errorf("domains-wrap: %+v, %v", ds, err)
	}
	// bare array with the hostname under "hostname"
	ds, err = parseDomains([]byte(`[{"hostname":"x.example.com"}]`))
	if err != nil || len(ds) != 1 || ds[0].name() != "x.example.com" {
		t.Errorf("bare-array: %+v, %v", ds, err)
	}
	if _, err := parseDomains([]byte(`not json`)); err == nil {
		t.Error("expected error for unparseable domains")
	}
}

func TestDomainID(t *testing.T) {
	if got := domainID("vm1", "api.example.com"); got != "vm1/api.example.com" {
		t.Errorf("domainID = %q", got)
	}
	vm, host, err := splitDomainID("vm1/api.example.com")
	if err != nil || vm != "vm1" || host != "api.example.com" {
		t.Errorf("splitDomainID = %q, %q, %v", vm, host, err)
	}
	if _, _, err := splitDomainID("no-slash"); err == nil {
		t.Error("expected error for id without slash")
	}
}

func TestSortedKeys(t *testing.T) {
	got := sortedKeys(map[string]string{"z": "1", "a": "2", "m": "3"})
	if !sliceEq(got, []string{"a", "m", "z"}) {
		t.Errorf("sortedKeys = %v", got)
	}
}
