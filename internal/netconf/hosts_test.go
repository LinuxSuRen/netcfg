package netconf

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTempHosts(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readBack(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestHostsAddCreatesManagedSection(t *testing.T) {
	path := writeTempHosts(t, "127.0.0.1 localhost\n# custom comment\n")
	h := NewHostsAt(path)
	if err := h.Add("10.1.2.3", "mydevice.local"); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, path)
	want := "127.0.0.1 localhost\n# custom comment\n\n# BEGIN netcfg\n10.1.2.3 mydevice.local\n# END netcfg\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestHostsAddUpdatesExisting(t *testing.T) {
	path := writeTempHosts(t, "# BEGIN netcfg\n10.0.0.1 old.local\n# END netcfg\n10.9.9.9 untouched\n")
	h := NewHostsAt(path)
	if err := h.Add("10.0.0.2", "old.local"); err != nil {
		t.Fatal(err)
	}
	if err := h.Add("10.0.0.3", "new.local"); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, path)
	want := "# BEGIN netcfg\n10.0.0.2 old.local\n10.0.0.3 new.local\n# END netcfg\n10.9.9.9 untouched\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestHostsRemoveOnlyManaged(t *testing.T) {
	path := writeTempHosts(t, "# BEGIN netcfg\n10.0.0.1 a.local b.local\n# END netcfg\n10.9.9.9 a.local\n")
	h := NewHostsAt(path)
	if err := h.Remove("a.local"); err != nil {
		t.Fatal(err)
	}
	got := readBack(t, path)
	want := "# BEGIN netcfg\n10.0.0.1 b.local\n# END netcfg\n10.9.9.9 a.local\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
	// removing the last hostname drops the line
	if err := h.Remove("b.local"); err != nil {
		t.Fatal(err)
	}
	want2 := "# BEGIN netcfg\n# END netcfg\n10.9.9.9 a.local\n"
	if got := readBack(t, path); got != want2 {
		t.Errorf("got:\n%q\nwant:\n%q", got, want2)
	}
	if err := h.Remove("a.local"); err == nil || CodeOf(err) != CodeNotFound {
		t.Errorf("removing foreign entry should report not_found, got %v", err)
	}
}

func TestHostsEntries(t *testing.T) {
	path := writeTempHosts(t, "127.0.0.1 localhost\n# BEGIN netcfg\n10.0.0.1 mine.local\n# END netcfg\n10.9.9.9 other.local\n")
	entries, err := NewHostsAt(path).Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("len = %d: %+v", len(entries), entries)
	}
	if entries[0].IP != "127.0.0.1" || entries[0].Managed {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if !entries[1].Managed || !reflect.DeepEqual(entries[1].Hostnames, []string{"mine.local"}) {
		t.Errorf("entry1 = %+v", entries[1])
	}
	if entries[2].Managed {
		t.Errorf("entry2 = %+v", entries[2])
	}
}

func TestHostsValidation(t *testing.T) {
	path := writeTempHosts(t, "")
	h := NewHostsAt(path)
	if err := h.Add("not-an-ip", "x.local"); err == nil || CodeOf(err) != CodeInvalidArgument {
		t.Errorf("bad ip accepted: %v", err)
	}
	if err := h.Add("10.0.0.1", "has space"); err == nil || CodeOf(err) != CodeInvalidArgument {
		t.Errorf("bad hostname accepted: %v", err)
	}
	if err := h.Add("10.0.0.1", "good.local"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readBack(t, path), hostsBeginMarker) {
		t.Error("managed section missing")
	}
}

func TestHostsAddToMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := NewHostsAt(path).Add("10.0.0.1", "x.local"); err != nil {
		t.Fatal(err)
	}
	want := "# BEGIN netcfg\n10.0.0.1 x.local\n# END netcfg\n"
	if got := readBack(t, path); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
