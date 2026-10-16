# Vitals

基于 [MyGo](https://github.com/egoist/mygo) 的 macOS 13+ 原生菜单栏健康工具：

1. 在菜单栏实时显示 CPU、内存、GPU 与网络速率。
2. 展示应用内存占用，并允许确认后退出单个应用释放内存。
3. 将应用内存、启动项和磁盘占用聚合为应用健康档案。
4. 扫描应用本体和关联文件，确认后统一移入废纸篓。
5. 只读体检可访问的第三方 LaunchAgent 和 LaunchDaemon，在状态页展示健康摘要，并跳转 macOS 管理启动项。
6. 在 MacBook 上显示电量、充电状态、估算健康度和循环次数。
7. 支持简体中文与英文，默认跟随系统语言，可在设置中切换。

## 安装

从 [Releases](https://github.com/xbenduan/vitals/releases) 下载最新的 DMG，打开后将 Vitals 拖入「应用程序」。

由于 Vitals 目前使用本地签名，**尚未经过 Apple 公证**，首次打开会提示“无法验证是否具有危险性”。任选一种方式放行即可：

1. **系统设置放行（推荐）**：关闭弹窗后，打开「系统设置 → 隐私与安全性」，向下找到“已阻止使用 Vitals”的提示，点击「仍要打开」，再次启动即可。
2. **右键打开**：在 Finder 中右键点击 Vitals，选择「打开」，弹窗中会出现「打开」按钮（双击不会出现）。
3. **命令行移除隔离属性**：

   ```bash
   xattr -dr com.apple.quarantine /Applications/Vitals.app
   ```

## 截图

| 菜单栏状态 | 状态概览 |
| :---: | :---: |
| ![菜单栏状态](resources/bar.png) | ![状态概览](resources/status.png) |

| 内存占用 | 应用管理 |
| :---: | :---: |
| ![内存占用](resources/ram-status.png) | ![应用管理](resources/remove-app.png) |

| 设置 |
| :---: |
| ![设置](resources/settings.png) |

## 开发

需要 Go 1.27.1 或更高版本。

```bash
go tool mygo dev
```

## 验证与构建

```bash
GOFLAGS=-buildvcs=false go test ./...
GOFLAGS=-buildvcs=false go tool mygo vet .
GOFLAGS=-buildvcs=false go tool mygo build
```

构建产物位于 `build/darwin-arm64/`。

## 安全边界

- 单应用内存释放会正常退出目标应用，未保存内容可能丢失。
- 卸载候选项根据 bundle id、应用名、启动项配置与同厂商更新器匹配；高置信度项目默认选中，中置信度项目由用户决定。
- 卸载不会永久删除文件，所有确认项都会移入废纸篓。
- 启动项扫描完全在本机执行，不上传配置、路径、签名或可执行文件。
- 卸载应用时，Vitals 会先停止已选的关联 launchd 启动项，再将其配置移入废纸篓。
- 启动项管理统一跳转到 macOS“登录项与扩展”设置处理。
