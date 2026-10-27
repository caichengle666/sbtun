//go:build !windows

package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func preserveFileMetadata(path string, info os.FileInfo) error {
	if err := os.Chmod(path, info.Mode().Perm()); err != nil {
		return err
	}
	if uid, gid, ok := invokerOwnership(); ok {
		return applyOwnership(path, uid, gid)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return applyOwnership(path, int(stat.Uid), int(stat.Gid))
}

func applyInvokerOwnership(path string) error {
	uid, gid, ok := invokerOwnership()
	if !ok {
		return nil
	}
	return applyOwnership(path, uid, gid)
}

func invokerOwnership() (int, int, bool) {
	uidText := os.Getenv("SUDO_UID")
	if uidText == "" {
		uidText = os.Getenv("PKEXEC_UID")
	}
	uid, err := strconv.Atoi(uidText)
	if err != nil || uid < 0 {
		return 0, 0, false
	}
	gid := -1
	if gidText := os.Getenv("SUDO_GID"); gidText != "" {
		if parsed, parseErr := strconv.Atoi(gidText); parseErr == nil && parsed >= 0 {
			gid = parsed
		}
	}
	return uid, gid, true
}

func applyOwnership(path string, uid, gid int) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	changeUID, changeGID := -1, -1
	if int(stat.Uid) != uid {
		changeUID = uid
	}
	if gid >= 0 && int(stat.Gid) != gid {
		changeGID = gid
	}
	if changeUID == -1 && changeGID == -1 {
		return nil
	}
	return os.Chown(path, changeUID, changeGID)
}

// ShareWithInvoker sets path's mode and, under sudo/pkexec, hands it back to
// the invoking user so unprivileged CLI commands can still read it.
func ShareWithInvoker(path string, mode os.FileMode) error {
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	return applyInvokerOwnership(path)
}

// ShareTreeWithInvoker hands every root-owned entry under dir back to the
// invoking user. It repairs files an older version created as root; it does
// nothing unless running as root with a known invoker. Private keys (*.key,
// the capture CA) stay root-only.
func ShareTreeWithInvoker(dir string) {
	uid, gid, ok := invokerOwnership()
	if !ok || os.Geteuid() != 0 {
		return
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink != 0 || strings.HasSuffix(d.Name(), ".key") {
			return nil
		}
		if info, err := d.Info(); err == nil {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid == 0 {
				_ = applyOwnership(path, uid, gid)
			}
		}
		return nil
	})
}
