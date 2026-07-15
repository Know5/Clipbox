# ClipBox

ClipBox 是一款面向 Windows 的轻量级剪贴板增强工具。它会自动记录文本和图片剪贴板内容，通过全局快捷键快速唤起历史面板，帮助用户在写作、开发、客服、运营等高频复制粘贴场景中更快找回、搜索和复用内容。

项目基于 Wails v2、Go、React 和 TypeScript 构建：Go 负责系统剪贴板监听、全局热键、窗口控制和本地存储，React 负责桌面端交互界面。

## 核心功能

- 自动记录剪贴板历史，支持文本和图片。
- 默认使用 `Ctrl + Alt + V` 唤起 ClipBox，快捷键可在设置页录制修改。
- 全键盘操作：唤起后直接输入即可过滤，`↑↓` 选择、`回车` 粘贴、`Space` 查看详情、`Del` 删除、`Esc` 逐层关闭。
- 支持深色 / 浅色 / 跟随系统三种主题。
- 遵守 Windows 剪贴板隐私标记（`ExcludeClipboardContentFromMonitorProcessing` 等）：1Password、KeePass、Bitwarden 等密码管理器复制的内容不会被记录。
- 支持按文本内容、来源应用、窗口标题、备注和标签搜索历史记录，并使用 SQLite FTS5 索引提升长期使用后的搜索性能。
- 支持按类型筛选全部、文本、图片，并可按标签快速筛选。
- 支持分页加载历史记录，长期使用时可继续加载更多结果。
- 支持记录来源应用和窗口标题，列表中可快速判断内容来自哪里，并可在设置中关闭。
- 支持查看记录详情，长文本可阅读全文，图片可按需查看原图，并可为记录编辑备注、标签或导出单条文本/图片。
- 点击历史项即可写回剪贴板，界面会显示复制成功/失败反馈，并可选择是否自动粘贴到上一个活动窗口。
- 支持置顶常用记录，清空历史时会保留置顶项。
- 支持二次确认删除单条记录，并可清空非置顶记录。
- 图片按需加载预览，避免列表页一次性加载大图。
- 支持暂停记录、敏感文本跳过、图片记录开关和最小文本长度过滤；设置中的开关和输入项即时保存，无需手动点击保存。
- 支持按前台应用进程/路径关键词和窗口标题关键词排除记录，并可一键把上个目标窗口加入排除规则，适合密码管理器、密钥工具等隐私场景。
- 支持历史数量、保留天数和图片空间上限策略，并可在设置页查看存储统计、手动清理、修复缺失图片记录和打开数据目录。
- 支持导出 `.clipbox-backup` 备份包，并可合并导入历史记录、图片、备注、标签和本地设置；也可配置启动时按间隔自动写入本地备份并保留指定数量。
- 支持导出诊断包，包含版本、设置快照、存储统计和运行日志，不包含剪贴板正文。
- 支持配置发布清单 URL，在设置页手动检查新版本，并可手动下载更新包和校验 SHA256，不会自动启动联网检查。
- 支持开机自启和单实例启动，重复启动会唤起已有窗口。
- 支持系统托盘入口，可显示/隐藏窗口、打开设置、暂停/恢复记录、打开数据目录和退出应用。
- 窗口可固定显示；未固定时，粘贴后自动隐藏。
- 本地 SQLite 存储，数据默认保存在用户目录，不依赖云服务。

## 适用平台

当前代码使用 Windows 剪贴板、热键和窗口 API，因此主要面向：

- Windows 10 / Windows 11
- Microsoft Edge WebView2 Runtime

如需支持 macOS 或 Linux，需要替换 `internal/clipboard`、`internal/hotkey`、`internal/windowutil` 等平台相关实现。

## 技术栈

- Desktop: Wails v2.12.0
- Backend: Go
- Frontend: React 18、TypeScript、Vite
- Storage: SQLite, FTS5, `modernc.org/sqlite`
- Windows API: `user32.dll`、`kernel32.dll`

## 项目结构

```text
.
├── api.go                         # 暴露给前端调用的应用 API
├── app.go                         # 应用启动、关闭、热键和窗口状态管理
├── main.go                        # Wails 入口和窗口配置
├── wails.json                     # Wails 项目配置
├── internal/
│   ├── clipboard/                 # Windows 剪贴板监听、读取和写入
│   ├── hotkey/                    # 全局快捷键注册和重绑定
│   ├── storage/                   # SQLite 存储、搜索、置顶和清理
│   └── windowutil/                # 前台窗口、聚焦和 Ctrl+V 模拟
├── frontend/
│   ├── src/                       # React 页面和样式
│   └── wailsjs/                   # Wails 生成的前后端绑定
└── build/                         # 图标、Windows manifest、安装器配置和构建输出目录
```

## 本地数据

运行后，ClipBox 会在当前 Windows 用户目录下创建数据目录：

```text
%USERPROFILE%\.clipbox
```

主要文件：

- `clipbox.db`: 剪贴板历史、置顶状态、备注、标签和快捷键设置。
- `images/`: 图片剪贴板内容保存为 DIB 文件。

图片不会以 base64 大字段直接存入 SQLite，列表查询也不会把图片数据一次性传给前端，这样可以减少数据库膨胀和内存占用。

## 开发环境准备

建议使用 PowerShell。需要先安装：

- Go，版本以 `go.mod` 为准。
- Node.js 和 npm。
- Wails v2 CLI。
- Windows WebView2 Runtime。多数 Windows 10/11 环境已经内置，可通过 `wails doctor` 检查。

安装 Wails v2 CLI：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor
```

如团队需要完全锁定构建环境，可将 Wails CLI 安装版本固定到项目使用的 v2 系列版本。

## 本地开发

克隆仓库后进入项目目录：

```powershell
git clone https://github.com/Know5/Clipbox.git
cd Clipbox
```

安装前端依赖：

```powershell
cd frontend
npm install
cd ..
```

下载 Go 依赖：

```powershell
go mod download
```

启动开发模式：

```powershell
wails dev
```

开发模式会同时启动 Wails 桌面窗口和 Vite 前端服务，修改前端代码后可以热更新。

## 构建部署

生成 Windows 可执行文件：

```powershell
wails build
```

构建完成后，产物位于：

```text
build\bin\clipbox.exe
```

部署到目标 Windows 电脑时，复制 `clipbox.exe` 即可作为便携版运行。首次启动会自动创建 `%USERPROFILE%\.clipbox` 数据目录。

如果需要生成指定平台构建，可使用：

```powershell
wails build -platform windows/amd64
```

如需制作安装包，可安装 NSIS 后生成安装器：

```powershell
wails build --nsis
```

安装器会创建桌面快捷方式、开始菜单文件夹、卸载入口和 README 快捷方式；卸载时会清理快捷方式和 ClipBox 的开机自启项，但不会默认删除 `%USERPROFILE%\.clipbox` 中的用户数据。

## 发布流程建议

1. 确认功能代码和 README 已更新。
2. 在本机执行 `wails doctor`，确认 Wails 环境正常。
3. 执行发布打包脚本：

```powershell
.\scripts\package-release.ps1
```

脚本会依次执行 `go test ./...`、`go vet ./...`、`npm audit --audit-level=high` 和 `wails build`，然后生成便携发布包：

```text
dist\release\ClipBox-<version>-windows-amd64.zip
dist\release\ClipBox-<version>-windows-amd64\release-manifest.json
dist\release\update-manifest.json
dist\release\checksums.sha256
```

如需在发布流程中同时生成并收进安装器，可使用：

```powershell
.\scripts\package-release.ps1 -BuildInstaller
```

如果已经提前运行过 `wails build --nsis`，也可以只把现有安装器收进发布目录：

```powershell
.\scripts\package-release.ps1 -SkipBuild -IncludeInstaller
```

如需让设置页的更新检查同时显示下载按钮、发布说明地址并校验更新包，可在打包时写入可选字段；脚本会自动把 ZIP 包 SHA256 写入 `update-manifest.json`：

```powershell
.\scripts\package-release.ps1 -DownloadUrl "https://example.com/ClipBox-0.2.0-windows-amd64.zip" -ReleaseNotesUrl "https://example.com/releases/0.2.0"
```

4. 将 `dist\release` 中的 ZIP、`update-manifest.json`、校验文件和可选安装器上传到 GitHub Releases，并把可公开访问的 `update-manifest.json` URL 填入设置页的更新源。

## 常用命令

```powershell
# 前端独立构建
cd frontend
npm run build
cd ..

# Wails 开发模式
wails dev

# 生产构建
wails build

# 发布打包
.\scripts\package-release.ps1

# 发布打包并生成 NSIS 安装器
.\scripts\package-release.ps1 -BuildInstaller

# 检查 ClipBox 及 WebView2 相关进程内存
.\check_mem.ps1
```

## 注意事项

- 当前版本偏向 Windows 桌面使用场景，不建议直接作为跨平台应用发布。
- 全局快捷键如果被其他软件占用，应用会提示注册失败，需要在设置中更换组合键。
- 文本记录超过安全阈值、图片过大或剪贴板格式不支持时，可能不会被记录。
- 来源窗口标题可能包含文档名、网页标题等上下文信息，如不希望保存，可在设置中关闭“记录来源窗口”。
- 搜索索引会随记录写入、删除和备注/标签更新自动同步；如搜索异常，可在设置页执行“修复存储”重建索引。
- 自动备份只会轮转 `clipbox-auto-*.clipbox-backup` 文件，不会清理手动导出的备份包。
- 备份导入会校验文件数、记录数、清单大小和图片大小；过大的图片会被跳过，异常备份包会被拒绝。
- `frontend/dist`、`frontend/node_modules` 和 `build/bin` 属于构建或依赖产物，不需要提交到仓库。

## License

如需开源发布，请在仓库中补充明确的 License 文件。
