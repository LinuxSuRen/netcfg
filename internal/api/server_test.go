package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

// fakeManager implements netconf.Manager for endpoint tests.
type fakeManager struct {
	netconf.Manager // embed nil; only used methods are overridden
	ifaces          []netconf.Interface
	scanErr         error
	proxy           netconf.ProxyConfig
	lastIP          map[string]netconf.IPConfig
	lastDNS         map[string]netconf.DNSConfig
	lastWifi        string
	lastAP          string
	stoppedAP       string
	lastProxy       netconf.ProxyConfig
}

func (f *fakeManager) Interfaces() ([]netconf.Interface, error) { return f.ifaces, nil }
func (f *fakeManager) SetIP(name string, cfg netconf.IPConfig) error {
	if name == "missing" {
		return &netconf.Error{Code: netconf.CodeNotFound, Err: errStr("interface missing not found")}
	}
	if cfg.IP == "999.999.1.1" {
		return &netconf.Error{Code: netconf.CodeInvalidArgument, Err: errStr("bad ip")}
	}
	if f.lastIP == nil {
		f.lastIP = map[string]netconf.IPConfig{}
	}
	f.lastIP[name] = cfg
	return nil
}
func (f *fakeManager) SetDNS(name string, cfg netconf.DNSConfig) error {
	if f.lastDNS == nil {
		f.lastDNS = map[string]netconf.DNSConfig{}
	}
	f.lastDNS[name] = cfg
	return nil
}
func (f *fakeManager) ScanWifi() ([]netconf.WifiNetwork, error) {
	if f.scanErr != nil {
		return nil, f.scanErr
	}
	return []netconf.WifiNetwork{{SSID: "Home", Signal: 80, Security: "WPA2"}}, nil
}
func (f *fakeManager) ConnectWifi(iface, ssid, password string) error {
	f.lastWifi = ssid + "/" + password + "@" + iface
	return nil
}

func (f *fakeManager) StartWifiAP(iface, ssid, password string, opts netconf.WifiAPOptions) error {
	if ssid == "-bad" {
		return &netconf.Error{Code: netconf.CodeInvalidArgument, Err: errStr("bad ssid")}
	}
	f.lastAP = ssid + "/" + password + "@" + iface + "/" + opts.ConnName
	return nil
}

func (f *fakeManager) StopWifiAP(opts netconf.WifiAPOptions) error {
	if opts.ConnName == "missing-ap" {
		return &netconf.Error{Code: netconf.CodeNotFound, Err: errStr("no such hotspot")}
	}
	f.stoppedAP = opts.ConnName
	return nil
}
func (f *fakeManager) Proxy(iface string) (netconf.ProxyConfig, error) { return f.proxy, nil }
func (f *fakeManager) SetProxy(iface string, cfg netconf.ProxyConfig) error {
	f.lastProxy = cfg
	return nil
}
func (f *fakeManager) Capabilities() netconf.Capabilities {
	return netconf.Capabilities{SetIP: true, SetDNS: true, WifiScan: true, WifiJoin: true, Proxy: true, Hosts: true}
}

type errStr string

func (e errStr) Error() string { return string(e) }

func newTestServer(t *testing.T) (*Server, *fakeManager, string) {
	t.Helper()
	mgr := &fakeManager{ifaces: []netconf.Interface{
		{Name: "Wi-Fi", Type: "wifi", Up: true, DHCP: true},
		{Name: "Ethernet", Type: "ethernet"},
	}}
	hostsPath := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsPath, []byte("# BEGIN netcfg\n10.0.0.1 mine.local\n# END netcfg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := New(mgr, netconf.NewHostsAt(hostsPath), nil)
	return srv, mgr, hostsPath
}

func doJSON(t *testing.T, srv *Server, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var parsed map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func TestInterfacesEndpoints(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec, body := doJSON(t, srv, http.MethodGet, "/api/interfaces", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %v", rec.Code, body)
	}
	data := body["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("data = %v", data)
	}
	rec, _ = doJSON(t, srv, http.MethodGet, "/api/interfaces/Wi-Fi", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("single code = %d", rec.Code)
	}
	rec, _ = doJSON(t, srv, http.MethodGet, "/api/interfaces/none", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing code = %d", rec.Code)
	}
}

func TestSetIPEndpoint(t *testing.T) {
	srv, mgr, _ := newTestServer(t)
	rec, body := doJSON(t, srv, http.MethodPost, "/api/interfaces/Wi-Fi/ip",
		`{"dhcp":false,"ip":"10.0.0.5","netmask":"255.255.255.0","gateway":"10.0.0.1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %v", rec.Code, body)
	}
	if got := mgr.lastIP["Wi-Fi"]; got.IP != "10.0.0.5" || got.DHCP {
		t.Errorf("lastIP = %+v", got)
	}
	rec, body = doJSON(t, srv, http.MethodPost, "/api/interfaces/Wi-Fi/ip", `{not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json code = %d body = %v", rec.Code, body)
	}
	rec, body = doJSON(t, srv, http.MethodPost, "/api/interfaces/missing/ip", `{"dhcp":true}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing iface code = %d body = %v", rec.Code, body)
	}
}

func TestDNSEndpoint(t *testing.T) {
	srv, mgr, _ := newTestServer(t)
	rec, _ := doJSON(t, srv, http.MethodPost, "/api/interfaces/Ethernet/dns",
		`{"servers":["8.8.8.8","1.1.1.1"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if got := mgr.lastDNS["Ethernet"]; len(got.Servers) != 2 {
		t.Errorf("lastDNS = %+v", got)
	}
}

func TestWifiEndpoints(t *testing.T) {
	srv, mgr, _ := newTestServer(t)
	rec, body := doJSON(t, srv, http.MethodGet, "/api/wifi", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("scan code = %d body = %v", rec.Code, body)
	}
	rec, _ = doJSON(t, srv, http.MethodPost, "/api/wifi/connect",
		`{"interface":"Wi-Fi","ssid":"Home","password":"secret"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect code = %d", rec.Code)
	}
	if mgr.lastWifi != "Home/secret@Wi-Fi" {
		t.Errorf("lastWifi = %q", mgr.lastWifi)
	}
	rec, _ = doJSON(t, srv, http.MethodPost, "/api/wifi/connect", `{"ssid":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty ssid code = %d", rec.Code)
	}
}

func TestWifiAPEndpoints(t *testing.T) {
	srv, mgr, _ := newTestServer(t)
	rec, _ := doJSON(t, srv, http.MethodPost, "/api/wifi/ap",
		`{"interface":"wlan0","ssid":"test-ap","password":"secret123","band":"bg","channel":6}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("ap start code = %d", rec.Code)
	}
	if want := "test-ap/secret123@wlan0/"; mgr.lastAP != want {
		t.Errorf("lastAP = %q want %q", mgr.lastAP, want)
	}
	rec, _ = doJSON(t, srv, http.MethodPost, "/api/wifi/ap", `{"ssid":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty ssid code = %d", rec.Code)
	}
	rec, _ = doJSON(t, srv, http.MethodDelete, "/api/wifi/ap", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ap stop code = %d", rec.Code)
	}
	if mgr.stoppedAP != "" {
		t.Errorf("stoppedAP = %q want empty default", mgr.stoppedAP)
	}
	rec, _ = doJSON(t, srv, http.MethodDelete, "/api/wifi/ap?name=missing-ap", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing ap code = %d", rec.Code)
	}
}

func TestHostsEndpoints(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec, body := doJSON(t, srv, http.MethodGet, "/api/hosts", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d body = %v", rec.Code, body)
	}
	rec, _ = doJSON(t, srv, http.MethodPost, "/api/hosts", `{"ip":"10.0.0.2","hostname":"added.local"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add code = %d", rec.Code)
	}
	rec, body = doJSON(t, srv, http.MethodDelete, "/api/hosts/mine.local", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove code = %d body = %v", rec.Code, body)
	}
	rec, body = doJSON(t, srv, http.MethodDelete, "/api/hosts/added.local", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("remove2 code = %d body = %v", rec.Code, body)
	}
}

func TestProxyEndpoints(t *testing.T) {
	srv, mgr, _ := newTestServer(t)
	srvProxy := &fakeManager{proxy: netconf.ProxyConfig{Mode: "manual", HTTPHost: "p", HTTPPort: 1}}
	srv2 := New(srvProxy, netconf.NewHostsAt(filepath.Join(t.TempDir(), "h")), nil)
	rec, body := doJSON(t, srv2, http.MethodGet, "/api/proxy", "")
	if rec.Code != http.StatusOK || body["data"].(map[string]any)["mode"] != "manual" {
		t.Fatalf("get code = %d body = %v", rec.Code, body)
	}
	rec, _ = doJSON(t, srv, http.MethodPut, "/api/proxy", `{"mode":"none"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set code = %d", rec.Code)
	}
	if mgr.lastProxy.Mode != "none" {
		t.Errorf("lastProxy = %+v", mgr.lastProxy)
	}
}

func TestVersionEndpoint(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec, body := doJSON(t, srv, http.MethodGet, "/api/version", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if body["data"].(map[string]any)["version"] != Version {
		t.Errorf("body = %v", body)
	}
}

func TestErrorEnvelopeShape(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rec, body := doJSON(t, srv, http.MethodGet, "/api/interfaces/none", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok || errObj["code"] != netconf.CodeNotFound {
		t.Fatalf("envelope = %v", body)
	}
	if _, ok := errObj["message"]; !ok {
		t.Fatal("message missing")
	}
}
