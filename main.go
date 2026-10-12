// SPDX-License-Identifier: GPL-2.0-or-later
package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"image"
	"image/color"
	_ "image/png"
	"Keen4/engine"
	"os"
)

//go:generate go run ./internal/iconresource

//go:embed icon.png
var iconPNG []byte

type game struct {
	engine      *engine.Runtime
	image       *ebiten.Image
	borderImage *ebiten.Image
	border      color.RGBA
	held        map[uint8]bool
	started     bool
	stopped     bool
	player      *audio.Player
	args        []string
	mute        bool
}

func (g *game) Update() error {
	if !g.started {
		g.updateGamepads()
		g.started = true
		go g.engine.Run(g.args)
		if !g.mute {
			ctx := audio.NewContext(engine.SampleRate)
			player, err := ctx.NewPlayer(g.engine)
			if err == nil {
				g.player = player
				player.Play()
			} else {
				fmt.Fprintln(os.Stderr, "Audio:", err)
			}
		}
	}
	if ebiten.IsWindowBeingClosed() && !g.stopped {
		g.stopped = true
		g.engine.Close()
	}
	select {
	case err := <-g.engine.Done:
		if g.player != nil {
			g.player.Close()
		}
		if err != nil {
			return err
		}
		return ebiten.Termination
	default:
	}
	downScans := map[uint8]bool{}
	for key, scan := range keyScans {
		if ebiten.IsKeyPressed(key) && ebiten.IsFocused() {
			downScans[scan] = true
		}
	}
	for _, scan := range keyScans {
		down := downScans[scan]
		if down != g.held[scan] {
			g.held[scan] = down
			g.engine.Keys <- engine.KeyEvent{Scan: scan, Down: down}
		}
	}
	g.updateGamepads()

	for {
		select {
		case f := <-g.engine.Frames:
			g.image.WritePixels(f.Pixels)
			g.border = color.RGBA{f.Border[0], f.Border[1], f.Border[2], 255}
		case fullscreen := <-g.engine.Fullscreen:
			ebiten.SetFullscreen(fullscreen)
		default:
			return nil
		}
	}
}
func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(color.Black)
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	scale := min(float64(w)/336, float64(h)/249.6)
	bw, bh := 336*scale, 249.6*scale
	x, y := (float64(w)-bw)/2, (float64(h)-bh)/2
	// The 320x200 EGA image was displayed as 4:3 on a CRT.
	border := g.borderImage
	border.Fill(g.border)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(bw, bh)
	op.GeoM.Translate(x, y)
	screen.DrawImage(border, op)
	op = &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale*1.2)
	op.GeoM.Translate(x+8*scale, y+4*1.2*scale)
	op.Filter = ebiten.FilterNearest
	screen.DrawImage(g.image, op)
}
func (g *game) Layout(w, h int) (int, int) { return w, h }
func main() {
	rt, err := engine.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ebiten.SetWindowTitle("Commander Keen 4 — Secret of the Oracle")
	ebiten.SetWindowSize(1008, 749)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowClosingHandled(true)
	ebiten.SetTPS(70)
	if icon, _, err := image.Decode(bytes.NewReader(iconPNG)); err == nil {
		ebiten.SetWindowIcon([]image.Image{icon})
	}
	g := &game{engine: rt, image: ebiten.NewImage(320, 200), borderImage: ebiten.NewImage(1, 1), held: map[uint8]bool{}}
	for _, arg := range os.Args[1:] {
		if arg == "--mute" {
			g.mute = true
		} else {
			g.args = append(g.args, arg)
		}
	}
	if err := ebiten.RunGame(g); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
