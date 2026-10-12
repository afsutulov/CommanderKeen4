// SPDX-License-Identifier: GPL-2.0-or-later
package engine

import (
	"fmt"
	"modernc.org/libc"
	"runtime/debug"
	"sync"
	"time"
	"unsafe"
)

const SampleRate = 44100
const pitRate = 1193182

type Gamepad struct {
	Present bool
	X, Y    int32
	Buttons uint16
}
type KeyEvent struct {
	Scan uint8
	Down bool
}
type Frame struct {
	Pixels []byte
	Border [3]byte
	Number uint64
}
type exitSignal int32
type Runtime struct {
	Gamepads    chan [2]Gamepad
	pads        [2]Gamepad
	Keys        chan KeyEvent
	Fullscreen  chan bool
	Frames      chan Frame
	Done        chan error
	Stop        chan struct{}
	stopOnce    sync.Once
	pcmMu       sync.Mutex
	pcm         []byte
	frameNumber uint64
}

var rt *Runtime
var clockStarted bool
var clockLast time.Time
var timerDivisor int16 = 8514
var interruptRemainder float64
var sampleRemainder float64
var chipMemory, soundMemory uintptr
var speakerOn bool
var speakerDivisor int32
var speakerPhase float64
var nextPresent time.Time

func New() (*Runtime, error) {
	if err := initResources(); err != nil {
		return nil, err
	}
	rt = &Runtime{Gamepads: make(chan [2]Gamepad, 4), Fullscreen: make(chan bool, 2), Keys: make(chan KeyEvent, 512), Frames: make(chan Frame, 2), Done: make(chan error, 1), Stop: make(chan struct{})}
	return rt, nil
}
func (r *Runtime) Close() { r.stopOnce.Do(func() { close(r.Stop) }) }
func (r *Runtime) Run(args []string) {
	tls := libc.NewTLS()
	defer tls.Close()
	defer func() {
		if p := recover(); p != nil {
			if e, ok := p.(exitSignal); ok {
				if e != 0 {
					r.Done <- fmt.Errorf("game exited with code %d", e)
				} else {
					r.Done <- nil
				}
			} else {
				r.Done <- fmt.Errorf("%v\n%s", p, debug.Stack())
			}
		} else {
			r.Done <- nil
		}
	}()
	args = append([]string{"keen4", "/EPISODE", "4"}, args...)
	argv := libc.Xcalloc(tls, uint64(len(args)+1), 8)
	defer libc.Xfree(tls, argv)
	for i, s := range args {
		p := libc.Xmalloc(tls, uint64(len(s)+1))
		copy(cbytes(p, uint64(len(s))), s)
		*(*uintptr)(unsafe.Pointer(argv + uintptr(i*8))) = p
		defer libc.Xfree(tls, p)
	}
	Xmain(tls, int32(len(args)), argv)
}
func runtimeExit(tls *libc.TLS, code int32) { panic(exitSignal(code)) }

// Audio samples are generated on the engine goroutine. The device only reads
// copies, so it never races with the game state or emulated OPL registers.
func (r *Runtime) Read(p []byte) (int, error) {
	r.pcmMu.Lock()
	defer r.pcmMu.Unlock()
	n := copy(p, r.pcm)
	r.pcm = r.pcm[n:]
	clear(p[n:])
	return len(p), nil
}
func renderAudio(tls *libc.TLS, n int) {
	if n <= 0 || chipMemory == 0 {
		return
	}
	if n > 4096 {
		n = 4096
	}
	XOPL3_GenerateStream(tls, chipMemory, soundMemory, uint32(n))
	samples := unsafe.Slice((*int16)(unsafe.Pointer(soundMemory)), n*2)
	if speakerOn && speakerDivisor > 0 {
		step := float64(pitRate) / float64(speakerDivisor) / SampleRate
		for i := 0; i < n; i++ {
			speakerPhase += step
			speakerPhase -= float64(int(speakerPhase))
			v := int32(2200)
			if speakerPhase < .5 {
				v = -v
			}
			for ch := 0; ch < 2; ch++ {
				a := int32(samples[i*2+ch]) + v
				if a > 32767 {
					a = 32767
				}
				if a < -32768 {
					a = -32768
				}
				samples[i*2+ch] = int16(a)
			}
		}
	}
	rt.pcmMu.Lock()
	// Avoid accumulating old audio if the output device was paused or unavailable.
	if len(rt.pcm) > SampleRate*4/4 {
		rt.pcm = rt.pcm[:0]
	}
	rt.pcm = append(rt.pcm, cbytes(soundMemory, uint64(n*4))...)
	rt.pcmMu.Unlock()
}
func advanceClock(tls *libc.TLS) {
	if !clockStarted {
		return
	}
	now := time.Now()
	elapsed := now.Sub(clockLast).Seconds()
	clockLast = now
	if elapsed > .1 {
		elapsed = .1
	}
	interruptRemainder += elapsed * pitRate
	for interruptRemainder >= float64(timerDivisor) {
		div := float64(timerDivisor)
		interruptRemainder -= div
		XSDL_t0Service(tls)
		sampleRemainder += div * SampleRate / pitRate
		n := int(sampleRemainder)
		sampleRemainder -= float64(n)
		renderAudio(tls, n)
	}
}
func SD_GetTimeCount(tls *libc.TLS) uint32 { advanceClock(tls); return sd_timeCount }
func SD_NULL_Startup(tls *libc.TLS) {
	chipMemory = libc.Xcalloc(tls, 1, uint64(unsafe.Sizeof(opl3_chip{})))
	soundMemory = libc.Xcalloc(tls, 4096, 4)
	XOPL3_Reset(tls, chipMemory, SampleRate)
	clockLast = time.Now()
	clockStarted = true
}
func SD_NULL_Shutdown(tls *libc.TLS) {
	clockStarted = false
	if chipMemory != 0 {
		libc.Xfree(tls, chipMemory)
		chipMemory = 0
	}
	if soundMemory != 0 {
		libc.Xfree(tls, soundMemory)
		soundMemory = 0
	}
}
func SD_NULL_SetTimer0(tls *libc.TLS, divisor int16) {
	if divisor > 0 {
		timerDivisor = divisor
	}
}
func SD_NULL_alOut(tls *libc.TLS, reg, val uint8) {
	if chipMemory != 0 {
		XOPL3_WriteReg(tls, chipMemory, uint16(reg), val)
	}
}
func SD_NULL_PCSpkOn(tls *libc.TLS, on uint8, freq int32) { speakerOn = on != 0; speakerDivisor = freq }
func SD_NULL_Detect(tls *libc.TLS) uint32                 { return 1 }
func SD_NULL_SetOPL3(tls *libc.TLS, on uint8)             {}
func SD_NULL_WaitTick(tls *libc.TLS) {
	time.Sleep(time.Millisecond)
	IN_NULL_PumpEvents(tls)
	advanceClock(tls)
}

func IN_NULL_PumpEvents(tls *libc.TLS) {
	select {
	case <-rt.Stop:
		XQuit(tls, 0)
	default:
	}
	for {
		select {
		case pads := <-rt.Gamepads:
			rt.pads = pads
		case e := <-rt.Keys:
			if e.Down {
				XIN_HandleKeyDown(tls, e.Scan, 0)
			} else {
				XIN_HandleKeyUp(tls, e.Scan, 0)
			}
		default:
			return
		}
	}
}
func IN_NULL_WaitKey(tls *libc.TLS) {
	select {
	case e := <-rt.Keys:
		if e.Down {
			XIN_HandleKeyDown(tls, e.Scan, 0)
		} else {
			XIN_HandleKeyUp(tls, e.Scan, 0)
		}
	case <-rt.Stop:
		XQuit(tls, 0)
	case <-time.After(time.Millisecond * 5):
	}
	advanceClock(tls)
}
func VL_NULL_Present(tls *libc.TLS, surface uintptr, sx, sy int32, single uint8) {
	IN_NULL_PumpEvents(tls)
	advanceClock(tls)
	if surface == 0 {
		return
	}
	s := (*VL_NULL_Surface)(unsafe.Pointer(surface))
	data := cbytes(s.Fdata, uint64(s.Fw*s.Fh))
	pixels := make([]byte, 320*200*4)
	for y := int32(0); y < 200; y++ {
		for x := int32(0); x < 320; x++ {
			px, py := x+sx, y+sy
			if px < 0 || py < 0 || px >= s.Fw || py >= s.Fh {
				continue
			}
			index := data[py*s.Fw+px] & 15
			color := XVL_EGARGBColorTable[Xvl_emuegavgaadapter.Fpalette[index]&15]
			dst := (y*320 + x) * 4
			pixels[dst] = color[0]
			pixels[dst+1] = color[1]
			pixels[dst+2] = color[2]
			pixels[dst+3] = 255
		}
	}
	rt.frameNumber++
	frame := Frame{Pixels: pixels, Border: XVL_EGARGBColorTable[Xvl_emuegavgaadapter.Fbordercolor&15], Number: rt.frameNumber}
	select {
	case rt.Frames <- frame:
	default:
		select {
		case <-rt.Frames:
		default:
		}
		select {
		case rt.Frames <- frame:
		default:
		}
	}
	now := time.Now()
	if nextPresent.IsZero() || nextPresent.Before(now.Add(-time.Second/70)) {
		nextPresent = now
	}
	nextPresent = nextPresent.Add(time.Second / 70)
	if d := time.Until(nextPresent); d > 0 {
		time.Sleep(d)
	}
	advanceClock(tls)
}
func VL_NULL_WaitVBLs(tls *libc.TLS, vbls int32) {
	if vbls < 1 {
		vbls = 1
	}
	time.Sleep(time.Duration(vbls) * time.Second / 70)
	IN_NULL_PumpEvents(tls)
	advanceClock(tls)
}
func VL_NULL_FlushParams(tls *libc.TLS) {
	select {
	case rt.Fullscreen <- Xvl_isFullScreen != 0:
	default:
	}
}

func padPresent(tls *libc.TLS, index int32) uint8 {
	if index >= 0 && index < 2 && rt.pads[index].Present {
		return 1
	}
	return 0
}
func padAxes(tls *libc.TLS, index int32, x, y uintptr) {
	var p Gamepad
	if index >= 0 && index < 2 {
		p = rt.pads[index]
	}
	if x != 0 {
		*(*int32)(unsafe.Pointer(x)) = p.X
	}
	if y != 0 {
		*(*int32)(unsafe.Pointer(y)) = p.Y
	}
}
func padButtons(tls *libc.TLS, index int32) uint16 {
	if index >= 0 && index < 2 {
		return rt.pads[index].Buttons
	}
	return 0
}
