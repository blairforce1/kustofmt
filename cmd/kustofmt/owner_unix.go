//go:build unix

package main

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// preserveOwner gives name the same owner and group as the file described by
// info.
//
// Replacing a file atomically means creating a new one, and a new file belongs
// to whoever ran the command. Without this, `sudo kustofmt -w .` would hand
// every manifest it reformatted to root, and the next write by the file's
// actual owner would fail with a permission error nobody would connect to a
// formatting run days earlier.
//
// Failure is reported rather than swallowed. "Best effort" here would mean
// silently changing the ownership of a file in someone's repository, which is
// the outcome this function exists to prevent; refusing the write leaves the
// original exactly as it was, which is always a defensible answer.
func preserveOwner(name string, info fs.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	uid, gid := int(st.Uid), int(st.Gid)

	// Every run where the person formatting a file is the person who owns it
	// lands here, and asking the kernel to change nothing can still fail --
	// setting a group you do not belong to is refused even when it is the
	// group the file already has.
	current, err := os.Stat(name)
	if err != nil {
		return err
	}
	if cur, ok := current.Sys().(*syscall.Stat_t); ok && int(cur.Uid) == uid && int(cur.Gid) == gid {
		return nil
	}

	if err := os.Chown(name, uid, gid); err != nil {
		return fmt.Errorf("cannot preserve ownership (uid %d, gid %d): %w", uid, gid, err)
	}
	return nil
}
