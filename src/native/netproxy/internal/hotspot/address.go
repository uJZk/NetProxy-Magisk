package hotspot

import (
	"errors"
	"fmt"
	"sort"
)

// DefaultSiteAddress is an additional address, not a replacement for Android's
// hotspot gateway. It is represented as a /32 when assigned to the AP.
const DefaultSiteAddress = "192.2.0.1"

type AccessPoint struct {
	Name  string
	Index int
}

// SelectAccessPoint avoids guessing if more than one AP interface is active.
func SelectAccessPoint(points []AccessPoint) (AccessPoint, error) {
	if len(points) == 0 {
		return AccessPoint{}, errors.New("未检测到无线热点接口")
	}
	if len(points) > 1 {
		names := make([]string, 0, len(points))
		for _, point := range points {
			names = append(names, point.Name)
		}
		sort.Strings(names)
		return AccessPoint{}, fmt.Errorf("检测到多个热点接口，无法确定要绑定的接口: %v", names)
	}
	return points[0], nil
}
