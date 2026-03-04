package storage

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/openhr/internal/models"
)

// MDADMOperator - MDADM operator implementation
type MDADMOperator struct {
	executor *Executor
}

// NewMDADMOperator - Creates MDADM operator
func NewMDADMOperator() *MDADMOperator {
	return &MDADMOperator{
		executor: NewExecutor(),
	}
}

// CreateRAID - Create RAID array
// level: 0, 1, 4, 5, 6, 10
// devices: device list
// name: array name
// bitmap: enable bitmap
// waitSync: wait for RAID sync to complete
func (m *MDADMOperator) CreateRAID(level int, devices []string, name string, bitmap bool) (string, error) {
	// Check if mdadm is available
	if !m.executor.CheckCommandExists("mdadm") {
		return "", fmt.Errorf("mdadm command not available, please install mdadm")
	}
	// Create device name
	device := fmt.Sprintf("/dev/md/%s", name)
	// Build command
	args := []string{
		"--create", device,
		"--level", fmt.Sprintf("%d", level),
		"--raid-devices", fmt.Sprintf("%d", len(devices)),
	}
	// Add bitmap
	if bitmap {
		args = append(args, "--bitmap", "internal")
	}
	// Add devices
	args = append(args, devices...)
	output, err := m.executor.Run("mdadm", args...)
	if err != nil {
		return "", fmt.Errorf("failed to create RAID: %v, output: %s", err, output)
	}
	return device, nil
}

// RemoveRAID - Remove RAID array
func (m *MDADMOperator) RemoveRAID(dev string) error {
	// Get component devices before stopping
	devices := m.getComponentDevices(dev)
	// Stop array first
	_, err := m.executor.Run("mdadm", "--stop", dev)
	if err != nil {
		return fmt.Errorf("failed to stop RAID: %v", err)
	}
	// Clear superblock from all component devices
	for _, d := range devices {
		m.executor.Run("mdadm", "--zero-superblock", "-f", d)
	}
	return nil
}

// QueryRAID - Query RAID info
func (m *MDADMOperator) QueryRAID(dev string) (*models.RAIDInfo, error) {
	output, err := m.executor.Run("mdadm", "--detail", dev)
	if err != nil {
		return nil, fmt.Errorf("failed to query RAID: %v", err)
	}
	return m.parseRAIDDetail(output)
}

// AddDeviceRAID - Add device to RAID
func (m *MDADMOperator) AddDeviceRAID(dev string, newDev string) error {
	_, err := m.executor.Run("mdadm", "--add", dev, newDev)
	if err != nil {
		return fmt.Errorf("failed to add device: %v", err)
	}
	return nil
}

// RemoveDeviceRAID - Remove device from RAID
func (m *MDADMOperator) RemoveDeviceRAID(dev string, rmDev string) error {
	_, err := m.executor.Run("mdadm", "--remove", dev, rmDev)
	if err != nil {
		return fmt.Errorf("failed to remove device: %v", err)
	}
	return nil
}

// SetDeviceFaultRAID - Set device to faulty state
func (m *MDADMOperator) SetDeviceFaultRAID(dev string, devToFail string) error {
	_, err := m.executor.Run("mdadm", "--set-faulty", dev, devToFail)
	if err != nil {
		return fmt.Errorf("failed to set fault: %v", err)
	}
	return nil
}

// GrowRAID - Grow RAID array to add more devices
func (m *MDADMOperator) GrowRAID(dev string, newDevices []string) error {
	if len(newDevices) == 0 {
		return nil
	}
	// First add the new devices
	for _, d := range newDevices {
		_, err := m.executor.Run("mdadm", "--add", dev, d)
		if err != nil {
			return fmt.Errorf("failed to add device %s: %v", d, err)
		}
	}
	// Then grow the array
	_, err := m.executor.Run("mdadm", "--grow", dev, "--raid-devices", "+"+strconv.Itoa(len(newDevices)))
	if err != nil {
		return fmt.Errorf("failed to grow RAID: %v", err)
	}
	return nil
}

// ListMDDevices - List all MD devices
func (m *MDADMOperator) ListMDDevices() ([]models.RAIDInfo, error) {
	output, err := m.executor.Run("mdadm", "--detail", "--scan")
	if err != nil {
		return nil, fmt.Errorf("failed to list MD devices: %v", err)
	}
	var devices []models.RAIDInfo
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Parse each line
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

// GetComponentDevices - Get RAID component devices (exported)
func (m *MDADMOperator) GetComponentDevices(dev string) []string {
	return m.getComponentDevices(dev)
}

// getComponentDevices - Get RAID component devices
func (m *MDADMOperator) getComponentDevices(dev string) []string {
	// Try to get component devices from /proc/mdstat
	output, err := m.executor.Run("cat", "/proc/mdstat")
	if err != nil {
		return nil
	}
	// Parse mdstat to find component devices
	var devices []string
	lines := strings.Split(output, "\n")
	var inDevice bool
	for _, line := range lines {
		if strings.Contains(line, strings.TrimPrefix(dev, "/dev/md/")) {
			inDevice = true
			continue
		}
		if inDevice {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				// Component device line
				parts := strings.Fields(line)
				if len(parts) > 0 && strings.HasPrefix(parts[0], "/dev/") {
					devices = append(devices, parts[0])
				}
			} else if strings.TrimSpace(line) == "" {
				break
			}
		}
	}
	// If we couldn't parse from mdstat, try using mdadm --detail
	if len(devices) == 0 {
		output, err := m.executor.Run("mdadm", "--detail", dev)
		if err == nil {
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "/dev/") {
					fields := strings.Fields(line)
					if len(fields) > 0 {
						devices = append(devices, fields[0])
					}
				}
			}
		}
	}
	return devices
}

// parseRAIDDetail - Parse RAID detail output
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
		case "Raid Level":
			// Parse RAID level from "raid5", "raid6", etc.
			levelStr := strings.TrimPrefix(strings.ToLower(value), "raid")
			level, err := strconv.Atoi(levelStr)
			if err == nil {
				info.Level = level
			}
		case "Level":
			// Alternative field name
			levelStr := strings.TrimPrefix(strings.ToLower(value), "raid")
			level, err := strconv.Atoi(levelStr)
			if err == nil {
				info.Level = level
			}
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
