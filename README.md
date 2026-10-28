# netcfg

<p align="center">
  <img src="docs/logo.svg" width="128" alt="netcfg logo" />
</p>

One single binary to **view and change this machine's network configuration** — interfaces, static IP / DHCP, DNS, Wi-Fi, the hosts file and the desktop proxy — on **macOS, Windows and Linux**, from the terminal (CLI) **and** from a small built-in Web UI. Written in Go, with a Vue 3 UI embedded in the same binary.

<p align="center">
  <a href="https://github.com/LinuxSuRen/netcfg">Repository</a> ·
  <a href="https://github.com/LinuxSuRen/netcfg/releases">Releases</a> ·
  <a href="https://github.com/LinuxSuRen/netcfg/issues">Issues</a> ·
  <a href="https://github.com/LinuxSuRen/netcfg/pulls">Pull Requests</a> ·
  <a href="LICENSE">License</a>
</p>

## Why

Every OS buries the same handful of settings behind different tools: `networksetup` on macOS, `netsh` / PowerShell on Windows, `nmcli` / `ip` on Linux. When you set up a lab machine, a robot host, or a freshly imaged laptop, you end up re-learning all three. netcfg gives the same commands and the same JSON the same way everywhere:

- `netcfg list` shows every interface with its IP, mode, gateway and DNS — one command on all three OSes;
- switch an interface to a static IP or back to DHCP without remembering `netsh interface ipv4 set address ...` syntax;
- set DNS servers per interface, join a Wi-Fi network, map a hostname in the hosts file, toggle the system proxy;
- `--dry-run` on every mutating command prints the exact system commands it would run;
- `netcfg serve` opens the same functionality in a browser, bound to `127.0.0.1` by default.

## Platform backends

| Capability | macOS | Windows | Linux |
|---|---|---|---|
| Interfaces, IP, gateway, DNS | `networksetup` + `ifconfig` | PowerShell `Get-Net*` (JSON) | NetworkManager (`nmcli`); read-only fallback via `ip` + `resolv.conf` |
| Static IP / DHCP | `networksetup -setmanual/-setdhcp` | `netsh interface ipv4` | `nmcli connection modify` |
| DNS servers | `networksetup -setdnsservers` | `netsh interface ipv4 set dnsservers` | `nmcli connection modify` |
| Wi-Fi scan / join | `airport -s` / `networksetup -setairportnetwork` | `netsh wlan show networks` / WLAN profile | `nmcli device wifi` |
| Wi-Fi hotspot (AP) | — | — | `nmcli connection` (AP mode + shared) |
| Hosts file | `/etc/hosts` | `%SystemRoot%\System32\drivers\etc\hosts` | `/etc/hosts` |
| Desktop proxy | per network service | WinINET registry (per user) | GNOME `gsettings` |

Linux without NetworkManager degrades to a read-only view; mutating commands report `unsupported` instead of guessing. Wi-Fi passwords passed on the `nmcli` command line are visible to other local users via `/proc` — prefer the prompt (`netcfg wifi connect <ssid>` without `--password`).

## Install

Prebuilt binaries for Linux, macOS and Windows (amd64 / arm64) are attached to [GitHub Releases](https://github.com/LinuxSuRen/netcfg/releases):

```bash
curl -fsSL https://github.com/LinuxSuRen/netcfg/releases/download/v0.1.0/netcfg-v0.1.0-linux-amd64.tar.gz | tar xz
./netcfg-v0.1.0-linux-amd64/netcfg list
```

With a Go toolchain (the web UI is embedded in releases; building from source needs Node for the frontend, see Development):

```bash
go install github.com/LinuxSuRen/netcfg@latest
```

## Quick start

```text
$ netcfg list
NAME          TYPE       STATE  MODE    ADDRESSES     GATEWAY      DNS
Wi-Fi         wifi       up     dhcp    192.168.1.5   192.168.1.1  192.168.1.1
Ethernet      ethernet   down   static  10.0.0.5      10.0.0.1
```

Switch an interface to a static IP (needs administrator privileges; `--dry-run` previews the exact system commands):

```bash
sudo netcfg set-ip Ethernet --ip 10.0.0.5 --mask 255.255.255.0 --gw 10.0.0.1
sudo netcfg set-ip Ethernet --dhcp          # and back
sudo netcfg set-ip Ethernet --ip 10.0.0.5 --dry-run
```

DNS per interface:

```bash
sudo netcfg set-dns Wi-Fi --servers 223.5.5.5,119.29.29.29
sudo netcfg set-dns Wi-Fi --auto           # back to DHCP-provided DNS
```

Wi-Fi:

```bash
netcfg wifi list
netcfg wifi connect "HomeNet"              # password prompt (input hidden)
```

Wi-Fi hotspot (AP mode, Linux via NetworkManager; macOS/Windows report
`unsupported` for now):

```bash
sudo netcfg wifi ap start "robot-ap" --password 'robot123456'   # WPA2, 2.4 GHz
sudo netcfg wifi ap start "robot-ap" --password 'robot123456' --band a --channel 36
sudo netcfg wifi ap start "robot-ap" --password ''              # open network
sudo netcfg wifi ap start "robot-ap" --password 'robot123456' --dry-run   # preview
sudo netcfg wifi ap stop
```

The hotspot shares this machine's connectivity (clients get DHCP from
NetworkManager's shared mode, gateway `10.42.0.1` unless `--gateway` says
otherwise). Starting replaces a leftover `netcfg-ap` connection of the same
name, so consecutive starts converge; stop removes the profile again.
Passwords on the command line are visible to other local users via `/proc` —
prefer the prompt (omit `--password`) or an empty value for an open network.

Hosts file — netcfg only edits a marked section, every other line is preserved byte-for-byte:

```bash
sudo netcfg hosts add 10.1.2.3 mydevice.local
netcfg hosts list
sudo netcfg hosts remove mydevice.local
```

System proxy (macOS: per network service; Windows: per user; Linux: GNOME session):

```bash
netcfg proxy get
sudo netcfg proxy set --http proxy.example.com:8080 --bypass "*.local,127.0.0.0/8"
sudo netcfg proxy off
```

Every command accepts `-o` (`--output`) for JSON output, e.g. `netcfg list -o json`.

## Web UI

```bash
sudo netcfg serve
# netcfg web UI: http://127.0.0.1:8190
```

The page offers the same four areas as the CLI — interfaces (edit IP/DNS in a dialog), Wi-Fi (scan & connect), hosts, proxy — in Chinese, and disables what the current platform backend cannot do. The server binds `127.0.0.1` only; pass `--addr` deliberately if you really need another interface (changing network settings is a privileged operation — keep it local or tunnel it). The REST API it serves is the same one the CLI capabilities map to, with `{"data": ...}` envelopes and stable error codes (`invalid_argument`, `not_found`, `unsupported`, `permission_denied`, `command_failed`).

## Development

```bash
make setup      # npm install for web/
make build      # build web (Vite) + embed + go build
make test       # go test + web build check
make lint       # go vet
```

Layout: `internal/netconf` holds the platform-agnostic model, the per-platform backends and pure parsers (table-driven tests run on every OS); `internal/api` the REST layer; `cmd` the cobra CLI; `web` the Vue 3 + Element Plus frontend whose build output is embedded via `go:embed`. CI runs Go tests on Linux/macOS/Windows plus a web build; releases are cut by goreleaser on tags.

## Contributing

Issues and PRs are welcome — one PR per kind of change, please. See [LICENSE](LICENSE) (Apache-2.0).
