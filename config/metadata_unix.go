//go:build !windows

package config

import (
	"os"
	"strconv"
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
