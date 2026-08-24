package main

import (
	"golang.org/x/sys/unix"
)

func makeMountsPrivate() error {
	return unix.Mount("", "/", "", unix.MS_PRIVATE|unix.MS_REC, "")
}

func mountProc() error {
	return unix.Mount("proc", "/proc", "proc", 0, "")
}
