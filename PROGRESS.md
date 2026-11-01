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
