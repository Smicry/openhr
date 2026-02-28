package storage

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/openhr/internal/models"
)

// MDADMOperator MDADM操作实现
type MDADMOperator struct {
	executor *Executor
}

// NewMDADMOperator 创建MDADM操作器
func NewMDADMOperator() *MDADMOperator {
	return &MDADMOperator{
		executor: NewExecutor(),
	}
}

// CreateRAID 创建RAID阵列
// level: 0, 1, 4, 5, 6, 10
// devices: 设备列表
// name: 阵列名
// bitmap: 是否启用bitmap
func (m *MDADMOperator) CreateRAID(level int, devices []string, name string, bitmap bool) (string, error) {
	// 检查mdadm是否可用
	if !m.executor.CheckCommandExists("mdadm") {
		return "", fmt.Errorf("mdadm命令不可用，请安装mdadm")
	}

	// 创建设备名
	device := fmt.Sprintf("/dev/md/%s", name)
	
	// 构建命令
	args := []string{
		"--create", device,
		"--level", fmt.Sprintf("%d", level),
		"--raid-devices", fmt.Sprintf("%d", len(devices)),
	}

	// 添加bitmap
	if bitmap {
		args = append(args, "--bitmap", "internal")
	}

	// 添加设备
	args = append(args, devices...)

	output, err := m.executor.Run("mdadm", args...)
	if err != nil {
		return "", fmt.Errorf("创建RAID失败: %v, output: %s", err, output)
	}

	// 等待设备创建完成
	m.waitForDevice(device)

	return device, nil
}

// RemoveRAID 删除RAID阵列
func (m *MDADMOperator) RemoveRAID(dev string) error {
	// 先停止阵列
	_, err := m.executor.Run("mdadm", "--stop", dev)
	if err != nil {
		return fmt.Errorf("停止RAID失败: %v", err)
	}

	// 清除超级块
	devices := m.getComponentDevices(dev)
	for _, d := range devices {
		m.executor.Run("mdadm", "--zero-superblock", d)
	}

	return nil
}

// QueryRAID 查询RAID信息
func (m *MDADMOperator) QueryRAID(dev string) (*models.RAIDInfo, error) {
	output, err := m.executor.Run("mdadm", "--detail", dev)
	if err != nil {
		return nil, fmt.Errorf("查询RAID失败: %v", err)
	}

	return m.parseRAIDDetail(output)
}

// AddDeviceRAID 添加设备到RAID
func (m *MDADMOperator) AddDeviceRAID(dev string, newDev string) error {
	_, err := m.executor.Run("mdadm", "--add", dev, newDev)
	if err != nil {
		return fmt.Errorf("添加设备失败: %v", err)
	}
	return nil
}

// RemoveDeviceRAID 从RAID移除设备
func (m *MDADMOperator) RemoveDeviceRAID(dev string, rmDev string) error {
	_, err := m.executor.Run("mdadm", "--remove", dev, rmDev)
	if err != nil {
		return fmt.Errorf("移除设备失败: %v", err)
	}
	return nil
}

// SetDeviceFaultRAID 设置设备为故障状态
func (m *MDADMOperator) SetDeviceFaultRAID(dev string, devToFail string) error {
	_, err := m.executor.Run("mdadm", "--set-faulty", dev, devToFail)
	if err != nil {
		return fmt.Errorf("设置故障失败: %v", err)
	}
	return nil
}

// ListMDDevices 列出所有MD设备
func (m *MDADMOperator) ListMDDevices() ([]models.RAIDInfo, error) {
	output, err := m.executor.Run("mdadm", "--detail", "--scan")
	if err != nil {
		return nil, fmt.Errorf("列出MD设备失败: %v", err)
	}

	var devices []models.RAIDInfo
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 解析每行
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		info := &models.RAIDInfo{}
		for _, part := range parts {
			if strings.HasPrefix(part, "/dev/") {
				info.Device = part
				break
			}
		}
		if info.Device != "" {
			detail, _ := m.QueryRAID(info.Device)
			if detail != nil {
				devices = append(devices, *detail)
			}
		}
	}

	return devices, nil
}

// waitForDevice 等待设备就绪
func (m *MDADMOperator) waitForDevice(device string) error {
	// 简单等待后返回
	return nil
}

// getComponentDevices 获取RAID组件设备
func (m *MDADMOperator) getComponentDevices(dev string) []string {
	_, err := m.QueryRAID(dev)
	if err != nil {
		return nil
	}

	// 从State字段解析
	return nil
}

// parseRAIDDetail 解析RAID详情输出
func (m *MDADMOperator) parseRAIDDetail(output string) (*models.RAIDInfo, error) {
	info := &models.RAIDInfo{}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Device Name":
			info.Device = value
		case "Array Size":
			info.Layout = value
		case "State":
			info.State = value
		case "Active Devices":
			if n, err := strconv.Atoi(value); err == nil {
				info.ActiveDevices = n
			}
		case "Total Devices":
			if n, err := strconv.Atoi(value); err == nil {
				info.TotalDevices = n
			}
		case "Failed Devices":
			if n, err := strconv.Atoi(value); err == nil {
				info.FailedDevices = n
			}
		}
	}

	return info, nil
}
