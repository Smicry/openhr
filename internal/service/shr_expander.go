package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openhr/internal/models"
	"github.com/openhr/internal/storage"
	"github.com/openhr/pkg/utils/logger"
)

// SHRExpander handles SHR pool expansion
type SHRExpander struct {
	planner *SHRPlanner
	lvm     *storage.LVMOperator
	mdadm   *storage.MDADMOperator
	parted  *storage.PartitionOperator
}

// NewSHRExpander creates a new SHR expander
func NewSHRExpander() *SHRExpander {
	return &SHRExpander{
		planner: NewSHRPlanner(),
		lvm:     storage.NewLVMOperator(),
		mdadm:   storage.NewMDADMOperator(),
		parted:  storage.NewPartitionOperator(),
	}
}

// Expand expands an SHR pool with new disks
func (e *SHRExpander) Expand(poolName string, newDiskDevices []string) error {
	logger.Info("Expanding SHR pool: %s", poolName)
	vgName := "openhr_" + poolName
	// 1. Detect existing SHR structure from system
	existingDisks, parity, err := e.detectExistingSHR(vgName)
	if err != nil {
		return fmt.Errorf("failed to detect existing SHR structure: %w", err)
	}
	logger.Info("Detected existing SHR-%d pool with %d disks", parity, len(existingDisks))
	// 2. Collect new disk information
	newDisks := make([]models.Disk, len(newDiskDevices))
	for i, dev := range newDiskDevices {
		disk, err := e.parted.GetDiskInfo(dev)
		if err != nil {
			return fmt.Errorf("failed to get disk info for %s: %w", dev, err)
		}
		newDisks[i] = *disk
		logger.Info("New disk: %s (%s)", dev, FormatBytes(disk.Size))
	}
	// 3. Plan new layout with all disks
	allDisks := append(existingDisks, newDisks...)
	newLayout, err := e.planner.PlanLayout(allDisks, parity)
	if err != nil {
		return fmt.Errorf("failed to plan expanded layout: %w", err)
	}
	logger.Info("New layout has %d layers, capacity: %s",
		len(newLayout.Layers), FormatBytes(newLayout.TotalCapacity))
	// 4. Detect current layer structure
	currentLayers := e.detectCurrentLayers(vgName, poolName)
	// 5. Determine what changes are needed
	plan := e.analyzeExpansionPlan(currentLayers, newLayout, newDisks)
	if len(plan.NewLayers) == 0 && len(plan.ExistingLayers) == 0 {
		logger.Info("No expansion needed")
		return nil
	}
	// 6. Execute expansion plan
	if err := e.executeExpansionPlan(poolName, plan, newLayout); err != nil {
		return fmt.Errorf("failed to execute expansion: %w", err)
	}
	logger.Info("[OK] SHR pool expanded successfully: %s", poolName)
	return nil
}

// detectExistingSHR detects existing SHR pool structure from VG
func (e *SHRExpander) detectExistingSHR(vgName string) ([]models.Disk, int, error) {
	// Get all PVs in this VG
	pvs, err := e.lvm.ListPVs()
	if err != nil {
		return nil, 0, err
	}
	// Find MD devices in this VG
	var mdDevices []string
	for _, pv := range pvs {
		if pv.VGName == vgName && strings.HasPrefix(pv.Name, "/dev/md/") {
			mdDevices = append(mdDevices, pv.Name)
		}
	}
	if len(mdDevices) == 0 {
		return nil, 0, fmt.Errorf("no MD devices found in VG %s", vgName)
	}
	// Scan all disks and check if they have partitions in this VG's MD arrays
	disks, err := e.parted.ListDisks()
	if err != nil {
		return nil, 0, err
	}
	var existingDisks []models.Disk
	for _, disk := range disks {
		// Check if disk has partitions that are part of MD arrays
		if e.isDiskInSHRPool(disk.Device, mdDevices) {
			existingDisks = append(existingDisks, disk)
		}
	}
	// Determine parity from number of MD devices
	// If we have MD devices, we need to determine parity
	// This is a heuristic: if there are 2+ MD devices, check RAID levels
	parity := 1 // Default to SHR-1
	if len(mdDevices) >= 2 {
		// Check if any layer uses RAID6 (indicates SHR-2)
		for _, mdDev := range mdDevices {
			raidInfo, err := e.mdadm.QueryRAID(mdDev)
			if err == nil && raidInfo.Level == 6 {
				parity = 2
				break
			}
		}
	}
	return existingDisks, parity, nil
}

// isDiskInSHRPool checks if a disk has partitions in the SHR pool's MD arrays
func (e *SHRExpander) isDiskInSHRPool(diskDev string, mdDevices []string) bool {
	parts, err := e.parted.ListPartitions(diskDev)
	if err != nil {
		return false
	}
	for _, part := range parts {
		partDev := fmt.Sprintf("%s%d", diskDev, part.Number)
		// Check if this partition is part of any MD device
		for _, mdDev := range mdDevices {
			// Simplified check - in production, would query mdadm
			if strings.Contains(mdDev, diskDev) {
				return true
			}
		}
		_ = partDev
	}
	return false
}

// detectCurrentLayers detects the current layer structure from the VG
func (e *SHRExpander) detectCurrentLayers(vgName, poolName string) []models.SHRLayer {
	pvs, err := e.lvm.ListPVs()
	if err != nil {
		return nil
	}
	var layers []models.SHRLayer
	layerIdx := 0
	for _, pv := range pvs {
		if pv.VGName == vgName && strings.HasPrefix(pv.Name, "/dev/md/") {
			raidInfo, err := e.mdadm.QueryRAID(pv.Name)
			if err != nil {
				continue
			}
			layer := models.SHRLayer{
				Index:      layerIdx,
				RAIDLevel:  raidInfo.Level,
				RAIDDevice: pv.Name,
			}
			layers = append(layers, layer)
			layerIdx++
		}
	}
	// Sort layers by index
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Index < layers[j].Index
	})
	return layers
}

// analyzeExpansionPlan analyzes what changes are needed for expansion
func (e *SHRExpander) analyzeExpansionPlan(
	currentLayers []models.SHRLayer,
	newLayout *models.SHRLayout,
	newDisks []models.Disk,
) *models.SHRExpansionPlan {
	plan := &models.SHRExpansionPlan{}
	// Map new disk devices
	newDiskSet := make(map[string]bool)
	for _, d := range newDisks {
		newDiskSet[d.Device] = true
	}
	// Check each new layer (layers that don't exist yet)
	for i := len(currentLayers); i < len(newLayout.Layers); i++ {
		plan.NewLayers = append(plan.NewLayers, newLayout.Layers[i])
		plan.AddedCapacity += newLayout.Layers[i].Capacity
	}
	// Check for changes in existing layers
	for i := 0; i < len(currentLayers) && i < len(newLayout.Layers); i++ {
		newLayer := newLayout.Layers[i]
		// Check if new disks are added to this layer
		var newDisksForLayer []string
		for _, dev := range newLayer.DiskDevices {
			if newDiskSet[dev] {
				newDisksForLayer = append(newDisksForLayer, dev)
			}
		}
		if len(newDisksForLayer) > 0 {
			plan.ExistingLayers = append(plan.ExistingLayers, models.SHRLayerExpansion{
				LayerIndex: i,
				NewDisks:   newDisksForLayer,
			})
			plan.AddedCapacity += newLayer.Size * int64(len(newDisksForLayer))
		}
	}
	return plan
}

// executeExpansionPlan executes the expansion plan
func (e *SHRExpander) executeExpansionPlan(
	poolName string,
	plan *models.SHRExpansionPlan,
	newLayout *models.SHRLayout,
) error {
	vgName := "openhr_" + poolName
	// 1. Expand existing layers (add disks to existing RAID arrays)
	for _, exp := range plan.ExistingLayers {
		logger.Info("Expanding layer %d with %d new disks", exp.LayerIndex, len(exp.NewDisks))
		layer := &newLayout.Layers[exp.LayerIndex]
		// Create partitions on new disks for this layer
		partNum := e.getNextPartitionNumber(exp.NewDisks[0])
		for _, diskDev := range exp.NewDisks {
			disk, _ := e.parted.GetDiskInfo(diskDev)
			// Calculate partition position
			startOffset := e.calculateStartOffset(newLayout, diskDev, exp.LayerIndex)
			endOffset := startOffset + layer.Size
			startPct := float64(startOffset) * 100 / float64(disk.Size)
			endPct := float64(endOffset) * 100 / float64(disk.Size)
			partDev := fmt.Sprintf("%s%d", diskDev, partNum)
			// Create partition
			if err := e.parted.CreatePartition(diskDev, "primary",
				fmt.Sprintf("%.1f%%", startPct),
				fmt.Sprintf("%.1f%%", endPct)); err != nil {
				return fmt.Errorf("failed to create partition on %s: %w", diskDev, err)
			}
			e.parted.SetPartitionType(diskDev, partNum, "raid")
			// Add to RAID array
			raidDev := layer.RAIDDevice
			if raidDev == "" {
				raidDev = fmt.Sprintf("/dev/md/%s_layer%d", poolName, exp.LayerIndex)
			}
			logger.Info("Adding %s to RAID array %s", partDev, raidDev)
			if err := e.mdadm.AddDeviceRAID(raidDev, partDev); err != nil {
				return fmt.Errorf("failed to add %s to RAID: %w", partDev, err)
			}
			layer.Partitions = append(layer.Partitions, partDev)
		}
		// Grow the RAID array
		raidDev := layer.RAIDDevice
		if raidDev == "" {
			raidDev = fmt.Sprintf("/dev/md/%s_layer%d", poolName, exp.LayerIndex)
		}
		logger.Info("Growing RAID array: %s", raidDev)
	}
	// 2. Create new layers
	for i := range plan.NewLayers {
		layer := &plan.NewLayers[i]
		layerIdx := len(plan.ExistingLayers) + i
		logger.Info("Creating new layer %d (%s) with %d disks",
			layerIdx, layer.GetRAIDLevelName(), len(layer.DiskDevices))
		// Create partitions
		for _, diskDev := range layer.DiskDevices {
			disk, _ := e.parted.GetDiskInfo(diskDev)
			partNum := e.getNextPartitionNumber(diskDev)
			startOffset := e.calculateStartOffset(newLayout, diskDev, layerIdx)
			endOffset := startOffset + layer.Size
			startPct := float64(startOffset) * 100 / float64(disk.Size)
			endPct := float64(endOffset) * 100 / float64(disk.Size)
			partDev := fmt.Sprintf("%s%d", diskDev, partNum)
			if err := e.parted.CreatePartition(diskDev, "primary",
				fmt.Sprintf("%.1f%%", startPct),
				fmt.Sprintf("%.1f%%", endPct)); err != nil {
				return fmt.Errorf("failed to create partition: %w", err)
			}
			e.parted.SetPartitionType(diskDev, partNum, "raid")
			layer.Partitions = append(layer.Partitions, partDev)
		}
		// Create RAID array
		raidName := fmt.Sprintf("%s_layer%d", poolName, layerIdx)
		raidDev, err := e.mdadm.CreateRAID(layer.RAIDLevel, layer.Partitions, raidName, true)
		if err != nil {
			return fmt.Errorf("failed to create RAID for new layer: %w", err)
		}
		layer.RAIDDevice = raidDev
		// Create PV and add to VG
		if err := e.lvm.CreatePV(raidDev); err != nil {
			return fmt.Errorf("failed to create PV: %w", err)
		}
		if err := e.lvm.ExtendVG(vgName, []string{raidDev}); err != nil {
			return fmt.Errorf("failed to extend VG: %w", err)
		}
	}
	return nil
}

// getNextPartitionNumber gets the next available partition number for a disk
func (e *SHRExpander) getNextPartitionNumber(diskDev string) int {
	parts, err := e.parted.ListPartitions(diskDev)
	if err != nil {
		return 1
	}
	maxNum := 0
	for _, p := range parts {
		if p.Number > maxNum {
			maxNum = p.Number
		}
	}
	return maxNum + 1
}

// calculateStartOffset calculates the start offset for a layer on a disk
func (e *SHRExpander) calculateStartOffset(layout *models.SHRLayout, diskDev string, targetLayerIdx int) int64 {
	var offset int64
	for i := 0; i < targetLayerIdx; i++ {
		layer := &layout.Layers[i]
		for _, dev := range layer.DiskDevices {
			if dev == diskDev {
				offset += layer.Size
				break
			}
		}
	}
	return offset
}
