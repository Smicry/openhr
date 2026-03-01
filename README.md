# OpenHR - Open Hybrid RAID Storage Management Tool

OpenHR (Open Hybrid RAID) is a SHR-like hybrid RAID storage management tool written in Go for Linux systems.

## Features

- **SHR-1**: Single Disk Fault Tolerance Hybrid RAID (Multi-layer Architecture)
- **SHR-2**: Dual Disk Fault Tolerance Hybrid RAID (Multi-layer Architecture)
- **Traditional RAID**: RAID 1/5/6, Basic, JBOD support
- **Capacity Calculation**: Smart estimation for different configurations
- **Pool Management**: Create, delete, expand storage pools
- **Volume Management**: Create and resize logical volumes

## Requirements

- Linux operating system
- Root privileges
- Required tools:
  - mdadm (Software RAID)
  - lvm2 (Logical Volume Manager)
  - parted (Partition Manager)

## Installation

```bash
# Clone the project
git clone https://github.com/your-repo/openhr.git
cd openhr

# Build
go build -o openhr .

# Install
sudo cp openhr /usr/local/bin/
```

## Usage

### Capacity Estimation

Before creating a pool, you can estimate the capacity:

```bash
# SHR-1 mode estimation
openhr capacity estimate --disks 4TB --disks 8TB --disks 16TB --mode shr1

# SHR-2 mode estimation
openhr capacity estimate --disks 4TB --disks 8TB --disks 16TB --disks 16TB --mode shr2

# RAID5 estimation
openhr capacity estimate --disks 4TB --disks 8TB --disks 16TB --mode raid5
```

### Pool Management

```bash
# Create a pool (use repeated --disks flags)
sudo openhr pool create --name mypool --mode shr1 --disks /dev/sda --disks /dev/sdb --disks /dev/sdc

# List pools
sudo openhr pool list

# Show pool details
sudo openhr pool info mypool

# Expand a pool
sudo openhr pool expand mypool --disks /dev/sdd --disks /dev/sde

# Delete a pool
sudo openhr pool delete mypool
```

### Disk Management

```bash
# List disks
openhr disk list

# Show disk details
openhr disk info /dev/sda
```

### Volume Management

```bash
# Create a volume
sudo openhr volume create --pool mypool --name myvolume --size 100G --fs ext4

# List volumes
openhr volume list
```

## Storage Modes

| Mode | Description | Min Disks | Fault Tolerance |
|------|-------------|-----------|-----------------|
| basic | Single Disk | 1 | None |
| jbod | Just a Bunch Of Disks | 1 | None |
| raid1 | Mirroring | 2 | n-1 |
| raid5 | Single Parity | 3 | 1 |
| raid6 | Dual Parity | 4 | 2 |
| shr1 | Hybrid RAID-1 | 2 | 1 |
| shr2 | Hybrid RAID-2 | 4 | 2 |

## SHR Implementation Details

OpenHR implements a **multi-layer architecture** similar to Synology's SHR, which maximizes storage efficiency when using mixed-size disks.

### Multi-Layer Algorithm

1. **Sort disks by size** (ascending order)
2. **Iteratively create layers**:
   - Find all disks with remaining space
   - Use the minimum remaining space as the layer size
   - Determine RAID level based on disk count and parity
   - Create a RAID array for this layer
   - Deduct allocated space from each participating disk
3. **Combine all layers** using LVM to form a single storage pool

### RAID Level Decision

| SHR Type | Disk Count | RAID Level | Capacity Formula |
|----------|------------|------------|------------------|
| SHR-1 | 2 disks | RAID 1 | `size × 1` |
| SHR-1 | 3+ disks | RAID 5 | `size × (n-1)` |
| SHR-2 | 4+ disks | RAID 6 | `size × (n-2)` |

### SHR-1 Example: 4TB + 8TB + 16TB disks

```
┌──────────────────────────────────────────────────────────────────┐
│ Layer 0: 3 disks × 4TB = 12TB raw                                │
│   Disk1 (4TB)  [====4TB====]                                     │
│   Disk2 (8TB)  [====4TB====]      → RAID5 (8TB usable)           │
│   Disk3 (16TB) [====4TB====]                                     │
│   Remaining: D1=0TB, D2=4TB, D3=12TB                             │
├──────────────────────────────────────────────────────────────────┤
│ Layer 1: 2 disks × 4TB = 8TB raw                                 │
│   Disk2 (8TB)           [====4TB====]                            │
│   Disk3 (16TB)          [====4TB====] → RAID1 (4TB usable)       │
│   Remaining: D2=0TB, D3=8TB                                      │
├──────────────────────────────────────────────────────────────────┤
│ Layer 2: 1 disk × 8TB (skipped - insufficient disks)             │
│   Disk3 (16TB)                   [========8TB========]           │
└──────────────────────────────────────────────────────────────────┘

Total Usable: 8TB + 4TB = 12TB
Wasted: 8TB (single disk leftover)
```

### SHR-2 Example: 4TB + 8TB + 16TB + 16TB disks

```
┌──────────────────────────────────────────────────────────────────┐
│ Layer 0: 4 disks × 4TB = 16TB raw                                │
│   Disk1 (4TB)  [====4TB====]                                     │
│   Disk2 (8TB)  [====4TB====]      → RAID6 (8TB usable)           │
│   Disk3 (16TB) [====4TB====]                                     │
│   Disk4 (16TB) [====4TB====]                                     │
│   Remaining: D1=0TB, D2=4TB, D3=12TB, D4=12TB                    │
├──────────────────────────────────────────────────────────────────┤
│ Layer 1: 3 disks × 4TB = 12TB raw                                │
│   Disk2 (8TB)           [====4TB====]                            │
│   Disk3 (16TB)          [====4TB====] → RAID5 (8TB usable)       │
│   Disk4 (16TB)          [====4TB====]                            │
│   Remaining: D2=0TB, D3=8TB, D4=8TB                              │
├──────────────────────────────────────────────────────────────────┤
│ Layer 2: 2 disks × 8TB = 16TB raw                                │
│   Disk3 (16TB)          [========8TB========]                    │
│   Disk4 (16TB)          [========8TB========] → RAID1 (8TB usable)│
└──────────────────────────────────────────────────────────────────┘

Total Usable: 8TB + 8TB + 8TB = 24TB
Wasted: 0TB
```

### Pool Expansion

SHR pools support dynamic expansion:

```bash
# Add new disks to expand an existing SHR pool
sudo openhr pool expand mypool --disks /dev/sdd --disks /dev/sde
```

When expanding:
1. New disks are added to existing layers if possible
2. New layers may be created if beneficial
3. All layers are combined through LVM for seamless capacity increase

### Traditional RAID Comparison

| Mode | Capacity Formula | Efficiency | Min Disks |
|------|-----------------|-----------|-----------|
| RAID 1 | Min disk size | 50% | 2 |
| RAID 5 | (n-1) x Min disk | ~67-93% | 3 |
| RAID 6 | (n-2) x Min disk | ~50-88% | 4 |
| SHR-1 | Multi-layer calculation | ~43-90% | 2 |
| SHR-2 | Multi-layer calculation | ~27-75% | 4 |

**Key Advantage of SHR:**
- Unlike traditional RAID, SHR allows mixing different size disks without wasting the extra space on larger drives
- More flexible expansion options
- Maximum storage efficiency for mixed-size disk configurations

## Notes

1. All pool operations require root privileges
2. Creating a pool will format the disks, please backup data first
3. It is recommended to test thoroughly before production use
4. SHR requires at least 2 disks for SHR-1, 4 disks for SHR-2

## License

MIT License - see [LICENSE](LICENSE) file for details.
