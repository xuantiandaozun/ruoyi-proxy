//go:build linux

package nodeinfo

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func collectPlatformResources() Resources {
	resources := readLinuxMemory()
	var stat syscall.Statfs_t
	if err := syscall.Statfs(".", &stat); err == nil {
		resources.DiskTotalBytes = stat.Blocks * uint64(stat.Bsize)
		resources.DiskFreeBytes = stat.Bavail * uint64(stat.Bsize)
	}
	return resources
}

func readLinuxMemory() Resources {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return Resources{}
	}
	defer file.Close()
	values := map[string]uint64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			values[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	return Resources{
		MemoryTotalBytes:     values["MemTotal"],
		MemoryAvailableBytes: values["MemAvailable"],
	}
}
