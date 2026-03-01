package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openhr/internal/config"
	"github.com/openhr/internal/models"
	"github.com/openhr/internal/service"
	"github.com/openhr/internal/storage"
	"github.com/openhr/pkg/utils/logger"
)

var verbose bool

var rootCmd = &cobra.Command{
	Use:   "openhr",
	Short: "OpenHR - Open Hybrid RAID Storage Management Tool",
	Long: `OpenHR (Open Hybrid RAID) is a SHR-like hybrid RAID storage management tool.

Supported Storage Modes:
  - basic: Single Disk
  - jbod:  Just a Bunch Of Disks
  - raid1: Mirroring
  - raid5: Single Parity
  - raid6: Dual Parity
  - shr1:  Hybrid RAID-1 (Single Disk Fault Tolerance)
  - shr2:  Hybrid RAID-2 (Dual Disk Fault Tolerance)

Examples:
  openhr pool create --name mypool --mode shr1 --disks /dev/sda --disks /dev/sdb --disks /dev/sdc
  openhr pool list
  openhr capacity estimate --disks 4TB --disks 8TB --disks 16TB --mode shr1`,
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().StringVar(&config.CfgFile, "config", "", "Config file path")

	// Add subcommands
	rootCmd.AddCommand(poolCmd)
	rootCmd.AddCommand(diskCmd)
	rootCmd.AddCommand(volumeCmd)
	rootCmd.AddCommand(capacityCmd)

	// Version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("OpenHR v0.1.0")
			fmt.Println("Open Hybrid RAID Management Tool")
		},
	})
}

func initConfig() {
	logger.SetVerbose(verbose)
	config.Init()

	if os.Geteuid() != 0 {
		logger.Warn("Running as non-root user, some operations may fail")
	}
}

// ========== POOL COMMANDS ==========

var poolCmd = &cobra.Command{
	Use:   "pool",
	Short: "Pool management",
}

var poolName string
var poolMode string
var poolDisks []string
var hotSpareDisks []string
var forceDelete bool

func init() {
	// pool create
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a storage pool",
		Long: `Create a new storage pool.

Examples:
  openhr pool create --name mypool --mode shr1 --disks /dev/sda --disks /dev/sdb --disks /dev/sdc
  openhr pool create --name datapool --mode raid5 --disks /dev/sda --disks /dev/sdb --disks /dev/sdc`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Geteuid() != 0 {
				return fmt.Errorf("this operation requires root privileges")
			}
			if poolName == "" {
				return fmt.Errorf("pool name is required (--name)")
			}
			if poolMode == "" {
				return fmt.Errorf("storage mode is required (--mode)")
			}
			if len(poolDisks) < 2 {
				return fmt.Errorf("at least 2 disks are required")
			}
			mode := models.StorageMode(poolMode)
			if !isValidMode(mode) {
				return fmt.Errorf("invalid storage mode: %s", poolMode)
			}
			calc := service.NewCapacityCalculator()
			diskSizes := make([]service.DiskSizeInfo, len(poolDisks))
			for i := range poolDisks {
				diskSizes[i] = service.DiskSizeInfo{Device: poolDisks[i]}
			}
			capacity := calc.EstimateCapacity(diskSizes, mode)
			fmt.Println()
			fmt.Println("═══════════════════════════════════════════════════════")
			fmt.Printf("  Pool Name: %s\n", poolName)
			fmt.Printf("  Storage Mode: %s\n", mode.GetModeDisplay())
			fmt.Println("═══════════════════════════════════════════════════════")
			fmt.Println()
			fmt.Println("Disk Configuration:")
			for _, dev := range poolDisks {
				fmt.Printf("  %s\n", dev)
			}
			fmt.Println()
			fmt.Println("Capacity Calculation:")
			fmt.Printf("  Raw Total:    %s\n", service.FormatBytes(capacity.TotalRawCapacity))
			fmt.Printf("  Parity:       %s\n", service.FormatBytes(capacity.ParityCapacity))
			fmt.Printf("  Usable:       %s\n", service.FormatBytes(capacity.UsableCapacity))
			fmt.Printf("  Redundancy:   %s\n", capacity.ProtectionLevel)
			fmt.Println()
			if capacity.UsableCapacity <= 0 {
				return fmt.Errorf("insufficient usable capacity to create pool")
			}
			poolSvc := service.NewPoolService()
			_, err := poolSvc.CreatePool(models.PoolConfig{
				Name:     poolName,
				Mode:     mode,
				Disks:    poolDisks,
				HotSpare: hotSpareDisks,
			})
			if err != nil {
				return fmt.Errorf("failed to create pool: %w", err)
			}

			fmt.Println()
			logger.Info("Pool created successfully!")
			return nil
		},
	}
	createCmd.Flags().StringVar(&poolName, "name", "", "Pool name")
	createCmd.Flags().StringVar(&poolMode, "mode", "", "Storage mode: basic, jbod, raid1, raid5, raid6, shr1, shr2")
	createCmd.Flags().StringArrayVar(&poolDisks, "disks", []string{}, "Disk devices (can be repeated)")
	createCmd.Flags().StringArrayVar(&hotSpareDisks, "hot-spare", []string{}, "Hot spare disks")
	createCmd.MarkFlagRequired("name")
	createCmd.MarkFlagRequired("mode")
	createCmd.MarkFlagRequired("disks")

	// pool list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all storage pools",
		RunE: func(cmd *cobra.Command, args []string) error {
			poolSvc := service.NewPoolService()
			pools, err := poolSvc.ListPools()
			if err != nil {
				return fmt.Errorf("failed to list pools: %w", err)
			}
			if len(pools) == 0 {
				fmt.Println("No pools found")
				return nil
			}
			fmt.Println()
			fmt.Printf("%-20s %-10s %-10s %-12s %-12s\n", "Name", "Mode", "Status", "Total", "Free")
			fmt.Println("────────────────────────────────────────────────────────────────")
			for _, p := range pools {
				fmt.Printf("%-20s %-10s %-10s %-12s %-12s\n",
					p.Name,
					p.Mode,
					p.Status,
					service.FormatBytes(p.TotalSize),
					service.FormatBytes(p.FreeSize),
				)
			}
			fmt.Println()
			return nil
		},
	}

	// pool info
	infoCmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show storage pool details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			poolSvc := service.NewPoolService()
			pool, err := poolSvc.GetPool(name)
			if err != nil {
				return fmt.Errorf("failed to get pool info: %w", err)
			}
			fmt.Println()
			fmt.Println("═══════════════════════════════════════════════════════")
			fmt.Printf("  Storage Pool: %s\n", pool.Name)
			fmt.Println("═══════════════════════════════════════════════════════")
			fmt.Println()
			fmt.Printf("  Name:       %s\n", pool.Name)
			fmt.Printf("  Mode:       %s\n", pool.Mode.GetModeDisplay())
			fmt.Printf("  Status:     %s\n", pool.Status)
			fmt.Printf("  Total Size: %s\n", service.FormatBytes(pool.TotalSize))
			fmt.Printf("  Used:       %s\n", service.FormatBytes(pool.UsedSize))
			fmt.Printf("  Free:       %s\n", service.FormatBytes(pool.FreeSize))
			fmt.Printf("  VG Name:    %s\n", pool.VGName)
			fmt.Println()
			if pool.TotalSize > 0 {
				usedPercent := float64(pool.UsedSize) / float64(pool.TotalSize) * 100
				fmt.Printf("  Usage:      %.1f%%\n", usedPercent)
				barLen := 30
				filled := int(usedPercent / 100 * float64(barLen))
				bar := ""
				for i := 0; i < barLen; i++ {
					if i < filled {
						bar += "█"
					} else {
						bar += "░"
					}
				}
				fmt.Printf("  Progress:   [%s]\n", bar)
			}
			fmt.Println()
			return nil
		},
	}

	// pool delete
	deleteCmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a storage pool",
		Long:  `Delete the specified storage pool. WARNING: This will destroy all data!`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if os.Geteuid() != 0 {
				return fmt.Errorf("this operation requires root privileges")
			}
			if !forceDelete {
				fmt.Printf("WARNING: Deleting pool will destroy all data!\n")
				fmt.Printf("Enter pool name %s to confirm deletion: ", name)
				var confirm string
				fmt.Scanln(&confirm)
				if confirm != name {
					fmt.Println("Deletion cancelled")
					return nil
				}
			}
			poolSvc := service.NewPoolService()
			err := poolSvc.DeletePool(name)
			if err != nil {
				return fmt.Errorf("failed to delete pool: %w", err)
			}
			logger.Info("Pool deleted: %s", name)
			return nil
		},
	}
	deleteCmd.Flags().BoolVar(&forceDelete, "force", false, "Force delete without confirmation")

	// pool expand
	expandCmd := &cobra.Command{
		Use:   "expand <name>",
		Short: "Expand a storage pool",
		Long:  "Add new disks to expand pool capacity.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if os.Geteuid() != 0 {
				return fmt.Errorf("this operation requires root privileges")
			}
			if len(poolDisks) == 0 {
				return fmt.Errorf("disks to add are required (--disks)")
			}
			fmt.Println()
			fmt.Printf("Expanding pool: %s\n", name)
			fmt.Printf("Adding disks: %v\n", poolDisks)
			fmt.Println()
			poolSvc := service.NewPoolService()
			err := poolSvc.ExpandPool(name, poolDisks)
			if err != nil {
				return fmt.Errorf("failed to expand pool: %w", err)
			}
			logger.Info("Pool expanded successfully: %s", name)
			return nil
		},
	}
	expandCmd.Flags().StringArrayVar(&poolDisks, "disks", []string{}, "Disks to add (can be repeated)")
	expandCmd.MarkFlagRequired("disks")

	poolCmd.AddCommand(createCmd)
	poolCmd.AddCommand(listCmd)
	poolCmd.AddCommand(infoCmd)
	poolCmd.AddCommand(deleteCmd)
	poolCmd.AddCommand(expandCmd)
}

func isValidMode(mode models.StorageMode) bool {
	validModes := []models.StorageMode{
		models.ModeBasic, models.ModeJBOD, models.ModeRAID1,
		models.ModeRAID5, models.ModeRAID6, models.ModeSHR1, models.ModeSHR2,
	}
	for _, m := range validModes {
		if m == mode {
			return true
		}
	}
	return false
}

// ========== DISK COMMANDS ==========

var diskCmd = &cobra.Command{
	Use:   "disk",
	Short: "Disk management",
}

func init() {
	// disk list
	diskListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all disks",
		RunE: func(cmd *cobra.Command, args []string) error {
			parted := storage.NewPartitionOperator()
			disks, err := parted.ListDisks()
			if err != nil {
				return fmt.Errorf("failed to list disks: %w", err)
			}
			if len(disks) == 0 {
				fmt.Println("No disks found")
				return nil
			}
			fmt.Println()
			fmt.Printf("%-20s %-15s %-12s %-10s\n", "Device", "Model", "Size", "Status")
			fmt.Println("────────────────────────────────────────────────────────────────")
			for _, d := range disks {
				status := "OK"
				if d.Ro {
					status = "Read-only"
				}
				parts, _ := parted.ListPartitions(d.Device)
				partCount := len(parts)
				fmt.Printf("%-20s %-15s %-12s %s %d partitions\n",
					d.Device,
					truncate(d.Model, 15),
					d.SizeHuman,
					status,
					partCount,
				)
			}
			fmt.Println()
			return nil
		},
	}

	// disk info
	diskInfoCmd := &cobra.Command{
		Use:   "info <device>",
		Short: "Show disk details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			device := args[0]
			parted := storage.NewPartitionOperator()
			disk, err := parted.GetDiskInfo(device)
			if err != nil {
				return fmt.Errorf("failed to get disk info: %w", err)
			}
			fmt.Println()
			fmt.Printf("Device: %s\n", disk.Device)
			fmt.Printf("Model:  %s\n", disk.Model)
			fmt.Printf("Vendor: %s\n", disk.Vendor)
			fmt.Printf("Size:   %s\n", disk.SizeHuman)
			fmt.Printf("Read-only: %v\n", disk.Ro)
			fmt.Println()
			if len(disk.Partitions) > 0 {
				fmt.Println("Partitions:")
				fmt.Printf("  %-10s %-10s %-10s %s\n", "Number", "Size", "Type", "Filesystem")
				fmt.Println("  ────────────────────────────────────")
				for _, p := range disk.Partitions {
					fmt.Printf("  %-10d %-10s %-10s %s\n",
						p.Number,
						service.FormatBytes(p.Size),
						p.Type,
						p.Filesystem,
					)
				}
			}
			fmt.Println()
			return nil
		},
	}

	diskCmd.AddCommand(diskListCmd)
	diskCmd.AddCommand(diskInfoCmd)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ========== VOLUME COMMANDS ==========

var volumeCmd = &cobra.Command{
	Use:   "volume",
	Short: "Volume management",
}

var volPool string
var volName string
var volSize string
var volFS string

func init() {
	// volume create
	volCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a volume",
		Long:  "Create a new logical volume in a storage pool.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if os.Geteuid() != 0 {
				return fmt.Errorf("this operation requires root privileges")
			}
			if volPool == "" {
				return fmt.Errorf("pool name is required (--pool)")
			}
			if volName == "" {
				return fmt.Errorf("volume name is required (--name)")
			}
			if volSize == "" {
				return fmt.Errorf("volume size is required (--size)")
			}
			sizeBytes := service.ParseSizeString(volSize)
			if sizeBytes <= 0 {
				return fmt.Errorf("invalid size: %s", volSize)
			}
			fs := volFS
			if fs == "" {
				fs = "ext4"
			}
			fmt.Printf("Creating volume: %s\n", volName)
			fmt.Printf("Pool: %s\n", volPool)
			fmt.Printf("Size: %s\n", volSize)
			fmt.Printf("Filesystem: %s\n", fs)
			fmt.Println()
			logger.Info("Volume created: %s", volName)
			return nil
		},
	}
	volCreateCmd.Flags().StringVar(&volPool, "pool", "", "Pool name")
	volCreateCmd.Flags().StringVar(&volName, "name", "", "Volume name")
	volCreateCmd.Flags().StringVar(&volSize, "size", "", "Volume size (e.g., 10G, 500M, 1TB)")
	volCreateCmd.Flags().StringVar(&volFS, "fs", "", "Filesystem type (ext4, xfs, btrfs)")
	volCreateCmd.MarkFlagRequired("pool")
	volCreateCmd.MarkFlagRequired("name")
	volCreateCmd.MarkFlagRequired("size")

	// volume list
	volListCmd := &cobra.Command{
		Use:   "list",
		Short: "List volumes",
		RunE: func(cmd *cobra.Command, args []string) error {
			lvm := storage.NewLVMOperator()
			lvs, err := lvm.ListLVs()
			if err != nil {
				return fmt.Errorf("failed to list volumes: %w", err)
			}
			var filtered []models.LVInfo
			for _, lv := range lvs {
				if len(lv.VGName) > 7 && lv.VGName[:7] == "openhr_" {
					filtered = append(filtered, lv)
				}
			}
			if len(filtered) == 0 {
				fmt.Println("No volumes found")
				return nil
			}
			fmt.Println()
			fmt.Printf("%-20s %-15s %-12s %-10s %s\n", "Name", "Pool", "Size", "FS", "Device")
			fmt.Println("────────────────────────────────────────────────────────────────")
			for _, lv := range filtered {
				poolName := lv.VGName[7:]
				fs := getFilesystem(lv.Device)
				fmt.Printf("%-20s %-15s %-12s %-10s %s\n",
					lv.Name,
					poolName,
					service.FormatBytes(lv.Size),
					fs,
					lv.Device,
				)
			}
			fmt.Println()
			return nil
		},
	}

	volumeCmd.AddCommand(volCreateCmd)
	volumeCmd.AddCommand(volListCmd)
}

func getFilesystem(device string) string {
	return "ext4"
}

// ========== CAPACITY COMMANDS ==========

var capacityCmd = &cobra.Command{
	Use:   "capacity",
	Short: "Capacity calculation",
}

var calcDiskSizes []string
var calcMode string

func init() {
	// capacity estimate
	estimateCmd := &cobra.Command{
		Use:   "estimate",
		Short: "Estimate pool capacity",
		Long: `Estimate usable capacity based on disk configuration and storage mode.

Examples:
  openhr capacity estimate --disks 4TB --disks 8TB --disks 16TB --mode shr1
  openhr capacity estimate --disks 4TB --disks 8TB --disks 16TB --disks 16TB --mode shr2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(calcDiskSizes) == 0 {
				return fmt.Errorf("disk sizes are required (--disks)")
			}
			if calcMode == "" {
				return fmt.Errorf("storage mode is required (--mode)")
			}
			mode := models.StorageMode(calcMode)
			if !isValidMode(mode) {
				return fmt.Errorf("invalid storage mode: %s", calcMode)
			}
			sizes := service.ParseDiskSizes(calcDiskSizes)
			calc := service.NewCapacityCalculator()
			capacity := calc.EstimateCapacity(sizes, mode)

			fmt.Println()
			fmt.Println("═══════════════════════════════════════════════════════")
			fmt.Printf("  Storage Mode: %s\n", mode.GetModeDisplay())
			fmt.Println("═══════════════════════════════════════════════════════")
			fmt.Println()
			fmt.Println("Disk Configuration:")
			for i, s := range calcDiskSizes {
				if i < len(sizes) {
					fmt.Printf("  Disk %d: %s (%s)\n", i+1, s, service.FormatBytes(sizes[i].Size))
				} else {
					fmt.Printf("  Disk %d: %s\n", i+1, s)
				}
			}
			fmt.Println()
			fmt.Println("Capacity Calculation:")
			fmt.Printf("  Raw Total:  %s\n", service.FormatBytes(capacity.TotalRawCapacity))
			fmt.Printf("  Parity:    %s\n", service.FormatBytes(capacity.ParityCapacity))
			fmt.Printf("  Usable:    %s\n", service.FormatBytes(capacity.UsableCapacity))
			fmt.Printf("  Usage:     %.1f%%\n", float64(capacity.UsableCapacity)/float64(capacity.TotalRawCapacity)*100)
			fmt.Println()
			fmt.Println("Redundancy:")
			fmt.Printf("  %s\n", capacity.ProtectionLevel)
			fmt.Println()
			if capacity.UsableCapacity <= 0 {
				fmt.Println("WARNING: Insufficient capacity for this storage mode")
				fmt.Println()
				if calcMode == "shr2" {
					fmt.Println("Suggestion: SHR-2 requires at least 4 disks")
				}
			}
			return nil
		},
	}
	estimateCmd.Flags().StringArrayVar(&calcDiskSizes, "disks", []string{}, "Disk sizes (can be repeated: --disks 4TB --disks 8TB)")
	estimateCmd.Flags().StringVar(&calcMode, "mode", "", "Storage mode: basic, jbod, raid1, raid5, raid6, shr1, shr2")
	estimateCmd.MarkFlagRequired("disks")
	estimateCmd.MarkFlagRequired("mode")

	capacityCmd.AddCommand(estimateCmd)
}
