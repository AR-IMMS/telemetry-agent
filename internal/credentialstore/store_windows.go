//go:build windows

package credentialstore

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const credentialDirectorySDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

func secureDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("credential path is not a real directory")
	}
	return secureWindowsPath(directory)
}

func writeAtomicFile(directory, path string, content []byte) error {
	temporary, err := os.CreateTemp(directory, ".credential-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := secureWindowsPath(temporaryPath); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return err
	}
	return nil
}

func readSecretFile(_ string, path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("credential file is not a regular file")
	}
	if err := secureWindowsPath(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func syncDirectory(string) error { return nil }

func protectSecret(value []byte) ([]byte, error) {
	input := windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
	var output windows.DataBlob
	flags := uint32(windows.CRYPTPROTECT_LOCAL_MACHINE | windows.CRYPTPROTECT_UI_FORBIDDEN)
	if err := windows.CryptProtectData(&input, nil, nil, 0, nil, flags, &output); err != nil {
		return nil, fmt.Errorf("Windows machine protection failed")
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	return append([]byte(nil), unsafe.Slice(output.Data, output.Size)...), nil
}

func unprotectSecret(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("protected credential is empty")
	}
	input := windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
	var output windows.DataBlob
	if err := windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return nil, fmt.Errorf("Windows machine credential could not be unprotected")
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	return append([]byte(nil), unsafe.Slice(output.Data, output.Size)...), nil
}

func secureWindowsPath(path string) error {
	descriptor, err := windows.SecurityDescriptorFromString(credentialDirectorySDDL)
	if err != nil {
		return fmt.Errorf("create service credential access descriptor")
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return fmt.Errorf("read service credential access descriptor")
	}
	return windows.SetNamedSecurityInfo(
		filepath.Clean(path), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil,
	)
}
