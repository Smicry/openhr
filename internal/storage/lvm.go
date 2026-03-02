package storage

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openhr/internal/models"
)

// LVMOperator - LVM operator implementation
type LVMOperator struct {
	executor *Executor
}

// NewLVMOperator - Creates LVM operator
func NewLVMOperator() *LVMOperator {
	return &LVMOperator{
		executor: NewExecutor(),
	}
}

// CreatePV - Create physical volume
func (l *LVMOperator) CreatePV(dev string) error {
	if !l.executor.CheckCommandExists("lvm") {
		return fmt.Errorf("lvm command not available, please install lvm2")
	}
	_, err := l.executor.Run("lvm", "pvcreate", "-y", dev)
	if err != nil {
		return fmt.Errorf("failed to create PV: %v", err)
	}
	return nil
}

// RemovePV - Remove physical volume
func (l *LVMOperator) RemovePV(dev string) error {
	_, err := l.executor.Run("lvm", "pvremove", "-y", "-ff", dev)
	if err != nil {
		return fmt.Errorf("failed to remove PV: %v", err)
	}
	return nil
}

// CreateVG - Create volume group
func (l *LVMOperator) CreateVG(name string, pvs []string) error {
	args := []string{"vgcreate", name}
	args = append(args, pvs...)
	_, err := l.executor.Run("lvm", args...)
	if err != nil {
		return fmt.Errorf("failed to create VG: %v", err)
	}
	return nil
}

// RemoveVG - Remove volume group
func (l *LVMOperator) RemoveVG(name string) error {
	_, err := l.executor.Run("lvm", "vgremove", "-y", "-ff", name)
	if err != nil {
		return fmt.Errorf("failed to remove VG: %v", err)
	}
	return nil
}

// ExtendVG - Extend volume group
func (l *LVMOperator) ExtendVG(name string, pvs []string) error {
	args := []string{"vgextend", name}
	args = append(args, pvs...)

	_, err := l.executor.Run("lvm", args...)
	if err != nil {
		return fmt.Errorf("failed to extend VG: %v", err)
	}
	return nil
}

// ReduceVG - Reduce volume group
func (l *LVMOperator) ReduceVG(name string, pvs []string) error {
	args := []string{"vgreduce", name}
	args = append(args, pvs...)

	_, err := l.executor.Run("lvm", args...)
	if err != nil {
		return fmt.Errorf("failed to reduce VG: %v", err)
	}
	return nil
}

// CreateLV - Create logical volume
func (l *LVMOperator) CreateLV(vg, name string, size int64, fs string) error {
	sizeStr := formatSize(size)
	_, err := l.executor.Run("lvm", "lvcreate", "-L", sizeStr, "-n", name, vg)
	if err != nil {
		return fmt.Errorf("failed to create LV: %v", err)
	}
	// Format
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)
	if fs != "" {
		_, err = l.executor.Run("mkfs."+fs, "-F", dev)
		if err != nil {
			return fmt.Errorf("failed to format: %v", err)
		}
	}
	return nil
}

// RemoveLV - Remove logical volume
func (l *LVMOperator) RemoveLV(vg, name string) error {
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)
	_, err := l.executor.Run("lvm", "lvremove", "-y", "-ff", dev)
	if err != nil {
		return fmt.Errorf("failed to remove LV: %v", err)
	}
	return nil
}

// ExtendLV - Extend logical volume
func (l *LVMOperator) ExtendLV(vg, name string, size int64) error {
	sizeStr := formatSize(size)
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)
	_, err := l.executor.Run("lvm", "lvextend", "-L", "+"+sizeStr, dev)
	if err != nil {
		return fmt.Errorf("failed to extend LV: %v", err)
	}
	return nil
}

// ExtendLVFilesystem - Extend filesystem on logical volume
func (l *LVMOperator) ExtendLVFilesystem(vg, name string, fs string) error {
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)
	switch strings.ToLower(fs) {
	case "ext4", "ext3", "ext2":
		_, err := l.executor.Run("resize2fs", dev)
		if err != nil {
			return fmt.Errorf("failed to resize ext filesystem: %v", err)
		}
	case "xfs":
		_, err := l.executor.Run("xfs_growfs", dev)
		if err != nil {
			return fmt.Errorf("failed to resize xfs filesystem: %v", err)
		}
	case "btrfs":
		_, err := l.executor.Run("btrfs", "filesystem", "resize", "max", dev)
		if err != nil {
			return fmt.Errorf("failed to resize btrfs filesystem: %v", err)
		}
	default:
		// Try resize2fs as default (works for ext2/3/4)
		_, err := l.executor.Run("resize2fs", dev)
		if err != nil {
			return fmt.Errorf("failed to resize filesystem: %v", err)
		}
	}
	return nil
}

// ReduceLV - Reduce logical volume
func (l *LVMOperator) ReduceLV(vg, name string, size int64) error {
	sizeStr := formatSize(size)
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)
	_, err := l.executor.Run("lvm", "lvreduce", "-L", "-"+sizeStr, dev)
	if err != nil {
		return fmt.Errorf("failed to reduce LV: %v", err)
	}
	return nil
}

// ListVGs - List volume groups
func (l *LVMOperator) ListVGs() ([]models.VGInfo, error) {
	output, err := l.executor.Run("lvm", "vgs", "--units", "b", "--noheadings", "-o", "vg_name,vg_size,vg_free,pv_count,lv_count,vg_uuid")
	if err != nil {
		return nil, fmt.Errorf("failed to list VGs: %v", err)
	}

	var vgs []models.VGInfo
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		vg := models.VGInfo{
			Name:    fields[0],
			Size:    parseSize(fields[1]),
			Free:    parseSize(fields[2]),
			PVCount: atoi(fields[3]),
			LVCount: atoi(fields[4]),
		}
		vgs = append(vgs, vg)
	}

	return vgs, nil
}

// ListPVs - List physical volumes
func (l *LVMOperator) ListPVs() ([]models.PVInfo, error) {
	output, err := l.executor.Run("lvm", "pvs", "--units", "b", "--noheadings", "-o", "pv_name,vg_name,pv_size,pv_free,dev_size,pv_uuid")
	if err != nil {
		return nil, fmt.Errorf("failed to list PVs: %v", err)
	}

	var pvs []models.PVInfo
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		pv := models.PVInfo{
			Name:    fields[0],
			VGName:  fields[1],
			Size:    parseSize(fields[2]),
			Free:    parseSize(fields[3]),
			DevSize: parseSize(fields[4]),
		}
		pvs = append(pvs, pv)
	}

	return pvs, nil
}

// ListLVs - List logical volumes
func (l *LVMOperator) ListLVs() ([]models.LVInfo, error) {
	output, err := l.executor.Run("lvm", "lvs", "--units", "b", "--noheadings", "-o", "lv_name,vg_name,lv_size,lv_pool,layout,role,device")
	if err != nil {
		return nil, fmt.Errorf("failed to list LVs: %v", err)
	}

	var lvs []models.LVInfo
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}

		lv := models.LVInfo{
			Name:   fields[0],
			VGName: fields[1],
			Size:   parseSize(fields[2]),
			Pool:   fields[3],
			Layout: fields[4],
			Role:   fields[5],
			Device: fields[6],
		}
		lvs = append(lvs, lv)
	}

	return lvs, nil
}

// GetVGInfo - Get volume group info
func (l *LVMOperator) GetVGInfo(name string) (*models.VGInfo, error) {
	output, err := l.executor.Run("lvm", "vgs", "--units", "b", "--noheadings", "-o", "vg_name,vg_size,vg_free,pv_count,lv_count", name)
	if err != nil {
		return nil, fmt.Errorf("failed to get VG info: %v", err)
	}

	fields := strings.Fields(output)
	if len(fields) < 5 {
		return nil, fmt.Errorf("VG does not exist: %s", name)
	}

	return &models.VGInfo{
		Name:    fields[0],
		Size:    parseSize(fields[1]),
		Free:    parseSize(fields[2]),
		PVCount: atoi(fields[3]),
		LVCount: atoi(fields[4]),
	}, nil
}

// formatSize - Format size to bytes string
func formatSize(bytes int64) string {
	return fmt.Sprintf("%db", bytes)
}

// parseSize - Parse size string (e.g., "10.00g")
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`^([\d.]+)([smgtbpk]?)$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0
	}
	value, _ := strconv.ParseFloat(matches[1], 64)
	unit := matches[2]
	// Convert to bytes
	multipliers := map[string]float64{
		"p": 1024 * 1024 * 1024 * 1024 * 1024,
		"t": 1024 * 1024 * 1024 * 1024,
		"g": 1024 * 1024 * 1024,
		"m": 1024 * 1024,
		"k": 1024,
		"b": 1,
	}
	if mult, ok := multipliers[unit]; ok {
		value *= mult
	}
	return int64(value)
}

// atoi - Simple string to int
func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
