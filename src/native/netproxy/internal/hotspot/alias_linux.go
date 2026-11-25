//go:build linux || android

package hotspot

import (
	"fmt"
	"net"

	"github.com/sagernet/netlink"
)

// AddSiteAddress adds the fixed address as a host address without replacing
// Android's gateway or connected subnet. A pre-existing address is never owned
// by this call. The caller must serialize calls and track ownership.
func AddSiteAddress(point AccessPoint) (bool, error) {
	link, err := netlink.LinkByIndex(point.Index)
	if err != nil {
		return false, err
	}
	if link.Attrs().Name != point.Name {
		return false, fmt.Errorf("热点接口已变化")
	}
	address, err := netlink.ParseAddr(point.SiteIP() + "/32")
	if err != nil {
		return false, err
	}
	links, err := netlink.LinkList()
	if err != nil {
		return false, err
	}
	for _, candidate := range links {
		assigned, err := netlink.AddrList(candidate, netlink.FAMILY_V4)
		if err != nil {
			return false, err
		}
		for _, existing := range assigned {
			if existing.IPNet == nil || !existing.IP.Equal(net.ParseIP(point.SiteIP())) {
				continue
			}
			if candidate.Attrs().Index == point.Index && existing.IPNet.String() == address.IPNet.String() {
				return false, nil
			}
			return false, fmt.Errorf("地址 %s 已分配给接口 %s，无法使用", point.SiteIP(), candidate.Attrs().Name)
		}
	}
	if err := netlink.AddrAdd(link, address); err != nil {
		return false, fmt.Errorf("添加热点地址: %w", err)
	}
	return true, nil
}

// RemoveSiteAddress removes only an address the caller actually added.
func RemoveSiteAddress(point AccessPoint, owned bool) error {
	if !owned {
		return nil
	}
	link, err := netlink.LinkByIndex(point.Index)
	if err != nil {
		return err
	}
	if link.Attrs().Name != point.Name {
		return fmt.Errorf("热点接口已变化，未删除地址")
	}
	address, err := netlink.ParseAddr(point.SiteIP() + "/32")
	if err != nil {
		return err
	}
	return netlink.AddrDel(link, address)
}
