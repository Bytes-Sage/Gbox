# gbox

A minimal container runtime for Linux, written from scratch in Go — no
`runc`, no `libcontainer`, no external container library. Every syscall
(`clone`, `chroot`, `mount`, cgroup file writes) is called by hand, on
purpose, to actually understand what a "container" is instead of trusting
a black box.

```bash
sudo ./gbox run /bin/sh
```

gives you a shell that has its own hostname, believes it's process #1,
sees a different root filesystem, and gets killed automatically if it uses
more memory than allowed — all built from four Linux primitives: namespaces,
`chroot`, cgroups v2, and a parent/child re-exec trick.

This is a learning project, not a production tool. See [Non-goals](#non-goals)
below for what it deliberately does not do.

---

## Requirements

- Linux only (developed/tested on Arch Linux, cgroups v2 / unified
  `/sys/fs/cgroup` hierarchy)
- Go (standard library + `golang.org/x/sys/unix`)
- Root (`sudo`) — namespace creation and cgroup setup require it
- An unpacked Alpine minirootfs at `./rootfs` (see [Setup](#setup))

## Setup

```bash
go mod tidy

# one-time: download and unpack a rootfs to chroot into
mkdir -p rootfs
curl -L -o alpine-minirootfs.tar.gz \
  https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-minirootfs-3.20.3-x86_64.tar.gz
tar -xzf alpine-minirootfs.tar.gz -C rootfs

go build -o gbox .
```

## Usage

```
sudo ./gbox run [--memory 100m] [--pids 20] [--rootfs ./rootfs] [--hostname foo] CMD [ARGS...]
```

| Flag         | Default    | Meaning                                    |
|--------------|------------|---------------------------------------------|
| `--memory`   | `50m`      | Memory limit (`k`/`m`/`g` suffix, or bytes) |
| `--pids`     | `20`       | Max number of processes inside the container |
| `--rootfs`   | `./rootfs` | Path to the root filesystem to `chroot` into |
| `--hostname` | `container`| Hostname visible inside the container       |

Examples:

```bash
sudo ./gbox run /bin/sh
sudo ./gbox run --memory 100m --pids 10 --hostname sandbox /bin/sh
sudo ./gbox run /bin/echo hello from inside
```

`gbox child ...` also exists but isn't meant to be run by hand — it's the
hidden re-exec target `run` uses internally (see below).

## Safety

Always pass an explicit, dedicated `--rootfs`. `chroot` only changes what
the process's `/` looks like — it does not sandbox arbitrary file access on
its own. Don't point it at a real system directory you care about.

---

## How it works

Linux won't let a running Go program just call `unshare()` mid-flight and
have it apply cleanly — the runtime already has multiple OS threads
running, and namespace flags apply per-thread at creation time via
`clone()`. So `gbox` runs itself twice:

1. **`gbox run CMD`** (the *parent*) — runs with your normal permissions.
   Builds a command that re-executes `/proc/self/exe` as `gbox child CMD`,
   attaching `clone` flags that create five new namespaces for it. Once
   that child process exists, the parent creates a cgroup, applies the
   memory/pids limits, and adds the child's PID to it.
2. **`gbox child CMD`** — born already inside the new namespaces. Sets the
   hostname, `chroot`s into the rootfs, mounts a fresh `/proc`, then
   **replaces its own process image** (`execve`) with `CMD` — which is what
   makes `CMD` become PID 1 inside the container, not just a child of some
   wrapper process.

A sync pipe coordinates the two: the child blocks right before the final
`execve` until the parent confirms the cgroup is set up, closing a race
where the real command could otherwise start running before its resource
limits are attached.

### The five namespaces

| Flag              | Isolates                                  |
|-------------------|---------------------------------------------|
| `CLONE_NEWUTS`     | hostname / domain name                    |
| `CLONE_NEWPID`     | process ID numbering                      |
| `CLONE_NEWNS`      | mount table                               |
| `CLONE_NEWIPC`     | System V IPC / message queues             |
| `CLONE_NEWNET`     | network stack (empty — no networking until milestone 6) |

`Unshareflags: CLONE_NEWNS` plus a recursive `MS_PRIVATE` remount of `/`
stop mounts made inside the container (like the fresh `/proc`) from leaking
onto the host, and vice versa.

### cgroups v2

cgroups are just a filesystem at `/sys/fs/cgroup` — no extra syscalls
needed. `gbox` creates `/sys/fs/cgroup/gbox/gbox-<pid>/`, writes the limits
into `memory.max` and `pids.max`, and writes the container's PID into
`cgroup.procs`. A controller (`memory`, `pids`) has to be enabled in the
**immediate parent** cgroup's `cgroup.subtree_control` before a leaf cgroup
can use it — `gbox` enables this on `/sys/fs/cgroup/gbox/` automatically.

Namespaces control what a process can **see**; cgroups control what it can
**use**. They're unrelated kernel mechanisms that container runtimes
combine.

---

## File layout

```
gbox/
  main.go     CLI entry point: parses flags, dispatches to run | child
  run.go      parent: builds the re-exec'd child, sets clone flags,
              creates the cgroup, forwards Ctrl+C/SIGTERM, cleans up
  child.go    child: sethostname, chroot, mount /proc, execve into CMD
  cgroup.go   cgroup create / limit / add-pid / cleanup
  mount.go    mount helpers (private remount, /proc)
  rootfs/     unpacked Alpine minirootfs (gitignored, downloaded locally)
```

## Status

| Milestone | What | Status |
|---|---|---|
| 0 | Run a command via `os/exec` | ✅ done |
| 1 | Namespaces (UTS/PID/NS/IPC/NET) | ✅ done |
| 2 | Root filesystem (`chroot` + `/proc`) | ✅ done |
| 3 | Resource limits (cgroups v2) | ✅ done |
| 4 | Real CLI, cleanup, signal handling, PID 1 | ✅ done |
| 5 | `pivot_root` (stretch) | not started |
| 6 | Networking — veth/bridge/NAT (stretch) | not started |
| 7 | Pull a real image from a registry (stretch) | not started |

## Non-goals

Deliberately out of scope — see `scope.md` for the full reasoning:

- No image format, layers, or overlayfs
- No Dockerfile / build system
- No daemon, no API, no client-server split
- No pulling images from a registry (unless milestone 7 happens)
- Linux only — no Windows/macOS support
- Not a Docker replacement — see project docs for how it compares

## Docs in this repo

- `scope.md` — the original project spec and milestone plan
- `GUIDE.md` — full code walkthrough per milestone, with explanations
- `EXPLAIN.md` — a plain-language explanation of the whole project
