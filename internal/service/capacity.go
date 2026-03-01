package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openhr/internal/models"
)

// CapacityCalculator calculates storage capacity
type CapacityCalculator struct{}

// NewCapacityCalculator creates a new capacity calculator
func NewCapacityCalculator() *CapacityCalculator {
	return &CapacityCalculator{}
}

// EstimateCapacity estimates the usable capacity
func (c *CapacityCalculator) EstimateCapacity(disks []DiskSizeInfo, mode models.StorageMode) *models.PoolCapacity {
	capacity := &models.PoolCapacity{
		DiskCount: len(disks),
	}

	// Collect disk sizes
	var sizes []int64
	for _, d := range disks {
		sizes = append(sizes, d.Size)
	}

	if len(sizes) == 0 {
		return capacity
	}

	// Find max and min
	capacity.MaxDiskSize = sizes[0]
	capacity.MinDiskSize = sizes[0]
	var total int64
	for _, s := range sizes {
		total += s
		if s > capacity.MaxDiskSize {
			capacity.MaxDiskSize = s
		}
		if s < capacity.MinDiskSize {
			capacity.MinDiskSize = s
		}
	}
	capacity.TotalRawCapacity = total

	switch mode {
	case models.ModeSHR1:
		return c.calculateSHR1(sizes, capacity)
	case models.ModeSHR2:
		return c.calculateSHR2(sizes, capacity)
	case models.ModeRAID1:
		return c.calculateRAID1(sizes, capacity)
	case models.ModeRAID5:
		return c.calculateRAID5(sizes, capacity)
	case models.ModeRAID6:
		return c.calculateRAID6(sizes, capacity)
	case models.ModeBasic:
		return c.calculateBasic(sizes, capacity)
	case models.ModeJBOD:
		return c.calculateJBOD(sizes, capacity)
	default:
		return capacity
	}
}

// calculateSHR1 calculates SHR-1 capacity using multi-layer algorithm
//
// SHR-1 (Synology Hybrid RAID 1) provides single disk fault tolerance
// with optimal storage efficiency for mixed-size disks.
//
// Multi-layer Algorithm:
// 1. Sort disks by size (ascending)
// 2. Iteratively create layers:
//   - Find disks with remaining space
//   - Use minimum remaining as layer size
//   - 2 disks → RAID1, 3+ disks → RAID5
//   - Deduct allocated space from each disk
//
// 3. Sum all layer capacities for total usable
//
// Example: 4TB + 8TB + 16TB disks
//
//	Layer 0: 3 disks × 4TB → RAID5 = 8TB
//	Layer 1: 2 disks × 4TB → RAID1 = 4TB
//	Layer 2: 1 disk × 8TB → skipped (insufficient disks)
//	Total: 12TB
//
// Fault tolerance: 1 disk
func (c *CapacityCalculator) calculateSHR1(sizes []int64, cap *models.PoolCapacity) *models.PoolCapacity {
	n := len(sizes)
	if n < 2 {
		cap.UsableCapacity = 0
		cap.Redundancy = 0
		return cap
	}
	// Use the SHR planner for accurate calculation
	usableCapacity := CalculateSHRCapacity(sizes, 1)
	cap.UsableCapacity = usableCapacity
	cap.ParityCapacity = cap.TotalRawCapacity - usableCapacity
	cap.Redundancy = 1
	cap.ProtectionLevel = "Can tolerate 1 disk failure"
	return cap
}

// calculateSHR2 calculates SHR-2 capacity using multi-layer algorithm
//
// SHR-2 (Synology Hybrid RAID 2) provides dual disk fault tolerance
// with optimal storage efficiency for mixed-size disks.
//
// Multi-layer Algorithm (similar to SHR-1 but with RAID6):
// 1. Sort disks by size (ascending)
// 2. Iteratively create layers:
//   - Find disks with remaining space (minimum 4 required)
//   - Use minimum remaining as layer size
//   - Always use RAID6 for dual parity
//   - Deduct allocated space from each disk
//
// 3. Sum all layer capacities for total usable
//
// Example: 4TB + 8TB + 16TB + 16TB disks
//
//	Layer 0: 4 disks × 4TB → RAID6 = 8TB
//	Layer 1: 2 disks × 4TB → RAID1 = 4TB
//	Layer 2: 1 disk × 8TB → skipped (insufficient disks)
//	Total: 12TB
//
// Minimum requirement: 4 disks
// Fault tolerance: 2 disks
func (c *CapacityCalculator) calculateSHR2(sizes []int64, cap *models.PoolCapacity) *models.PoolCapacity {
	n := len(sizes)
	if n < 4 {
		cap.UsableCapacity = 0
		cap.Redundancy = 0
		cap.ProtectionLevel = "Requires at least 4 disks"
		return cap
	}
	// Use the SHR planner for accurate calculation
	usableCapacity := CalculateSHRCapacity(sizes, 2)
	cap.UsableCapacity = usableCapacity
	cap.ParityCapacity = cap.TotalRawCapacity - usableCapacity
	cap.Redundancy = 2
	cap.ProtectionLevel = "Can tolerate 2 disk failures"
	return cap
}

// calculateRAID1 calculates RAID1 capacity
// Formula: Usable = MinDiskSize (mirrored)
// Fault tolerance: n-1 disks
func (c *CapacityCalculator) calculateRAID1(sizes []int64, cap *models.PoolCapacity) *models.PoolCapacity {
	if len(sizes) < 2 {
		cap.UsableCapacity = 0
		cap.Redundancy = 0
		return cap
	}

	cap.ParityCapacity = cap.TotalRawCapacity - cap.MinDiskSize
	cap.UsableCapacity = cap.MinDiskSize
	cap.Redundancy = len(sizes) - 1
	cap.ProtectionLevel = fmt.Sprintf("Can tolerate %d disk failures", len(sizes)-1)

	return cap
}

// calculateRAID5 calculates RAID5 capacity
// Formula: Usable = (n-1) * MinDiskSize
// Fault tolerance: 1 disk
func (c *CapacityCalculator) calculateRAID5(sizes []int64, cap *models.PoolCapacity) *models.PoolCapacity {
	if len(sizes) < 3 {
		cap.UsableCapacity = 0
		cap.Redundancy = 0
		return cap
	}

	cap.ParityCapacity = cap.MinDiskSize
	cap.UsableCapacity = (int64(len(sizes)) - 1) * cap.MinDiskSize
	cap.Redundancy = 1
	cap.ProtectionLevel = "Can tolerate 1 disk failure"

	return cap
}

// calculateRAID6 calculates RAID6 capacity
// Formula: Usable = (n-2) * MinDiskSize
// Fault tolerance: 2 disks
func (c *CapacityCalculator) calculateRAID6(sizes []int64, cap *models.PoolCapacity) *models.PoolCapacity {
	if len(sizes) < 4 {
		cap.UsableCapacity = 0
		cap.Redundancy = 0
		return cap
	}

	cap.ParityCapacity = 2 * cap.MinDiskSize
	cap.UsableCapacity = (int64(len(sizes)) - 2) * cap.MinDiskSize
	cap.Redundancy = 2
	cap.ProtectionLevel = "Can tolerate 2 disk failures"

	return cap
}

// calculateBasic calculates Basic capacity
// Formula: Usable = Sum of all disks
// Fault tolerance: None
func (c *CapacityCalculator) calculateBasic(sizes []int64, cap *models.PoolCapacity) *models.PoolCapacity {
	cap.UsableCapacity = cap.TotalRawCapacity
	cap.ParityCapacity = 0
	cap.Redundancy = 0
	cap.ProtectionLevel = "No fault tolerance"

	return cap
}

// calculateJBOD calculates JBOD capacity
// Formula: Usable = Sum of all disks
// Fault tolerance: None
func (c *CapacityCalculator) calculateJBOD(sizes []int64, cap *models.PoolCapacity) *models.PoolCapacity {
	return c.calculateBasic(sizes, cap)
}

// DiskSizeInfo - Disk size information
type DiskSizeInfo struct {
	Device  string
	Size    int64  // bytes
	SizeStr string // human readable
}

// ParseDiskSizes - Parse disk size strings
func ParseDiskSizes(sizes []string) []DiskSizeInfo {
	var result []DiskSizeInfo
	for _, s := range sizes {
		size := parseSizeString(s)
		result = append(result, DiskSizeInfo{
			Size:    size,
			SizeStr: s,
		})
	}
	return result
}

// ParseDiskSizesWithDevices - Parse disk size strings with devices
func ParseDiskSizesWithDevices(devices []string, sizes []string) []DiskSizeInfo {
	var result []DiskSizeInfo
	for i, dev := range devices {
		sizeStr := ""
		if i < len(sizes) {
			sizeStr = sizes[i]
		}
		size := parseSizeString(sizeStr)
		result = append(result, DiskSizeInfo{
			Device:  dev,
			Size:    size,
			SizeStr: sizeStr,
		})
	}
	return result
}

// parseSizeString - Parse size string
func parseSizeString(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Match number + unit
	re := regexp.MustCompile(`^([\d.]+)\s*(B|KB|MB|GB|TB|PB|TiB|GiB|MiBi)?$`)
	matches := re.FindStringSubmatch(strings.ToUpper(s))
	if matches == nil {
		return 0
	}

	value, _ := strconv.ParseFloat(matches[1], 64)
	unit := strings.ToUpper(matches[2])

	multipliers := map[string]float64{
		"PB":  1024 * 1024 * 1024 * 1024 * 1024,
		"TB":  1024 * 1024 * 1024 * 1024,
		"GB":  1024 * 1024 * 1024,
		"MB":  1024 * 1024,
		"KB":  1024,
		"B":   1,
		"PIB": 1024 * 1024 * 1024 * 1024 * 1024,
		"TIB": 1024 * 1024 * 1024 * 1024,
		"GIB": 1024 * 1024 * 1024,
		"MIB": 1024 * 1024,
		"KIB": 1024,
	}

	if mult, ok := multipliers[unit]; ok {
		value *= mult
	}

	return int64(value)
}

// FormatBytes - Format bytes to human readable string
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ParseSizeString - Parse size string (public API)
func ParseSizeString(s string) int64 {
	return parseSizeString(s)
}
