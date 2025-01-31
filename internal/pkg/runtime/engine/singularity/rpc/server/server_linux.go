// Copyright (c) 2018-2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package server

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	args "github.com/sylabs/singularity/internal/pkg/runtime/engine/singularity/rpc"
	"github.com/sylabs/singularity/internal/pkg/sylog"
	"github.com/sylabs/singularity/internal/pkg/util/mainthread"
	"github.com/sylabs/singularity/internal/pkg/util/user"
	"github.com/sylabs/singularity/pkg/util/crypt"
	"github.com/sylabs/singularity/pkg/util/loop"
	"github.com/sylabs/singularity/pkg/util/namespaces"
)

var diskGID = -1

// Methods is a receiver type.
type Methods int

// Mount performs a mount with the specified arguments.
func (t *Methods) Mount(arguments *args.MountArgs, mountErr *error) (err error) {
	mainthread.Execute(func() {
		*mountErr = syscall.Mount(arguments.Source, arguments.Target, arguments.Filesystem, arguments.Mountflags, arguments.Data)
	})
	return nil
}

// Decrypt decrypts the loop device
func (t *Methods) Decrypt(arguments *args.CryptArgs, reply *string) (err error) {
	cryptDev := &crypt.Device{}

	// cryptsetup requires to run in the host IPC namespace
	// so we enter temporarily in the host IPC namespace
	// via the master processus ID if its greater than zero
	// which means that a container IPC namespace was requested
	if arguments.MasterPid > 0 {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if err := namespaces.Enter(arguments.MasterPid, "ipc"); err != nil {
			return fmt.Errorf("while joining host IPC namespace: %s", err)
		}
	}

	cryptName, err := cryptDev.Open(arguments.Key, arguments.Loopdev)

	// return to the container IPC namespace if required
	if arguments.MasterPid > 0 {
		if err := namespaces.Enter(os.Getpid(), "ipc"); err != nil {
			return fmt.Errorf("while joining container IPC namespace: %s", err)
		}
	}

	*reply = "/dev/mapper/" + cryptName

	return err
}

// Mkdir performs a mkdir with the specified arguments.
func (t *Methods) Mkdir(arguments *args.MkdirArgs, reply *int) (err error) {
	mainthread.Execute(func() {
		oldmask := syscall.Umask(0)
		err = os.Mkdir(arguments.Path, arguments.Perm)
		syscall.Umask(oldmask)
	})
	return err
}

// Chroot performs a chroot with the specified arguments.
func (t *Methods) Chroot(arguments *args.ChrootArgs, reply *int) error {
	root := arguments.Root

	if root != "." {
		sylog.Debugf("Change current directory to %s", root)
		if err := syscall.Chdir(root); err != nil {
			return fmt.Errorf("failed to change directory to %s", root)
		}
	} else {
		cwd, err := os.Getwd()
		if err == nil {
			root = cwd
		}
	}

	switch arguments.Method {
	case "pivot":
		// idea taken from libcontainer (and also LXC developers) to avoid
		// creation of temporary directory or use of existing directory
		// for pivot_root.

		sylog.Debugf("Hold reference to host / directory")
		oldroot, err := os.Open("/")
		if err != nil {
			return fmt.Errorf("failed to open host root directory: %s", err)
		}
		defer oldroot.Close()

		sylog.Debugf("Called pivot_root on %s\n", root)
		if err := syscall.PivotRoot(".", "."); err != nil {
			return fmt.Errorf("pivot_root %s: %s", root, err)
		}

		sylog.Debugf("Change current directory to host / directory")
		if err := syscall.Fchdir(int(oldroot.Fd())); err != nil {
			return fmt.Errorf("failed to change directory to old root: %s", err)
		}

		sylog.Debugf("Apply slave mount propagation for host / directory")
		if err := syscall.Mount("", ".", "", syscall.MS_SLAVE|syscall.MS_REC, ""); err != nil {
			return fmt.Errorf("failed to apply slave mount propagation for host / directory: %s", err)
		}

		sylog.Debugf("Called unmount(/, syscall.MNT_DETACH)\n")
		if err := syscall.Unmount(".", syscall.MNT_DETACH); err != nil {
			return fmt.Errorf("unmount pivot_root dir %s", err)
		}
	case "move":
		sylog.Debugf("Move %s as / directory", root)
		if err := syscall.Mount(".", "/", "", syscall.MS_MOVE, ""); err != nil {
			return fmt.Errorf("failed to move %s as / directory: %s", root, err)
		}

		sylog.Debugf("Chroot to %s", root)
		if err := syscall.Chroot("."); err != nil {
			return fmt.Errorf("chroot failed: %s", err)
		}
	case "chroot":
		sylog.Debugf("Chroot to %s", root)
		if err := syscall.Chroot("."); err != nil {
			return fmt.Errorf("chroot failed: %s", err)
		}
	}

	sylog.Debugf("Changing directory to / to avoid getpwd issues\n")
	if err := syscall.Chdir("/"); err != nil {
		return fmt.Errorf("chdir / %s", err)
	}
	return nil
}

// LoopDevice attaches a loop device with the specified arguments.
func (t *Methods) LoopDevice(arguments *args.LoopArgs, reply *int) error {
	var image *os.File

	loopdev := &loop.Device{}
	loopdev.MaxLoopDevices = arguments.MaxDevices
	loopdev.Info = &arguments.Info
	loopdev.Shared = arguments.Shared

	if strings.HasPrefix(arguments.Image, "/proc/self/fd/") {
		strFd := strings.TrimPrefix(arguments.Image, "/proc/self/fd/")
		fd, err := strconv.ParseUint(strFd, 10, 32)
		if err != nil {
			return fmt.Errorf("failed to convert image file descriptor: %v", err)
		}
		image = os.NewFile(uintptr(fd), "")
	} else {
		var err error
		image, err = os.OpenFile(arguments.Image, arguments.Mode, 0600)
		if err != nil {
			return fmt.Errorf("could not open image file: %v", err)
		}
	}

	if diskGID == -1 {
		if gr, err := user.GetGrNam("disk"); err == nil {
			diskGID = int(gr.GID)
		} else {
			diskGID = 0
		}
	}

	runtime.LockOSThread()
	syscall.Setfsuid(0)
	syscall.Setfsgid(diskGID)
	defer runtime.UnlockOSThread()
	defer syscall.Setfsuid(os.Getuid())
	defer syscall.Setfsgid(os.Getgid())

	err := loopdev.AttachFromFile(image, arguments.Mode, reply)
	if err != nil {
		return fmt.Errorf("could not attach image file to loop device: %v", err)
	}
	return nil
}

// SetHostname sets hostname with the specified arguments.
func (t *Methods) SetHostname(arguments *args.HostnameArgs, reply *int) error {
	return syscall.Sethostname([]byte(arguments.Hostname))
}

// SetFsID sets filesystem uid and gid.
func (t *Methods) SetFsID(arguments *args.SetFsIDArgs, reply *int) error {
	mainthread.Execute(func() {
		syscall.Setfsuid(arguments.UID)
		syscall.Setfsgid(arguments.GID)
	})
	return nil
}

// Chdir changes current working directory to path.
func (t *Methods) Chdir(arguments *args.ChdirArgs, reply *int) error {
	return mainthread.Chdir(arguments.Dir)
}
