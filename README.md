# SevenDaysLocalizer

七日杀（7 Days to Die）Mod 自动汉化工具。

SevenDaysLocalizer 是一个基于 Wails 的桌面端开源工具，用于读取 7 Days to Die Mod 或游戏目录中的 `Localization.csv` / `Localization.txt`，通过兼容 OpenAI Chat Completions 格式的 AI 接口批量生成简体中文翻译，并写回 `schinese` 列。

## 功能特性

- 支持选择或拖拽 Mod / 游戏目录，也支持直接选择 `Localization.csv` / `Localization.txt`。
- 自动查找常见本地化文件：
  - `Config/Localization.csv`
  - `Config/Localization.txt`
  - 根目录下的 `Localization.csv`
  - 根目录下的 `Localization.txt`
- 按 CSV 规则解析 `.csv` / `.txt`，自动识别逗号、Tab、分号分隔符。
- 动态识别 `english`、`schinese`、`NoTranslate` 等列，不写死列索引。
- 缺少 `schinese` 列时自动追加。
- 自动跳过空英文、`NoTranslate` 标记行、已有可用中文翻译的行。
- 可选择覆盖已有 `schinese` 内容。
- 兼容 OpenAI / New API 风格接口：
  - `GET /v1/models`
  - `POST /v1/chat/completions`
- 支持批量翻译、并发请求、暂停、继续、终止。
- 翻译成功后立即写回原文件，便于中断后继续。
- 保护颜色标签和占位符，例如 `[FFFF33]`、`[-]`、`%s`、`{0}`。
- 校验 AI 返回的 JSON 数组长度、ID、保护 token 和中文结果。
- 表格实时展示原文和译文，支持双击手动编辑并保存。
- 支持单条翻译，便于人工校对时快速补全。

## 下载使用

推荐普通用户直接下载 Release 中的 Windows 可执行文件：

1. 打开项目右侧或顶部的 **Releases**。
2. 下载最新版本中的 `SevenDaysLocalizer.exe`。
3. 双击运行。

> 如果 Windows 提示未知发布者，这是因为当前项目没有商业代码签名证书。可在确认来源后选择继续运行。

## 使用流程

1. 启动 `SevenDaysLocalizer.exe`。
2. 点击“选择目录”，选择 7 Days to Die Mod 目录或游戏目录；也可以直接拖入目录或 Localization 文件。
3. 填写 AI 接口配置：
   - `Base URL`：例如 `https://api.example.com`
   - `API Key`：你的接口密钥
4. 点击“获取模型”，选择要使用的模型；也可以手动输入模型名。
5. 调整批量翻译参数：
   - 推荐单批数量：`10 - 20`
   - 推荐并发数量：`3 - 5`
6. 点击“开始汉化”。
7. 如需人工修正，双击表格中的“简体中文”单元格，编辑后保存。

## 注意事项

- 工具会直接修改当前加载的 `Localization.csv` / `Localization.txt`，首次使用前建议自行备份 Mod。
- 保存格式统一为 UTF-8 BOM，以兼容 7 Days to Die 的本地化文件读取习惯。
- `.txt` 本地化文件在很多 Mod 中本质仍是 CSV 结构，因此本工具按 CSV 规则解析和保存。
- API Key 会保存在本机浏览器存储中，便于下次打开自动填充；请勿在公共电脑上保存私人密钥。
- 如果网络中断或接口额度不足，已成功翻译的批次会保留在文件中；下次重新加载后默认会跳过已有译文的行。

## 开发环境

需要安装：

- Go 1.23 或更高版本
- Node.js 18 或更高版本
- Wails v2

安装 Wails：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

安装前端依赖：

```bash
cd frontend
npm ci
```

启动开发环境：

```bash
wails dev
```

## 本地构建

在项目根目录执行：

```bash
wails build
```

Windows 构建产物默认位于：

```text
build/bin/SevenDaysLocalizer.exe
```

## 项目结构

```text
.
├── app.go                         # Wails 应用方法
├── localization.go                # Localization 文件查找、解析、保存
├── translation.go                 # 翻译任务调度和事件推送
├── translation_batch.go           # 批量请求、重试和降级拆分
├── translation_parse.go           # AI JSON 响应解析
├── translation_prompt.go          # 翻译系统提示词
├── translation_validate.go        # 翻译结果校验
├── types.go                       # 前后端共享数据结构
├── frontend/                      # Vue 3 前端
├── build/                         # Wails 构建配置和图标资源
└── wails.json                     # Wails 项目配置
```

## 发布流程

本项目包含 GitHub Actions 发布工作流。

推送形如 `v1.0.0` 的 tag 后，GitHub Actions 会自动：

1. 安装 Go、Node.js 和 Wails。
2. 构建 Windows 可执行文件。
3. 创建 GitHub Release。
4. 上传 `SevenDaysLocalizer.exe`。

示例：

```bash
git tag v1.0.0
git push origin v1.0.0
```

## 贡献

欢迎提交 Issue 和 Pull Request。

适合贡献的方向：

- 增加术语表和自定义 Prompt。
- 增加翻译前自动备份。
- 优化大批量翻译的错误恢复。
- 优化前端包体积和加载速度。
- 增加更多本地化质量检查。

## License

本项目基于 MIT License 开源。
