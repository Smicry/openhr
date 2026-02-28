package storage

import (
	"github.com/openhr/internal/models"
)

// Operator 存储操作接口
type Operator interface {
	// 分区操作
	CreatePartition(dev string, partType string, start, end string) error
	DeletePartition(dev string, num int) error
	ListPartitions(dev string) ([]models.Partition, error)
	SetPartitionType(dev string, num int, partType string) error

	// PV操作
	CreatePV(dev string) error
	RemovePV(dev string) error

	// VG操作
	CreateVG(name string, pvs []string) error
	RemoveVG(name string) error
	ExtendVG(name string, pvs []string) error
	ReduceVG(name string, pvs []string) error

	// LV操作
	CreateLV(vg, name string, size int64, fs string) error
	RemoveLV(vg, name string) error
	ExtendLV(vg, name string, size int64) error
	ReduceLV(vg, name string, size int64) error

	// RAID操作
	CreateRAID(level int, devices []string, name string, bitmap bool) (string, error)
	RemoveRAID(dev string) error
	QueryRAID(dev string) (*models.RAIDInfo, error)
	AddDeviceRAID(dev string, newDev string) error
	RemoveDeviceRAID(dev string, rmDev string) error
	SetDeviceFaultRAID(dev string, devToFail string) error

	// 查询操作
	ListVGs() ([]models.VGInfo, error)
	ListPVs() ([]models.PVInfo, error)
	ListLVs() ([]models.LVInfo, error)
	ListMDDevices() ([]models.RAIDInfo, error)

	// 设备操作
	GetDiskInfo(dev string) (*models.Disk, error)
	ListDisks() ([]models.Disk, error)
	IsDiskExists(dev string) bool
	IsMounted(dev string) bool

	// 文件系统操作
	Format(dev string, fs string) error
	Mount(dev string, mountPoint string) error
	Unmount(mountPoint string) error
}
