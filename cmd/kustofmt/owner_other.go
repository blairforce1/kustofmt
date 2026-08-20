//go:build !unix

package main

import "io/fs"

// preserveOwner does nothing on platforms that do not report a numeric owner.
//
// Windows expresses ownership as a security descriptor rather than a uid and
// gid pair, and copying one onto a replacement file needs the security API
// rather than a chown. A new file there inherits its ACL from the containing
// directory, which is the same thing every other tool that writes a file in
// that directory produces.
func preserveOwner(_ string, _ fs.FileInfo) error { return nil }
