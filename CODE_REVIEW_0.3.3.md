**ClipBox 0.3.3 当前代码整体评审**

评审日期：2026-11-10。对象：本地 0.3.3 工作树，包含尚未提交的整改代码；Git HEAD 仍为 0.3.2。本次评审覆盖 37 个 Go/TypeScript 源文件，约 11126 行，以及相关测试、构建和发布配置。

当前项目的功能和技术方向已经比较成熟。此前的事务删除、文件回收队列、历史事件同步、图片请求管理、托盘资源管理、备份边界与干净构建等整改仍然有效。性能方面，分页、增量更新、设置与图标缓存、缩略图按需使用、FTS、GPU 合成等基础都已存在。

剩余问题主要集中在旧数据兼容、异常恢复、详情与设置的状态协同，以及大内容和大量标签的处理。以下 P1 建议优先修复；P2 应安排修复或性能整改。本次新增临时测试验证了具体触发条件，评审没有修改生产源码和仓库内测试。

**验证与证据**

- 当前仓库的 `go test -count=1 ./...`、`go vet ./...`、22 项前端测试及 TypeScript 严格检查通过。
- 57 个被比较的 Go/TypeScript 文件与已构建、已审计的源码副本一致；此前同源码的干净发布构建和源码/EXE 漏洞扫描已通过。
- 通过 Go overlay 和源码副本中的前端测试，共验证了 11 个新增失败场景；这些是针对剩余问题的复现实验，不计入仓库现有测试数。
- 热键测试实际进行了临时快捷键注册、冲突和重绑；没有注入真实按键。前端实验使用真实组件和模拟的 Wails 返回顺序。
- 性能测量使用临时数据库和合成图片，不读取用户剪贴板正文。下方数字是本机平均处理成本，未包含真实按键、WebView 绘制或目标应用响应。

**需要优先修复的功能问题**

1. **[P1] 存储维护会删除有效的旧格式图片，置顶项也会被删除。**

   读取和复制接口仍支持数据库中的旧版 base64 BMP，但 `missingImageIDs` 对所有图片内容直接执行文件 `Stat`。合法的 base64 被当作不存在的路径，随后 `Maintain` 删除其记录。临时测试中，一条可以完整解码、带置顶和备注的旧图片经过维护后消失，操作没有返回错误。应先迁移或识别旧格式，只把明确不存在的文件当作缺失，并区分权限、I/O 等访问错误。

   位置：[缺失图片判断](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/storage/store.go:1093>)、[旧格式读取支持](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/api.go:584>)。

2. **[P1] 再次捕获不能修复已丢失的图片，还会回收刚捕获的有效文件。**

   原图片文件丢失后，复制相同图片会命中原内容哈希。普通 `SaveOrUpdate` 保留旧路径并把新路径当作重复文件回收。测试结果是原记录的时间更新，但旧文件仍不存在，新文件也已被删除。备份合并导入已实现缺失图片修复，普通捕获分支仍缺这一步。应统一两条路径的文件选择规则，保留可读取的新文件，并在提交后回收真正多余的文件。

   位置：[普通图片去重](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/storage/store.go:433>)、[备份导入中的修复规则](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/storage/backup.go:422>)。

3. **[P1] 详情打开时，列表快捷键仍可粘贴背景中的另一条记录。**

   全局键盘处理器在详情打开时只对 Esc 做了特殊处理，其余方向键和回车仍操作列表。测试中打开记录 1 的详情，把焦点放到详情的“复制”按钮，再按上方向键、回车，实际调用了 `CopyAndPaste(2)`。这也会截获详情按钮的回车激活。应在详情打开时暂停背景列表快捷键，让详情按钮和编辑器拥有自己的键盘作用域，并管理面板焦点。

   位置：[全局键盘处理](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/App.tsx:1025>)、[回车动作](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/App.tsx:1090>)。

4. **[P1] 编辑设置时，外部暂停状态会被后续提交覆盖。**

   设置页检测到输入框正在编辑就丢弃整个 `settings:updated` 事件，随后又提交完整配置对象。测试中编辑“最小文本长度”期间收到托盘暂停事件，失焦保存发出的配置仍为 `capturePaused: false`，会重新开启记录。应保留外部状态更新，仅保护正在编辑的字段；保存使用字段补丁或带版本的合并，避免整个旧快照覆盖其他操作。

   位置：[忽略设置事件](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/Settings.tsx:336>)、[完整配置提交](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/Settings.tsx:292>)。

5. **[P2] 热键的失败恢复、持久化和导入校验尚未统一。**

   已验证三个场景：启动注册冲突后，设置页换到空闲组合仍返回“热键线程未运行”；数据库写入被拒绝后，热键已在运行时改变，API 却返回成功，重启后会恢复旧组合；导入旧备份后，`Ctrl+1` 等已禁止的组合仍被保留，导入恢复逻辑直接重绑且忽略错误。应支持失败启动后的重新注册，两个持久化字段使用事务，并使普通修改、启动和备份导入共用校验与回滚规则。

   位置：[未运行时无法重绑](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/hotkey/hotkey_windows.go:241>)、[忽略保存错误](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/api.go:878>)、[导入恢复](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/api.go:799>)。

6. **[P2] 详情请求没有取消或序号保护，迟到响应会重新打开面板或串图。**

   测试中关闭仍在加载的详情，原请求返回后面板再次出现；先打开图片 1，再关闭并打开图片 2，图片 1 的迟到结果覆盖了图片 2 的画面。`GetClipDetails`、`GetImageDataURL` 和备注保存返回后都直接写详情状态。应建立独立详情会话，以记录 ID 和请求序号校验返回结果，关闭、切换或删除时使旧会话失效。

   位置：[详情关闭与加载](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/App.tsx:874>)、[备注保存后写详情](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/App.tsx:910>)。

7. **[P2] 修复图片后，同一 ID 的空元数据缓存不会失效。**

   后端省略无法读取的图片 ID，前端把该结果缓存为 `null`，并把它当作已经完成的缓存项。测试中图片第一次不可读，随后以同一 ID 修复并刷新列表，尺寸和大小仍为空。该 ID 持续留在列表时不会重试。应在文件修复、导入或完整历史刷新时使相关缓存失效，也可使用文件版本或有限的负缓存有效期。

   位置：[空结果缓存](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/useImageMetadata.ts:29>)、[后端省略不可读图片](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/api.go:302>)。

8. **[P2] V4/V5 位图头的颜色掩码偏移计算错误。**

   对于 108/124 字节头、使用 `BI_BITFIELDS` 的合法 DIB，掩码已经包含在头内，当前代码仍额外增加 12 字节。测试中缩略图转换报“像素数据截断”；生成 BMP 的像素偏移分别为 134/150，应为 122/138。应根据头版本判断外置掩码，并让缩略图与 BMP 包装共用解析规则。

   位置：[缩略图解析偏移](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/clipboard/clipboard_windows.go:622>)、[BMP 包装偏移](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/clipboard/clipboard_windows.go:756>)。

**性能测量和剩余瓶颈**

硬件：AMD Ryzen 9 8940HX，Windows/amd64，Go 1.27.3。数据库测试为 10000 条合成文本，多数正文约 512 字节，最新记录中包含 5 条 512KiB 正文；关键词匹配全部记录。标签排序测试单独构造 1000/5000 个不同、等计数的标签。原图测试为 3840×2160 的 32 位 DIB，约 31.64MiB，文件读取使用已写入的临时文件。

| 测试项 | 本机结果 | 说明 |
|---|---:|---|
| 当前 50 条列表 JSON | 2651282 字节，编码约 1.5ms | 包含列表未使用的完整正文 |
| 同记录去除正文的对照 JSON | 5852 字节 | 对照方案没有应用到生产代码 |
| 10000 条历史，取第一页 | 约 40.6ms | 包含读取正文与当前排序成本 |
| 同库搜索并计数 | 约 62.9ms | 未包含标签查询和 WebView 更新 |
| 同库标签聚合，全部使用一个标签 | 约 14.8ms | 每次列表加载仍会请求标签 |
| 1000 个不同标签排序 | 约 27.2ms | 累计分配约 16MB |
| 5000 个不同标签排序 | 约 724.6ms | 累计分配约 400MB，约 2500 万次分配 |
| 4K 图片转 data URL | 约 58–62ms | 累计分配约 190MiB/次，未包含跨进程传输与渲染 |

这些分配量是单次操作期间的累计分配，不代表常驻内存或峰值工作集。样本为合成负载，数字用于定位处理成本，不能代替端到端延迟和成功率验收。

- **列表仍传完整正文，排序索引也不匹配。** 列表只渲染预览，复制和详情会按 ID 读取完整记录，当前分页和捕获事件却仍带正文。查询计划显示 `SCAN clips` 和 `USE TEMP B-TREE FOR ORDER BY`；临时加入 `(pinned DESC, timestamp DESC)` 对照索引后，查询计划直接使用该索引。建议先使用轻量列表对象与匹配的排序索引。[列表查询](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/storage/store.go:483>)。
- **标签排序是平方复杂度，并阻塞列表提交。** `sortTagCounts` 使用二重循环，比较时反复转换小写；前端用 `Promise.all` 把标签结果与每次分页绑定。大量不同标签时，50 条分页也必须等整份标签排序结束。建议使用排序库与预计算比较键，缓存标签统计，并减少重复分页请求。[排序](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/storage/store.go:955>)、[列表与标签绑定](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/App.tsx:560>)。
- **搜索和焦点仍有固定等待。** 搜索防抖仍为 120ms，搜索框在唤起事件后延迟 100ms 才聚焦。当前代码无法宣称已达到规格中的搜索响应 ≤100ms；需要调整等待策略并测试连续输入。[搜索](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/App.tsx:676>)、[聚焦](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/frontend/src/App.tsx:641>)。
- **原图的大字符串桥接成本仍高。** 原图经过 DIB 读取、BMP 包装、base64 和字符串拼接，再通过 Wails 传给前端。4K 测试中累计分配约 190MiB，50MiB Go 内存软限制没有消除这类分配。建议评估文件或流式资源接口，并单独测量应用和 WebView2 的整体内存。[原图转换](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/clipboard/clipboard_windows.go:515>)。
- **剪贴板锁内仍包含文件 I/O 和缩略图生成。** `readClipboard` 延迟关闭剪贴板，图片分支在返回前完成落盘和缩略图；暂停记录和关闭图片记录的判断在读取之后。建议尽早判断捕获策略，复制必要快照后释放系统剪贴板，再有序处理文件、哈希、入库和清理。缩略图已经直接采样到小尺寸，本次没有发现全分辨率 PNG 解码这一额外成本。[锁范围](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/clipboard/clipboard_windows.go:377>)、[图片处理](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/internal/clipboard/clipboard_windows.go:442>)、[捕获策略](<C:/Users/QGS/Desktop/个人项目/Finish-Weihu/Clipbox/clipbox/settings.go:291>)。

**整体评价与维护建议**

| 维度 | 当前评价 |
|---|---|
| 产品范围 | 清晰，记录、搜索、选择、粘贴及维护功能完整 |
| 技术栈 | Go + Wails + React + SQLite 与 Windows 个人工具的定位匹配 |
| 数据安全 | 提交后回收和重试队列已有保障；旧格式与恢复分支仍需统一 |
| 界面状态 | 列表同步明显完善；详情、表单编辑和快捷键还缺统一的会话边界 |
| 性能 | 已实施多项优化；大正文、标签和原图成本仍高，100ms 目标未完成 |
| 工程验证 | 有 22 项前端测试和 Windows CI 配置；本轮复现说明关键边界仍未覆盖，远程 CI 尚未运行 |
| 可维护性 | App.tsx 1408 行、Settings.tsx 1331 行、store.go 1331 行、api.go 903 行，职责仍集中 |

建议继续沿用当前技术栈。下一轮先处理四项 P1，然后完善热键恢复、详情请求、图片缓存和位图格式，再处理列表数据与索引、标签排序和原图传输。将已复现的场景纳入正式回归测试，并逐步拆出详情会话、设置草稿和持久化状态逻辑；前端优先复用生成的绑定类型，减少手工重复定义。

真实键盘在记事本、Chrome、VS Code、管理员窗口中的粘贴成功率，多屏/DPI 行为，Win+V 延迟基线，10 分钟空闲内存，以及远程 CI，仍需独立验收。本次原生热键注册测试不能替代这些验收。

复现材料：[Go overlay](<C:/Users/QGS/AppData/Local/Temp/clipbox-review-20261110-d120f5d0970f4609a7e96a3b63b8c87d/overlay.json>)、[存储复现与性能测试](<C:/Users/QGS/AppData/Local/Temp/clipbox-review-20261110-d120f5d0970f4609a7e96a3b63b8c87d/review_storage_test.go>)、[热键复现](<C:/Users/QGS/AppData/Local/Temp/clipbox-review-20261110-d120f5d0970f4609a7e96a3b63b8c87d/review_hotkey_test.go>)、[位图复现](<C:/Users/QGS/AppData/Local/Temp/clipbox-review-20261110-d120f5d0970f4609a7e96a3b63b8c87d/review_dib_test.go>)、[前端复现实验生成脚本](<C:/Users/QGS/AppData/Local/Temp/clipbox-review-20261110-d120f5d0970f4609a7e96a3b63b8c87d/make_frontend_review.mjs>)。
