package platform

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
)

type cpuTimes struct {
	idle, total time.Duration
	at          time.Time
}

var (
	cpuMu   sync.Mutex
	cpuLast cpuTimes
)

func readCPUTimes() (cpuTimes, bool) {
	var idle, kernel, user windows.Filetime
	r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return cpuTimes{}, false
	}
	return cpuTimes{idle: ticks(idle), total: ticks(kernel) + ticks(user), at: time.Now()}, true
}

func cpuBusy(prev, now cpuTimes) float64 {
	total := now.total - prev.total
	if total <= 0 {
		return 0
	}
	return 100 * float64(total-(now.idle-prev.idle)) / float64(total)
}

func sampleCPU() float64 {
	cpuMu.Lock()
	defer cpuMu.Unlock()
	prev := cpuLast
	if prev.at.IsZero() || time.Since(prev.at) > time.Minute || time.Since(prev.at) < time.Second {
		first, ok := readCPUTimes()
		if !ok {
			return 0
		}
		time.Sleep(time.Second)
		prev = first
	}
	now, ok := readCPUTimes()
	if !ok {
		return 0
	}
	cpuLast = now
	return cpuBusy(prev, now)
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func ReadLoad() Load {
	s := Load{Cores: runtime.NumCPU(), ByCPU: true, CPUBusy: sampleCPU()}
	m := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r != 0 && m.TotalPhys > 0 {
		s.MemFree = int(m.AvailPhys * 100 / m.TotalPhys)
	}
	return s
}

func ReadProcs() []Proc {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var rows []Proc
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		r := Proc{pid: int(e.ProcessID), ppid: int(e.ParentProcessID),
			comm: strings.TrimSuffix(filepath.Base(windows.UTF16ToString(e.ExeFile[:])), ".exe")}
		r.cpu, r.rss = procUsage(e.ProcessID)
		rows = append(rows, r)
	}
	return rows
}

func procUsage(pid uint32) (cpu float64, rssKB int64) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, 0
	}
	defer windows.CloseHandle(h)
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(h, &created, &exited, &kernel, &user) == nil {
		busy := ticks(kernel) + ticks(user)
		if age := time.Since(time.Unix(0, created.Nanoseconds())); age > 0 {
			cpu = 100 * busy.Seconds() / age.Seconds()
		}
	}
	m := processMemoryCounters{Cb: uint32(unsafe.Sizeof(processMemoryCounters{}))}
	if r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&m)), uintptr(m.Cb)); r != 0 {
		rssKB = int64(m.WorkingSetSize / 1024)
	}
	return cpu, rssKB
}

func ticks(ft windows.Filetime) time.Duration {
	return time.Duration(int64(ft.HighDateTime)<<32|int64(ft.LowDateTime)) * 100
}
