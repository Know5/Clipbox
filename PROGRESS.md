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

## 第 6 步：2026-11-09 代码评审整改（0.3.3，本地构建，未发布）

### 按优先级完成的修复

1. 图片删除与记录删除放在同一数据库事务中登记，提交后再回收文件。新增 `pending_image_deletes` 持久化队列，被占用的文件会在清理或下次启动时重试；批量删除失败会回滚整批记录。跳过、去重和保存失败的图片也通过引用检查安全回收。
2. 粘贴前等待 Ctrl/Alt/Shift/Win 释放，最多等待 500ms，再切换焦点并发送普通 Ctrl+V；中途新按下修饰键会取消注入。复制/粘贴操作串行执行，API 在得到注入结果后返回，失败能正确反馈给界面。
3. 捕获事件使用数据库保存后的条目，保留去重时已有的置顶、备注、标签和缩略图。
4. 捕获、手动删除、置顶、备注和清理统一发送 `clips:updated`，携带最终条目、删除的 ID 和实际总数。后端从写入到事件发送串行执行，前端按事件更新，避免再次扣减计数或被迟到的详情响应覆盖。前端限制后台缓存容量；正在加载的旧请求从更新后的偏移重新请求，避免恢复已删除记录。取消置顶可能改变分页边界时刷新第一页；再次唤起会取消未完成的下一页请求。
5. 图片尺寸请求拆到 `useImageMetadata`，缓存请求与结果并分别管理订阅者；支持列表变化、乱序返回、StrictMode 和失败重试，删除记录后不会被迟到响应恢复。
6. 托盘图标按控制器缓存，只在添加图标时提取，退出时释放自己拥有的 HICON，共享系统图标不销毁；隐藏窗口由创建它的线程销毁，退出会关闭动作通道。
7. 备份导出与导入使用一致的数量、JSON 大小和图片大小限制。记录 JSON 逐条编码，超限取消导出并保留旧备份；导入损坏图片时先关闭文件句柄再删除临时文件。合并导入可以修复已有记录缺失的图片，并保留 ID、置顶和备注；重复导入的图片在事务提交后通过回收队列处理，文件被占用时也能重试。
8. 标签筛选使用与 Go 统计一致的 Unicode 小写转换，`Work/work` 以及非 ASCII 标签的数量和筛选结果一致，保留原显示名称。
9. 发布脚本从 `npm ci`、前端测试、TypeScript 严格检查和构建开始，再运行 Go 检查及 Wails 打包；Wails 安装依赖也改为 `npm ci`。增加 Windows CI，第三方 Actions 固定到已核实的提交；发布清单和每个版本的 ZIP 校验文件同时生成。

### 验证

- `gofmt`、`git diff --check`、`go test -count=1 -cover ./...`、`go vet ./...` 通过。
- 前端 22 项自动化测试、TypeScript 严格检查和 Vite 生产构建通过。
- `npm audit`、Go 源码和成品 EXE 的 `govulncheck` 未发现已知漏洞；测试依赖采用已修复安全问题的 Vitest 4.1.12。
- 原生托盘隐藏窗口启动、退出和资源释放测试通过。
- 复制当前源码到不含 `node_modules` 和 `frontend/dist` 的临时目录，完整执行发布脚本成功，生成 0.3.3 Windows EXE、ZIP、发布清单和 SHA256 校验文件。
- 成品已同步到 `dist/release`；逐个验证 ZIP 内文件与发布清单、更新清单中的大小和 SHA256 一致。验证报告位于 `dist/release/validation-0.3.3.json`，本次 ZIP SHA256 为 `c1a21ccaf0eefada9d29251032ad6c2f9095aa6c8d82bcf9a4e770dd403b26e8`。
- 补充测试复现并修复：删除后的重复计数、迟到的置顶响应、重新唤起时的分页取消、取消置顶时的分页边界、备份对缺失图片的恢复，以及重复图片的提交后回收。并发捕获与删除的事件顺序测试通过。
- 测试覆盖率：主程序 42.2%，存储 72.3%，托盘 54.8%，窗口模块 15.4%。

### 仍需实机验收

- [ ] 真实键盘用 Ctrl+Alt+V 唤起、回车和 Alt+数字粘贴，覆盖记事本、Chrome、VS Code 和管理员窗口。
- [ ] 唤起与粘贴的延迟指标；本轮没有进行真实按键注入或 Win+V 基线测量。
- CI 工作流已加入源码，尚未通过推送触发远程运行。

## 第 7 步：2026-11-10 性能优化（0.3.4）

- 列表和实时历史事件仅投影预览及元数据，完整正文按需获取；增加列表及类型排序索引。
- 标签排序改为 O(n log n)，预计算 Unicode 小写键；SQL 先聚合相同标签组合，仅在标签菜单打开时查询，列表和分页不再等待标签。
- 时间／置顶更新不再重建全文索引；图片哈希改为文件流；文本捕获不再扫描全部图片文件容量。
- 图片详情通过应用内部二进制接口返回最长边 1280 像素预览；逐行采样，避免全图读取和 base64 桥接。原图复制、导出、OCR 保持可用，特殊格式回退 BMP。修复 V4/V5 位图头内嵌颜色掩码偏移。
- 设置、识别、翻译按需加载；搜索等待 120ms → 40ms，取消唤起聚焦 100ms 等待；隐藏时释放详情和额外分页、停止时间刷新；取消固定 30 秒强制 GC。
- 剪贴板在生成文件／缩略图前释放；暂停记录和禁用图片时提前跳过读取。详情请求用序号失效，避免迟到响应重新显示和串图。
- 35 项前端测试、TypeScript、全部 Go 测试、`go vet`、Windows 构建及真实 WebView2 列表／标签／4K 预览验证通过。
- 已替换固定安装目录并启动 0.3.4；保留原有数据和自启路径，升级前生成数据库快照。
- 基准结果、场景和内存限制详见 `PERFORMANCE_0.3.4.md`。已证实处理分配与查询延迟下降；当前快照不足以证明整体常驻内存下降。

## 第 8 步：2026-11-11 质量与后台内存整改（0.4.0）

- 后台与 UI 分离，当前用户命名管道、类型化 facade、UI 只读数据库连接。界面闲置 15～600 秒可整组回收，草稿与任务保护、状态恢复、Windows Job Object 和隐藏确认已接入。
- 旧图片维护／缺失修复、热键失败恢复／事务保存／导入校验、设置补丁合并与可变缓存复制、图像负缓存重试、详情请求失效和位图兼容完成回归。
- 设置、行、详情、存储查询／搜索／标签／维护拆分；ESLint、Prettier、EditorConfig、DTO 复用、焦点管理和错误恢复加入。
- 38 项前端测试、所有 Go 测试、静态检查、依赖审计及 Windows 构建通过。真实生产进程循环 50 次通过，干净源码完整发布构建通过，成品 SHA256 与工作目录构建一致。
- 实际安装版后台私有提交内存：隐藏启动 50.8MiB，打开界面并回收后 57.1MiB；均只剩 1 个后台进程。界面打开仍有 WebView2 成本，本次截图场景私有提交 436.5MiB，不宣称打开界面也低内存。
- 已更新日常安装目录，自启启用、500 条历史及配置校验保留。数据库升级前快照已保存。
- 按个人日常工具、本机验证标准自评 90/100，依据与边界见 `QUALITY_0.4.0.md`；结构见 `ARCHITECTURE.md`。

## 第 9 步：2026-11-11 纯翻译请求与 OCR 空格归一（工作树）

- 翻译恢复为纯译文输出：不再要求解释；把“只输出译文、错误信息/问题也按字面翻译”的指令放进 user 消息，并用每次随机的定界符包住原文，兼容忽略 system 角色的网关；返回结果会去掉模型回显的定界符。
- OCR 结果归一化：删除 Windows OCR 在中文/日文字符之间及 CJK 标点周围插入的空格，保留英文、数字、换行和中英边界空格；日文长音、韩文词间空格不被破坏。
- 验证：`go test ./...`、`go vet ./...`、前端 38 项测试、ESLint、Prettier、TypeScript 与 Vite 生产构建通过；本机中文 OCR 实测输出由“你 好 世 界 ClipBox 识 别 123”变为“你好世界 ClipBox 识别 123”。
- 当前系统只安装中文 OCR 语言包；识别英文等语言时仍需安装对应 Windows OCR 组件并在设置中选择，否则准确率受语言包限制。真实服务商翻译输出需在配置 Key 后人工确认。

## 第 10 步：2026-11-12 界面内存归因与方向决策（WebView2 参数实测）

### 实测（隔离数据目录，40 文本 + 8 图片种子，界面打开 6 秒后采样；check_mem.ps1 汇总）

| 场景 | 总私有提交 | WebView2 六进程 | ClipBox 两进程 |
|---|---|---|---|
| 0.4.0 QA 二进制，无参数 | 297.9 MiB | 189.9 MiB | ~108 MiB |
| 默认参数（禁 SpareRenderer） | 297.5 MiB | 192.0 MiB | ~105 MiB |
| + `--disable-gpu` | 297.1 MiB | 190.4 MiB | ~107 MiB |
| + `--enable-low-end-device-mode` | 298.9 MiB | 192.0 MiB | ~107 MiB |
| + `--renderer-process-limit=1` | 298.9 MiB | 190.9 MiB | ~108 MiB |

- 结论：所有参数组合在噪声范围内（±2 MiB）。WebView2 六进程是 Chromium 多进程运行时成本，参数调优无法降到目标范围；QA 截图场景的 436.5 MiB 里 315 MiB 是引擎本身。
- 对照同类：Ditto（C++ 原生）10–30 MB、Maccy（SwiftUI 原生）约 50–90 MB、CopyQ（Qt 原生）80–170 MB——≤150 MB 的都是原生 UI；Tauri 在 Windows 上同样用 WebView2，换框架不解决。ClipBox 后台本身 50.8 MiB 已是原生级。
- 保留改动：`main.go` 接入 `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS` 管道（UI 进程继承，用户可覆盖），默认只禁 SpareRenderer（无害保险）；不启用 `--disable-gpu`（实测无收益，且有第 1.5 步黑窗闪烁回归风险）。`check_mem.ps1` 加 UTF-8 BOM（Windows PowerShell 5.1 解析修复）。
- 本步还提交了第 9 步及更早未提交的 0.4.0 工作树（commit 36b3b45）。

### 方向决策（使用者确认）

- **混合方案**：常驻进程内原生 GDI 弹窗列表（热路径，无浏览器运行时），WebView2 仅用于设置／详情／识别翻译等低频页面。
- 目标：界面打开时总私有提交约 75–100 MiB；打开设置时短暂约 300 MiB（低频，可接受）。
- 依据：0.4.0 的后台独占与 IPC 门面（`dispatchBackend`）不变，只换表现层；常驻进程直连 `App` 服务，无 IPC 开销，粘贴流程的隐藏等待（`__hidden` 确认）在同步隐藏下自然消失。
- 验证方法沿用：`CLIPBOX_APP_EXE` 生命周期测试、check_mem.ps1 前后对比。

## 第 11 步：2026-11-12 原生弹窗上线（工作树）

### 实现
- `internal/popup`：常驻进程内原生 GDI 弹窗。独立线程消息循环；无边框 EDIT 搜索框（IME/中文输入免费可用）；双缓冲绘制：选中行强调色实底（白/近黑按对比度自动取）、置顶三角、时间列、1–9 序号、筛选段、搜索 40ms 防抖；光标附近定位 + 工作区收边；深浅主题 + 系统强调色。
- 键盘路由逐条对照 React 实现：搜索框只拦截 ↑↓/Enter/Ctrl+回车/Ctrl+1~9/Alt+1~9/Esc（有搜索词先清空）；P 置顶、Delete 二次确认、Space/i 详情、Home 回顶部在非输入焦点生效；IME 组合期间不触发动作。
- 接线：`resident.show` 三模式（列表/设置/详情）。列表走弹窗（无子进程、无 IPC、粘贴前隐藏同步返回，`__hidden` 等待机制只剩 WebView2 路径）；设置/详情交接 WebView2 子进程，新增 `detail:open` 事件（前端 `EventsOn` 打开详情面板）；`notifyHistoryChange` 刷新弹窗、设置变更同步主题。
- 回归修复：WS_POPUP 显示不会自动失效 → 首个 WM_PAINT 永不触发、窗口整黑；onShow/onCreate 显式 `InvalidateRect` 修复。弹窗冒烟测试加真实像素断言（背景与选中行颜色），防止无声回归。
- 生产生命周期测试改双路径：弹窗显隐全程无子进程；`__showSettings` 交接走完整 WebView2 生命周期（草稿保护、闲置回收）。

### 实测（隔离数据目录，40 文本 + 8 图片种子，check_mem.ps1）

| 状态 | 总私有提交 | 进程数 | WebView2 |
|---|---|---|---|
| 后台隐藏 | 50.8 MiB | 1 | 0 |
| **弹窗打开（日常）** | **50.8 MiB** | 1 | 0 |
| 设置打开（低频） | 385.8 MiB | 8 | 277.2 |

- 弹窗打开与后台隐藏完全一致（弹窗本身 ≈ 0 额外私有提交）；0.4.0 界面打开 436.5 MiB → **−88%**，进入 Ditto/Maccy 原生档。
- 验证：生产生命周期 3 循环通过（设置交接冷启动 347–606 ms）；`go test ./...`、`go vet`、gofmt、前端 38 项测试、tsc、ESLint 全过；175% DPI 下窗口 DC 像素采样确认渲染（EDIT 白底、选中行 #0078D4、时间列白字）。
- 待手动验收：真实键盘唤起/搜索/粘贴，深浅主题观感，设置往返，管理员窗口粘贴。
