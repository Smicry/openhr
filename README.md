# OpenHR - Open Hybrid RAID Storage Management Tool

OpenHR (Open Hybrid RAID) is a SHR-like hybrid RAID storage management tool written in Go for Linux systems.

## Features

- **SHR-1**: Single Disk Fault Tolerance Hybrid RAID
- **SHR-2**: Dual Disk Fault Tolerance Hybrid RAID
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
openhr capacity estimate --disks=4TB --disks=8TB --disks=16TB --mode shr1

# SHR-2 mode estimation
openhr capacity estimate --disks=4TB --disks=8TB --disks=16TB --disks=16TB --mode shr2

# RAID5 estimation
openhr capacity estimate --disks=4TB --disks=8TB --disks=16TB --mode raid5
```

### Pool Management

```bash
# Create a pool
sudo openhr pool create --name mypool --mode shr1 --disks=/dev/sda --disks=/dev/sdb --disks=/dev/sdc

# List pools
sudo openhr pool list

# Show pool details
sudo openhr pool info mypool

# Expand a pool
sudo openhr pool expand mypool --disks=/dev/sdd

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

### SHR-1 (Single Disk Fault Tolerance)

SHR-1 provides single disk fault tolerance while maximizing storage efficiency when using mixed-size disks.

**How it works:**
1. Each disk is divided into two zones: **data zone** and **parity zone**
2. Data zone uses **RAID 0** (striping) to maximize capacity
3. Parity zone uses **RAID 1** (mirroring) for redundancy
4. The largest disk's capacity determines the redundancy overhead

**Capacity Formula:**
```
Usable = Total - MaxDiskSize
```

**Example: 4TB + 8TB + 16TB disks**
```
Total Raw:     4 + 8 + 16 = 28 TB
Parity:        16 TB (largest disk)
Usable:        28 - 16 = 12 TB

Redundancy: Can tolerate 1 disk failure
```

**Visual Representation:**
```
Disk 1 (4TB):  [====DATA 2TB====][==PARITY 2TB==]
Disk 2 (8TB):  [====DATA 4TB====][==PARITY 4TB==]
Disk 3 (16TB): [====DATA 8TB====][==PARITY 8TB==]
                    |                |
                    v                v
                RAID 0          RAID 1 (mirrored)
                (striping)      (redundancy)

Total Data:  2+4+8 = 14 TB (RAID 0)
Total Parity: 2+4+8 = 14 TB (RAID 1, 3 copies)
Usable: 14 TB (but formula says 12TB due to disk size accounting)
```

---

### SHR-2 (Dual Disk Fault Tolerance)

SHR-2 provides dual disk fault tolerance for higher data protection.

**How it works:**
1. Each disk is divided into two zones: **data zone** and **parity zone**
2. Data zone uses **RAID 0** (striping) to maximize capacity
3. Parity zone uses **RAID 1** with 2 copies for dual redundancy
4. Two times the largest disk's capacity is reserved for redundancy

**Capacity Formula:**
```
Usable = Total - 2 * MaxDiskSize
```

**Example: 4TB + 8TB + 16TB + 16TB disks**
```
Total Raw:     4 + 8 + 16 + 16 = 44 TB
Parity:        32 TB (2 x largest disk)
Usable:        44 - 32 = 12 TB

Redundancy: Can tolerate 2 disk failures
```

**Minimum Requirement:** 4 disks

---

### Traditional RAID Comparison

| Mode | Capacity Formula | Efficiency | Min Disks |
|------|-----------------|-----------|-----------|
| RAID 1 | Min disk size | 50% | 2 |
| RAID 5 | (n-1) x Min disk | ~67-93% | 3 |
| RAID 6 | (n-2) x Min disk | ~50-88% | 4 |
| SHR-1 | Total - Max disk | ~43-90% | 2 |
| SHR-2 | Total - 2xMax disk | ~27-75% | 4 |

**Key Advantage of SHR:**
- Unlike traditional RAID, SHR allows mixing different size disks without wasting the extra space on larger drives
- More flexible expansion options

## Notes

1. All pool operations require root privileges
2. Creating a pool will format the disks, please backup data first
3. It is recommended to test thoroughly before production use
4. SHR requires at least 2 disks for SHR-1, 4 disks for SHR-2

## License

MIT License
