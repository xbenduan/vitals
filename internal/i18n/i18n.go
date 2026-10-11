// Package i18n 提供 Vitals 的中英文文案与系统语言检测。
//
// 设计原则：
//   - 文案单一真相源：所有用户可见字符串集中在 messages 表中。
//   - 语言三态：auto（跟随系统）、zh、en。系统语言非中文时自动使用英文。
//   - Localizer 是不可变值语义，SetLang 返回新实例而非原地修改。
package i18n

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Lang 表示应用当前使用的语言。
type Lang string

const (
	// LangAuto 表示跟随系统语言。
	LangAuto Lang = "auto"
	// LangZH 表示简体中文。
	LangZH Lang = "zh"
	// LangEN 表示英文。
	LangEN Lang = "en"
)

// Localizer 持有当前语言并提供文案查询。
type Localizer struct {
	lang Lang
}

// SystemLang 读取操作系统首选语言；非中文统一返回英文。
//
// 检测顺序：LC_ALL → LC_MESSAGES → LANG → macOS defaults 中的 AppleLanguages。
func SystemLang() Lang {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); isChineseLocale(value) {
			return LangZH
		}
	}
	if lang := appleLanguagesFirst(); isChineseLocale(lang) {
		return LangZH
	}
	return LangEN
}

func isChineseLocale(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "c" || value == "posix" {
		return false
	}
	// 取语言代码（如 zh_CN.UTF-8 → zh，zh-Hans-CN → zh）
	language := value
	for _, sep := range []string{".", "@", "_", "-"} {
		if index := strings.Index(language, sep); index >= 0 {
			language = language[:index]
		}
	}
	return language == "zh"
}

// appleLanguagesFirst 返回 macOS 系统首选语言列表的第一项。
func appleLanguagesFirst() string {
	output, err := exec.Command("/usr/bin/defaults", "read", "NSGlobalDomain", "AppleLanguages").Output()
	if err != nil {
		return ""
	}
	// 输出形如：( "zh-Hans-CN", "en" )
	text := strings.TrimSpace(string(output))
	text = strings.TrimPrefix(text, "(")
	text = strings.TrimSuffix(text, ")")
	parts := strings.SplitN(text, ",", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(parts[0]), `"`)
}

// New 根据给定语言创建 Localizer。LangAuto 会解析为系统语言。
func New(lang Lang) Localizer {
	return Localizer{lang: resolve(lang)}
}

// Lang 返回当前生效语言（已解析，不会是 LangAuto）。
func (l Localizer) Lang() Lang { return l.lang }

// BundleLocales 返回读取 macOS 应用本地化名称时依次尝试的 .lproj 目录名。
func (l Lang) BundleLocales() []string {
	if l == LangZH {
		return []string{"zh-Hans", "zh_CN", "zh"}
	}
	return []string{"en", "English", "Base"}
}

// SetLang 返回切换到指定语言后的新 Localizer。
func (l Localizer) SetLang(lang Lang) Localizer {
	return Localizer{lang: resolve(lang)}
}

func resolve(lang Lang) Lang {
	switch lang {
	case LangZH, LangEN:
		return lang
	default:
		return SystemLang()
	}
}

// T 返回 key 对应语言的文案；缺失时返回 key 本身。
func (l Localizer) T(key string) string {
	if entry, ok := messages[key]; ok {
		if text, ok := entry[l.lang]; ok && text != "" {
			return text
		}
	}
	return key
}

// Tf 返回格式化后的文案，等价于 fmt.Sprintf(l.T(key), args...)。
func (l Localizer) Tf(key string, args ...any) string {
	return fmt.Sprintf(l.T(key), args...)
}

// messages 是全部用户可见文案的唯一真相源。
//
//	key → {zh: 中文, en: English}
var messages = map[string]map[Lang]string{
	// 菜单栏
	"menu.tooltip.reading": {LangZH: "Vitals 正在读取系统状态", LangEN: "Vitals is reading system status"},
	"menu.tooltip.download": {LangZH: "下载 %s", LangEN: "↓ %s"},
	"menu.tooltip.upload":   {LangZH: "上传 %s", LangEN: "↑ %s"},
	"menu.tooltip.updated":  {LangZH: "更新于 %s", LangEN: "Updated %s"},

	// 通用按钮与操作
	"common.cancel":      {LangZH: "取消", LangEN: "Cancel"},
	"common.confirm":     {LangZH: "知道了", LangEN: "OK"},
	"common.open_settings": {LangZH: "打开设置", LangEN: "Open Settings"},
	"common.open_vitals": {LangZH: "打开 Vitals", LangEN: "Open Vitals"},
	"common.quit_vitals": {LangZH: "退出 Vitals", LangEN: "Quit Vitals"},

	// 错误对话框
	"error.full_disk_title": {LangZH: "需要完全磁盘访问", LangEN: "Full Disk Access Required"},
	"error.full_disk_body":  {LangZH: "无法移除受 macOS 保护的数据：\n%s\n\n请允许 Vitals 完全磁盘访问后重试。应用本体尚未移除。", LangEN: "Cannot remove macOS-protected data:\n%s\n\nGrant Full Disk Access to Vitals and retry. The app bundle has not been removed."},
	"error.action_title":    {LangZH: "操作未完成", LangEN: "Action Incomplete"},
	"error.startup_scan":    {LangZH: "暂时无法检查启动项", LangEN: "Unable to check startup items right now"},

	// 内存释放确认
	"memory.release_title":   {LangZH: "释放 “%s” 的内存？", LangEN: "Free memory of “%s”?"},
	"memory.release_body":    {LangZH: "这会正常退出该应用及其 %d 个进程，未保存的内容可能丢失。", LangEN: "This will quit the app and its %d process(es). Unsaved content may be lost."},
	"memory.release_confirm": {LangZH: "退出并释放", LangEN: "Quit & Free"},
	"memory.release_action":  {LangZH: "退出应用并释放其内存", LangEN: "Quit the app and free its memory"},
	"memory.releasing":       {LangZH: "释放中…", LangEN: "Freeing…"},
	"memory.release":         {LangZH: "释放", LangEN: "Free"},
	"memory.release_done":    {LangZH: "%s 已退出，常驻内存减少约 %s", LangEN: "%s quit, resident memory reduced by ~%s"},
	"memory.release_pending": {LangZH: "已请求 %s 退出，应用仍在处理退出操作", LangEN: "Requested %s to quit; the app is still shutting down"},

	// 卸载确认
	"uninstall.title":   {LangZH: "移到废纸篓？", LangEN: "Move to Trash?"},
	"uninstall.confirm": {LangZH: "卸载", LangEN: "Uninstall"},
	"uninstall.summary": {LangZH: "%s 及 %d 个已选关联项将移到废纸篓，可在清空废纸篓前恢复。", LangEN: "%s and %d selected related items will be moved to Trash. You can restore them before emptying the Trash."},

	// 顶部分页
	"tabs.status":   {LangZH: "状态", LangEN: "Status"},
	"tabs.memory":   {LangZH: "内存", LangEN: "Memory"},
	"tabs.apps":     {LangZH: "应用", LangEN: "Apps"},
	"tabs.settings": {LangZH: "设置", LangEN: "Settings"},
	"tabs.label":    {LangZH: "功能", LangEN: "Section"},

	// 状态页
	"overview.cpu_detail":      {LangZH: "处理器", LangEN: "Processor"},
	"overview.ram_detail":      {LangZH: "图形负载", LangEN: "Graphics load"}, // GPU 详情
	"overview.gpu_title":       {LangZH: "图形负载", LangEN: "Graphics"},
	"overview.network_title":   {LangZH: "网络", LangEN: "Network"},
	"overview.disk_used":       {LangZH: "已使用 %s，共 %s", LangEN: "%s used of %s"},
	"overview.disk_label":      {LangZH: "磁盘使用率", LangEN: "Disk Usage"},
	"overview.startup_healthy": {LangZH: "%d 个启动项 · 未发现明显异常", LangEN: "%d startup items · no obvious issues"},
	"overview.startup_loading": {LangZH: "正在检查可读取的启动项…", LangEN: "Checking readable startup items…"},
	"overview.startup_review":  {LangZH: "%d 个启动项 · %d 个需关注", LangEN: "%d startup items · %d need attention"},
	"overview.startup_title":   {LangZH: "启动项健康", LangEN: "Startup Health"},
	"overview.startup_refresh": {LangZH: "重新检查启动项", LangEN: "Recheck Startup Items"},
	"overview.startup_refresh_tooltip": {LangZH: "重新检查", LangEN: "Recheck"},
	"overview.system_manage":   {LangZH: "系统管理", LangEN: "System Settings"},

	// 电池
	"battery.on_battery":    {LangZH: "使用电池", LangEN: "On Battery"},
	"battery.charging":      {LangZH: "正在充电", LangEN: "Charging"},
	"battery.external":      {LangZH: "已连接电源", LangEN: "Power Adapter"},
	"battery.detecting":     {LangZH: "检测中", LangEN: "Detecting"},
	"battery.health":        {LangZH: "健康度约 %d%%", LangEN: "Health ~%d%%"},
	"battery.status":        {LangZH: "电池 %d%% · %s", LangEN: "Battery %d%% · %s"},
	"battery.cycles":        {LangZH: "%s · %d 次循环", LangEN: "%s · %d cycles"},

	// 健康摘要
	"health.good_title":   {LangZH: "运行状态良好", LangEN: "Running Smoothly"},
	"health.good_detail":  {LangZH: "没有发现持续资源压力", LangEN: "No sustained resource pressure detected"},
	"health.check_title":  {LangZH: "建议检查", LangEN: "Review Recommended"},
	"health.issue_memory": {LangZH: "内存紧张", LangEN: "Memory pressure"},
	"health.issue_cpu":    {LangZH: "CPU 高负载", LangEN: "High CPU load"},
	"health.issue_disk":   {LangZH: "磁盘空间不足", LangEN: "Low disk space"},
	"health.issue_startup": {LangZH: "%d 个启动项需关注", LangEN: "%d startup item(s) need attention"},

	// 健康提醒通知
	"alert.memory_title":   {LangZH: "内存持续紧张", LangEN: "Sustained Memory Pressure"},
	"alert.memory_body":    {LangZH: "内存使用率已持续达到 %.0f%%", LangEN: "Memory usage has been at %.0f%% for a while"},
	"alert.cpu_title":      {LangZH: "CPU 持续高负载", LangEN: "Sustained High CPU Load"},
	"alert.cpu_body":       {LangZH: "CPU 使用率已持续达到 %.0f%%", LangEN: "CPU usage has been at %.0f%% for a while"},
	"alert.disk_title":     {LangZH: "磁盘空间不足", LangEN: "Low Disk Space"},
	"alert.disk_body":      {LangZH: "磁盘已使用 %.0f%%，建议在系统设置中管理存储空间", LangEN: "Disk is %.0f%% full. Manage storage in System Settings."},
	"alert.test_title":     {LangZH: "Vitals 健康提醒", LangEN: "Vitals Health Alert"},
	"alert.test_body":      {LangZH: "通知工作正常。只有资源持续异常时才会提醒。", LangEN: "Notifications work. You'll only be alerted for sustained anomalies."},
	"alert.test_sent":      {LangZH: "测试提醒已发送", LangEN: "Test alert sent"},

	// 内存页
	"memory.free":           {LangZH: "可用内存", LangEN: "Free Memory"},
	"memory.usage_label":    {LangZH: "内存使用率", LangEN: "Memory Usage"},
	"memory.used":           {LangZH: "已用 %s", LangEN: "%s used"},
	"memory.total":          {LangZH: "总计 %s", LangEN: "%s total"},
	"memory.composition":    {LangZH: "内存构成", LangEN: "Memory Breakdown"},
	"memory.swap":           {LangZH: "交换区 %s / %s", LangEN: "Swap %s / %s"},
	"memory.active":         {LangZH: "活跃", LangEN: "Active"},
	"memory.inactive":       {LangZH: "非活跃", LangEN: "Inactive"},
	"memory.wired":          {LangZH: "联动", LangEN: "Wired"},
	"memory.compressed":     {LangZH: "压缩", LangEN: "Compressed"},
	"memory.kernel_free":    {LangZH: "空闲", LangEN: "Free"},
	"memory.free_legend":    {LangZH: "可用", LangEN: "Available"},
	"memory.by_app":         {LangZH: "按应用占用", LangEN: "By App"},
	"memory.top_items":      {LangZH: "前 %d 项 · 常驻内存 RSS", LangEN: "Top %d · Resident RSS"},
	"memory.process_count":  {LangZH: "%d 个进程", LangEN: "%d process(es)"},
	"memory.summarizing":    {LangZH: "正在汇总进程内存…", LangEN: "Summarizing process memory…"},

	// 设置页
	"settings.startup":         {LangZH: "启动", LangEN: "Startup"},
	"settings.open_at_login":   {LangZH: "登录时启动", LangEN: "Open at Login"},
	"settings.open_at_login_detail": {LangZH: "登录 macOS 后在菜单栏运行", LangEN: "Run in the menu bar after logging in to macOS"},
	"settings.status_bar":      {LangZH: "状态栏", LangEN: "Menu Bar"},
	"settings.cpu_detail":      {LangZH: "处理器使用率", LangEN: "Processor usage"},
	"settings.ram_detail":      {LangZH: "内存使用率", LangEN: "Memory usage"},
	"settings.gpu_detail":      {LangZH: "图形处理器使用率", LangEN: "GPU usage"},
	"settings.network_detail":  {LangZH: "实时下载与上传速率", LangEN: "Real-time download & upload speed"},
	"settings.health_alerts":   {LangZH: "健康提醒", LangEN: "Health Alerts"},
	"settings.sustained":       {LangZH: "持续异常提醒", LangEN: "Sustained Anomaly Alerts"},
	"settings.sustained_detail": {LangZH: "CPU 或内存持续高负载、磁盘空间不足时提醒", LangEN: "Alert on sustained high CPU/memory load or low disk space"},
	"settings.notify_permission": {LangZH: "首次发送时由 macOS 请求通知权限", LangEN: "macOS will request notification permission on first send"},
	"settings.test_alert":      {LangZH: "测试提醒", LangEN: "Test Alert"},
	"settings.test_sending":    {LangZH: "发送中…", LangEN: "Sending…"},
	"settings.update_appearance": {LangZH: "更新与外观", LangEN: "Refresh & Appearance"},
	"settings.refresh_rate":    {LangZH: "刷新频率", LangEN: "Refresh Rate"},
	"settings.refresh_detail":  {LangZH: "状态栏与系统数据的更新间隔", LangEN: "Interval for menu bar and system data updates"},
	"settings.appearance":      {LangZH: "外观", LangEN: "Appearance"},
	"settings.appearance_detail": {LangZH: "自动跟随 macOS 深色或浅色模式", LangEN: "Automatically follows macOS dark/light mode"},
	"settings.follow_system":   {LangZH: "跟随系统", LangEN: "Follow System"},
	"settings.language":        {LangZH: "语言", LangEN: "Language"},
	"settings.language_detail": {LangZH: "应用界面显示语言", LangEN: "App interface language"},
	"settings.lang_auto":       {LangZH: "跟随系统", LangEN: "Follow System"},
	"settings.lang_zh":         {LangZH: "简体中文", LangEN: "Simplified Chinese"},
	"settings.lang_en":         {LangZH: "English", LangEN: "English"},
	"settings.version_debug":   {LangZH: "调试版本", LangEN: "Debug Build"},
	"settings.version_format":  {LangZH: "版本 %s", LangEN: "Version %s"},
	"settings.local_data":      {LangZH: "%s · 数据仅保存在本机", LangEN: "%s · Data stays on this Mac"},

	// 刷新频率选项（存储值与显示值分离，这里仅显示）
	"refresh.1s": {LangZH: "1 秒", LangEN: "1 sec"},
	"refresh.2s": {LangZH: "2 秒", LangEN: "2 sec"},
	"refresh.5s": {LangZH: "5 秒", LangEN: "5 sec"},

	// 应用管理页
	"apps.title":         {LangZH: "应用管理", LangEN: "App Management"},
	"apps.search_label":  {LangZH: "搜索应用", LangEN: "Search Apps"},
	"apps.search_placeholder": {LangZH: "搜索应用", LangEN: "Search apps"},
	"apps.loading":       {LangZH: "正在读取应用…", LangEN: "Loading apps…"},
	"apps.running":       {LangZH: "正在运行 · %s", LangEN: "Running · %s"},
	"apps.count":         {LangZH: "%d 个应用", LangEN: "%d app(s)"},
	"apps.back_label":    {LangZH: "返回应用列表", LangEN: "Back to App List"},
	"apps.back_tooltip":  {LangZH: "返回应用列表", LangEN: "Back to app list"},
	"apps.profile_title": {LangZH: "应用健康档案", LangEN: "App Health Profile"},
	"apps.quit":          {LangZH: "退出应用", LangEN: "Quit App"},
	"apps.quitting":      {LangZH: "正在退出…", LangEN: "Quitting…"},
	"apps.resident_memory": {LangZH: "常驻内存", LangEN: "Resident Memory"},
	"apps.startup_items": {LangZH: "启动项", LangEN: "Startup Items"},
	"apps.startup_total": {LangZH: "共 %d 项", LangEN: "%d total"},
	"apps.disk_usage":    {LangZH: "磁盘占用", LangEN: "Disk Usage"},
	"apps.disk_detail":   {LangZH: "应用与已选关联项", LangEN: "App + selected related items"},
	"apps.startup_linked": {LangZH: "该应用关联 %d 个启动项，其中 %d 个已启用", LangEN: "%d startup item(s) linked to this app, %d enabled"},
	"apps.startup_linking": {LangZH: "正在关联启动项…", LangEN: "Linking startup items…"},
	"apps.to_trash":      {LangZH: "将移到废纸篓", LangEN: "Will Move to Trash"},
	"apps.trash_summary": {LangZH: "%d 项 · %s", LangEN: "%d item(s) · %s"},
	"apps.app_body":      {LangZH: "应用本体", LangEN: "App Bundle"},
	"apps.scanning":      {LangZH: "正在扫描关联文件…", LangEN: "Scanning related files…"},
	"apps.confidence_high": {LangZH: "高置信度", LangEN: "High confidence"},
	"apps.confidence_medium": {LangZH: "中置信度", LangEN: "Medium confidence"},
	"apps.no_related":    {LangZH: "未发现额外关联文件。", LangEN: "No additional related files found."},
	"apps.recoverable":   {LangZH: "移除后可从废纸篓恢复", LangEN: "Recoverable from Trash after removal"},
	"apps.moving":        {LangZH: "正在移动…", LangEN: "Moving…"},
}
