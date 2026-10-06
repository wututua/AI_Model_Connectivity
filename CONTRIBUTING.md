# 贡献指南

[项目首页](README.md) · [开发指南](docs/development.md) · [文档索引](docs/README.md)

欢迎通过问题反馈、文档改进、测试或代码参与项目。较大的功能和行为变更建议先开 Issue 讨论范围，避免重复工作。

## 提交问题

先搜索已有 [Issues](https://github.com/wututua/AI_Model_Connectivity/issues)，再使用错误报告或功能建议模板。

错误报告请包含：

- 版本号和提交信息，可通过 `model-connectivity --version` 获取。
- 操作系统、架构、部署方式和相关配置。
- 最小复现步骤、预期结果和实际结果。
- 必要的截图或脱敏日志。

不要上传数据库、会话 Cookie、CSRF 值、初始密码、API Key 或包含凭据的通知地址。安全漏洞请遵循 [SECURITY.md](SECURITY.md)，不要直接公开利用步骤。

## 开发流程

1. Fork 仓库，从 `main` 创建描述清晰的工作分支。
2. 按[开发指南](docs/development.md#本地运行)准备 Go 和前端环境。
3. 保持改动聚焦，行为变更附带对应测试，配置或 API 变化同步更新文档。
4. 执行与改动相关的检查，再提交面向 `main` 的 Pull Request。

```bash
git switch -c fix/short-description
```

提交说明建议使用简短、明确的动词描述；可以使用 `fix:`、`feat:`、`docs:`、`test:` 等前缀。

## 提交前检查

后端变更：

```bash
go vet ./...
go test ./...
go test -race ./...
```

`-race` 需要受支持的平台和 CGO/C 编译环境。本地无法运行时，请在 PR 中说明，并确认 CI 的 Linux race 检查通过。

前端变更：

```bash
npm ci --prefix frontend
npm test --prefix frontend
npm run build --prefix frontend
```

涉及交互、登录、请求顺序或响应式布局时，还应运行[浏览器回归](docs/frontend.md#浏览器回归)。前端源码变更后需同步提交 `web/` 构建产物。

安装脚本变更：

```bash
bash -n install-model-connectivity.sh
shellcheck install-model-connectivity.sh
python3 -m unittest discover -s scripts/tests -p test_installer.py -v
```

安装器测试使用隔离目录和模拟下载、账户、systemd 操作，不安装真实服务。Linux CI 及发布工作流会执行这些检查。Windows 可通过 `BASH_BIN` 指定 Git Bash 路径，运行 Python 测试；这不能替代真实 Linux/systemd 的部署验证。

仅修改文档时，核对相对链接、标题锚点、命令与实际代码的一致性，不必重新构建应用。不要修改第三方许可证，也不要用当前行为重写旧版本的发布事实。

## Pull Request

请在说明中写清：

- 要解决的问题及相关 Issue。
- 本次变更范围，以及是否影响已有配置、数据或 API。
- 已执行的测试和未覆盖的环境。
- 界面变化的桌面与手机截图，或文档变化的渲染预览。

不要提交运行数据库、日志、下载产物或凭据。避免将无关的格式化和依赖升级混入同一个 PR。

## 文档约定

- README 面向首次接触项目的使用者，复杂配置与实现细节放入 `docs/`。
- 文档以中文为主，代码、字段名和 API 路径保持原文。
- 新的 Release 说明使用完整中文部分在前、完整英文部分在后，不逐段交错。
- 更新章节标题时，同时检查引用它的锚点链接。
- 命令注明运行目录、前置条件和需要替换的占位值。

## 许可与交流

提交前请确认你有权贡献相应代码、文档和资源。项目代码采用 [MIT License](LICENSE)，第三方资源应保留自己的许可与来源说明。

请围绕问题和实现讨论，尊重参与者的时间与意见，不在 Issue 或 PR 中公开他人的个人信息。
