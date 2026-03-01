package service

import (
	"errors"
	"fmt"
	"sort"

	"github.com/openhr/internal/models"
	"github.com/openhr/pkg/utils/logger"
)

// Errors
var (
	ErrInsufficientDisks = errors.New("insufficient disks for SHR configuration")
)

// SHRPlanner plans SHR layout based on disk sizes
type SHRPlanner struct{}

// NewSHRPlanner creates a new SHR planner
func NewSHRPlanner() *SHRPlanner {
	return &SHRPlanner{}
}

// PlanLayout plans the SHR layout for given disks
// This implements the Synology SHR multi-layer algorithm
func (p *SHRPlanner) PlanLayout(disks []models.Disk, parity int) (*models.SHRLayout, error) {
	logger.Info("Planning SHR-%d layout for %d disks", parity, len(disks))
	// Validate minimum disk requirements
	minDisks := p.minDisksNeeded(parity)
	if len(disks) < minDisks {
		return nil, ErrInsufficientDisks
	}
	// Sort disks by size (ascending)
	sortedDisks := make([]models.Disk, len(disks))
	copy(sortedDisks, disks)
	sort.Slice(sortedDisks, func(i, j int) bool {
		return sortedDisks[i].Size < sortedDisks[j].Size
	})
	// Track remaining space on each disk
	remaining := make(map[string]int64)
	for _, d := range sortedDisks {
		remaining[d.Device] = d.Size
	}
	// Calculate layers
	var layers []models.SHRLayer
	layerIndex := 0
	for {
		// Find disks with remaining space
		activeDisks := p.getActiveDisks(sortedDisks, remaining)
		if len(activeDisks) < minDisks {
			break // Cannot form RAID with fewer disks
		}
		// Find minimum remaining space among active disks
		minRemaining := p.findMinRemaining(activeDisks, remaining)
		if minRemaining <= 0 {
			break
		}
		// Determine RAID level for this layer
		raidLevel := p.determineRAIDLevel(len(activeDisks), parity)
		// Calculate layer capacity
		capacity := p.calculateLayerCapacity(minRemaining, len(activeDisks), raidLevel)
		// Create layer
		layer := models.SHRLayer{
			Index:       layerIndex,
			Size:        minRemaining,
			DiskDevices: activeDisks,
			RAIDLevel:   raidLevel,
			Capacity:    capacity,
		}
		layers = append(layers, layer)
		logger.Info("Layer %d: %d disks × %s = %s raw → %s (%s)",
			layerIndex,
			len(activeDisks),
			FormatBytes(minRemaining),
			FormatBytes(minRemaining*int64(len(activeDisks))),
			FormatBytes(capacity),
			layer.GetRAIDLevelName())
		// Deduct allocated space from remaining
		for _, dev := range activeDisks {
			remaining[dev] -= minRemaining
		}
		layerIndex++
	}
	// Calculate total capacity and wasted space
	var totalCapacity int64
	wastedDetails := make(map[string]int64)
	var wastedSpace int64
	for _, layer := range layers {
		totalCapacity += layer.Capacity
	}
	for dev, space := range remaining {
		if space > 0 {
			wastedDetails[dev] = space
			wastedSpace += space
		}
	}
	layout := &models.SHRLayout{
		Parity:        parity,
		Layers:        layers,
		TotalCapacity: totalCapacity,
		Disks:         sortedDisks,
		WastedSpace:   wastedSpace,
		WastedDetails: wastedDetails,
	}
	logger.Info("SHR-%d layout planned: %d layers, total usable: %s, wasted: %s",
		parity, len(layers), FormatBytes(totalCapacity), FormatBytes(wastedSpace))
	return layout, nil
}

// PlanExpansion plans how to expand an existing SHR pool with new disks
func (p *SHRPlanner) PlanExpansion(currentLayout *models.SHRLayout, newDisks []models.Disk) (*models.SHRLayout, error) {
	logger.Info("Planning SHR-%d expansion with %d new disks", currentLayout.Parity, len(newDisks))
	// Combine existing and new disks
	allDisks := make([]models.Disk, len(currentLayout.Disks))
	copy(allDisks, currentLayout.Disks)
	allDisks = append(allDisks, newDisks...)
	// Re-plan the entire layout
	newLayout, err := p.PlanLayout(allDisks, currentLayout.Parity)
	if err != nil {
		return nil, err
	}
	return newLayout, nil
}

// getActiveDisks returns disks that have remaining space
func (p *SHRPlanner) getActiveDisks(disks []models.Disk, remaining map[string]int64) []string {
	var active []string
	for _, d := range disks {
		if remaining[d.Device] > 0 {
			active = append(active, d.Device)
		}
	}
	return active
}

// findMinRemaining finds the minimum remaining space among given disks
func (p *SHRPlanner) findMinRemaining(devices []string, remaining map[string]int64) int64 {
	if len(devices) == 0 {
		return 0
	}
	min := remaining[devices[0]]
	for _, dev := range devices[1:] {
		if remaining[dev] < min {
			min = remaining[dev]
		}
	}
	return min
}

// minDisksNeeded returns minimum disks required for given parity level
func (p *SHRPlanner) minDisksNeeded(parity int) int {
	switch parity {
	case 1:
		return 2 // SHR-1: minimum 2 disks
	case 2:
		return 4 // SHR-2: minimum 4 disks
	default:
		return 2
	}
}

// determineRAIDLevel determines the RAID level for a layer
// SHR-1: 2 disks -> RAID1, 3+ disks -> RAID5
// SHR-2: 4+ disks -> RAID6
func (p *SHRPlanner) determineRAIDLevel(diskCount, parity int) int {
	if parity == 1 {
		// SHR-1
		if diskCount == 2 {
			return 1 // RAID1
		}
		return 5 // RAID5 for 3+ disks
	}
	// SHR-2
	return 6 // RAID6
}

// calculateLayerCapacity calculates usable capacity for a layer
func (p *SHRPlanner) calculateLayerCapacity(sizePerDisk int64, diskCount int, raidLevel int) int64 {
	switch raidLevel {
	case 1:
		// RAID1: mirrored, capacity = size of one disk
		return sizePerDisk
	case 5:
		// RAID5: one disk for parity
		return sizePerDisk * int64(diskCount-1)
	case 6:
		// RAID6: two disks for parity
		return sizePerDisk * int64(diskCount-2)
	default:
		return 0
	}
}

// CalculateSHRCapacity is a convenience function to calculate SHR capacity from sizes
func CalculateSHRCapacity(sizes []int64, parity int) int64 {
	if len(sizes) == 0 {
		return 0
	}
	// Create virtual disks from sizes with unique device names
	disks := make([]models.Disk, len(sizes))
	for i, size := range sizes {
		disks[i] = models.Disk{
			Device: fmt.Sprintf("disk%d", i),
			Size:   size,
		}
	}
	planner := NewSHRPlanner()
	layout, err := planner.PlanLayout(disks, parity)
	if err != nil {
		return 0
	}
	return layout.TotalCapacity
}
