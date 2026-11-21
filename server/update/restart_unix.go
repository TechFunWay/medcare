//go:build !windows

package update

// unix 平台的自更新底层操作：文件系统余量查询与原地重启。

import (
	"os"
	"path/filepath"
	"syscall"
)

type syscallStatfsT = syscall.Statfs_t

func statfs(path string, st *syscallStatfsT) error {
	return syscall.Statfs(path, st)
}

// freeBytes 取 statfs 结果里的可用字节数。
func freeBytes(st *syscallStatfsT) uint64 {
	return st.Bavail * uint64(st.Bsize)
}

// restartProcess 以相同参数与环境变量重新执行自身二进制。
// 替换后的二进制已在原路径上，exec 之后 PID 不变，
// 飞牛的进程监管与 PID 文件继续有效；网关 unix socket 会在
// 新进程绑定时先清理旧文件（server 包既有行为）。
func restartProcess() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	if exePath, err = filepath.EvalSymlinks(exePath); err != nil {
		return err
	}
	return syscall.Exec(exePath, append([]string{exePath}, os.Args[1:]...), os.Environ())
}
