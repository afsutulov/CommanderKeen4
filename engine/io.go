// SPDX-License-Identifier: GPL-2.0-or-later
package engine

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"io"
	"modernc.org/libc"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

type memoryFile struct {
	data  []byte
	pos   int
	path  string
	dirty bool
}

var files = map[uintptr]*memoryFile{}
var nextFile uintptr = 100
var gameResources = map[string][]byte{}
var userDir string

func initResources() error {
	for name, encoded := range packedResources {
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return err
		}
		r, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return err
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return err
		}
		gameResources[name] = data
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	userDir = filepath.Join(dir, "Keen4Go")
	return os.MkdirAll(userDir, 0700)
}

func cbytes(p uintptr, n uint64) []byte {
	if p == 0 || n == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n))
}
func newFile(f *memoryFile) uintptr { nextFile++; files[nextFile] = f; return nextFile }
func FSL_OpenFileInDirCaseInsensitive(tls *libc.TLS, dir, name uintptr, write uint8) uintptr {
	n := strings.ToUpper(filepath.Base(libc.GoString(name)))
	if strings.ContainsAny(n, "/\\") || n == "." || n == ".." {
		return 0
	}
	if write == 0 {
		if data, ok := gameResources[n]; ok {
			return newFile(&memoryFile{data: data})
		}
		data, err := os.ReadFile(filepath.Join(userDir, n))
		if err != nil {
			return 0
		}
		return newFile(&memoryFile{data: data})
	}
	return newFile(&memoryFile{path: filepath.Join(userDir, n), dirty: true})
}
func FSL_CreateFileInDir(tls *libc.TLS, dir, name uintptr) uintptr {
	return FSL_OpenFileInDirCaseInsensitive(tls, dir, name, 1)
}
func FSL_IsDirWritable(tls *libc.TLS, dir uintptr) uint8 { return 1 }
func FS_Read(tls *libc.TLS, dst uintptr, size, count uint64, handle uintptr) uint64 {
	f := files[handle]
	if f == nil || size == 0 || count == 0 {
		return 0
	}
	n := size * count
	available := len(f.data) - f.pos
	if available <= 0 {
		return 0
	}
	if n > uint64(available) {
		n = uint64(available)
	}
	copy(cbytes(dst, n), f.data[f.pos:f.pos+int(n)])
	f.pos += int(n)
	return n / size
}
func FS_Write(tls *libc.TLS, src uintptr, size, count uint64, handle uintptr) uint64 {
	f := files[handle]
	if f == nil || f.path == "" || size == 0 {
		return 0
	}
	n := int(size * count)
	end := f.pos + n
	if end > len(f.data) {
		f.data = append(f.data, make([]byte, end-len(f.data))...)
	}
	copy(f.data[f.pos:end], cbytes(src, uint64(n)))
	f.pos = end
	f.dirty = true
	return count
}
func FS_SeekTo(tls *libc.TLS, handle uintptr, offset uint64) uint64 {
	f := files[handle]
	if f == nil {
		return 0
	}
	old := f.pos
	f.pos = int(offset)
	if f.pos > len(f.data) && f.path == "" {
		f.pos = len(f.data)
	}
	return uint64(old)
}
func FS_GetFileSize(tls *libc.TLS, handle uintptr) uint64 {
	if f := files[handle]; f != nil {
		return uint64(len(f.data))
	}
	return 0
}
func FS_CloseFile(tls *libc.TLS, handle uintptr) {
	f := files[handle]
	if f == nil {
		return
	}
	if f.dirty && f.path != "" {
		tmp := f.path + ".tmp"
		if err := os.WriteFile(tmp, f.data, 0600); err == nil {
			if err = os.Rename(tmp, f.path); err != nil {
				fmt.Fprintln(os.Stderr, "Save:", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "Save:", err)
		}
	}
	delete(files, handle)
}
func FS_PrintF(tls *libc.TLS, handle, format, args uintptr) int32 {
	p := tls.Alloc(8192)
	defer tls.Free(8192)
	n := libc.Xvsnprintf(tls, p, 8192, format, args)
	if n < 0 {
		return n
	}
	length := n
	if length > 8191 {
		length = 8191
	}
	FS_Write(tls, p, 1, uint64(length), handle)
	return n
}

func runtimeRemove(tls *libc.TLS, path uintptr) int32 {
	if err := os.Remove(filepath.Join(userDir, filepath.Base(libc.GoString(path)))); err != nil {
		return -1
	}
	return 0
}
