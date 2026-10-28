// Package api exposes the netconf manager and hosts manager over a small
// REST surface used by the embedded Web UI. Responses use {"data": ...} and
// {"error": {"code": ..., "message": ...}} envelopes; codes are stable so
// the frontend can map them to friendly wording.
package api

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"runtime"
	"strings"

	"github.com/LinuxSuRen/netcfg/internal/netconf"
)

// Version metadata, wired from -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
)

// Server wires the platform manager to HTTP.
type Server struct {
	mgr   netconf.Manager
	hosts netconf.HostsManager
	mux   *http.ServeMux
}

// New builds the API server. webFS is the embedded frontend (may be nil in
// tests).
func New(mgr netconf.Manager, hosts netconf.HostsManager, webFS fs.FS) *Server {
	s := &Server{mgr: mgr, hosts: hosts, mux: http.NewServeMux()}
	s.routes(webFS)
	return s
}

func (s *Server) routes(webFS fs.FS) {
	m := s.mux
	m.HandleFunc("GET /api/version", s.handleVersion)
	m.HandleFunc("GET /api/interfaces", s.handleInterfaces)
	m.HandleFunc("GET /api/interfaces/{name}", s.handleInterface)
	m.HandleFunc("POST /api/interfaces/{name}/ip", s.handleSetIP)
	m.HandleFunc("POST /api/interfaces/{name}/dns", s.handleSetDNS)
	m.HandleFunc("GET /api/wifi", s.handleWifiScan)
	m.HandleFunc("POST /api/wifi/connect", s.handleWifiConnect)
	m.HandleFunc("POST /api/wifi/ap", s.handleWifiAPStart)
	m.HandleFunc("DELETE /api/wifi/ap", s.handleWifiAPStop)
	m.HandleFunc("GET /api/hosts", s.handleHostsList)
	m.HandleFunc("POST /api/hosts", s.handleHostsAdd)
	m.HandleFunc("DELETE /api/hosts/{hostname}", s.handleHostsRemove)
	m.HandleFunc("GET /api/proxy", s.handleProxyGet)
	m.HandleFunc("PUT /api/proxy", s.handleProxySet)
	if webFS != nil {
		m.Handle("GET /", spaHandler(webFS))
	}
}

// Handler returns the full handler (API + embedded UI).
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{
		"version":      Version,
		"commit":       Commit,
		"os":           runtime.GOOS,
		"arch":         runtime.GOARCH,
		"capabilities": s.mgr.Capabilities(),
	})
}

func (s *Server) handleInterfaces(w http.ResponseWriter, r *http.Request) {
	ifaces, err := s.mgr.Interfaces()
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, ifaces)
}

func (s *Server) handleInterface(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ifaces, err := s.mgr.Interfaces()
	if err != nil {
		writeError(w, err)
		return
	}
	for _, iface := range ifaces {
		if strings.EqualFold(iface.Name, name) {
			writeData(w, http.StatusOK, iface)
			return
		}
	}
	writeError(w, &netconf.Error{Code: netconf.CodeNotFound,
		Err: fmt.Errorf("interface %s not found", name)})
}

func (s *Server) handleSetIP(w http.ResponseWriter, r *http.Request) {
	var cfg netconf.IPConfig
	if err := decodeBody(r, &cfg); err != nil {
		writeError(w, err)
		return
	}
	if err := s.mgr.SetIP(r.PathValue("name"), cfg); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSetDNS(w http.ResponseWriter, r *http.Request) {
	var cfg netconf.DNSConfig
	if err := decodeBody(r, &cfg); err != nil {
		writeError(w, err)
		return
	}
	if err := s.mgr.SetDNS(r.PathValue("name"), cfg); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleWifiScan(w http.ResponseWriter, r *http.Request) {
	nets, err := s.mgr.ScanWifi()
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, nets)
}

func (s *Server) handleWifiConnect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Interface string `json:"interface"`
		SSID      string `json:"ssid"`
		Password  string `json:"password"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.SSID == "" {
		writeError(w, &netconf.Error{Code: netconf.CodeInvalidArgument,
			Err: errors.New("ssid is required")})
		return
	}
	if err := s.mgr.ConnectWifi(body.Interface, body.SSID, body.Password); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleWifiAPStart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Interface string `json:"interface"`
		SSID      string `json:"ssid"`
		Password  string `json:"password"`
		Channel   int    `json:"channel"`
		Band      string `json:"band"`
		Gateway   string `json:"gateway"`
		ConnName  string `json:"conn_name"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.SSID == "" {
		writeError(w, &netconf.Error{Code: netconf.CodeInvalidArgument,
			Err: errors.New("ssid is required")})
		return
	}
	opts := netconf.WifiAPOptions{
		Channel:  body.Channel,
		Band:     body.Band,
		Gateway:  body.Gateway,
		ConnName: body.ConnName,
	}
	if err := s.mgr.StartWifiAP(body.Interface, body.SSID, body.Password, opts); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleWifiAPStop(w http.ResponseWriter, r *http.Request) {
	conn := r.URL.Query().Get("name")
	if err := s.mgr.StopWifiAP(netconf.WifiAPOptions{ConnName: conn}); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleHostsList(w http.ResponseWriter, r *http.Request) {
	entries, err := s.hosts.Entries()
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, entries)
}

func (s *Server) handleHostsAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IP       string `json:"ip"`
		Hostname string `json:"hostname"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if err := s.hosts.Add(body.IP, body.Hostname); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleHostsRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.hosts.Remove(r.PathValue("hostname")); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleProxyGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.mgr.Proxy(r.URL.Query().Get("interface"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, cfg)
}

func (s *Server) handleProxySet(w http.ResponseWriter, r *http.Request) {
	var cfg netconf.ProxyConfig
	if err := decodeBody(r, &cfg); err != nil {
		writeError(w, err)
		return
	}
	if err := s.mgr.SetProxy(r.URL.Query().Get("interface"), cfg); err != nil {
		writeError(w, err)
		return
	}
	writeData(w, http.StatusOK, map[string]bool{"ok": true})
}
