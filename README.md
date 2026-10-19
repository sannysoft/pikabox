<p align="center">
  <img src="assets/pikabox-logo.svg" width="160" alt="Pika-box logo: a pika peeking out of a cardboard box">
</p>

<h1 align="center">Pika-box</h1>

<p align="center">
  <em>sing-box for OpenWrt, with subscriptions that pick their fastest servers, fallback groups and rule-group routing.</em>
</p>

<p align="center">
  <a href="https://github.com/sannysoft/pikabox/actions/workflows/ci.yml"><img src="https://github.com/sannysoft/pikabox/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/sannysoft/pikabox/releases/latest"><img src="https://img.shields.io/github/v/release/sannysoft/pikabox?sort=semver" alt="Release"></a>
</p>

**Pika-box** is a router-side manager for [sing-box](https://sing-box.sagernet.org/) on
OpenWrt: a small Go daemon (`pikaboxd`) plus a LuCI app. You describe *what* goes *where*
— outbounds, filtered subscriptions, outbound groups, rule groups and an ordered route
list — and Pika-box fetches, filters and delay-tests servers, picks the fastest, keeps
fallback groups on the first live member, and renders a sing-box config from it. Settings
live in UCI (`/etc/config/pikabox`) and are exposed over `ubus call pikabox …`.

## Features

- **Outbounds** — each one is a share URI (vless/vmess/trojan/ss/hysteria2/tuic) or raw
  sing-box outbound JSON (any type/option, passed through as-is), with real
  through-the-tunnel latency.
- **Subscriptions** — URL + an [expr](https://expr-lang.org) filter over each outbound
  (`type == "vless" && name matches "(?i)nl|de" && port == 443`, or anything in the full
  JSON via `outbound.tls?.reality?.enabled`) + a count: the N fastest alive survivors are
  used, by latency or (`select_by speed`) by measured download speed. Shows fetched / filtered / alive counts and best latency; re-tested every probe
  interval, re-fetched on its own interval.
- **Outbound groups** — named, ordered member lists (outbounds, subscriptions, other
  groups, direct/block). Mode *fastest by latency* (sing-box `urltest`), *fastest by speed*
  (highest measured Mb/s from a ~4 s, ≤10 MB download, redone every 3 h; switches only when
  another member is 25% faster) or *fallback* (first alive in written order). Speed and
  fallback groups are selectors driven by the daemon through the Clash API every 30 s. A
  fallback group also moves off a server that answers latency tests but fails too many real
  connections (over 5% in 5 min by default, counted from sing-box's log; failures beyond
  the server are ignored), then retries it after a cooldown that doubles on repeats.
- **Rule groups** — named sets of `type:value` rules, ORed: any sing-box match field
  (`domain`, `domain_suffix`, `domain_keyword`, `domain_regex`, `ip_cidr`, `port`,
  `protocol`, `process_name`, …) or `remote:<rule-set URL>` (downloaded through the tunnel
  and cached; uncached sets are left out so sing-box never fails to start on them).
- **Routing** — ordered `rule group → target` list plus a default target; first match wins.
- **DNS**, **transparent proxy (TProxy)**, **TUN**, per-device pins — as before; DNS rules
  match rule groups, DNS detours can point at any outbound group.

Targets OpenWrt 25.12+ with the feed's sing-box (1.13.x; renders no legacy fields —
sniffing is a route action).

### Config model (`/etc/config/pikabox`)

```
config outbound 'vp1'
	option source 'vless://…#vp1'
config subscription 'prov1'
	option url 'https://…'
	option filter 'name matches "^NL"'
	option count '3'
config outbound_group 'us_out'
	option name 'us-out'
	option mode 'fallback'
	list member 'vp1'
	list member 'prov1'
config rule_group 'tg'
	list rule 'domain_suffix:telegram.org'
	list rule 'remote:https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-telegram.srs'
config route 'route_1'
	option rule_group 'tg'
	option outbound 'us_out'
config manager 'main'
	option final 'us_out'
```

Subscription candidates and their top-N selection live in
`/etc/pikabox/state/`; latency results in `/var/run/pikabox/probe.json`.

## Components

| Path | What it is |
| --- | --- |
| `pikabox/` | The `pikaboxd` Go daemon + OpenWrt package (Makefile, UCI config, init script). |
| `luci-app-pikabox/` | LuCI web app (Dashboard, Outbounds, Subscriptions, Outbound Groups, Rule Groups, Routing, DNS, Network, Settings). |
| `install.sh` | One-line installer for OpenWrt 25.12+ (attached to every release). |
| `Makefile` | Top-level dev/build orchestration: Go tests, SDK download, package builds, QEMU test lab. |
| `flake.nix` | Nix dev shell with the full toolchain (Go, QEMU, OpenWrt SDK prerequisites). |

### How it works

`pikaboxd` runs three roles from one binary:

- `serve` — the long-running procd service. It reconciles the UCI config into a generated
  `sing-box` config, supervises the `sing-box` process, and programs the nftables TProxy
  include and fwmark policy routing.
- `rpcd` — the `rpcd`/`ubus` plugin (symlinked as `/usr/libexec/rpcd/pikabox`) that
  backs the LuCI UI and `ubus call pikabox …`.
- `cleanup` — tears down the nftables include and policy routing (run on service stop).

## Install

Supports **OpenWrt 24.10** (opkg) and **25.12+** (apk). On the router, as root:

```sh
wget -qO- https://github.com/sannysoft/pikabox/releases/latest/download/install.sh | sh
```

The script installs `sing-box` from the official OpenWrt feed and the Pika-box packages
from the latest GitHub release, enables the service, and leaves the proxy off until you
press **Start** in **LuCI → Services → Pika-box**. Re-run it to upgrade.

| Option | Effect |
| --- | --- |
| `PIKABOX_VERSION=v0.1.0` | install that release instead of the latest |
| `PIKABOX_TPROXY=1` | also install `kmod-nft-tproxy` (needed for transparent proxy mode) |

e.g. `wget -qO- …/install.sh | PIKABOX_TPROXY=1 sh`.

Prebuilt architectures: `x86_64`, `aarch64_generic` (armsr/armv8), `aarch64_cortex-a53`
(mediatek/filogic), `arm_cortex-a15_neon-vfpv4` (armsr/armv7), `mipsel_24kc`
(ramips/mt7621) — check yours with `grep DISTRIB_ARCH /etc/openwrt_release`. Others: build
from source below.

### Manual install

Download the packages for your series from the
[latest release](https://github.com/sannysoft/pikabox/releases/latest), then:

```sh
# OpenWrt 25.12+ (apk)
apk update && apk add sing-box
apk add --allow-untrusted ./pikabox_25.12_<arch>.apk ./luci-app-pikabox_25.12.apk

# OpenWrt 24.10 (opkg)
opkg update && opkg install sing-box
opkg install ./pikabox_24.10_<arch>.ipk ./luci-app-pikabox_24.10.ipk

/etc/init.d/pikaboxd enable && /etc/init.d/pikaboxd start
```

`--allow-untrusted` applies only to those two local files; sing-box and all dependencies
come from the signed OpenWrt feeds. Configuration lives in `/etc/config/pikabox`.

> **Transparent / TProxy mode** needs `kmod-nft-tproxy`, which is not a package
> dependency (it pulls netfilter modules the SDK cannot build). `PIKABOX_TPROXY=1`
> installs it, or run `apk add kmod-nft-tproxy`.

## Build from source

Packages are built with the OpenWrt SDK. The top-level `Makefile` downloads and prepares
the SDK automatically.

```sh
make ipk-x86_64      # build for one arch into dist/x86_64/
make ipk-aarch64     # armsr/armv8 (aarch64_generic)
make ipk-aarch64_cortexa53  # mediatek/filogic (aarch64_cortex-a53)
make ipk-armv7       # armsr/armv7
make ipk-mipsel      # ramips/mt7621
make ipk-all         # build every arch in ARCHS into dist/<arch>/
```

Builds default to OpenWrt 25.12 (`.apk`). Target 24.10 (`.ipk`) — or any release —
by setting `OPENWRT_VERSION`; the matching toolchain version is derived automatically:

```sh
make ipk-x86_64 OPENWRT_VERSION=24.10.7
```

Override the target without a preset:

```sh
make ipk OPENWRT_TARGET_PATH=ath79/generic OPENWRT_VERSION=24.10.7
```

## Develop

```sh
make test     # run the Go test suite
make build    # build the daemon locally to /tmp/pikaboxd
make smoke    # local rpcd smoke checks (list + status)
make help     # list all targets
```

A reproducible toolchain is available via Nix (`nix develop`) or direnv (`direnv allow`).

### Test in a VM

The `Makefile` can stand up an isolated OpenWrt QEMU lab (x86) with an Alpine LAN client
for end-to-end proxy testing:

```sh
make vm-image          # download the OpenWrt image + create a qcow2 overlay
make vm-run            # boot OpenWrt on an isolated lab LAN + host-NAT WAN
make deploy            # build (dist/) then install packages onto the VM
make alpine-run        # boot an Alpine LAN client (separate terminal)
make proxy-test-help   # print connectivity / proxy test commands
make undeploy          # remove packages and wipe leftover state
```

## CI / Releases

- **CI** (`.github/workflows/ci.yml`) — `gofmt`, `go vet` and the Go tests on every push to
  `main` and every pull request.
- **Release** (`.github/workflows/release.yml`) — on a `v*` tag, builds the packages with
  the OpenWrt 24.10 (`.ipk`) and 25.12 (`.apk`) SDKs for every architecture above and
  publishes a GitHub Release with the packages, `install.sh`, and install instructions.
- **Optional signed apk feed** — set the repo variable `PUBLISH_FEED=true`, add an EC
  private key (PEM) as the `APK_SIGN_KEY` secret, and enable GitHub Pages with "GitHub
  Actions" as the source; tagged releases then also publish a signed feed whose public
  key is derived from that secret. Not needed for `install.sh`.

Cut a release:

```sh
git tag v0.1.0
git push origin v0.1.0
```
