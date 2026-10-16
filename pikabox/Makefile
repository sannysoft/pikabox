include $(TOPDIR)/rules.mk

PKG_NAME:=pikabox
# Stamped from the release tag via PIKABOX_PKG_VERSION (exported by the top-level
# Makefile / release workflow). Without a bumped version the published package
# matches what is installed and OpenWrt never offers an upgrade. Falls back to a
# base version for standalone/dev builds.
PKG_VERSION:=$(if $(PIKABOX_PKG_VERSION),$(PIKABOX_PKG_VERSION),0.1.0)
PKG_RELEASE:=1

PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_NAME)-$(PKG_VERSION)
PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
# cgo is incompatible with MIPS16 (Go's gcc_mipsx.S fails to assemble), so
# disable it for MIPS targets like ramips/mt7621. PKG_BUILD_FLAGS:=no-mips16 is
# the mechanism the upstream sing-box package uses; the older PKG_USE_MIPS16:=0
# is not honored on current OpenWrt.
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/sannysoft/pikabox
GO_PKG_BUILD_PKG:=github.com/sannysoft/pikabox/cmd/pikaboxd

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

GO_PKG_BUILD_VARS+=GOFLAGS="-buildvcs=false"
# Shown in the LuCI dashboard banner and Settings → About.
GO_PKG_LDFLAGS_X:=main.version=$(PKG_VERSION)

define Package/pikabox
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Pika-box daemon for OpenWrt
  # kmod-nft-tproxy is required at runtime for transparent (TProxy) mode but is
  # intentionally NOT declared here. Unlike sing-box's leaf kmod deps
  # (kmod-tun/kmod-inet-diag), kmod-nft-tproxy pulls sibling netfilter modules
  # whose .ko's the OpenWrt SDK cannot build, which breaks every SDK/CI package
  # build. Install it on the router when enabling transparent proxying:
  #   apk add kmod-nft-tproxy   (or: opkg install kmod-nft-tproxy)
  # The daemon reports a clear, actionable error if the module is missing.
  DEPENDS:=$(GO_ARCH_DEPENDS) +sing-box +rpcd +ubus +uci +jsonfilter
endef

define Package/pikabox/description
 Lightweight OpenWrt-native Sing-Box manager daemon with UCI-backed
 configuration, rpcd/ubus API exposure, and generated sing-box runtime
 configuration.

 Transparent (TProxy) mode additionally requires the kmod-nft-tproxy kernel
 module; install it with `apk add kmod-nft-tproxy` (or `opkg install
 kmod-nft-tproxy`) when enabling that feature.
endef

define Build/Prepare
	$(call Build/Prepare/Default)
	$(CP) ./src/. $(PKG_BUILD_DIR)/
endef

define Package/pikabox/conffiles
/etc/config/pikabox
endef

define Package/pikabox/install
	$(call GoPackage/Package/Install/Bin,$(1))
	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/etc/config/pikabox $(1)/etc/config/pikabox
	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/pikaboxd $(1)/etc/init.d/pikaboxd
	$(INSTALL_DIR) $(1)/etc/uci-defaults
	$(INSTALL_BIN) ./files/etc/uci-defaults/90-pikabox $(1)/etc/uci-defaults/90-pikabox
	$(INSTALL_DIR) $(1)/etc/pikabox/generated
	$(INSTALL_DIR) $(1)/etc/pikabox/rulesets
	$(INSTALL_DIR) $(1)/usr/libexec/rpcd
	$(LN) /usr/bin/pikaboxd $(1)/usr/libexec/rpcd/pikabox
endef

# Restart the long-running daemon on (re)install so it re-execs the new binary.
# Without this, an upgrade leaves the old `serve` process running a deleted
# inode and write RPCs added in the new build return "unknown method".
define Package/pikabox/postinst
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] || {
	/etc/init.d/pikaboxd enable
	/etc/init.d/pikaboxd restart
}
exit 0
endef

$(eval $(call GoBinPackage,pikabox))
$(eval $(call BuildPackage,pikabox))
