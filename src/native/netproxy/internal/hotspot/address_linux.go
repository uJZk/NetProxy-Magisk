//go:build linux || android

package hotspot

import (
	"fmt"
	"net"
	"time"

	"github.com/mdlayher/wifi"
)

// DetectAccessPoint uses nl80211 AP mode, not device-specific interface names.
func DetectAccessPoint() (AccessPoint, error) {
	client, err := wifi.New()
	if err != nil {
		return AccessPoint{}, fmt.Errorf("查询无线接口: %w", err)
	}
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return AccessPoint{}, fmt.Errorf("设置无线查询时限: %w", err)
	}
	interfaces, err := client.Interfaces()
	if err != nil {
		return AccessPoint{}, fmt.Errorf("查询无线接口: %w", err)
	}
	var points []AccessPoint
	for _, iface := range interfaces {
		if iface.Type != wifi.InterfaceTypeAP {
			continue
		}
		link, err := net.InterfaceByIndex(iface.Index)
		if err != nil {
			return AccessPoint{}, fmt.Errorf("查询热点接口 %s: %w", iface.Name, err)
		}
		if link.Flags&net.FlagUp == 0 {
			continue
		}
		points = append(points, AccessPoint{Name: iface.Name, Index: iface.Index})
	}
	return SelectAccessPoint(points)
}
