# ClipBox

Windows 剪贴板历史工具。`Ctrl+Alt+V` 唤起，输入即搜，选中即粘贴回原窗口。

## 安装

到 [Releases](https://github.com/Know5/Clipbox/releases) 下载 `ClipBox-<版本号>-windows-amd64.zip`，解压到固定位置（如 `%LOCALAPPDATA%\Programs\ClipBox`）后运行 `ClipBox.exe`，无需安装。开机自启记的是这个文件的路径；升级时先在托盘菜单里退出，再用新版覆盖它。

- 需要 Microsoft Edge WebView2 运行时（Windows 11 和新版 Windows 10 已内置）。
- 未做代码签名，SmartScreen 拦截时点「更多信息」→「仍要运行」。
- 校验下载：同一 Release 的 `checksums.sha256`。
- 如果任务管理器 →「启动」里 ClipBox 被禁用(系统或优化工具关的),登录时会被静默跳过。设置 → 常规 → 开机自启 会提示,并可按「启用启动项」一键恢复。

## 用法

| 操作 | 按键 |
|---|---|
| 唤起 / 隐藏 | `Ctrl+Alt+V`（设置里可改） |
| 搜索 | 直接输入 |
| 选择 | `↑` `↓`；`Home` 回到最新 |
| 粘贴回原窗口 | 单击条目 或 `回车` |
| 只复制 | `Ctrl+回车` |
| 第 n 条 | `Ctrl+1`~`9` 复制，`Alt+1`~`9` 复制并粘贴 |
| 详情 | `Space` |
| 置顶 | `P` |
| 删除 | `Delete`（需二次确认） |
| 关闭 | `Esc`（有搜索词时先清空） |

右键条目或搜索框可打开菜单：复制、复制并粘贴、置顶、详情、删除。

界面顶部一行是搜索框、固定窗口、快捷键说明（`?`）和设置；下面一行是「全部 / 文本 / 图片」筛选、「标签」下拉和「⋯」菜单（清空未置顶记录）。列表单行显示，一屏约 15 条，前 9 条右侧标出序号。

`Ctrl+回车` 和 `Ctrl+数字` 默认只复制；在设置中打开「复制时顺带粘贴」后，它们也会直接粘贴。

## 功能

- 记录文本和图片，默认保留 500 条，超量自动删除最旧的未置顶项。
- 搜索正文、来源应用、窗口标题、备注和标签，按类型和标签筛选。
- 备注、标签、置顶；列表里显示图片尺寸和文件大小，原图在详情中按需加载。
- 来源应用显示为图标（取自该程序的 exe）；悬停条目可看到完整来源和窗口标题。
- 隐私：密码管理器加了系统隐私标记的内容不记录；可按进程/路径、窗口标题排除；可随时暂停记录。
- 深色 / 浅色 / 跟随系统三种主题，强调色跟随 Windows 系统强调色；选中行用强调色实底，文字颜色按对比度自动取黑或白。
- 托盘菜单、开机自启、单实例、窗口固定。
- 备份导出与合并导入（可恢复已有记录缺失的图片）、定时自动备份、诊断包（不含剪贴板正文）。
- 手动检查更新，不会自动联网。
- 常驻后台与界面分离：隐藏后按设置回收界面和 WebView2，后台继续记录；未保存的修改和处理任务会阻止回收。
- 图片本地 OCR、文字手动翻译；翻译服务地址、模型和 API Key 自行配置。

### 识别与翻译

1. 用 Snipaste 或其他工具截图并复制，让图片进入 ClipBox。
2. 右键图片 →「识别文字 / 翻译」→「识别文字」。识别结果可编辑，也可以只复制原文。
3. 在「设置 → 识别与翻译」填写 Chat Completions 兼容接口的地址、模型名称和 API Key，点击保存。地址支持 API 根地址（通常以 `/v1` 结尾）或完整的 `/chat/completions` 地址；除本机回环服务外需要 HTTPS。
4. 在识别结果中选择目标语言，主动点击「翻译」，再按需复制译文。已有文本记录可直接右键 →「翻译文字」。详情页也有入口。

OCR 使用 Windows 自带引擎，无需 Key，不上传图片。识别时临时启动隐藏的系统辅助进程，完成后退出，不额外捆绑模型。可在设置中选择系统已安装的 OCR 语言；缺少语言时，到 Windows「设置 → 语言和区域」安装对应的文字识别组件。

翻译只在点击「翻译」时将原文发送给所配置的服务，按该服务计费；打开记录、识别图片或更改目标语言都不会触发翻译。单次最多 20,000 字符，支持取消及超时提示。原始记录保持原样，复制结果后由现有记录规则决定是否加入历史。

API Key 使用 Windows 当前账户加密保存，不回传给界面，也不随备份或诊断包导出。留空保存会保留已有 Key；勾选「移除已保存的 Key」并保存后清除。迁移到其他电脑或 Windows 账户需要重新填写 Key。

图片详情使用最长边 1280 像素的预览，减少大图加载与显示内存；复制、导出和 OCR 使用原始图片。0.3.4 的性能改动、测量方法及限制见 [性能记录](PERFORMANCE_0.3.4.md)。

备份格式最多包含 10000 条记录，记录 JSON 上限 64 MiB，单张图片上限 50 MiB，图片合计上限 1 GiB。超限时会明确取消导出并保留原有备份；缺失或无法读取的图片会在导出结果中提示。

## 数据位置

历史存在 `%USERPROFILE%\.clipbox`：

- `clipbox.db`：历史记录、备注、标签和设置
- `images/`：图片内容
- `clipbox.log`：运行日志

退出或卸载都不会删除该目录，需要备份时用「设置 → 导出备份」。

## 常见问题

- **热键没反应**：组合键被其他软件占用，到设置里换一个。
- **热键设不上**：`Ctrl`/`Alt` + `1`~`9`、`Ctrl+回车`、`Ctrl+C`/`V`/`X` 是列表和粘贴要用的键，不能当唤起热键。旧版本设成这些的，升级后首次启动会改回 `Ctrl+Alt+V` 并提示一次。
- **复制了但列表没变化**：文件、富文本和超大内容不记录；剪贴板被其他程序占用；命中排除规则或敏感内容。界面会给一行提示。
- **粘贴没落到目标窗口**：目标窗口以管理员权限运行或系统阻止了焦点切换。内容已在剪贴板，手动 `Ctrl+V` 即可。
- **开机不自启**：自启只记一个 `ClipBox.exe` 的路径。那个文件被删除或移走后，下次运行 ClipBox 会自动改指当前这份；另一份还在时不会被抢走，想换成当前这份，到设置里把「开机自启」关掉再打开。
- **搜索不准**：「设置 → 修复存储」重建索引。

## 开发

需要 Go 1.26+（`go.mod` 固定 `toolchain go1.27.3`，缺了会自动下载）、Node.js 20.19+ 或 22.12+、Wails CLI v2.17.1 和 WebView2。

```powershell
git clone https://github.com/Know5/Clipbox.git
cd Clipbox
wails dev     # 开发模式（前端热更新）
wails build   # 产物：build\bin\clipbox.exe
```

技术栈：Go + Wails v2，React + TypeScript，SQLite FTS5。

生产版为常驻 Go 后台加按需界面进程，开发模式继续支持 Wails 热更新。模块职责和生命周期见 [架构文档](ARCHITECTURE.md)。

运行检查时先构建前端（Go 主程序会嵌入 `frontend/dist`）：

```powershell
cd frontend
npm ci
npm test
npm run lint
npm run format:check
npm exec -- tsc --noEmit --noUnusedLocals --noUnusedParameters
npm run build
cd ..
go test -count=1 ./...
go vet ./...
```

验证生产版的进程回收与恢复：先构建，再设置 `CLIPBOX_APP_EXE` 为 EXE 的绝对路径，执行 `go test -count=1 -run TestProductionResidentLifecycle -v .`。测试使用独立数据目录、独立热键，并禁止写真实自启配置。设置 `CLIPBOX_LIFECYCLE_CYCLES=50` 可扩展循环次数。

内存测量使用 `check_mem.ps1`。它按后台及其子进程汇总，区分私有提交内存和私有工作集；不能把大图处理期间的累计分配量当作常驻内存。开发依赖中的 Go 示例通过安装脚本与主模块隔离，不参与应用测试。

前端测试使用 Vitest 和 React Testing Library，覆盖历史事件、手动删除和置顶、分页竞态和图片元数据请求。Windows CI 会从干净源码按同一顺序执行检查，并构建应用。

目录：`internal/clipboard`（剪贴板）、`internal/hotkey`（全局热键）、`internal/storage`（存储与搜索）、`internal/windowutil`（窗口焦点与粘贴）、`frontend/src`（界面）。

## 发布

```powershell
.\scripts\package-release.ps1
```

依次跑 `npm ci`、前端测试、ESLint、格式检查、TypeScript 检查与构建、`go test`、`go vet`、`npm audit`、`wails build -s -trimpath` 和生产生命周期测试，产物在 `dist\release`：ZIP 包、`release-manifest.json`、`update-manifest.json`、`checksums.sha256`，以及每个版本 ZIP 对应的 `.sha256` 文件。

把 ZIP、`update-manifest.json`、`checksums.sha256` 上传到 GitHub Releases，并把 `update-manifest.json` 的下载地址填进设置页的更新源。加 `-BuildInstaller` 同时生成 NSIS 安装器，加 `-SkipBuild` 复用已有构建。

## License

[MIT](LICENSE)
