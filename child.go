package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

const containerPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

func child(cfg config) {
	must(syscall.Sethostname([]byte(cfg.hostname)), "sethostname")
	must(makeMountsPrivate(), "make mounts private")
	must(unix.Chroot(cfg.rootfs), "chroot")
	must(os.Chdir("/"), "chdir")
	must(mountProc(), "mount proc")

	// Block here until the parent has finished adding our PID to the
	// cgroup. fd 3 is the sync pipe's read end, passed via ExtraFiles in
	// run.go. See the comment there for why this matters.
	syncFile := os.NewFile(3, "sync")
	buf := make([]byte, 1)
	syncFile.Read(buf)
	syncFile.Close()

	// os.Setenv makes exec.LookPath search containerPath (evaluated against
	// the chroot's filesystem, since we're already inside it) instead of
	// whatever PATH gbox itself inherited from the host/sudo.
	os.Setenv("PATH", containerPath)
	binPath, err := exec.LookPath(cfg.cmd[0])
	must(err, "resolving "+cfg.cmd[0])

	// unix.Exec (execve) replaces THIS process's image with cfg.cmd — it
	// doesn't fork. That's what makes cfg.cmd become PID 1 in the new
	// namespace directly, matching scope.md's "the shell is PID 1" test.
	// On success it never returns; there is no code after this line.
	env := []string{"PATH=" + containerPath}
	must(unix.Exec(binPath, cfg.cmd, env), "exec "+binPath)
}

func must(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "gbox: %s: %v\n", what, err)
		os.Exit(1)
	}
}
