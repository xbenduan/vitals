# Vitals · 剪贴板

使用 Go + MyGo 渲染的 macOS / Windows 托盘剪贴板应用，无 HTML、JavaScript 或 WebView UI，支持 `CGO_ENABLED=0` 构建。macOS 通过 purego 调用原生 API，Windows 通过 Win32 API 处理剪贴板变化、窗口定位和自动粘贴。

| 系统 | 安装包 | 默认呼出快捷键 |
| --- | --- | --- |
| macOS 13+（Apple Silicon / Intel） | `build/darwin-universal/Vitals 0.5.0.dmg`，拖入 Applications | `Command + Shift + V` |
| Windows 10 1803+ / 11（x64） | `build/windows-amd64/Vitals Setup 0.5.0.exe` | `Ctrl + Shift + V` |

Windows 也可直接运行同目录的 `Vitals.exe`。安装包按当前用户安装，无需管理员权限。本地构建未配置发行签名：macOS 包采用 ad-hoc 签名，未公证；Windows 包未进行 Authenticode 签名。

## 使用

- 按系统对应的快捷键，或点击 macOS 状态栏 / Windows 通知区域图标，在鼠标附近展开面板；靠近屏幕边缘时自动避让，支持多显示器及 Windows 不同缩放比例。再次按快捷键、按 Esc 或点击面板外部可收起。
- 快捷键可在设置中录制或输入，冲突时保留原快捷键。Windows 使用 `Ctrl + Shift + V`，系统的 `Win + V` 继续可用；托盘图标可能位于通知区域的折叠菜单中。
- 重复运行程序会唤起已有实例，不会重复监听或占用同一份历史文件。
- 自动记录文字、图片及 Finder / 文件资源管理器中的文件复制，支持「全部 / 文字 / 文件 / 收藏」筛选，以及内容、文件路径和来源应用搜索。
- 单击记录立即回到打开面板前的应用并粘贴。右键文字可编辑、翻译或删除；右键图片和文件可预览或删除，文件可在所在文件夹中查看。
- 按 `1`–`9` 粘贴当前分类和搜索结果中的前九项，编号显示在记录右侧；按 `Enter` 粘贴当前选中项。输入框内数字正常输入，`↓` / `↑` 可从搜索框回到列表。
- 星标收藏；记录时间精确到秒。重复内容会合并并更新复制时间，收藏状态保留。
- 点击「正在监听」可暂停；恢复后从下一次复制开始记录。
- 首页显示图标和 Clipboard；设置、编辑、翻译和预览共用同一个面板，页头提供返回和页面标题。

macOS 首次使用自动粘贴时，在设置中点击「打开辅助功能设置」，允许当前 Vitals 应用。未授权时仍可记录和复制，面板会提示如何开启自动粘贴。

Windows 自动粘贴会恢复原窗口并发送 `Ctrl + V`，不会只按进程切换到同一应用的另一个窗口。Windows 可能拒绝向以管理员身份运行的目标程序注入按键，或拒绝恢复前台焦点；此时内容仍在剪贴板中，可切回目标窗口手动粘贴。

目标应用需要支持 `Command + V` / `Ctrl + V`，并且有可粘贴的位置；应用只能确认焦点和按键发送，无法确认每个第三方应用是否接受了内容。

## 面板快捷键

| macOS | Windows | 操作 |
| --- | --- | --- |
| `↑` / `↓` | `↑` / `↓` | 选择记录 |
| `Enter` | `Enter` | 粘贴选中记录 |
| `1`–`9` | `1`–`9` | 粘贴当前筛选结果的第 1–9 项（输入框内不触发） |
| `Command + F` | `Ctrl + F` | 搜索 |
| `Command + E` | `Ctrl + E` | 编辑或预览选中记录 |
| `Command + D` | `Ctrl + D` | 收藏或取消收藏 |
| `Command + S` | `Ctrl + S` | 保存文字编辑 |
| `Command + Enter` | `Ctrl + Enter` | 保存文字并粘贴 |
| `Esc` / `Command + W` | `Esc` / `Ctrl + W` | 返回 / 关闭面板 |
| `Command + Q` | `Ctrl + Q` | 退出应用 |

## 翻译

右键文字记录选择「翻译」，结果会在编辑页中显示。译文可以修改，点击「保存」或「保存并粘贴」作为新内容加入历史；原文不会被改写，历史仍遵循条数限制和去重规则。返回、隐藏窗口或点击「停止」会取消请求，失败后可重试或进入翻译设置。

翻译通过 [腾讯交互翻译（TranSmart）](https://transmart.qq.com/) API 提供：

1. 在腾讯交互翻译的个人中心获取具备 API 权限的 **User** 和 **Token**。
2. 打开「设置 → 翻译」，填写这两个值。这里使用 TranSmart 的开发者凭据，不使用腾讯云机器翻译的 SecretId / SecretKey。
3. 点击「测试连接」验证当前填写的凭据，再点击「保存翻译配置」。测试仅发送固定示例 `Hello, world!`，成功不代表配置已保存。
4. 选择目标语言：默认「自动互译」，中文译英文，英文及其他语言译简体中文；也可固定为「简体中文」或「英文」。检测到原文已经是所选语言时，直接保留原文。

客户端向 `https://transmart.qq.com/api/imt` 发送带有 `header.user` / `header.token` 的 JSON 请求：先用 `text_analysis` 识别语言，再用 `auto_translation` 明确指定原文与目标语言，按 Markdown 格式提交原文。认证协议参考 [Tencent AI Lab 的 TranSmart 论文 §4 / Figure 8](https://arxiv.org/html/2105.13072v1#S4)。译文内容及代码保留效果取决于服务返回，保存前可在编辑页调整。

应用限制单次最多 5,000 个 Unicode 字符，整个翻译流程最多等待 45 秒；这不是账户额度说明，实际额度和语言支持以腾讯服务为准。只有主动翻译的所选原文会发往腾讯，启动应用不会调用翻译 API。User / Token 保存在本机用户目录的设置文件中（macOS 权限 `0600`，Windows 遵循用户目录 ACL），Token 在界面上隐藏显示。清空两个凭据并保存可断开账号。

旧版翻译设置会在新版启动时自动移除；快捷键、历史和收藏继续保留。读取失败的设置文件不会被自动覆盖。

## 本地数据

数据目录：

- macOS：`~/Library/Application Support/Vitals/`
- Windows：`%APPDATA%\Vitals\`

目录内包含：

- `clipboard-settings.json`：快捷键、监听状态、历史条数及 TranSmart User / Token、目标语言。
- `Clipboard/history.json`：历史记录和收藏，使用原子替换保存。
- `Clipboard/images/`：复制的 PNG 图片。删除历史时回收对应图片。

默认保留最近 500 条未收藏记录，可设置为 100 / 500 / 1000 条；收藏不受条数限制。清空历史保留收藏，不会删除原始文件。文件历史保存原始路径，文件被移动或删除后不能再次粘贴。

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
    window*.go             # 面板显示、隐藏及鼠标附近定位
    clipboard.go           # 监听、编辑、自动粘贴
    images.go              # 图片加载、缩略图与预览
    settings.go            # 设置动作与持久化调度
    translation.go         # 翻译、连接测试、请求取消与状态回传
    shortcut.go            # 快捷键解析、录制与注册
    view*.go               # 主框架、历史、详情和设置页面
    theme.go               # 主题、图标与共享界面控件
  clipboard/               # 内容模型、历史存储、去重和搜索
  config/                  # 配置默认值、读取与保存
  translation/             # 腾讯交互翻译 HTTP 客户端
  atomicfile/              # 原子文件写入
  platform/                # 跨平台目标身份与定位；macOS / Windows 原生实现
resources/                 # 应用图标等构建资源
mygo.json                  # MyGo 开发与打包配置
```

依赖方向为 `cmd/vitals → app → clipboard / config / translation / platform`，
`config` 使用 `translation.Config`，与 `clipboard` 共用 `atomicfile`。存储、配置和翻译客户端不依赖 MyGo；
界面状态集中在 `app` 的主线程，存储由单独的串行 worker 管理，
通过 `Window.Update` 回传只读快照。翻译请求独立于存储 worker，通过可取消的 context 和请求序号防止过期响应覆盖页面。页面按文件拆分，复用同一份私有应用状态。

## 开发与构建

需要 Go 1.27.1 或更高版本，以及 macOS 13+ 或 Windows 10 1803+ / 11。macOS 通用版 / DMG 需在 Mac 上构建；在 Mac 上生成 Windows 安装包需先安装 NSIS：`brew install nsis`。

```bash
go tool mygo dev
```

应用默认只显示状态栏 / 托盘图标。直接运行程序时可传入 `--show` 在鼠标附近展开面板。

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

在 Mac 上一次生成双平台发行包：

```bash
go tool mygo build -platform darwin/universal,windows/amd64
```

产物分别位于 `build/darwin-universal/`、`build/windows-amd64/`，包含 `.app` / `.dmg` 和 `.exe` / 安装程序。开发包使用独立的 `Vitals Dev` 数据目录。全局快捷键和 macOS 辅助功能权限绑定当前运行的应用；验证自动粘贴时，请为实际运行的构建包授权。

UI 使用 MyGo `ui.NewTester` 验证搜索、分类、右键菜单、即时粘贴、数字快捷键、编辑、翻译设置及快捷键录制；平台测试覆盖边缘避让、负坐标显示器、缩放工作区、小屏收缩及目标窗口身份。存储测试覆盖去重、收藏、保留条数、图片清理、磁盘写入失败及损坏文件。HTTP 测试使用本地测试服务验证 TranSmart 认证、语言识别和翻译方向、输入与响应上限、错误响应及两个请求阶段的取消；配置测试验证旧配置迁移及适用平台的凭据文件权限。

跨平台编译不能替代 Windows 实机验证；托盘呼出、不同缩放显示器间切换、前台焦点恢复及文字 / 图片 / 文件粘贴需在 Windows 上实际测试。

可选的真实 API 集成测试只发送固定测试文字。在环境中设置 `TRANSMART_USER` 和 `TRANSMART_TOKEN` 后运行：

```bash
go test ./internal/translation -run TestTranSmartIntegration -v -count=1
```

没有配置凭据时该测试会跳过。测试检查中译英、英译中及短英文的输出方向；真实账号认证、额度与服务可用性需要有效凭据验证。
