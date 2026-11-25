package install

import (
	"syscall"
	"unsafe"
)

func importFreeSpace(path string) (uint64, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var available uint64
	ok, _, callErr := getDiskFreeSpace.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&available)), 0, 0)
	if ok == 0 {
		return 0, callErr
	}
	return available, err
}

func publishImport(from, to string) error {
	f, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	t, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return syscall.MoveFile(f, t)
}

func recoverImportPublication(to string) error { return nil }

var getDiskFreeSpace = kernel32.NewProc("GetDiskFreeSpaceExW")
