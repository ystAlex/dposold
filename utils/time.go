// utils/time.go
package utils

import "time"

// TimeNow 获取当前UTC时间
func TimeNow() time.Time {
	return time.Now().UTC()
}
