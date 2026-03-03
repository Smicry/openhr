package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/openhr/internal/models"
	"github.com/openhr/internal/storage"
	"github.com/openhr/pkg/utils/logger"
)

// PoolService storage pool service
type PoolService struct {
	lvm    *storage.LVMOperator
	mdadm  *storage.MDADMOperator
	parted *storage.PartitionOperator
	calc   *CapacityCalculator
}

// NewPoolService creates a new pool service
func NewPoolService() *PoolService {
	return &PoolService{
		lvm:    storage.NewLVMOperator(),
		mdadm:  storage.NewMDADMOperator(),
		parted: storage.NewPartitionOperator(),
		calc:   NewCapacityCalculator(),
	}
}

// CreatePool creates a storage pool
func (s *PoolService) CreatePool(cfg models.PoolConfig) (*models.Pool, error) {
	logger.Info("Creating pool: %s", cfg.Name)
	logger.Info("Mode: %s", cfg.Mode.GetModeDisplay())
	logger.Info("Disks: %v", cfg.Disks)
	// 1. Collect disk information
	disks, err := s.collectDiskInfo(cfg.Disks)
	if err != nil {
		return nil, fmt.Errorf("failed to collect disk info: %w", err)
	}
	// 2. Estimate capacity
	sizes := make([]DiskSizeInfo, len(disks))
	for i, d := range disks {
		sizes[i] = DiskSizeInfo{
			Device: d.Device,
			Size:   d.Size,
		}
	}
	capacity := s.calc.EstimateCapacity(sizes, cfg.Mode)
	logger.Info("Raw Total: %s", FormatBytes(capacity.TotalRawCapacity))
	logger.Info("Parity: %s", FormatBytes(capacity.ParityCapacity))
	logger.Info("Usable: %s", FormatBytes(capacity.UsableCapacity))
	logger.Info("Redundancy: %s", capacity.ProtectionLevel)
	if capacity.UsableCapacity <= 0 {
		return nil, fmt.Errorf("insufficient usable capacity to create pool")
	}
	// 3. Create pool based on mode
	switch cfg.Mode {
	case models.ModeSHR1:
		return s.createSHRPool(disks, cfg.Name, 1)
	case models.ModeSHR2:
		return s.createSHRPool(disks, cfg.Name, 2)
	case models.ModeRAID1:
		return s.createRAID1Pool(disks, cfg.Name)
	case models.ModeRAID5:
		return s.createRAID5Pool(disks, cfg.Name)
	case models.ModeRAID6:
		return s.createRAID6Pool(disks, cfg.Name)
	case models.ModeBasic:
		return s.createBasicPool(disks, cfg.Name)
	default:
		return nil, fmt.Errorf("unsupported storage mode: %s", cfg.Mode)
	}
}

// collectDiskInfo collects disk information
func (s *PoolService) collectDiskInfo(devices []string) ([]models.Disk, error) {
	var disks []models.Disk
	for _, dev := range devices {
		disk, err := s.parted.GetDiskInfo(dev)
		if err != nil {
			return nil, fmt.Errorf("failed to get disk info %s: %w", dev, err)
		}
		// Check if disk is already in use
		inUse, reason, err := s.parted.IsDiskInUse(dev)
		if err != nil {
			return nil, fmt.Errorf("failed to check disk usage for %s: %w", dev, err)
		}
		if inUse {
			return nil, fmt.Errorf("disk %s is already in use: %s", dev, reason)
		}
		// Check if disk is read-only
		if disk.Ro {
			return nil, fmt.Errorf("disk %s is read-only and cannot be used", dev)
		}
		disks = append(disks, *disk)
	}
	return disks, nil
}

// createSHRPool creates an SHR pool using Synology's multi-layer algorithm
// Each disk is divided into multiple layers, each layer forms its own RAID array
// All RAID arrays are combined using LVM to form a single storage pool
func (s *PoolService) createSHRPool(disks []models.Disk, name string, parity int) (*models.Pool, error) {
	logger.Info("Creating SHR-%d pool with multi-layer architecture", parity)
	planner := NewSHRPlanner()
	// 1. Plan the SHR layout
	layout, err := planner.PlanLayout(disks, parity)
	if err != nil {
		return nil, fmt.Errorf("failed to plan SHR layout: %w", err)
	}
	if len(layout.Layers) == 0 {
		return nil, fmt.Errorf("no valid layers can be formed")
	}
	logger.Info("SHR-%d layout: %d layers, total capacity: %s",
		parity, len(layout.Layers), FormatBytes(layout.TotalCapacity))
	// 2. Create partitions for each layer on each disk
	logger.Info("Creating partitions for each layer...")
	if err := s.createSHRPartitions(layout, name); err != nil {
		return nil, fmt.Errorf("failed to create partitions: %w", err)
	}
	// 3. Create RAID arrays for each layer
	logger.Info("Creating RAID arrays for each layer...")
	if err := s.createSHRRAIDs(layout, name); err != nil {
		return nil, fmt.Errorf("failed to create RAID arrays: %w", err)
	}
	// 4. Create LVM PVs for all RAID devices
	logger.Info("Creating LVM physical volumes...")
	var pvDevices []string
	for i := range layout.Layers {
		raidDev := layout.Layers[i].RAIDDevice
		if err := s.lvm.CreatePV(raidDev); err != nil {
			return nil, fmt.Errorf("failed to create PV for layer %d: %w", i, err)
		}
		pvDevices = append(pvDevices, raidDev)
	}
	// 5. Create VG combining all layers
	vgName := "openhr_" + name
	logger.Info("Creating volume group: %s", vgName)
	if err := s.lvm.CreateVG(vgName, pvDevices); err != nil {
		return nil, fmt.Errorf("failed to create VG: %w", err)
	}
	// Determine the mode string
	var mode models.StorageMode
	if parity == 1 {
		mode = models.ModeSHR1
	} else {
		mode = models.ModeSHR2
	}
	pool := &models.Pool{
		ID:        vgName,
		Name:      name,
		Mode:      mode,
		Status:    models.PoolStatusActive,
		TotalSize: layout.TotalCapacity,
		Disks:     disks,
		VGName:    vgName,
		RAIDDev:   layout.Layers[0].RAIDDevice,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	logger.Info("[OK] SHR-%d pool created successfully: %s (%d layers)", parity, name, len(layout.Layers))
	return pool, nil
}

// createSHRPartitions creates partitions for each layer on each disk
func (s *PoolService) createSHRPartitions(layout *models.SHRLayout, poolName string) error {
	// Track partition numbers for each disk
	partitionNumbers := make(map[string]int)
	for _, disk := range layout.Disks {
		partitionNumbers[disk.Device] = 1
	}
	// For each layer, create partitions on participating disks
	for layerIdx := range layout.Layers {
		layer := &layout.Layers[layerIdx]
		logger.Info("Creating partitions for layer %d (%s)...", layerIdx, layer.GetRAIDLevelName())
		for _, diskDev := range layer.DiskDevices {
			disk := s.findDiskByDevice(layout.Disks, diskDev)
			if disk == nil {
				return fmt.Errorf("disk not found: %s", diskDev)
			}
			// Calculate partition boundaries
			partNum := partitionNumbers[diskDev]
			startOffset := s.calculateLayerStartOffset(layout, diskDev, layerIdx)
			endOffset := startOffset + layer.Size
			// Create partition
			startPct := float64(startOffset) * 100 / float64(disk.Size)
			endPct := float64(endOffset) * 100 / float64(disk.Size)
			partDev := fmt.Sprintf("%s%d", diskDev, partNum)
			logger.Debug("  Creating partition %s: %.1f%% - %.1f%% (%s)",
				partDev, startPct, endPct, FormatBytes(layer.Size))
			err := s.parted.CreatePartition(diskDev, "primary",
				fmt.Sprintf("%.1f%%", startPct),
				fmt.Sprintf("%.1f%%", endPct))
			if err != nil {
				return fmt.Errorf("failed to create partition %s: %w", partDev, err)
			}
			// Set partition type to RAID
			if err := s.parted.SetPartitionType(diskDev, partNum, "raid"); err != nil {
				logger.Warn("Failed to set partition type for %s: %v", partDev, err)
			}
			layer.Partitions = append(layer.Partitions, partDev)
			partitionNumbers[diskDev]++
		}
	}
	return nil
}

// createSHRRAIDs creates RAID arrays for each layer
func (s *PoolService) createSHRRAIDs(layout *models.SHRLayout, poolName string) error {
	for i := range layout.Layers {
		layer := &layout.Layers[i]
		raidName := fmt.Sprintf("%s_layer%d", poolName, i)
		logger.Info("Creating RAID array for layer %d: %s (%s) with %d devices",
			i, raidName, layer.GetRAIDLevelName(), len(layer.Partitions))
		raidDev, err := s.mdadm.CreateRAID(layer.RAIDLevel, layer.Partitions, raidName, true)
		if err != nil {
			return fmt.Errorf("failed to create RAID for layer %d: %w", i, err)
		}
		layer.RAIDDevice = raidDev
		logger.Info("  Created RAID device: %s", raidDev)
	}
	return nil
}

// calculateLayerStartOffset calculates the start offset for a layer on a disk
func (s *PoolService) calculateLayerStartOffset(layout *models.SHRLayout, diskDev string, targetLayerIdx int) int64 {
	var offset int64
	for i := 0; i < targetLayerIdx; i++ {
		layer := &layout.Layers[i]
		// Check if this disk participates in this layer
		for _, dev := range layer.DiskDevices {
			if dev == diskDev {
				offset += layer.Size
				break
			}
		}
	}
	return offset
}

// findDiskByDevice finds a disk by its device path
func (s *PoolService) findDiskByDevice(disks []models.Disk, device string) *models.Disk {
	for i := range disks {
		if disks[i].Device == device {
			return &disks[i]
		}
	}
	return nil
}

// createRAID1Pool creates RAID1 pool
func (s *PoolService) createRAID1Pool(disks []models.Disk, name string) (*models.Pool, error) {
	devices := make([]string, len(disks))
	for i, d := range disks {
		devices[i] = d.Device
	}
	raidDev, err := s.mdadm.CreateRAID(1, devices, name, true)
	if err != nil {
		return nil, err
	}
	// Create LVM PV
	err = s.lvm.CreatePV(raidDev)
	if err != nil {
		return nil, err
	}
	vgName := "openhr_" + name
	err = s.lvm.CreateVG(vgName, []string{raidDev})
	if err != nil {
		return nil, err
	}
	return &models.Pool{
		ID:        vgName,
		Name:      name,
		Mode:      models.ModeRAID1,
		Status:    models.PoolStatusActive,
		TotalSize: disks[0].Size,
		Disks:     disks,
		VGName:    vgName,
		RAIDDev:   raidDev,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// createRAID5Pool creates RAID5 pool
func (s *PoolService) createRAID5Pool(disks []models.Disk, name string) (*models.Pool, error) {
	devices := make([]string, len(disks))
	for i, d := range disks {
		devices[i] = d.Device
	}
	raidDev, err := s.mdadm.CreateRAID(5, devices, name, true)
	if err != nil {
		return nil, err
	}
	err = s.lvm.CreatePV(raidDev)
	if err != nil {
		return nil, err
	}
	vgName := "openhr_" + name
	err = s.lvm.CreateVG(vgName, []string{raidDev})
	if err != nil {
		return nil, err
	}
	minSize := disks[0].Size
	for _, d := range disks {
		if d.Size < minSize {
			minSize = d.Size
		}
	}
	return &models.Pool{
		ID:        vgName,
		Name:      name,
		Mode:      models.ModeRAID5,
		Status:    models.PoolStatusActive,
		TotalSize: minSize * int64(len(disks)-1),
		Disks:     disks,
		VGName:    vgName,
		RAIDDev:   raidDev,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// createRAID6Pool creates RAID6 pool
func (s *PoolService) createRAID6Pool(disks []models.Disk, name string) (*models.Pool, error) {
	devices := make([]string, len(disks))
	for i, d := range disks {
		devices[i] = d.Device
	}
	raidDev, err := s.mdadm.CreateRAID(6, devices, name, true)
	if err != nil {
		return nil, err
	}
	err = s.lvm.CreatePV(raidDev)
	if err != nil {
		return nil, err
	}
	vgName := "openhr_" + name
	err = s.lvm.CreateVG(vgName, []string{raidDev})
	if err != nil {
		return nil, err
	}
	minSize := disks[0].Size
	for _, d := range disks {
		if d.Size < minSize {
			minSize = d.Size
		}
	}
	return &models.Pool{
		ID:        vgName,
		Name:      name,
		Mode:      models.ModeRAID6,
		Status:    models.PoolStatusActive,
		TotalSize: minSize * int64(len(disks)-2),
		Disks:     disks,
		VGName:    vgName,
		RAIDDev:   raidDev,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// createBasicPool creates Basic pool
func (s *PoolService) createBasicPool(disks []models.Disk, name string) (*models.Pool, error) {
	if len(disks) != 1 {
		return nil, fmt.Errorf("Basic mode can only use 1 disk")
	}
	disk := disks[0]
	// Create partition
	err := s.parted.CreatePartition(disk.Device, "primary", "0%", "100%")
	if err != nil {
		return nil, err
	}
	partDev := fmt.Sprintf("%s1", disk.Device)
	// Create LVM PV
	err = s.lvm.CreatePV(partDev)
	if err != nil {
		return nil, err
	}
	// Create VG
	vgName := "openhr_" + name
	err = s.lvm.CreateVG(vgName, []string{partDev})
	if err != nil {
		return nil, err
	}
	return &models.Pool{
		ID:        vgName,
		Name:      name,
		Mode:      models.ModeBasic,
		Status:    models.PoolStatusActive,
		TotalSize: disk.Size,
		Disks:     disks,
		VGName:    vgName,
		RAIDDev:   partDev,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// DeletePool deletes a storage pool
func (s *PoolService) DeletePool(name string) error {
	vgName := "openhr_" + name
	// Check if VG exists
	_, err := s.lvm.GetVGInfo(vgName)
	if err != nil {
		return fmt.Errorf("pool does not exist: %s", name)
	}
	// Get all PVs in this VG
	pvs, err := s.lvm.ListPVs()
	if err != nil {
		return fmt.Errorf("failed to list PVs: %w", err)
	}
	// Collect all MD devices and component devices before removing VG
	var mdDevices []string
	var componentDevices []string
	for _, pv := range pvs {
		if pv.VGName == vgName {
			if strings.HasPrefix(pv.Name, "/dev/md/") {
				mdDevices = append(mdDevices, pv.Name)
				// Get component devices from RAID
				comps := s.mdadm.GetComponentDevices(pv.Name)
				componentDevices = append(componentDevices, comps...)
			} else {
				// Non-RAID device (e.g., partition)
				componentDevices = append(componentDevices, pv.Name)
			}
		}
	}
	// Remove VG first (this releases the PVs)
	err = s.lvm.RemoveVG(vgName)
	if err != nil {
		return fmt.Errorf("failed to remove VG: %w", err)
	}
	// Stop and clean up RAID arrays
	for _, mdDev := range mdDevices {
		logger.Info("Cleaning up RAID array: %s", mdDev)
		if err := s.mdadm.RemoveRAID(mdDev); err != nil {
			logger.Warn("Failed to clean up RAID %s: %v", mdDev, err)
		}
	}
	// Clean up partitions on component devices
	diskPartitions := make(map[string][]int)
	for _, dev := range componentDevices {
		// Extract disk name (e.g., /dev/sda1 -> /dev/sda)
		diskDev := dev
		partNum := 0
		if len(dev) > len("/dev/sd") {
			// Try to parse partition number
			for i := len(dev) - 1; i >= len("/dev/sd"); i-- {
				if dev[i] >= '0' && dev[i] <= '9' {
					partNum = partNum*10 + int(dev[i]-'0')
				} else {
					break
				}
			}
			// Reverse the number
			newPartNum := 0
			for tmp := partNum; tmp > 0; tmp /= 10 {
				newPartNum = newPartNum*10 + tmp%10
			}
			partNum = newPartNum

			if partNum > 0 {
				diskDev = dev[:len(dev)-len(fmt.Sprintf("%d", partNum))]
			}
		}
		if partNum > 0 {
			diskPartitions[diskDev] = append(diskPartitions[diskDev], partNum)
		}
	}
	// Remove PV signatures from partitions
	for _, pv := range pvs {
		if pv.VGName == vgName {
			logger.Info("Removing PV signature from: %s", pv.Name)
			if err := s.lvm.RemovePV(pv.Name); err != nil {
				logger.Warn("Failed to remove PV %s: %v", pv.Name, err)
			}
		}
	}
	logger.Info("[OK] Pool deleted: %s", name)
	return nil
}

// ListPools lists all storage pools
func (s *PoolService) ListPools() ([]*models.Pool, error) {
	vgs, err := s.lvm.ListVGs()
	if err != nil {
		return nil, err
	}
	var pools []*models.Pool
	for _, vg := range vgs {
		// Only list VGs starting with openhr_
		if !strings.HasPrefix(vg.Name, "openhr_") {
			continue
		}
		pool := &models.Pool{
			ID:        vg.Name,
			Name:      strings.TrimPrefix(vg.Name, "openhr_"),
			TotalSize: vg.Size,
			FreeSize:  vg.Free,
			UsedSize:  vg.Size - vg.Free,
			VGName:    vg.Name,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		pools = append(pools, pool)
	}
	return pools, nil
}

// GetPool gets pool details
func (s *PoolService) GetPool(name string) (*models.Pool, error) {
	vgName := "openhr_" + name
	vginfo, err := s.lvm.GetVGInfo(vgName)
	if err != nil {
		return nil, fmt.Errorf("pool does not exist: %s", name)
	}
	return &models.Pool{
		ID:        vgName,
		Name:      name,
		TotalSize: vginfo.Size,
		FreeSize:  vginfo.Free,
		UsedSize:  vginfo.Size - vginfo.Free,
		VGName:    vgName,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// ExpandPool expands a storage pool
func (s *PoolService) ExpandPool(name string, newDisks []string) error {
	vgName := "openhr_" + name
	// Check if this is an SHR pool by detecting if VG has multiple /dev/md/* devices
	isSHR := s.isSHRPool(vgName)
	if isSHR {
		// Use SHR expander for SHR pools
		expander := NewSHRExpander()
		return expander.Expand(name, newDisks)
	}
	// Standard pool expansion (non-SHR)
	// Collect new disk info
	disks, err := s.collectDiskInfo(newDisks)
	if err != nil {
		return err
	}
	// Create partitions
	var pvs []string
	for _, disk := range disks {
		err := s.parted.CreatePartition(disk.Device, "primary", "0%", "100%")
		if err != nil {
			return err
		}
		partDev := fmt.Sprintf("%s1", disk.Device)
		pvs = append(pvs, partDev)
	}
	// Create PV
	for _, pv := range pvs {
		err = s.lvm.CreatePV(pv)
		if err != nil {
			return err
		}
	}
	// Extend VG
	err = s.lvm.ExtendVG(vgName, pvs)
	if err != nil {
		return err
	}
	logger.Info("[OK] Pool expanded: %s", name)
	return nil
}

// isSHRPool checks if a pool is an SHR pool by detecting multiple /dev/md/* devices in VG
func (s *PoolService) isSHRPool(vgName string) bool {
	pvs, err := s.lvm.ListPVs()
	if err != nil {
		return false
	}
	mdCount := 0
	for _, pv := range pvs {
		if pv.VGName == vgName && strings.HasPrefix(pv.Name, "/dev/md/") {
			mdCount++
		}
	}
	// SHR pool has multiple MD devices (one per layer)
	return mdCount >= 2
}

// GetPoolByVGName gets pool by VG name (internal use)
func (s *PoolService) GetPoolByVGName(vgName string) (*models.Pool, error) {
	vginfo, err := s.lvm.GetVGInfo(vgName)
	if err != nil {
		return nil, err
	}
	return &models.Pool{
		ID:        vgName,
		Name:      strings.TrimPrefix(vgName, "openhr_"),
		TotalSize: vginfo.Size,
		FreeSize:  vginfo.Free,
		UsedSize:  vginfo.Size - vginfo.Free,
		VGName:    vgName,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// GetPoolDevices gets devices in the pool (internal use)
func (s *PoolService) GetPoolDevices(poolName string) ([]string, error) {
	vgName := "openhr_" + poolName
	// Get VG info to verify pool exists
	_, err := s.lvm.GetVGInfo(vgName)
	if err != nil {
		return nil, fmt.Errorf("pool does not exist: %s", poolName)
	}
	// Get all PVs in this VG
	pvs, err := s.lvm.ListPVs()
	if err != nil {
		return nil, fmt.Errorf("failed to list PVs: %w", err)
	}
	var devices []string
	for _, pv := range pvs {
		if pv.VGName == vgName {
			devices = append(devices, pv.Name)
		}
	}
	return devices, nil
}
