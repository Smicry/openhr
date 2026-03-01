# OpenHR 项目代码规范

## Go 代码格式化规则

1. **每次修改 Go 文件后必须执行 gofmt 格式化**
   ```bash
   gofmt -w <file>
   ```

2. **函数内语句之间不加空行**
   
   ❌ 错误示例：
   ```go
   func CreatePV(dev string) error {
       if !checkCommand("lvm") {
           return errors.New("lvm not found")
       }
       
       _, err := run("pvcreate", dev)
       return err
   }
   ```
   
   ✅ 正确示例：
   ```go
   func CreatePV(dev string) error {
       if !checkCommand("lvm") {
           return errors.New("lvm not found")
       }
       _, err := run("pvcreate", dev)
       return err
   }
   ```
