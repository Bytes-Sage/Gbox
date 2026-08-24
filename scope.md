# Project Scope: Gbox — a mini container runtime in Go

## Goal

Build a small program that starts a shell which _thinks_ it is alone on the
computer. Its own hostname, its own process list, its own root filesystem, its
own memory limit.

Target command:

```bash
sudo ./gbox run /bin/sh
```

The point is learning, not shipping. When this works, I should be able to
explain what a container actually is without hand-waving.

## Non-goals (do not build these)

- No image format, no layers, no overlayfs
- No Dockerfile / build system
- No daemon, no API, no client-server split
- No pulling images from a registry (milestone 6 only, if there's time)
- No Windows/macOS support — Linux only, on purpose

## Hard requirements

- Language: Go, standard library plus `golang.org/x/sys/unix`
- Runs on Arch Linux, cgroups v2 (unified `/sys/fs/cgroup`)
- Must be run with `sudo` (root). User namespaces are a stretch goal.
- No container libraries (no libcontainer, no runc). Writing the syscalls by
  hand is the whole exercise.

## The one weird trick to know up front

I cannot just call `unshare()` and keep going, because the Go runtime is
already running many threads and namespaces apply per-thread. So the program
runs itself twice:

1. `gbox run /bin/sh` — the **parent**. Sets up the flags and starts a
   child.
2. The child is `/proc/self/exe child /bin/sh` — the same binary, called again
   with a hidden `child` command. The child is born already inside the new
   namespaces, and does the rest of the setup.

Every tutorial does this. It will look strange the first time. It is correct.

---

## Milestones

Do these in order. Each one should compile, run, and be tested before moving
on.

### Milestone 0 — Run a command at all

Just `os/exec`. Run the command the user typed, wire `Stdin`/`Stdout`/`Stderr`
to the terminal, return the exit code.

- Learn: `exec.Command`, `cmd.Run()`, error handling, exit codes
- Test: `sudo ./gbox run /bin/echo hi` prints `hi`

### Milestone 1 — Namespaces (the isolation)

Add the parent/child re-exec split. Add `SysProcAttr.Cloneflags` with:

- `CLONE_NEWUTS` — own hostname
- `CLONE_NEWPID` — own process numbers
- `CLONE_NEWNS` — own mount list
- `CLONE_NEWIPC` — own shared memory
- `CLONE_NEWNET` — own network (will have _no_ network at all, that's expected)

In the child, set the hostname to `container`.

- Learn: what each namespace actually separates
- Test: inside the shell, `hostname` says `container`, and the host's hostname
  is unchanged after exiting

### Milestone 2 — Root filesystem

Download an Alpine minirootfs tarball, unpack it to `./rootfs`. In the child:
`chroot("./rootfs")`, then `chdir("/")`, then mount a fresh `/proc`.

Important: also set `SysProcAttr.Unshareflags = CLONE_NEWNS` in the parent, and
remount `/` as private. Without this, mounts leak onto the host and I will be
confused and annoyed.

- Learn: `chroot`, `mount`, mount propagation, why `/proc` must be re-mounted
- Test: inside, `ps aux` shows only 2–3 processes and the shell is PID 1. `ls /`
  shows Alpine's files, not mine.
- Cleanup: unmount `/proc` when the container exits

### Milestone 3 — Resource limits (cgroups v2)

Create `/sys/fs/cgroup/gbox/<name>/`, write the child's PID into
`cgroup.procs`, write limits into `memory.max` and `pids.max`.

Watch out: the controller must be listed in the parent's `cgroup.subtree_control`
or writing the limit file fails. Check this and give a clear error.

- Learn: cgroups v2 layout, why it's just files, why it's separate from
  namespaces (namespaces = what you can _see_, cgroups = what you can _use_)
- Test: set memory to 20MB, then run something memory-hungry inside and watch
  it get killed
- Cleanup: remove the cgroup directory after exit (it must be empty first)

### Milestone 4 — Make it usable

Proper CLI with flags:

```
gbox run [--memory 100m] [--pids 20] [--rootfs ./rootfs] [--hostname foo] CMD [ARGS...]
```

Handle errors properly. Always clean up mounts and cgroups, even on failure or
Ctrl+C. Reap zombie children, since the shell inside is PID 1 and PID 1 has to
adopt orphans.

- Learn: `flag` package, `defer`, signal handling, why PID 1 is special

### Milestone 5 — `pivot_root` (stretch)

Replace `chroot` with `pivot_root`. `chroot` can be escaped; `pivot_root` is
what real runtimes use. Learn _why_ by reading about the escape.

### Milestone 6 — Networking (stretch, this is the hard one)

Give the container a working network: veth pair, one end moved into the
container's netns, a bridge on the host, IP addresses, NAT. Either shell out to
`ip` commands first (easier), then redo it with a netlink library.

Budget real time for this. It's a project on its own.

### Milestone 7 — Pull a real image (stretch)

Talk to the Docker Hub API, download layers, untar them in order to make a
rootfs. Now `gbox run` works without me downloading Alpine by hand.

---

## Suggested file layout

```
gbox/
  main.go          # CLI, dispatch: run | child
  run.go           # parent: build the child command, set clone flags
  child.go         # child: hostname, mounts, chroot, exec
  cgroup.go        # create / add pid / limit / cleanup
  mount.go         # mount and unmount helpers
```

## Things I know will go wrong

- Forgetting `sudo` → confusing permission errors
- Forgetting to mount `/proc` → `ps` shows the host's processes and it looks
  like isolation failed
- Forgetting `Unshareflags` → mounts leak to the host
- Trying to remove a cgroup directory that still has processes in it → fails
- Expecting network to work before milestone 6 → it won't, that's correct
- Testing on my real system without a rootfs → I could `chroot` into something
  I didn't mean to. Always pass an explicit rootfs path.

## Done means

- `sudo ./gbox run --memory 50m /bin/sh` gives me a shell
- inside: own hostname, PID 1, Alpine's filesystem, killed if it eats 50MB
- exiting leaves no stray mounts and no leftover cgroup directories
- I can explain each of the five namespaces out loud

## How to use this doc with Claude Code

Work one milestone at a time. Ask for an explanation _before_ the code for each
new syscall — the goal is understanding, not a finished binary. After each
milestone, run the test listed for it before moving on.
