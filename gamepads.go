// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"Keen4/engine"
)

func (g *game) updateGamepads() {
	var pads [2]engine.Gamepad
	ids := ebiten.AppendGamepadIDs(nil)
	for i, id := range ids {
		if i >= len(pads) {
			break
		}
		p := &pads[i]
		p.Present = true
		if ebiten.IsStandardGamepadLayoutAvailable(id) {
			p.X = int32(ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal) * 1000)
			p.Y = int32(ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical) * 1000)
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftTop) {
				p.Y = -1000
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftBottom) {
				p.Y = 1000
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftLeft) {
				p.X = -1000
			}
			if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftRight) {
				p.X = 1000
			}
			for b := 0; b < 16; b++ {
				if ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButton(b)) {
					p.Buttons |= 1 << b
				}
			}
		} else {
			if ebiten.GamepadAxisCount(id) > 0 {
				p.X = int32(ebiten.GamepadAxisValue(id, 0) * 1000)
			}
			if ebiten.GamepadAxisCount(id) > 1 {
				p.Y = int32(ebiten.GamepadAxisValue(id, 1) * 1000)
			}
			for b := 0; b < min(16, ebiten.GamepadButtonCount(id)); b++ {
				if ebiten.IsGamepadButtonPressed(id, ebiten.GamepadButton(b)) {
					p.Buttons |= 1 << b
				}
			}
		}
	}
	select {
	case g.engine.Gamepads <- pads:
	default:
	}
}
