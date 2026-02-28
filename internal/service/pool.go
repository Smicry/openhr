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
		disks = append(disks, *disk)
	}
	return disks, nil
}

// createSHRPool creates an SHR pool
func (s *PoolService) createSHRPool(disks []models.Disk, name string, parity int) (*models.Pool, error) {
	logger.Info("Creating SHR-%d pool", parity)

	// Find min and max disk size
	minSize := disks[0].Size
	maxSize := disks[0].Size
	for _, d := range disks {
		if d.Size < minSize {
			minSize = d.Size
		}
		if d.Size > maxSize {
			maxSize = d.Size
		}
	}

	// 计算每个分区的容量
	dataSize := minSize
	paritySize := maxSize

	logger.Info("Data zone size: %s", FormatBytes(dataSize))
	logger.Info("Parity zone size: %s", FormatBytes(paritySize))

	// Create partitions
	logger.Info("Creating partitions...")
	raidDevices := make([]string, len(disks))
	
	for i, disk := range disks {
		logger.Info("Partitioning disk: %s", disk.Device)
		
		// Create data partition (50%)
		dataPart := fmt.Sprintf("%s1", disk.Device)
		err := s.parted.CreatePartition(disk.Device, "primary", "0%", "50%")
		if err != nil {
			return nil, fmt.Errorf("failed to create data partition: %w", err)
		}
		raidDevices[i] = dataPart

		// Create parity partition (50%)
		parityPart := fmt.Sprintf("%s2", disk.Device)
		_ = parityPart // Keep partition path for later use
		err = s.parted.CreatePartition(disk.Device, "primary", "50%", "100%")
		if err != nil {
			return nil, fmt.Errorf("failed to create parity partition: %w", err)
		}

		// Set partition type to RAID
		s.parted.SetPartitionType(disk.Device, 1, "raid")
		s.parted.SetPartitionType(disk.Device, 2, "raid")
	}

	// Create RAID (use RAID0 for data zone to maximize space)
	logger.Info("Creating data RAID...")
	dataRAID, err := s.mdadm.CreateRAID(0, raidDevices, name+"_data", false)
	if err != nil {
		return nil, fmt.Errorf("failed to create data RAID: %w", err)
	}

	// Create parity RAID (RAID1)
	parityDevices := make([]string, len(disks))
	for i := range disks {
		parityDevices[i] = fmt.Sprintf("%s2", disks[i].Device)
	}
	logger.Info("Creating parity RAID...")
	parityRAID, err := s.mdadm.CreateRAID(1, parityDevices, name+"_parity", true)
	if err != nil {
		return nil, fmt.Errorf("failed to create parity RAID: %w", err)
	}

	// Create LVM PV
	logger.Info("Creating LVM physical volumes...")
	err = s.lvm.CreatePV(dataRAID)
	if err != nil {
		return nil, fmt.Errorf("failed to create data PV: %w", err)
	}
	err = s.lvm.CreatePV(parityRAID)
	if err != nil {
		return nil, fmt.Errorf("failed to create parity PV: %w", err)
	}

	// Create VG
	vgName := "openhr_" + name
	logger.Info("Creating volume group: %s", vgName)
	err = s.lvm.CreateVG(vgName, []string{dataRAID, parityRAID})
	if err != nil {
		return nil, fmt.Errorf("failed to create VG: %w", err)
	}

	// Return pool information
	pool := &models.Pool{
		ID:        vgName,
		Name:      name,
		Mode:      models.ModeSHR1,
		Status:    models.PoolStatusActive,
		TotalSize: (minSize * int64(len(disks)-parity)) + (maxSize * int64(parity)),
		Disks:     disks,
		VGName:    vgName,
		RAIDDev:   dataRAID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	logger.Info("[OK] Pool created successfully: %s", name)
	return pool, nil
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

	// 创建LVM
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
		ID:         vgName,
		Name:       name,
		Mode:       models.ModeRAID1,
		Status:     models.PoolStatusActive,
		TotalSize:  disks[0].Size,
		Disks:      disks,
		VGName:     vgName,
		RAIDDev:    raidDev,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
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
		ID:         vgName,
		Name:       name,
		Mode:       models.ModeRAID5,
		Status:     models.PoolStatusActive,
		TotalSize:  minSize * int64(len(disks)-1),
		Disks:      disks,
		VGName:     vgName,
		RAIDDev:    raidDev,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
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
		ID:         vgName,
		Name:       name,
		Mode:       models.ModeRAID6,
		Status:     models.PoolStatusActive,
		TotalSize:  minSize * int64(len(disks)-2),
		Disks:      disks,
		VGName:     vgName,
		RAIDDev:    raidDev,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

// createBasicPool creates Basic pool
func (s *PoolService) createBasicPool(disks []models.Disk, name string) (*models.Pool, error) {
	if len(disks) != 1 {
		return nil, fmt.Errorf("Basic模式只能使用1块硬盘")
	}

	disk := disks[0]
	
	// 创建分区
	err := s.parted.CreatePartition(disk.Device, "primary", "0%", "100%")
	if err != nil {
		return nil, err
	}

	partDev := fmt.Sprintf("%s1", disk.Device)

	// 创建LVM PV
	err = s.lvm.CreatePV(partDev)
	if err != nil {
		return nil, err
	}

	// 创建VG
	vgName := "openhr_" + name
	err = s.lvm.CreateVG(vgName, []string{partDev})
	if err != nil {
		return nil, err
	}

	return &models.Pool{
		ID:         vgName,
		Name:       name,
		Mode:       models.ModeBasic,
		Status:     models.PoolStatusActive,
		TotalSize:  disk.Size,
		Disks:      disks,
		VGName:     vgName,
		RAIDDev:    partDev,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
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

	// Remove VG
	err = s.lvm.RemoveVG(vgName)
	if err != nil {
		return fmt.Errorf("failed to remove VG: %w", err)
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
		// 只列出openhr开头的卷组
		if !strings.HasPrefix(vg.Name, "openhr_") {
			continue
		}

		pool := &models.Pool{
			ID:          vg.Name,
			Name:        strings.TrimPrefix(vg.Name, "openhr_"),
			TotalSize:   vg.Size,
			FreeSize:   vg.Free,
			UsedSize:   vg.Size - vg.Free,
			VGName:      vg.Name,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
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
		ID:          vgName,
		Name:        name,
		TotalSize:   vginfo.Size,
		FreeSize:   vginfo.Free,
		UsedSize:   vginfo.Size - vginfo.Free,
		VGName:      vgName,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// ExpandPool expands a storage pool
func (s *PoolService) ExpandPool(name string, newDisks []string) error {
	vgName := "openhr_" + name

	// 收集新硬盘信息
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

// GetPoolByVGName gets pool by VG name (internal use)
func (s *PoolService) GetPoolByVGName(vgName string) (*models.Pool, error) {
	vginfo, err := s.lvm.GetVGInfo(vgName)
	if err != nil {
		return nil, err
	}

	return &models.Pool{
		ID:          vgName,
		Name:        strings.TrimPrefix(vgName, "openhr_"),
		TotalSize:   vginfo.Size,
		FreeSize:   vginfo.Free,
		UsedSize:   vginfo.Size - vginfo.Free,
		VGName:      vgName,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// GetPoolDevices gets devices in the pool (internal use)
func GetPoolDevices(dev string) []string {
	// Simplified implementation
	return nil
}
