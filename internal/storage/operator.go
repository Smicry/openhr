package storage

import (
	"github.com/openhr/internal/models"
)

// Operator - Storage operator interface
type Operator interface {
	// Partition operations
	CreatePartition(dev string, partType string, start, end string) error
	DeletePartition(dev string, num int) error
	ListPartitions(dev string) ([]models.Partition, error)
	SetPartitionType(dev string, num int, partType string) error

	// PV operations
	CreatePV(dev string) error
	RemovePV(dev string) error

	// VG operations
	CreateVG(name string, pvs []string) error
	RemoveVG(name string) error
	ExtendVG(name string, pvs []string) error
	ReduceVG(name string, pvs []string) error

	// LV operations
	CreateLV(vg, name string, size int64, fs string) error
	RemoveLV(vg, name string) error
	ExtendLV(vg, name string, size int64) error
	ReduceLV(vg, name string, size int64) error

	// RAID operations
	CreateRAID(level int, devices []string, name string, bitmap bool) (string, error)
	RemoveRAID(dev string) error
	QueryRAID(dev string) (*models.RAIDInfo, error)
	AddDeviceRAID(dev string, newDev string) error
	RemoveDeviceRAID(dev string, rmDev string) error
	SetDeviceFaultRAID(dev string, devToFail string) error

	// Query operations
	ListVGs() ([]models.VGInfo, error)
	ListPVs() ([]models.PVInfo, error)
	ListLVs() ([]models.LVInfo, error)
	ListMDDevices() ([]models.RAIDInfo, error)

	// Device operations
	GetDiskInfo(dev string) (*models.Disk, error)
	ListDisks() ([]models.Disk, error)
	IsDiskExists(dev string) bool
	IsMounted(dev string) bool

	// Filesystem operations
	Format(dev string, fs string) error
	Mount(dev string, mountPoint string) error
	Unmount(mountPoint string) error
}
