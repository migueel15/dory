//go:build windows

package runtime

import (
	"bytes"
	"context"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	agyAdvapi32 = windows.NewLazySystemDLL("advapi32.dll")
	agyCredRead = agyAdvapi32.NewProc("CredReadW")
	agyCredFree = agyAdvapi32.NewProc("CredFree")
)

// Prefix of the Windows CREDENTIALW struct through CredentialBlob.
type agyWindowsCredential struct {
	flags, typ          uint32
	targetName, comment *uint16
	lastWritten         windows.Filetime
	size                uint32
	blob                *byte
}

func readAgyKeyring(_ context.Context) []byte {
	target, err := windows.UTF16PtrFromString("gemini:antigravity")
	if err != nil {
		return nil
	}
	var credential *agyWindowsCredential
	const genericCredential = 1
	ok, _, _ := agyCredRead.Call(uintptr(unsafe.Pointer(target)), genericCredential, 0, uintptr(unsafe.Pointer(&credential)))
	if ok == 0 || credential == nil {
		return nil
	}
	defer agyCredFree.Call(uintptr(unsafe.Pointer(credential)))
	if credential.blob == nil || credential.size == 0 || credential.size > antigravitySecretLimit {
		return nil
	}
	raw := unsafe.Slice(credential.blob, credential.size)
	if len(raw)%2 == 0 && raw[1] == 0 {
		return []byte(string(utf16.Decode(unsafe.Slice((*uint16)(unsafe.Pointer(credential.blob)), len(raw)/2))))
	}
	return bytes.Clone(raw)
}
