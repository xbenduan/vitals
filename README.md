# Vitals · 剪贴板

使用 Go + MyGo 渲染的 macOS 13+ 状态栏剪贴板应用，无 HTML、JavaScript 或 WebView UI，支持 `CGO_ENABLED=0` 构建。macOS 系统交互通过 purego 调用原生 API。

## 使用

- 点击状态栏图标，或按 `Command + Shift + V` 展开面板。快捷键可在设置中录制或输入，冲突时保留原快捷键。
- 自动记录文字、图片及 Finder 文件复制，支持「全部 / 文字 / 文件 / 收藏」筛选，以及内容、文件路径和来源应用搜索。
- 单击记录立即回到打开面板前的应用并粘贴。右键文字可编辑、翻译或删除；右键图片和文件可预览或删除，文件可在 Finder 中查看。
- 按 `1`–`9` 粘贴当前分类和搜索结果中的前九项，编号显示在记录右侧；按 `Enter` 粘贴当前选中项。输入框内数字正常输入，`↓` / `↑` 可从搜索框回到列表。
- 星标收藏；记录时间精确到秒。重复内容会合并并更新复制时间，收藏状态保留。
- 点击「正在监听」可暂停；恢复后从下一次复制开始记录。
- 首页显示图标和 Clipboard；设置、编辑、翻译和预览共用同一个面板，页头提供返回和页面标题。

首次使用自动粘贴时，在设置中点击「打开辅助功能设置」，允许当前 Vitals 应用。未授权时仍可记录和复制，面板会提示如何开启自动粘贴。目标应用需要支持 `Command + V`，并且有可粘贴的位置；应用只能确认焦点和按键发送，无法确认每个第三方应用是否接受了内容。

## 面板快捷键

| 按键 | 操作 |
| --- | --- |
| `↑` / `↓` | 选择记录 |
| `Enter` | 粘贴选中记录 |
| `1`–`9` | 粘贴当前筛选结果的第 1–9 项（输入框内不触发） |
| `Command + F` | 搜索 |
| `Command + E` | 编辑或预览选中记录 |
| `Command + D` | 收藏或取消收藏 |
| `Command + S` | 保存文字编辑 |
| `Command + Enter` | 保存文字并粘贴 |
| `Esc` | 返回上一级，或关闭面板 |
| `Command + Q` | 退出应用 |

## 翻译

右键文字记录选择「翻译」，结果会在编辑页中显示。译文可以修改，点击「保存」或「保存并粘贴」作为新内容加入历史；原文不会被改写，历史仍遵循条数限制和去重规则。返回、隐藏窗口或点击「停止」会取消请求，失败后可重试或进入翻译设置。

翻译通过 [腾讯交互翻译（TranSmart）](https://transmart.qq.com/) API 提供：

1. 在腾讯交互翻译的个人中心获取具备 API 权限的 **User** 和 **Token**。
2. 打开「设置 → 翻译」，填写这两个值。这里使用 TranSmart 的开发者凭据，不使用腾讯云机器翻译的 SecretId / SecretKey。
3. 点击「测试连接」验证当前填写的凭据，再点击「保存翻译配置」。测试仅发送固定示例 `Hello, world!`，成功不代表配置已保存。
4. 选择目标语言：默认「自动互译」，中文译英文，英文及其他语言译简体中文；也可固定为「简体中文」或「英文」。检测到原文已经是所选语言时，直接保留原文。

客户端向 `https://transmart.qq.com/api/imt` 发送带有 `header.user` / `header.token` 的 JSON 请求：先用 `text_analysis` 识别语言，再用 `auto_translation` 明确指定原文与目标语言，按 Markdown 格式提交原文。认证协议参考 [Tencent AI Lab 的 TranSmart 论文 §4 / Figure 8](https://arxiv.org/html/2105.13072v1#S4)。译文内容及代码保留效果取决于服务返回，保存前可在编辑页调整。

应用限制单次最多 5,000 个 Unicode 字符，整个翻译流程最多等待 45 秒；这不是账户额度说明，实际额度和语言支持以腾讯服务为准。只有主动翻译的所选原文会发往腾讯，启动应用不会调用翻译 API。User / Token 保存在权限为 `0600` 的本地设置文件中，Token 在界面上隐藏显示。清空两个凭据并保存可断开账号。

旧版翻译设置会在新版启动时自动移除；快捷键、历史和收藏继续保留。读取失败的设置文件不会被自动覆盖。

## 本地数据

数据位于 `~/Library/Application Support/Vitals/`：

- `clipboard-settings.json`：快捷键、监听状态、历史条数及 TranSmart User / Token、目标语言。
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
    translation.go         # 翻译、连接测试、请求取消与状态回传
    shortcut.go            # 快捷键解析、录制与注册
    view*.go               # 主框架、历史、详情和设置页面
    theme.go               # 主题、图标与共享界面控件
  clipboard/               # 内容模型、历史存储、去重和搜索
  config/                  # 配置默认值、读取与保存
  translation/             # 腾讯交互翻译 HTTP 客户端
  atomicfile/              # 原子文件写入
  platform/macos/          # macOS 权限、剪贴板版本、焦点和按键事件
resources/                 # 应用图标等构建资源
mygo.json                  # MyGo 开发与打包配置
```

依赖方向为 `cmd/vitals → app → clipboard / config / translation / platform/macos`，
`config` 使用 `translation.Config`，与 `clipboard` 共用 `atomicfile`。存储、配置和翻译客户端不依赖 MyGo；
界面状态集中在 `app` 的主线程，存储由单独的串行 worker 管理，
通过 `Window.Update` 回传只读快照。翻译请求独立于存储 worker，通过可取消的 context 和请求序号防止过期响应覆盖页面。页面按文件拆分，复用同一份私有应用状态。

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

UI 使用 MyGo `ui.NewTester` 验证搜索、分类、右键菜单、即时粘贴、数字快捷键、编辑、翻译设置及快捷键录制；存储测试覆盖去重、收藏、保留条数、图片清理、磁盘写入失败及损坏文件。HTTP 测试使用本地测试服务验证 TranSmart 认证、语言识别和翻译方向、输入与响应上限、错误响应及两个请求阶段的取消；配置测试验证旧配置迁移及凭据文件权限。

可选的真实 API 集成测试只发送固定测试文字。在环境中设置 `TRANSMART_USER` 和 `TRANSMART_TOKEN` 后运行：

```bash
go test ./internal/translation -run TestTranSmartIntegration -v -count=1
```

没有配置凭据时该测试会跳过。测试检查中译英、英译中及短英文的输出方向；真实账号认证、额度与服务可用性需要有效凭据验证。
