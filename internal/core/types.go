package core

import "time"

// Snapshot 是菜单栏与主界面共享的唯一系统状态快照。
type Snapshot struct {
	CPUPercent       float64
	MemoryPercent    float64
	MemoryUsed       uint64
	MemoryTotal      uint64
	MemoryFree       uint64
	MemoryKernelFree uint64
	MemoryActive     uint64
	MemoryInactive   uint64
	MemoryWired      uint64
	MemoryCompressed uint64
	SwapUsed         uint64
	SwapTotal        uint64
	MemoryConsumers  []MemoryConsumer
	GPUPercent       float64
	NetworkDown      uint64
	NetworkUp        uint64
	DiskPercent      float64
	DiskUsed         uint64
	DiskTotal        uint64
	Battery          BatteryInfo
	UpdatedAt        time.Time
	HistoryCPU       []float64
	HistoryMemory    []float64
	HistoryNetwork   []float64
}

// BatteryInfo 描述内置电池的当前状态；Present 为 false 时设备没有可用电池。
type BatteryInfo struct {
	Present       bool
	ChargePercent int
	HealthPercent int
	CycleCount    int
	TemperatureC  float64
	Charging      bool
	ExternalPower bool
}

// MemoryConsumer 是按应用或进程名聚合的常驻内存占用。
type MemoryConsumer struct {
	Name         string
	AppPath      string
	RSS          uint64
	ProcessCount int
	Percent      float64
}

// AppInfo 描述一个可卸载的 macOS 应用及其关联文件。
type AppInfo struct {
	Name       string
	Path       string
	BundleID   string
	Version    string
	Size       uint64
	Running    bool
	Selected   bool
	Related    []RelatedFile
	Scanned    bool
	ScanError  string
	TotalBytes uint64
}

// RelatedFile 是根据 bundle id 或应用名命中的卸载候选项。
type RelatedFile struct {
	Path       string
	Size       uint64
	Kind       string
	Confidence string
	Selected   bool
}
