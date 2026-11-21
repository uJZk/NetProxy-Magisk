//go:build !linux && !android

package hotspot

import "errors"

func AddSiteAddress(AccessPoint) (bool, error) {
	return false, errors.New("此平台不支持热点地址管理")
}
func RemoveSiteAddress(AccessPoint, bool) error { return nil }
