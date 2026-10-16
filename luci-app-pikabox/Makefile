include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-pikabox
# Stamped from the release tag via PIKABOX_PKG_VERSION; see pikabox/Makefile.
PKG_VERSION:=$(if $(PIKABOX_PKG_VERSION),$(PIKABOX_PKG_VERSION),0.1.0)
PKG_RELEASE:=1

LUCI_TITLE:=LuCI support for Pika-box
LUCI_DEPENDS:=+pikabox
LUCI_PKGARCH:=all

include $(TOPDIR)/feeds/luci/luci.mk

# call BuildPackage - OpenWrt buildroot signature
