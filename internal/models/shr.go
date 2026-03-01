package models

// SHRLayer represents a single storage layer in SHR
// Each layer uses RAID to provide redundancy
type SHRLayer struct {
	// Index is the layer number (0, 1, 2...)
	Index int `json:"index"`
	// Size is the contribution size per disk in this layer (bytes)
	Size int64 `json:"size"`
	// DiskDevices is the list of disk devices participating in this layer
	DiskDevices []string `json:"disk_devices"`
	// RAIDLevel is the RAID level for this layer (1, 5, or 6)
	RAIDLevel int `json:"raid_level"`
	// Capacity is the usable capacity of this layer (bytes)
	Capacity int64 `json:"capacity"`
	// Partitions is the list of partition devices created for this layer
	// e.g., ["/dev/sda1", "/dev/sdb1", "/dev/sdc1"]
	Partitions []string `json:"partitions"`
	// RAIDDevice is the RAID array device path for this layer
	// e.g., "/dev/md/openhr_poolname_layer0"
	RAIDDevice string `json:"raid_device"`
}

// SHRLayout represents the complete SHR layout plan
type SHRLayout struct {
	// Parity is 1 for SHR-1, 2 for SHR-2
	Parity int `json:"parity"`
	// Layers is the list of all storage layers
	Layers []SHRLayer `json:"layers"`
	// TotalCapacity is the sum of all layer capacities (bytes)
	TotalCapacity int64 `json:"total_capacity"`
	// Disks is the original disk information
	Disks []Disk `json:"disks"`
	// WastedSpace is the space that cannot be used for RAID (bytes)
	WastedSpace int64 `json:"wasted_space"`
	// WastedDetails shows which disks have wasted space
	WastedDetails map[string]int64 `json:"wasted_details"`
}

// SHRExpansionPlan represents the plan for expanding an SHR pool
type SHRExpansionPlan struct {
	// ExistingLayers are the layers that will be modified
	ExistingLayers []SHRLayerExpansion `json:"existing_layers"`
	// NewLayers are the new layers to be created
	NewLayers []SHRLayer `json:"new_layers"`
	// AddedCapacity is the additional capacity from expansion
	AddedCapacity int64 `json:"added_capacity"`
}

// SHRLayerExpansion represents how an existing layer will be expanded
type SHRLayerExpansion struct {
	// LayerIndex is the index of the layer to expand
	LayerIndex int `json:"layer_index"`
	// NewDisks are the new disks to add to this layer
	NewDisks []string `json:"new_disks"`
	// NewPartitions are the new partitions to create
	NewPartitions []string `json:"new_partitions"`
	// AddedCapacity is the additional capacity from expanding this layer
	AddedCapacity int64 `json:"added_capacity"`
}

// GetRAIDLevelName returns the human-readable RAID level name
func (l *SHRLayer) GetRAIDLevelName() string {
	switch l.RAIDLevel {
	case 1:
		return "RAID1"
	case 5:
		return "RAID5"
	case 6:
		return "RAID6"
	default:
		return "Unknown"
	}
}

// GetModeName returns the SHR mode name (SHR-1 or SHR-2)
func (l *SHRLayout) GetModeName() string {
	if l.Parity == 1 {
		return "SHR-1"
	}
	return "SHR-2"
}

// GetTotalRawCapacity returns the sum of all disk sizes
func (l *SHRLayout) GetTotalRawCapacity() int64 {
	var total int64
	for _, d := range l.Disks {
		total += d.Size
	}
	return total
}

// GetEfficiency returns the storage efficiency as a percentage
func (l *SHRLayout) GetEfficiency() float64 {
	raw := l.GetTotalRawCapacity()
	if raw == 0 {
		return 0
	}
	return float64(l.TotalCapacity) / float64(raw) * 100
}
