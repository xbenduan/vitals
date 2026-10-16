//go:build darwin

package core

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"howett.net/plist"
)

const (
	historyLength           = 60
	memoryConsumersInterval = 5 * time.Second
	memoryConsumersTimeout  = 5 * time.Second
	batterySampleInterval   = 5 * time.Minute
	procPIDTaskInfo         = 4
	procPIDPathInfoMaxSize  = 4 * 1024
	procNameMaxSize         = 1024
	libSystemPath           = "/usr/lib/libSystem.B.dylib"
)

// Sampler 按固定间隔生成系统状态快照，并在内部计算网络速率。
type Sampler struct {
	mu            sync.Mutex
	appsMu        sync.Mutex
	lastNet       gnet.IOCountersStat
	lastSampleAt  time.Time
	lastAppsAt    time.Time
	lastBatteryAt time.Time
	appsSampling  bool
	snapshot      Snapshot
}

type processMemorySample struct {
	name       string
	executable string
	rss        uint64
	cpuPercent float64
}

type procTaskInfo struct {
	VirtualSize      uint64
	ResidentSize     uint64
	TotalUser        uint64
	TotalSystem      uint64
	ThreadsUser      uint64
	ThreadsSystem    uint64
	Policy           int32
	Faults           int32
	Pageins          int32
	CowFaults        int32
	MessagesSent     int32
	MessagesReceived int32
	SyscallsMach     int32
	SyscallsUnix     int32
	ContextSwitches  int32
	ThreadCount      int32
	RunningCount     int32
	Priority         int32
}

type libprocFunctions struct {
	pidPath func(int32, uintptr, uint32) int32
	pidInfo func(int32, int32, uint64, uintptr, int32) int32
	name    func(int32, uintptr, uint32) int32
}

var (
	libprocOnce sync.Once
	libprocAPI  libprocFunctions
	libprocErr  error
)

// NewSampler 创建系统采样器。
func NewSampler() *Sampler {
	return &Sampler{}
}

// RefreshMemoryConsumers 立即重新汇总应用内存并返回更新后的快照。
func (s *Sampler) RefreshMemoryConsumers(ctx context.Context) Snapshot {
	s.mu.Lock()
	totalMemory := s.snapshot.MemoryTotal
	s.mu.Unlock()

	consumers, err := s.collectMemoryConsumers(ctx, totalMemory)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.snapshot.MemoryConsumers = consumers
		s.lastAppsAt = time.Now()
	}
	return cloneSnapshot(s.snapshot)
}

// Sample 读取一次系统状态。个别指标不可用时保留其他有效数据。
func (s *Sampler) Sample(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	next := s.snapshot
	var firstErr error

	if values, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(values) > 0 {
		next.CPUPercent = clampPercent(values[0])
	} else {
		firstErr = fmt.Errorf("cpu: %w", err)
	}
	if value, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		next.MemoryPercent = clampPercent(value.UsedPercent)
		next.MemoryUsed = value.Used
		next.MemoryTotal = value.Total
		next.MemoryFree = value.Available
		next.MemoryKernelFree = value.Free
		next.MemoryActive = value.Active
		next.MemoryInactive = value.Inactive
		next.MemoryWired = value.Wired
		next.MemoryCompressed = compressedMemory(ctx)
		if !s.appsSampling && (s.lastAppsAt.IsZero() || now.Sub(s.lastAppsAt) >= memoryConsumersInterval) {
			s.appsSampling = true
			s.lastAppsAt = now
			go s.sampleMemoryConsumers(value.Total)
		}
	} else if firstErr == nil {
		firstErr = fmt.Errorf("memory: %w", err)
	}
	if value, err := mem.SwapMemoryWithContext(ctx); err == nil {
		next.SwapUsed = value.Used
		next.SwapTotal = value.Total
	}
	if value, err := disk.UsageWithContext(ctx, "/"); err == nil {
		next.DiskPercent = clampPercent(value.UsedPercent)
		next.DiskUsed = value.Used
		next.DiskTotal = value.Total
	}
	if s.lastBatteryAt.IsZero() || now.Sub(s.lastBatteryAt) >= batterySampleInterval {
		if battery, ok := batteryInfo(ctx); ok {
			next.Battery = battery
		}
		s.lastBatteryAt = now
	}
	if counters, err := gnet.IOCountersWithContext(ctx, false); err == nil && len(counters) > 0 {
		current := counters[0]
		if !s.lastSampleAt.IsZero() {
			seconds := now.Sub(s.lastSampleAt).Seconds()
			if seconds > 0 {
				next.NetworkDown = rate(current.BytesRecv, s.lastNet.BytesRecv, seconds)
				next.NetworkUp = rate(current.BytesSent, s.lastNet.BytesSent, seconds)
			}
		}
		s.lastNet = current
	}
	if gpu, ok := gpuUtilization(ctx); ok {
		next.GPUPercent = gpu
	}

	next.UpdatedAt = now
	next.HistoryCPU = appendHistory(next.HistoryCPU, next.CPUPercent)
	next.HistoryMemory = appendHistory(next.HistoryMemory, next.MemoryPercent)
	networkScale := float64(next.NetworkDown + next.NetworkUp)
	next.HistoryNetwork = appendHistory(next.HistoryNetwork, networkScale)
	next.HistoryNetworkDown = appendHistory(next.HistoryNetworkDown, float64(next.NetworkDown))
	next.HistoryNetworkUp = appendHistory(next.HistoryNetworkUp, float64(next.NetworkUp))
	s.lastSampleAt = now
	s.snapshot = next
	return cloneSnapshot(next), firstErr
}

func (s *Sampler) sampleMemoryConsumers(totalMemory uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), memoryConsumersTimeout)
	defer cancel()
	consumers, err := s.collectMemoryConsumers(ctx, totalMemory)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.appsSampling = false
	if err == nil {
		s.snapshot.MemoryConsumers = consumers
	}
}

func (s *Sampler) collectMemoryConsumers(ctx context.Context, totalMemory uint64) ([]MemoryConsumer, error) {
	s.appsMu.Lock()
	defer s.appsMu.Unlock()
	return memoryConsumers(ctx, totalMemory)
}

func rate(current, previous uint64, seconds float64) uint64 {
	if current < previous {
		return 0
	}
	return uint64(float64(current-previous) / seconds)
}

func appendHistory(values []float64, value float64) []float64 {
	values = append(values, value)
	if len(values) > historyLength {
		values = values[len(values)-historyLength:]
	}
	return values
}

func cloneSnapshot(value Snapshot) Snapshot {
	value.HistoryCPU = append([]float64(nil), value.HistoryCPU...)
	value.HistoryMemory = append([]float64(nil), value.HistoryMemory...)
	value.HistoryNetwork = append([]float64(nil), value.HistoryNetwork...)
	value.HistoryNetworkDown = append([]float64(nil), value.HistoryNetworkDown...)
	value.HistoryNetworkUp = append([]float64(nil), value.HistoryNetworkUp...)
	value.MemoryConsumers = append([]MemoryConsumer(nil), value.MemoryConsumers...)
	return value
}

func memoryConsumers(ctx context.Context, totalMemory uint64) ([]MemoryConsumer, error) {
	if err := loadLibproc(); err != nil {
		return nil, err
	}
	pids, err := process.PidsWithContext(ctx)
	if err != nil {
		return nil, err
	}
	samples := make([]processMemorySample, 0, len(pids))
	for _, pid := range pids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var info procTaskInfo
		size := int32(unsafe.Sizeof(info))
		if libprocAPI.pidInfo(pid, procPIDTaskInfo, 0, uintptr(unsafe.Pointer(&info)), size) != size || info.ResidentSize == 0 {
			continue
		}
		samples = append(samples, processMemorySample{
			name:       processName(pid),
			executable: processExecutable(pid),
			rss:        info.ResidentSize,
			cpuPercent: processCPUPercent(ctx, pid),
		})
	}
	return aggregateMemoryConsumers(samples, totalMemory), nil
}

func processCPUPercent(ctx context.Context, pid int32) float64 {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return 0
	}
	usage, err := proc.CPUPercentWithContext(ctx)
	if err != nil || usage < 0 {
		return 0
	}
	return usage
}

func aggregateMemoryConsumers(samples []processMemorySample, totalMemory uint64) []MemoryConsumer {
	grouped := make(map[string]*MemoryConsumer)
	for _, sample := range samples {
		appPath := rootApplicationPath(sample.executable)
		name := ""
		key := appPath
		if appPath != "" {
			name = strings.TrimSuffix(filepath.Base(appPath), ".app")
		} else {
			name = sample.name
			if name == "" {
				name = filepath.Base(sample.executable)
			}
			if name == "" {
				continue
			}
			key = "process:" + strings.ToLower(name)
		}
		consumer := grouped[key]
		if consumer == nil {
			consumer = &MemoryConsumer{Name: name, AppPath: appPath}
			grouped[key] = consumer
		}
		consumer.RSS += sample.rss
		consumer.ProcessCount++
		consumer.CPUPercent += sample.cpuPercent
	}
	consumers := make([]MemoryConsumer, 0, len(grouped))
	for _, consumer := range grouped {
		if totalMemory > 0 {
			consumer.Percent = float64(consumer.RSS) / float64(totalMemory) * 100
		}
		consumers = append(consumers, *consumer)
	}
	slices.SortFunc(consumers, func(a, b MemoryConsumer) int {
		switch {
		case a.RSS > b.RSS:
			return -1
		case a.RSS < b.RSS:
			return 1
		default:
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
	})
	if len(consumers) > 20 {
		consumers = consumers[:20]
	}
	return consumers
}

func loadLibproc() error {
	libprocOnce.Do(func() {
		handle, err := purego.Dlopen(libSystemPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			libprocErr = err
			return
		}
		purego.RegisterLibFunc(&libprocAPI.pidPath, handle, "proc_pidpath")
		purego.RegisterLibFunc(&libprocAPI.pidInfo, handle, "proc_pidinfo")
		purego.RegisterLibFunc(&libprocAPI.name, handle, "proc_name")
	})
	return libprocErr
}

func processExecutable(pid int32) string {
	buffer := make([]byte, procPIDPathInfoMaxSize)
	size := libprocAPI.pidPath(pid, uintptr(unsafe.Pointer(&buffer[0])), uint32(len(buffer)))
	if size <= 0 {
		return ""
	}
	return string(buffer[:size])
}

func processName(pid int32) string {
	buffer := make([]byte, procNameMaxSize)
	size := libprocAPI.name(pid, uintptr(unsafe.Pointer(&buffer[0])), uint32(len(buffer)))
	if size <= 0 {
		return ""
	}
	return string(buffer[:size])
}

func rootApplicationPath(executable string) string {
	index := strings.Index(strings.ToLower(executable), ".app/")
	if index < 0 {
		return ""
	}
	return executable[:index+4]
}

func compressedMemory(ctx context.Context) uint64 {
	output, err := exec.CommandContext(ctx, "/usr/bin/vm_stat").Output()
	if err != nil {
		return 0
	}
	var pageSize uint64 = 4096
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "page size of") {
			fields := strings.Fields(line)
			for index, field := range fields {
				if field == "of" && index+1 < len(fields) {
					if value, parseErr := strconv.ParseUint(fields[index+1], 10, 64); parseErr == nil {
						pageSize = value
					}
				}
			}
			continue
		}
		key, raw, ok := strings.Cut(line, ":")
		if !ok || key != "Pages occupied by compressor" {
			continue
		}
		pages, parseErr := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(raw), "."), 10, 64)
		if parseErr == nil {
			return pages * pageSize
		}
	}
	return 0
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func gpuUtilization(parent context.Context) (float64, bool) {
	ctx, cancel := context.WithTimeout(parent, 800*time.Millisecond)
	defer cancel()
	data, err := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-a", "-r", "-d", "1", "-c", "AGXAccelerator").Output()
	if err != nil || len(data) == 0 {
		return 0, false
	}
	var root any
	if _, err := plist.Unmarshal(data, &root); err != nil {
		return 0, false
	}
	value, ok := findNumber(root, "Device Utilization %")
	return clampPercent(value), ok
}

func findNumber(value any, key string) (float64, bool) {
	switch typed := value.(type) {
	case map[string]any:
		if raw, ok := typed[key]; ok {
			switch number := raw.(type) {
			case uint64:
				return float64(number), true
			case int64:
				return float64(number), true
			case float64:
				return number, true
			}
		}
		for _, child := range typed {
			if number, ok := findNumber(child, key); ok {
				return number, true
			}
		}
	case []any:
		for _, child := range typed {
			if number, ok := findNumber(child, key); ok {
				return number, true
			}
		}
	}
	return 0, false
}
