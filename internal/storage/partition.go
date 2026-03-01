package storage

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/openhr/internal/models"
)

// PartitionOperator - Partition operator implementation
type PartitionOperator struct {
	executor *Executor
}

// NewPartitionOperator - Creates partition operator
func NewPartitionOperator() *PartitionOperator {
	return &PartitionOperator{
		executor: NewExecutor(),
	}
}

// CreatePartition - Create partition
// dev: device name, e.g., /dev/sda
// partType: partition type, e.g., "primary", "extended", "logical"
// start, end: position, e.g., "0%", "50%", "10GiB"
func (p *PartitionOperator) CreatePartition(dev string, partType string, start, end string) error {
	if !p.executor.CheckCommandExists("parted") {
		return fmt.Errorf("parted command not available, please install parted")
	}
	// Use parted to create partition
	script := fmt.Sprintf("mkpart %s %s %s", partType, start, end)
	_, err := p.executor.Run("parted", "-s", dev, "--", script)
	if err != nil {
		return fmt.Errorf("failed to create partition: %v", err)
	}
	return nil
}

// DeletePartition - Delete partition
func (p *PartitionOperator) DeletePartition(dev string, num int) error {
	_, err := p.executor.Run("parted", "-s", dev, "--", "rm", strconv.Itoa(num))
	if err != nil {
		return fmt.Errorf("failed to delete partition: %v", err)
	}
	return nil
}

// ListPartitions - List partitions
func (p *PartitionOperator) ListPartitions(dev string) ([]models.Partition, error) {
	output, err := p.executor.Run("parted", "-m", dev, "unit", "B", "print")
	if err != nil {
		return nil, fmt.Errorf("failed to list partitions: %v", err)
	}
	return p.parsePartitions(output, dev)
}

// SetPartitionType - Set partition type
func (p *PartitionOperator) SetPartitionType(dev string, num int, partType string) error {
	_, err := p.executor.Run("parted", "-s", dev, "--", "set", strconv.Itoa(num), "raid", "on")
	if err != nil {
		return fmt.Errorf("failed to set partition type: %v", err)
	}
	return nil
}

// IsPartitionExists - Check if partition exists
func (p *PartitionOperator) IsPartitionExists(dev string, num int) bool {
	parts, err := p.ListPartitions(dev)
	if err != nil {
		return false
	}
	for _, part := range parts {
		if part.Number == num {
			return true
		}
	}
	return false
}

// GetPartitionDevice - Get partition device name
func GetPartitionDevice(dev string, num int) string {
	return fmt.Sprintf("%s%d", dev, num)
}

// parsePartitions - Parse parted output
func (p *PartitionOperator) parsePartitions(output string, dev string) ([]models.Partition, error) {
	var partitions []models.Partition
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip header info
		if !strings.HasPrefix(line, strconv.Itoa(len(partitions)+1)+":") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 6 {
			continue
		}
		// Parse fields
		num, _ := strconv.Atoi(fields[0])
		startStr := strings.TrimSuffix(fields[1], "B")
		endStr := strings.TrimSuffix(fields[2], "B")
		sizeStr := strings.TrimSuffix(fields[3], "B")
		fsType := fields[4]
		name := fields[5]
		_, _ = strconv.ParseInt(startStr, 10, 64)
		_, _ = strconv.ParseInt(endStr, 10, 64)
		size, _ := strconv.ParseInt(sizeStr, 10, 64)
		part := models.Partition{
			Device:     GetPartitionDevice(dev, num),
			Number:     num,
			Start:      fields[1],
			End:        fields[2],
			Size:       size,
			Type:       name,
			Filesystem: fsType,
		}
		partitions = append(partitions, part)
	}
	return partitions, nil
}

// GetDiskInfo - Get disk info
func (p *PartitionOperator) GetDiskInfo(dev string) (*models.Disk, error) {
	disk := &models.Disk{
		Device: dev,
	}
	// Get device size
	output, err := p.executor.Run("blockdev", "--getsize64", dev)
	if err == nil {
		size, _ := strconv.ParseInt(strings.TrimSpace(output), 10, 64)
		disk.Size = size
		disk.SizeHuman = formatBytes(size)
	}
	// Get device info
	output, err = p.executor.Run("lsblk", "-d", "-o", "NAME,MODEL,VENDOR,RO,TRANTYPE", "-n", dev)
	if err == nil {
		fields := strings.Fields(output)
		if len(fields) >= 4 {
			disk.Model = fields[1]
			disk.Vendor = fields[2]
			disk.Ro = fields[3] == "1"
		}
	}
	// Get partition info
	parts, err := p.ListPartitions(dev)
	if err == nil {
		disk.Partitions = parts
	}
	return disk, nil
}

// ListDisks - List all disks
func (p *PartitionOperator) ListDisks() ([]models.Disk, error) {
	output, err := p.executor.Run("lsblk", "-d", "-o", "NAME,SIZE,MODEL,VENDOR,RO,TRAN", "-n", "-e", "2,11")
	if err != nil {
		return nil, fmt.Errorf("failed to list disks: %v", err)
	}
	var disks []models.Disk
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		disk := models.Disk{
			Device: "/dev/" + fields[0],
		}
		// Parse size
		disk.Size = parseSize(fields[1])
		disk.SizeHuman = fields[1]
		// Only add non-system, non-external storage disks
		if len(fields) >= 6 && (fields[5] == "sata" || fields[5] == "nvme" || fields[5] == "usb") {
			disk.Partitions, _ = p.ListPartitions(disk.Device)
			disks = append(disks, disk)
		}
	}

	return disks, nil
}

// IsDiskExists - Check if disk exists
func (p *PartitionOperator) IsDiskExists(dev string) bool {
	_, err := p.executor.Run("lsblk", "-d", "-n", dev)
	return err == nil
}

// IsMounted - Check if mounted
func (p *PartitionOperator) IsMounted(dev string) bool {
	output, err := p.executor.Run("mount")
	if err != nil {
		return false
	}
	return strings.Contains(output, dev)
}

// GetMountPoint - Get mount point
func (p *PartitionOperator) GetMountPoint(dev string) string {
	output, err := p.executor.Run("findmnt", "-n", "-o", "TARGET", "-S", dev)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

// formatBytes - Format bytes
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
