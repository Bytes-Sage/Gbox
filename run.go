package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
)

func run(cfg config) {
	selfArgs := append([]string{"child",
		"--memory", cfg.memory,
		"--pids", strconv.Itoa(cfg.pids),
		"--rootfs", cfg.rootfs,
		"--hostname", cfg.hostname,
	}, cfg.cmd...)

	cmd := exec.Command("/proc/self/exe", selfArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNS |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
	}

	// Sync pipe: the child blocks on reading this (see child.go) right
	// before it forks the real user command, so it can't fork anything
	// until we've finished adding its PID to the cgroup below. Without
	// this, the child can win the race and fork the real command into the
	// wrong (inherited, host) cgroup.
	syncRead, syncWrite, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gbox: creating sync pipe:", err)
		os.Exit(1)
	}
	cmd.ExtraFiles = []*os.File{syncRead} // becomes fd 3 in the child

	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "gbox:error starting child:", err)
		os.Exit(1)
	}
	syncRead.Close() // parent doesn't need the read end

	memBytes, err := parseMemory(cfg.memory)
	if err != nil {
		_ = cmd.Process.Kill()
		fmt.Fprintln(os.Stderr, "gbox: parsing memory:", err)
		os.Exit(1)
	}

	name := "gbox-" + strconv.Itoa(cmd.Process.Pid)
	groupPath, err := setupCgroup(name, memBytes, cfg.pids)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gbox:cgroup setup:", err)
		_ = cmd.Process.Kill()
		os.Exit(1)
	}
	if err := addPidToCgroup(groupPath, cmd.Process.Pid); err != nil {
		fmt.Fprintln(os.Stderr, "gbox: cgroup add pid:", err)
		_ = cmd.Process.Kill()
		os.Exit(1)
	}

	// Cgroup membership is set — let the child proceed to fork the real
	// command now.
	syncWrite.Write([]byte{0})
	syncWrite.Close()

	// Forward Ctrl+C / SIGTERM into the child instead of abandoning the
	// namespaced process and its cgroup.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		_ = cmd.Process.Signal(sig)
	}()

	waitErr := cmd.Wait()
	signal.Stop(sigCh)

	if err := cleanCgroup(groupPath); err != nil {
		fmt.Fprintln(os.Stderr, "gbox: warning: cgroup cleanup failed:", err)
	}

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "gbox: error:", waitErr)
		os.Exit(1)
	}
}
