// SPDX-License-Identifier: GPL-3.0-or-later
package engine

import (
	"strconv"
	"strings"
)

// Parse the C-style integer prefix used in the original action and configuration
// tables. An explicit radix avoids the Windows libc strtol(base=0) bug. Read only
// the token's bytes: tokens in the resource buffer are not NUL-terminated.
func integerTokenValue(text string) int32 {
	base := 0
	if strings.HasPrefix(text, "$") {
		text = text[1:]
		base = 16
	}
	text = strings.TrimLeft(text, " \t\r\n\v\f")
	i := 0
	sign := ""
	if len(text) != 0 && (text[0] == '-' || text[0] == '+') {
		sign = text[:1]
		i++
	}
	if i == len(text) {
		return 0
	}
	if base == 0 {
		base = 10
		if text[i] == '0' {
			base = 8
		}
	}
	if (base == 8 || base == 16) && len(text)-i >= 3 && text[i] == '0' &&
		(text[i+1] == 'x' || text[i+1] == 'X') && integerDigit(text[i+2]) < 16 {
		base = 16
		i += 2
	}
	start := i
	for i < len(text) && integerDigit(text[i]) < base {
		i++
	}
	if i == start {
		return 0
	}
	// ParseInt saturates on 64-bit overflow, as the source engine's strtol does.
	// The final int32 conversion keeps its original table-field semantics.
	value, _ := strconv.ParseInt(sign+text[start:i], base, 64)
	return int32(value)
}

func integerDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	default:
		return 36
	}
}
