# ClipBox 进度记录

## 第1步:粘贴可靠性(代码已完成,待手动验收)

日期:2026-10-11

### 改动
- `internal/windowutil/window_windows.go`
  - `FocusWindow` 返回 bool:`SetForegroundWindow` 后轮询前台窗口,最多 100ms,去掉固定 Sleep(60ms/20ms)。
  - `SendCtrlV` 返回 error:检查 `SendInput` 注入的事件数是否为 4。
  - 新增 `waitForForeground`,便于测试。
- `app.go` `pasteToLastTarget`:去掉 Sleep(150ms/100ms);失败只写日志,不弹提示(与 Win+V 一致)。
- 新增 `internal/windowutil/window_windows_test.go`:覆盖焦点成功与超时两个分支。

### 自动验证
- `gofmt`、`go vet`、`go test ./...` 全部通过。

### 待手动验收(规格第 4 节)
- [ ] 记事本 20 次粘贴成功
- [ ] Chrome 20 次粘贴成功
- [ ] VS Code 20 次粘贴成功
- [ ] 管理员窗口一次,确认失败时日志有记录且无崩溃
- [ ] 记录"回车到内容出现"耗时

### 已知风险
- 去掉固定延时后,个别程序可能需要极短等待才接收按键。若出现,再加小的兜底延时并记录原因。

## 决定记录

- **窗口高度固定,不做随条目自适应**(使用者决定,不再调整)。
  - 注:Maccy 源码中窗口高度会随条目数变化(`Observables/Popup.swift` 的 `suitableHeight`)。使用者选择保持固定,以此为准。
- 自动粘贴默认关,历史上限 500 条(使用者确认 B 方案)。数据库 `auto_paste` 已改为 false,改动前的数据库已另存备份。
- 开机自启已存在且状态正确(注册表 Run 项 + `start_at_login = true`),无需改动。

## 历史清理规则(已确认)

- **自动裁剪**:每次新记录入库后,按设置执行 `applyCurrentCleanup`:超过 `max_clips`(当前 500)删最旧的未置顶项;图片总量超过 `maxImageStorageMB`(当前 1024MB)删最旧的图片。
- **"设置 > 立即清理"**:只执行上面的裁剪规则,不会清空。未超上限时不删任何东西。
- **主界面"清空"**:手动删除所有未置顶项及其图片,置顶项保留。不会按数量或大小自适应。
- 使用者确认:接受按数量和大小自动裁剪,不要求清空按钮自适应。

## 第1.5步:2026-10-29 自启失效归因 + 打开慢排查 + 交互统一

### 已修(代码)
- `startup_windows.go` 新增 `underTest()`:测试二进制(`.test.exe`)一律不写 HKCU Run。
  - 归因:`TestUpdateAppSettingsFailureKeepsPersistedState` → `UpdateAppSettings` → `reconcileStartAtLogin(true)`
    → 把测试自己的临时路径写进 Run;DB 写失败后的"回滚"因注册表已指向自己而空转。
  - 后果:用户重启后自启指向已删除的 `Temp\go-build*\clipbox.test.exe`,静默不启动(日志 2026-10-27 后无 startup)。
  - 实验证据:单跑该测试,Run 值从 `go-build832412524` 变为 `go-build1350816098`。
- 新增回归测试 `TestSetStartAtLoginNeverWritesFromTestBinary`;`go test ./...` 后 Run 值保持不变(已验证)。
- 现场修复:启动当前构建触发自愈,Run 现指向 `build\bin\clipbox.exe --hidden`。
- `main.go`:`WebviewGpuIsDisabled` true → false(GPU 合成),待体感验证;黑窗/闪烁则改回 true。
- `App.tsx`:回车 = 复制并粘贴(与单击统一);Ctrl+回车 = 只复制。原来回车只复制,看起来像无反应。
- `app.go`:`unsupported type`(复制文件/富文本)时前端提示一行,10 秒限频一次;`read failed` 仍只记日志。

### 已测数据(说明"打开慢"与数据量无关)
- 全库 500 条 = 1.3 MB;最新 50 条全字段 115 KB;平均文本 67 B、缩略图 550 B;SQL 毫秒内。
- 结论:提高 `max_clips` 不会拖慢打开;"不敢提上限"的前提不成立。

### 待用户手动确认
- [ ] 热键是 **Ctrl+1**(DB: modifiers=2, key=49),不是规格里的 Ctrl+Alt+V。是否为有意设置?
- [ ] 回车能粘贴回原窗口;Ctrl+回车只复制。
- [ ] 复制文件时出现"不是文本或图片"提示。
- [ ] GPU 渲染下窗口无黑屏/闪烁。

## 第 2 步:2026-11-06 0.2.2 维护版

### 改动
- 工具链与依赖:`go.mod` 改为 go 1.26.0 + `toolchain go1.27.3`;Wails v2.12.0 → v2.17.1,x/net v0.62.0、x/sys v0.49.0、x/text v0.43.0、modernc sqlite v1.61.1;前端按 semver 范围更新(vite 8.3.6、@vitejs/plugin-react 6.2.1)。本机 Wails CLI 升到 v2.17.1。
  - 依据:0.2.1 用 go1.25.3 构建,`govulncheck` 源码分析可达 22 个标准库漏洞;新构建 `govulncheck -mode=binary` 无漏洞。
- 开机自启(`startup_windows.go` `startupEntryNeedsRepair`):启动和保存设置时,只在 Run 值缺失、指向自己但缺 `--hidden`、或指向的 exe 已不存在时改写;指向另一份仍存在的副本时不动。
  - 归因:Run 值指向已删除的 `dist\release\verify-exe.exe`(发版验证的临时副本),11-01 之后开机不再启动。旧逻辑只要 Run 值不指向当前 exe 就改写,任何临时副本运行一次都会抢走自启。
  - 代价:新版本解压到新目录运行时不会自动接管,需在设置里把开机自启关掉再打开(README 已写)。
  - 回归测试 `TestStartupEntryNeedsRepair`。
- 唤起热键(`api.go` `reservedHotkeyReason`):拒绝 Ctrl/Alt/Ctrl+Alt + 1~9(含小键盘)、Ctrl+回车、Ctrl+C/V/X。启动时存量热键命中则改回 Ctrl+Alt+V、落盘,并异步弹一次提示(`app.go` startup)。
  - 归因:本机热键是 Ctrl+1,全局注册后列表里 Ctrl+1(复制第 1 条)失效,其他程序里的 Ctrl+1 也失效。
  - 回归测试 `TestReservedHotkeyReason`。
- 版本号 0.2.2(`metadata.go`、`wails.json`)。

### 修复:隐藏启动抢焦点(0.2.1 起就有)
- 预热(`app.go` startup 末尾 `runtime.WindowShow` → `WindowHide`)整个去掉。Wails v2.17.1 的 `WindowShow` 会调用 `SetForegroundWindow` + `SetFocus`(`internal/frontend/desktop/windows/frontend.go` `ShowWindow`),隐藏启动后把用户当前窗口(本机是全屏游戏)的前台和键盘焦点抢进 ClipBox;按键落进列表就会 `CopyAndPaste`,窗口却始终不可见。
  - 现象(0.2.1/0.2.2 实测):隐藏启动 2 秒后日志出现 `CopyAndPaste called id=22`、`paste start target=0`,测试没有发送按键。
- 为什么不是"改成屏幕外不激活显示":实测这次显示+隐藏之间的可见期不到 5 ms(5 ms 轮询只抓到 1 个样本),60Hz 下连一帧都排不出来;而且屏幕外的窗口不在任何显示器上,Chromium 会把它算作被遮挡、暂停出帧[推断]。这个预热从 0.2.1 起就没画过东西,只是白抢焦点;留着还多一次启动时动窗口的风险。
  - 代价:首次唤起要付 WebView2 首次合成的开销。若体感变慢,再做"真的预热"并先测量,不凭猜。

### 修复:窗口定位与主窗口查找
- `showWindowInternal` 原用 `runtime.WindowSetPosition` 定位。Wails 把参数当作**相对窗口当前所在显示器工作区原点**的偏移(`winc/controlbase.go` `SetPos` 加 `workRect.Left/Top`),而 `windowutil.PlaceNearCursor` 返回的是屏幕绝对坐标。单屏且任务栏在底部时工作区原点是 (0,0),看不出问题;任务栏在顶部/左侧或窗口在副屏时会多挪一个工作区原点,可能挪出屏幕。
- 改为 `windowutil.MoveWindow`(直接 `SetWindowPos`,带 `SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE`)。
- 找窗口从 `FindWindowByTitle` 换成 `windowutil.FindOwnWindow(类名, 标题)`:只按标题找会命中别的进程的同名窗口(例如资源管理器打开名为 `ClipBox` 的文件夹),按类名 + 本进程 PID 过滤。主窗口类名由 `mainWindowClass` 写进 Wails 选项(`main.go` `WindowClassName`)。

### 自动验证
- `gofmt`、`go vet`、`go test ./...` 通过;`wails build -trimpath` 产物文件版本 0.2.2.0、go1.27.3,`govulncheck -mode=binary` 无漏洞。

### 实机验证(临时 USERPROFILE,隔离数据目录)
- 隐藏启动 0.9~1.4 秒内托盘就绪;主窗口在整个启动过程中**一次都不可见**(5 ms 轮询 3 秒,可见样本 0),位置保持不动。
- 前台窗口全程是原窗口(pid 未变),ClipBox 从未取得前台;日志无 `CopyAndPaste`。另建一个"同类名同标题"的隐藏窗口(另一个进程)干扰,应用仍能找回自己的窗口。
- 托盘"显示"路径:6 ms 内窗口可见,出现在光标附近、工作区内。
- Run 值从已删除的 `verify-exe.exe` 自动改指 `%LOCALAPPDATA%\Programs\ClipBox\ClipBox.exe --hidden`(本机日常版已部署到这里);本轮测试前后 Run 值不变。
- 存量热键 Ctrl+1:启动时弹「唤起热键已改回默认」,DB 改为 modifiers=3、key=86,Ctrl+Alt+V 被注册,Ctrl+1 未被占用。
- 单实例:第二个进程 0.10 秒退出,原实例弹出窗口。
- 设置页在 WebView2 + Wails v2.17.1 下渲染正常,绑定(存储统计)和托盘事件(打开设置)可用。

### 待手动验收
- [ ] 真实键盘按 Ctrl+Alt+V 唤起、回车粘贴到记事本/Chrome/VS Code。注入的按键在本机被丢弃(自己注册的 Ctrl+Alt+Shift+F12 也收不到 WM_HOTKEY),推断是游戏反作弊的键盘钩子,未能自动测。
- [ ] 下次登录自动启动;首次启动弹一次热键改回提示。

## 第 3 步:2026-11-06 0.3.0 界面改版

### 改动
- **列表改单行**(`frontend/src/App.tsx` `ClipItem`):文本行 32px、图片行 48px,一屏约 15 条(原 6 条)。行内:序号 · 来源图标(或 32×32 缩略图)· 单行预览 · 图钉 · 时间;1~9 行右侧标出序号,对应 `Ctrl+数字` / `Alt+数字` 的前 9 条。来源应用名移到悬停提示(仍是「应用 · 窗口标题」)。
- **头部合并**:标题栏与搜索框并成一行 36px,窗口拖动区不变;去掉最小化按钮。按钮:固定窗口、快捷键说明(`?`)、设置、关闭。设置页打开时只留拖动区和关闭。
- **筛选行**:`全部 / 文本 / 图片` 改分段控件;标签收进「标签」下拉;「清空未置顶记录」收进「⋯」菜单。底栏快捷键提示删除,改由 `?` 面板列出全部快捷键(空列表时也提示入口)。
- **选中/置顶**:选中改强调色实底,文字颜色按强调色亮度在白/近黑中取对比度更高者(本机 #0078D4 用白字 4.53:1;浅蓝 #4ca6ff 用近黑 6.9:1)。置顶改为行内图钉 + 未选中时淡强调色底,不再用左侧竖条,避免和选中态混淆。
- **样式收口**(`App.css` 重写为令牌驱动):字号 12/13/14 三档,字重 400/600 两档,圆角 4/8;状态色改用 `color-mix()` 从 `--accent` / `--danger` 派生;浅色主题 `--text-tertiary` 0.55 → 0.64。
- **搜索命中加粗**(`Highlight`),按空白拆词、逐词加粗。
- **后端新增**:
  - `App.GetSystemAccentColor` + `windowutil.SystemAccentColor/AccentColorHex`:读 `HKCU\Software\Microsoft\Windows\DWM\AccentColor`(AABBGGRR),前端在启动、切主题、每次唤起时刷新。
  - `App.GetSourceIcon` + `windowutil.ExtractIconDataURL`:按 `source_path` 用 `ExtractIconEx` 取图标 → PNG data URL;进程内按小写路径缓存 64 条,失败也缓存,不重复记日志。
  - `App.GetClipImageMeta` + `clipboard.DibDimensions`:批量返回图片尺寸(DIB 头)和文件大小,前端按需批量取并记住,不重试失败项。
- **设置页分 4 个标签**(常规 / 隐私 / 存储 / 高级):热键、主题、开机自启、复制时顺带粘贴归常规;记录规则归隐私;统计与保留归存储;备份、诊断、更新、数据目录、退出归高级(规格 2.3 的「运维功能收起来」)。
- **窗口底色跟随主题**:Wails `BackgroundColour` 默认 #202020;前端每次切主题调 `WindowSetBackgroundColour`,浅色主题下首次绘制不再闪深色。
- **清理**:删除未被引用的 Nunito 字体(原本打进 exe)和 `logo-universal.png`;删掉顶部「最小化」和底栏提示的 CSS 与代码。
- 版本号 0.3.0(`metadata.go`、`wails.json`)。

### 实测数据(浏览器原型 + 真实 WebView2)
- 一屏条数:真实窗口 400×560 CSS(本机 175% 缩放 → 700×980 物理像素),头部 36 + 工具栏 30,列表 494px → **15.4 行**;原型实测 15.4 行,与真实窗口一致。
- 对比度(选中行按强调色实底计算):
  - 深色:次要文字 6.34:1,预览 15.11:1,分段控件 9.04:1。
  - 浅色:次要文字 5.05:1(改前 3.72:1),预览 14.8:1。
  - 选中行:强调色 #0078D4 + 白字 4.53:1(达到 WCAG AA);浅蓝强调色自动改用近黑字。
- 真实 WebView2(隔离数据目录 + 种子数据,窗口用 `SWP_NOACTIVATE` 显示后 `PrintWindow`,未抢前台):
  - 两套主题均正常渲染;来源图标取到 Chrome / VS Code / WeChat / PowerShell / Windows Terminal / ClipBox 自身;图片行显示 `3840×2160 · 31.6 MB`、`1280×720 · 3.5 MB`(与 DIB 头、文件大小一致);序号 1~9、图钉、时间列对齐。
  - 日志无 ERROR。
- 设置页 4 个标签的内容高度:常规 668 / 隐私 741 / 存储 547 / 高级 809(视口 440),热键从原来的第 1597px 提前到常规页第一屏内。

### 部署验证(本机日常版)
- `%LOCALAPPDATA%\Programs\ClipBox\ClipBox.exe` 已换成 0.3.0(16,633,344 字节)。
- 隐藏启动:托盘 0.79 秒就绪;3 秒内主窗口一次都不可见,前台窗口零变化(没有抢焦点);托盘退出正常;Run 值未变。
- 真实库里存的唤起热键还是 Ctrl+1,本次启动按规则改回 Ctrl+Alt+V 并弹出一次提示(与第 2 步的规则一致),已落盘。

### 自动验证
- `gofmt -l .`、`go vet ./...`、`go test ./...`、`npx tsc --noEmit`、`vite build`、`wails build -trimpath` 全部通过。
- `update_test.go` 的「有新版本」样例从 0.3.0 改为 9.9.9,不再随版本号失效。

### 待手动确认
- [ ] 真实键盘唤起后,选中行用强调色实底是否顺眼;浅色主题下是否够清楚。
- [ ] 一屏 15 条是否足够(如需更多,只能减行高或加窗口高度)。
- [ ] 来源图标在 `%LOCALAPPDATA%` 下安装的普通程序中是否都能取到。

## 第 4 步:2026-11-09 开机自启再次失效的归因(0.3.1)

### 现象与证据
- 11-06 部署 0.3.1 之前之后都没有新的登录启动记录(`%USERPROFILE%\.clipbox\clipbox.log` 最后一次 `startup ... startHidden=true` 是 11-06 的手动运行)。
- Run 值正确:`"C:\Users\QGS\AppData\Local\Programs\ClipBox\ClipBox.exe" --hidden`,exe 存在且为 0.3.0。
- 真正原因:Windows 自己的启动项开关是关的。
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run\ClipBox` = `03 00 00 00 CA FE 4F 0E 0D 5D DD 01`,首字节 `0x03` = 已禁用,时间戳 **2026-10-16 09:24:14**(本地)。
  也就是说从 10-16 起,无论 Run 值怎么修,Windows 每次登录都直接跳过 ClipBox;开发期一直手动启动,所以没人发现。不是 0.3.0 的回归,也不是自启逻辑写错。

### 修复
- `startup_windows.go`:读取并解码 `StartupApproved\Run`(0x02 启用、0x00 未记录,其余视为禁用);`setStartAtLogin(true)` 在写 Run 值之后把标志写回 `02 00 00 00` + 零时间戳(Task Manager 自己写的就是这个形状)。
- 启动时:意图为开但被 Windows 禁用 → 只写日志提示,**不偷偷改回**(可能是用户自己在任务管理器里关的)。
- 设置页(常规 → 开机自启):检测到被禁用时显示提示和「启用启动项」按钮(新 API `RepairStartAtLogin`),不再让用户猜。
- 回归测试 `TestStartupApprovedDisabled`(缺值/启用/未记录/0x03/0x06/0x0a)。

### 验证
- 标志保持 `0x03` 启动 0.3.1:日志出现 `start-at-login entry is disabled by Windows startup manager`。
- 把标志写回 `02 00 00 00`(与代码写入的字节相同)后启动:不再提示,Run 值不变。
- 部署 0.3.1 到 `%LOCALAPPDATA%\Programs\ClipBox\ClipBox.exe`:隐藏启动 0.39 秒托盘就绪,前台零变化,托盘退出正常。
- 本机启动项已重新启用;下次登录应自动启动。若再被某些「启动项管理」工具关掉,设置页会直接提示。

### 仍不能保证的部分
- 如果外部工具(杀软、优化软件、系统设置)把启动项再关一次,任何程序都无法单方面阻止;能做的是**检测到并明确告诉用户**,这版已经做到。

## 第 5 步:2026-11-09 清理死代码与旧启动器(0.3.2,无行为变化)

### 删除的内容
| 项 | 原因 |
|---|---|
| `cmd/hotkeytest/` | 独立的调试用 main 程序,全仓库零引用 |
| `check_ci.ps1` | 查询 GitHub Actions 运行记录,但 CI 工作流已在 07-15 删除 |
| `build/darwin/`、`build/README.md` | 项目只支持 Windows;`build/README.md` 是 Wails 模板说明,且指向已删除的 darwin |
| `dist/release/ClipBox-0.2.1-*`、`notes.md` | 旧版本产物,只保留最新 0.3.1/0.3.2 的 ZIP 与校验文件 |
| `.github/`(空目录) | 无内容 |
| `internal/clipboard.WriteImageToClipboard` | 无调用的别名(真正的实现是 `WriteDibToClipboard`) |
| `internal/clipboard` 的 `CF_BITMAP`、`WM_DESTROY` | 未被引用的常量 |
| `internal/windowutil` 的 `swShow`、`swShowNoActivate` | 未被引用的常量 |
| `settings.go shouldCapture` | 测试专用的 `prepareClipEntry` 包装,生产代码零调用 |
| `startup_windows.go startupValuePointsToSelf` | 同上;测试改为直接用 `startupCommandMatchesExecutable` |
| `frontend/src/App.tsx clipSourceLabel` | 重写列表后不再被引用 |

### 保留并说明理由
- `check_mem.ps1`:规格第 3 节仍要用它测空闲内存,是还能用的手动工具。
- `build/windows/installer/*`:`scripts/package-release.ps1 -BuildInstaller` 仍然依赖。
- `frontend/package.json.md5`:Wails 判断是否需要 `npm install` 的缓存标记。

### 验证
- `gofmt -l .` 无输出;`go vet ./...`、`go test ./...` 通过;`tsc --noEmit --noUnusedLocals --noUnusedParameters` 无告警;`wails build -trimpath` 成功。
- 版本 0.3.2 只为让产物和源码一致(0.3.1 的 ZIP 里是清理前的二进制),功能与 0.3.1 相同。
