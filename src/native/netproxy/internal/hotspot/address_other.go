//go:build !linux && !android

package hotspot

import "errors"

func DetectAccessPoint() (AccessPoint, error) {
	return AccessPoint{}, errors.New("此平台不支持热点自动检测")
}
