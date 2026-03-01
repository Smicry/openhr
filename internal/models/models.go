package models

import "time"

// StorageMode represents the storage mode type
type StorageMode string

const (
	ModeBasic StorageMode = "basic"
	ModeJBOD  StorageMode = "jbod"
	ModeRAID1 StorageMode = "raid1"
	ModeRAID5 StorageMode = "raid5"
	ModeRAID6 StorageMode = "raid6"
	ModeSHR1  StorageMode = "shr1"
	ModeSHR2  StorageMode = "shr2"
)

// PoolStatus represents the pool status
type PoolStatus string

const (
	PoolStatusActive    PoolStatus = "active"
	PoolStatusDegraded  PoolStatus = "degraded"
	PoolStatusRepairing PoolStatus = "repairing"
	PoolStatusOffline   PoolStatus = "offline"
	PoolStatusUnknown   PoolStatus = "unknown"
)

// Disk represents disk information
type Disk struct {
	Device     string
	Model      string
	Serial     string
	Size       int64
	SizeHuman  string
	Vendor     string
	Ro         bool
	Trashed    bool
	MountPoint string
	Partitions []Partition
}

// Partition represents partition information
type Partition struct {
	Device     string
	Number     int
	Start      string
	End        string
	Size       int64
	Type       string
	Filesystem string
}

// Pool represents a storage pool
type Pool struct {
	ID        string
	Name      string
	Mode      StorageMode
	Status    PoolStatus
	TotalSize int64
	FreeSize  int64
	UsedSize  int64
	Disks     []Disk
	VGName    string
	RAIDDev   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Volume represents a logical volume
type Volume struct {
	Name      string
	PoolName  string
	Size      int64
	FS        string
	MountPath string
	Device    string
	Status    string
}

// RAIDInfo represents RAID array information
type RAIDInfo struct {
	Device        string
	Level         int
	State         string
	ActiveDevices int
	TotalDevices  int
	FailedDevices int
	Layout        string
	ChunkSize     string
}

// PoolCapacity represents pool capacity information
type PoolCapacity struct {
	TotalRawCapacity int64
	UsableCapacity   int64
	ParityCapacity   int64
	UsedCapacity     int64
	FreeCapacity     int64
	DiskCount        int
	MaxDiskSize      int64
	MinDiskSize      int64
	Redundancy       int
	ProtectionLevel  string
}

// PoolConfig represents pool creation configuration
type PoolConfig struct {
	Name     string
	Mode     StorageMode
	Disks    []string
	HotSpare []string
}

// GetModeDisplay returns the display name for the storage mode
func (m StorageMode) GetModeDisplay() string {
	switch m {
	case ModeBasic:
		return "Basic (Single Disk)"
	case ModeJBOD:
		return "JBOD (Just a Bunch Of Disks)"
	case ModeRAID1:
		return "RAID 1 (Mirroring)"
	case ModeRAID5:
		return "RAID 5 (Single Parity)"
	case ModeRAID6:
		return "RAID 6 (Dual Parity)"
	case ModeSHR1:
		return "SHR-1 (Single Disk Fault Tolerance)"
	case ModeSHR2:
		return "SHR-2 (Dual Disk Fault Tolerance)"
	default:
		return string(m)
	}
}

// IsSHR returns true if the mode is SHR
func (m StorageMode) IsSHR() bool {
	return m == ModeSHR1 || m == ModeSHR2
}

// VGInfo represents volume group information
type VGInfo struct {
	Name    string
	Status  string
	Size    int64
	Free    int64
	LVCount int
	PVCount int
	UUID    string
}

// PVInfo represents physical volume information
type PVInfo struct {
	Name    string
	VGName  string
	Size    int64
	Free    int64
	DevSize int64
	UUID    string
}

// LVInfo represents logical volume information
type LVInfo struct {
	Name   string
	VGName string
	Size   int64
	Pool   string
	Layout string
	Role   string
	Device string
}
