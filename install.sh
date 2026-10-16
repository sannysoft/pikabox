#!/bin/sh
# Pika-box installer for OpenWrt 24.10 (opkg) and 25.12+ (apk).
#
#   wget -qO- https://github.com/sannysoft/pikabox/releases/latest/download/install.sh | sh
#
# Options (environment variables):
#   PIKABOX_VERSION=v0.1.0   install a specific release instead of the latest
#   PIKABOX_TPROXY=1         also install kmod-nft-tproxy (transparent proxy mode)
#
# Installs sing-box from the official OpenWrt feed and the Pika-box packages from
# the GitHub release, enables the service, and leaves the proxy itself off until
# you start it from LuCI (Services -> Pika-box). Re-running it upgrades.

set -eu

REPO="sannysoft/pikabox"
VERSION="${PIKABOX_VERSION:-latest}"
WORKDIR="$(mktemp -d /tmp/pikabox-install.XXXXXX)"
trap 'rm -rf "$WORKDIR"' EXIT INT TERM

say() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" = "0" ] || die "run as root"
[ -f /etc/openwrt_release ] || die "this is not OpenWrt"
. /etc/openwrt_release

# apk (25.12+) or opkg (24.10) decides the package format and build series.
if command -v apk >/dev/null 2>&1; then
	PM=apk SERIES=25.12 EXT=apk
	case "${DISTRIB_RELEASE:-}" in
		25.12*|SNAPSHOT) ;;
		*) say "OpenWrt ${DISTRIB_RELEASE:-unknown} detected; packages are built against $SERIES — continuing" ;;
	esac
elif command -v opkg >/dev/null 2>&1; then
	PM=opkg SERIES=24.10 EXT=ipk
	case "${DISTRIB_RELEASE:-}" in
		24.10*) ;;
		*) die "OpenWrt ${DISTRIB_RELEASE:-unknown} is not supported — use 24.10 or 25.12+" ;;
	esac
else
	die "neither apk nor opkg found"
fi

# Map the device's package architecture to the release asset label.
PKG_ARCH="${DISTRIB_ARCH:-$(cat /etc/apk/arch 2>/dev/null || true)}"
case "$PKG_ARCH" in
	x86_64)                    ARCH=x86_64 ;;
	aarch64_generic)           ARCH=aarch64 ;;
	aarch64_cortex-a53)        ARCH=aarch64_cortexa53 ;;
	arm_cortex-a15_neon-vfpv4) ARCH=armv7 ;;
	mipsel_24kc)               ARCH=mipsel ;;
	*) die "no prebuilt package for architecture '$PKG_ARCH' yet — build it from source (see README: make ipk-<arch>)" ;;
esac

if [ "$VERSION" = "latest" ]; then
	BASE="https://github.com/$REPO/releases/latest/download"
else
	BASE="https://github.com/$REPO/releases/download/$VERSION"
fi

fetch() {
	# uclient-fetch (OpenWrt's wget) follows GitHub's release redirects.
	wget -q -O "$2" "$1" || die "download failed: $1"
	[ -s "$2" ] || die "empty download: $1"
}

say "Pika-box $VERSION for $PKG_ARCH (${DISTRIB_DESCRIPTION:-OpenWrt}, $PM)"
fetch "$BASE/pikabox_${SERIES}_${ARCH}.${EXT}" "$WORKDIR/pikabox.${EXT}"
fetch "$BASE/luci-app-pikabox_${SERIES}.${EXT}" "$WORKDIR/luci-app-pikabox.${EXT}"

say "Updating package lists"
$PM update >/dev/null || die "$PM update failed — check the router's internet connection"

if [ "$PM" = apk ]; then
	say "Installing sing-box (official feed)"
	apk add sing-box
	[ "${PIKABOX_TPROXY:-0}" = "1" ] && { say "Installing kmod-nft-tproxy"; apk add kmod-nft-tproxy; }
	say "Installing Pika-box"
	# Release assets are not in a signed feed, hence --allow-untrusted for these
	# two local files only; sing-box and every dependency come from the
	# official, signed OpenWrt feeds.
	apk add --allow-untrusted "$WORKDIR/pikabox.apk" "$WORKDIR/luci-app-pikabox.apk"
else
	say "Installing sing-box (official feed)"
	opkg install sing-box
	[ "${PIKABOX_TPROXY:-0}" = "1" ] && { say "Installing kmod-nft-tproxy"; opkg install kmod-nft-tproxy; }
	say "Installing Pika-box"
	opkg install "$WORKDIR/pikabox.ipk" "$WORKDIR/luci-app-pikabox.ipk"
fi

/etc/init.d/pikaboxd enable
/etc/init.d/pikaboxd restart
# LuCI caches its menu index; drop it so the new entry shows up immediately.
rm -f /tmp/luci-indexcache* /tmp/luci-modulecache/* 2>/dev/null || true

LAN_IP="$(uci -q get network.lan.ipaddr 2>/dev/null | cut -d/ -f1)"
say "Done. Open LuCI → Services → Pika-box: http://${LAN_IP:-<router-ip>}/cgi-bin/luci/admin/services/pikabox"
say "The proxy stays off until you add an outbound and press Start on the dashboard."
