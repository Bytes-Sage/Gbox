package main

import (
	"flag"
	"fmt"
	"os"
)

type config struct {
	memory   string
	pids     int
	rootfs   string
	hostname string
	cmd      []string
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		cfg := parseRunFlags(os.Args[2:])
		run(cfg)
	case "child":
		cfg := parseRunFlags(os.Args[2:])
		child(cfg)
	default:
		usage()
		os.Exit(1)
	}
}

func parseRunFlags(args []string) config {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	memory := fs.String("memory", "50m", "memory limit, e.g. 100m")
	pids := fs.Int("pids", 20, "max number of processes")
	rootfs := fs.String("rootfs", "./rootfs", "path to root filesystem")
	hostname := fs.String("hostname", "container", "hostname inside the container")
	fs.Parse(args)

	if fs.NArg() == 0 {
		usage()
		os.Exit(1)
	}

	return config{
		memory:   *memory,
		pids:     *pids,
		rootfs:   *rootfs,
		hostname: *hostname,
		cmd:      fs.Args(),
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: gbox run [--memory 100m] [--pids 20] [--rootfs ./rootfs] [--hostname foo] CMD [ARGS...]`)
}
