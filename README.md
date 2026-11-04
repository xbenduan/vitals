# Vitals · 剪贴板

使用 Go + MyGo 渲染的 macOS 13+ 状态栏剪贴板应用，无 HTML、JavaScript 或 WebView UI，支持 `CGO_ENABLED=0` 构建。macOS 系统交互通过 purego 调用原生 API。

## 使用

- 点击状态栏图标，或按 `Command + Shift + V` 展开面板。快捷键可在设置中录制或输入，冲突时保留原快捷键。
- 自动记录文字、图片及 Finder 文件复制，支持「全部 / 文字 / 文件 / 收藏」筛选，以及内容、文件路径和来源应用搜索。
- 单击记录后自动回到打开面板前的应用并粘贴；双击文字编辑，双击图片预览，普通文件可在 Finder 中查看。
- 单击会等待双击判定；按 `Enter` 可立即粘贴当前选中项。
- 星标收藏；记录时间精确到秒。重复内容会合并并更新复制时间，收藏状态保留。
- 点击「正在监听」可暂停；恢复后从下一次复制开始记录。
- 设置、编辑和预览都在同一个状态栏面板中。

首次使用自动粘贴时，在设置中点击「打开辅助功能设置」，允许当前 Vitals 应用。未授权时仍可记录和复制，面板会提示如何开启自动粘贴。目标应用需要支持 `Command + V`，并且有可粘贴的位置；应用只能确认焦点和按键发送，无法确认每个第三方应用是否接受了内容。

## 面板快捷键

| 按键 | 操作 |
| --- | --- |
| `↑` / `↓` | 选择记录 |
| `Enter` | 粘贴选中记录 |
| `Command + F` | 搜索 |
| `Command + E` | 编辑或预览选中记录 |
| `Command + D` | 收藏或取消收藏 |
| `Command + S` | 保存文字编辑 |
| `Command + Enter` | 保存文字并粘贴 |
| `Esc` | 返回上一级，或关闭面板 |
| `Command + Q` | 退出应用 |

## 本地数据

数据位于 `~/Library/Application Support/Vitals/`：

- `clipboard-settings.json`：快捷键、监听状态、历史条数。
- `Clipboard/history.json`：历史记录和收藏，使用原子替换保存。
- `Clipboard/images/`：复制的 PNG 图片。删除历史时回收对应图片。

默认保留最近 500 条未收藏记录，可设置为 100 / 500 / 1000 条；收藏不受条数限制。清空历史保留收藏，不会删除 Finder 中的原始文件。文件历史保存原始路径，文件被移动或删除后不能再次粘贴。

单条文字上限 2 MB；图片上限 24 MB、4000 万像素。图片列表使用 72 px 缩略图，详情仅保留一张最长边 1280 px 的预览，粘贴使用原图。当前保存的是纯文本、PNG 图片或文件列表，不保留富文本样式，也不提供 OCR、图片编辑或云同步。

历史损坏时保留原文件并停止写入。旧版系统监控工具的 `settings.json` 不参与新配置。

## 代码结构

采用 Go 常用的 `cmd` + `internal` 布局，测试放在被测代码所在的包中：

```text
cmd/vitals/
  main.go                  # 程序入口
internal/
  app/                     # 应用编排、MyGo 界面与交互状态
    run.go                 # 事件循环与开发环境初始化
    app.go                 # 应用生命周期与串行存储任务
    window.go              # 状态栏面板显示、隐藏及定位
    clipboard.go           # 监听、编辑、自动粘贴
    images.go              # 图片加载、缩略图与预览
    settings.go            # 设置动作与持久化调度
    shortcut.go            # 快捷键解析、录制与注册
    view*.go               # 主框架、历史、详情和设置页面
    theme.go               # 主题、图标与共享界面控件
  clipboard/               # 内容模型、历史存储、去重和搜索
  config/                  # 配置默认值、读取与保存
  atomicfile/              # 原子文件写入
  platform/macos/          # macOS 权限、剪贴板版本、焦点和按键事件
resources/                 # 应用图标等构建资源
mygo.json                  # MyGo 开发与打包配置
```

依赖方向为 `cmd/vitals → app → clipboard / config / platform/macos`，
`clipboard` 与 `config` 共用 `atomicfile`。存储和配置包不依赖 MyGo；
界面状态集中在 `app` 的主线程，存储由单独的串行 worker 管理，
通过 `Window.Update` 回传只读快照。页面按文件拆分，复用同一份私有应用状态。

## 开发与构建

需要 macOS 13+、Go 1.27.1 或更高版本。

```bash
go tool mygo dev
```

应用默认只显示状态栏图标。直接运行程序时可传入 `--show` 展开面板。

```bash
go run ./cmd/vitals --show
```

`mygo.json` 的 `main` 指向 `./cmd/vitals`，开发和打包命令仍从仓库根目录执行。

```bash
CGO_ENABLED=0 go test ./...
go test -race ./...
go tool mygo vet .
go tool mygo build
```

构建产物位于 `build/darwin-arm64/`。开发包使用独立的 `Vitals Dev` 数据目录。全局快捷键和辅助功能权限绑定当前运行的应用；验证自动粘贴时，请为实际运行的构建包授权。

UI 使用 MyGo `ui.NewTester` 验证搜索、分类、双击、编辑、键盘选择及快捷键录制；存储测试覆盖去重、收藏、保留条数、图片清理、磁盘写入失败及损坏文件。
