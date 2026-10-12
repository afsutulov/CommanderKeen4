// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import "time"

// The DOS engine seeds its own random table from Unix seconds. Keep that
// algorithm, while avoiding the unimplemented Windows libc time() function.
func gameUnixTime() int64 {
	return time.Now().Unix()
}

// The retained creature-question code uses only the local hour and day;
// obtaining them directly also avoids platform-specific struct tm layouts.
func creatureCalendarSeed() int32 {
	now := time.Now()
	return int32(now.Hour() + now.Day())
}
