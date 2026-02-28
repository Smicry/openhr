package storage

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/openhr/internal/models"
)

// LVMOperator LVM操作实现
type LVMOperator struct {
	executor *Executor
}

// NewLVMOperator 创建LVM操作器
func NewLVMOperator() *LVMOperator {
	return &LVMOperator{
		executor: NewExecutor(),
	}
}

// CreatePV 创建物理卷
func (l *LVMOperator) CreatePV(dev string) error {
	if !l.executor.CheckCommandExists("lvm") {
		return fmt.Errorf("lvm命令不可用，请安装lvm2")
	}

	_, err := l.executor.Run("lvm", "pvcreate", "-y", dev)
	if err != nil {
		return fmt.Errorf("创建PV失败: %v", err)
	}
	return nil
}

// RemovePV 移除物理卷
func (l *LVMOperator) RemovePV(dev string) error {
	_, err := l.executor.Run("lvm", "pvremove", "-y", "-ff", dev)
	if err != nil {
		return fmt.Errorf("移除PV失败: %v", err)
	}
	return nil
}

// CreateVG 创建卷组
func (l *LVMOperator) CreateVG(name string, pvs []string) error {
	args := []string{"vgcreate", name}
	args = append(args, pvs...)

	_, err := l.executor.Run("lvm", args...)
	if err != nil {
		return fmt.Errorf("创建VG失败: %v", err)
	}
	return nil
}

// RemoveVG 移除卷组
func (l *LVMOperator) RemoveVG(name string) error {
	_, err := l.executor.Run("lvm", "vgremove", "-y", "-ff", name)
	if err != nil {
		return fmt.Errorf("移除VG失败: %v", err)
	}
	return nil
}

// ExtendVG 扩展卷组
func (l *LVMOperator) ExtendVG(name string, pvs []string) error {
	args := []string{"vgextend", name}
	args = append(args, pvs...)

	_, err := l.executor.Run("lvm", args...)
	if err != nil {
		return fmt.Errorf("扩展VG失败: %v", err)
	}
	return nil
}

// ReduceVG 缩小卷组
func (l *LVMOperator) ReduceVG(name string, pvs []string) error {
	args := []string{"vgreduce", name}
	args = append(args, pvs...)

	_, err := l.executor.Run("lvm", args...)
	if err != nil {
		return fmt.Errorf("缩小VG失败: %v", err)
	}
	return nil
}

// CreateLV 创建逻辑卷
func (l *LVMOperator) CreateLV(vg, name string, size int64, fs string) error {
	sizeStr := formatSize(size)
	
	_, err := l.executor.Run("lvm", "lvcreate", "-L", sizeStr, "-n", name, vg)
	if err != nil {
		return fmt.Errorf("创建LV失败: %v", err)
	}

	// 格式化
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)
	if fs != "" {
		_, err = l.executor.Run("mkfs."+fs, "-F", dev)
		if err != nil {
			return fmt.Errorf("格式化失败: %v", err)
		}
	}

	return nil
}

// RemoveLV 移除逻辑卷
func (l *LVMOperator) RemoveLV(vg, name string) error {
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)
	_, err := l.executor.Run("lvm", "lvremove", "-y", "-ff", dev)
	if err != nil {
		return fmt.Errorf("移除LV失败: %v", err)
	}
	return nil
}

// ExtendLV 扩展逻辑卷
func (l *LVMOperator) ExtendLV(vg, name string, size int64) error {
	sizeStr := formatSize(size)
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)

	_, err := l.executor.Run("lvm", "lvextend", "-L", "+"+sizeStr, dev)
	if err != nil {
		return fmt.Errorf("扩展LV失败: %v", err)
	}

	return nil
}

// ReduceLV 缩小逻辑卷
func (l *LVMOperator) ReduceLV(vg, name string, size int64) error {
	sizeStr := formatSize(size)
	dev := fmt.Sprintf("/dev/%s/%s", vg, name)

	_, err := l.executor.Run("lvm", "lvreduce", "-L", "-"+sizeStr, dev)
	if err != nil {
		return fmt.Errorf("缩小LV失败: %v", err)
	}

	return nil
}

// ListVGs 列出卷组
func (l *LVMOperator) ListVGs() ([]models.VGInfo, error) {
	output, err := l.executor.Run("lvm", "vgs", "--units", "b", "--noheadings", "-o", "vg_name,vg_size,vg_free,pv_count,lv_count,vg_uuid")
	if err != nil {
		return nil, fmt.Errorf("列出VG失败: %v", err)
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

// ListPVs 列出物理卷
func (l *LVMOperator) ListPVs() ([]models.PVInfo, error) {
	output, err := l.executor.Run("lvm", "pvs", "--units", "b", "--noheadings", "-o", "pv_name,vg_name,pv_size,pv_free,dev_size,pv_uuid")
	if err != nil {
		return nil, fmt.Errorf("列出PV失败: %v", err)
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

// ListLVs 列出逻辑卷
func (l *LVMOperator) ListLVs() ([]models.LVInfo, error) {
	output, err := l.executor.Run("lvm", "lvs", "--units", "b", "--noheadings", "-o", "lv_name,vg_name,lv_size,lv_pool,layout,role,device")
	if err != nil {
		return nil, fmt.Errorf("列出LV失败: %v", err)
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

// GetVGInfo 获取卷组信息
func (l *LVMOperator) GetVGInfo(name string) (*models.VGInfo, error) {
	output, err := l.executor.Run("lvm", "vgs", "--units", "b", "--noheadings", "-o", "vg_name,vg_size,vg_free,pv_count,lv_count", name)
	if err != nil {
		return nil, fmt.Errorf("获取VG信息失败: %v", err)
	}

	fields := strings.Fields(output)
	if len(fields) < 5 {
		return nil, fmt.Errorf("VG不存在: %s", name)
	}

	return &models.VGInfo{
		Name:    fields[0],
		Size:    parseSize(fields[1]),
		Free:    parseSize(fields[2]),
		PVCount: atoi(fields[3]),
		LVCount: atoi(fields[4]),
	}, nil
}

// formatSize 格式化大小为字节字符串
func formatSize(bytes int64) string {
	return fmt.Sprintf("%db", bytes)
}

// parseSize 解析大小字符串 (如 "10.00g")
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	
	re := regexp.MustCompile(`^([\d.]+)([smgtbpk]?)$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0
	}

	value, _ := strconv.ParseFloat(matches[1], 64)
	unit := matches[2]

	// 转换为字节
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

// atoi 简单字符串转整数
func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
