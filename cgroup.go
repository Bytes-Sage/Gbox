package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupRoot = "/sys/fs/cgroup"
const gboxGroup = "gbox"

func checkControllers() error {
	data, err := os.ReadFile(filepath.Join(cgroupRoot, "cgroup.subtree_control"))
	if err != nil {
		return fmt.Errorf("reading subtree_control: %w", err)
	}
	enabled := strings.Fields(string(data))
	need := map[string]bool{"memory": false, "pids": false}
	for _, c := range enabled {
		if _, ok := need[c]; ok {
			need[c] = true
		}
	}
	for c, ok := range need {
		if !ok {
			return fmt.Errorf(
				"controller %q not enabled in %s/cgroup.subtree_control - run:\n"+
					"echo '+%s' | sudo tee %s/cgroup.subtree_control",
				c, cgroupRoot, c, cgroupRoot)
		}
	}
	return nil
}

func setupCgroup(name string, memoryLimit int64, pidsLimit int) (string, error) {
	if err := checkControllers(); err != nil {
		return "", err
	}

	// A controller must be enabled in a cgroup's OWN subtree_control before
	// any of ITS children can use it — enabling it at cgroupRoot only lets
	// gboxRoot use the controller, not gboxRoot's children. So gboxRoot
	// needs it enabled here, one level down from cgroupRoot.
	gboxRoot := filepath.Join(cgroupRoot, gboxGroup)
	if err := os.MkdirAll(gboxRoot, 0755); err != nil {
		return "", fmt.Errorf("creating gbox cgroup dir: %w", err)
	}
	if err := writeCgroupFile(gboxRoot, "cgroup.subtree_control", "+memory +pids"); err != nil {
		return "", fmt.Errorf("enabling controllers on %s: %w", gboxRoot, err)
	}

	groupPath := filepath.Join(gboxRoot, name)
	if err := os.MkdirAll(groupPath, 0755); err != nil {
		return "", fmt.Errorf("creating cgroup dir: %w", err)
	}

	if memoryLimit > 0 {
		if err := writeCgroupFile(groupPath, "memory.max", strconv.FormatInt(memoryLimit, 10)); err != nil {
			return "", err
		}
	}

	if pidsLimit > 0 {
		if err := writeCgroupFile(groupPath, "pids.max", strconv.Itoa(pidsLimit)); err != nil {
			return "", err
		}
	}

	return groupPath, nil
}

func addPidToCgroup(groupPath string, pid int) error {
	return writeCgroupFile(groupPath, "cgroup.procs", strconv.Itoa(pid))
}
func writeCgroupFile(groupPath, file, value string) error {
	path := filepath.Join(groupPath, file)
	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
func cleanCgroup(groupPath string) error {
	return os.Remove(groupPath)
}

// memory string parser
func parseMemory(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}

	suffix := s[len(s)-1]
	mult := int64(1)
	numPart := s
	switch suffix {
	case 'k', 'K':
		mult = 1024
		numPart = s[:len(s)-1]
	case 'm', 'M':
		mult = 1024 * 1024
		numPart = s[:len(s)-1]
	case 'g', 'G':
		mult = 1024 * 1024 * 1024
		numPart = s[:len(s)-1]
	}

	n, err := strconv.ParseInt(numPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory limit %q: %w", s, err)
	}
	return n * mult, nil
}
