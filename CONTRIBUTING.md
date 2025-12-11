# 贡献指南

感谢你考虑为 Ruoyi Proxy 做出贡献！

## 如何贡献

### 报告 Bug

如果你发现了 bug，请[创建一个 Issue](https://github.com/xuantiandaozun/ruoyi-proxy/issues/new) 并包含以下信息：

- **Bug 描述**：清晰简洁地描述问题
- **重现步骤**：详细的重现步骤
- **期望行为**：你期望发生什么
- **实际行为**：实际发生了什么
- **环境信息**：
  - 操作系统和版本
  - Go 版本
  - Ruoyi Proxy 版本
- **日志和截图**：如果可能，提供相关日志和截图

### 提出新功能

如果你有新功能的想法，请先[创建一个 Issue](https://github.com/xuantiandaozun/ruoyi-proxy/issues/new) 讨论：

- 描述你想要的功能
- 解释为什么这个功能有用
- 如果可能，提供使用示例

### 提交代码

1. **Fork 仓库**
   ```bash
   # 点击 GitHub 页面上的 Fork 按钮
   ```

2. **克隆你的 Fork**
   ```bash
   git clone https://github.com/你的用户名/ruoyi-proxy.git
   cd ruoyi-proxy
   ```

3. **创建特性分支**
   ```bash
   git checkout -b feature/amazing-feature
   ```

4. **进行修改**
   - 遵循现有的代码风格
   - 添加必要的注释
   - 更新相关文档

5. **测试你的修改**
   ```bash
   # 编译测试
   make build
   
   # 运行测试（如果有）
   go test ./...
   ```

6. **提交修改**
   ```bash
   git add .
   git commit -m "feat: 添加某某功能"
   ```

   提交信息格式：
   - `feat:` 新功能
   - `fix:` Bug 修复
   - `docs:` 文档更新
   - `style:` 代码格式调整
   - `refactor:` 代码重构
   - `test:` 测试相关
   - `chore:` 构建/工具相关

7. **推送到你的 Fork**
   ```bash
   git push origin feature/amazing-feature
   ```

8. **创建 Pull Request**
   - 访问你的 Fork 页面
   - 点击 "New Pull Request"
   - 填写 PR 描述，说明你的修改
   - 等待审核

## 代码规范

### Go 代码风格

- 遵循 [Effective Go](https://golang.org/doc/effective_go.html)
- 使用 `gofmt` 格式化代码
- 使用有意义的变量和函数名
- 添加必要的注释，特别是导出的函数和类型

### 提交规范

- 每个提交应该是一个逻辑单元
- 提交信息应该清晰描述修改内容
- 避免提交无关的文件（使用 .gitignore）

### 文档

- 更新 README.md（如果需要）
- 更新 CHANGELOG.md
- 为新功能添加使用示例

## 开发环境设置

### 环境要求

- Go 1.24+
- Git
- Make（可选，但推荐）

### 本地开发

```bash
# 安装依赖
make install

# 开发模式运行
make run

# 编译
make build

# 清理
make clean
```

### 调试

```bash
# 启动 CLI 进行测试
make cli

# 查看日志
tail -f logs/proxy.log
```

## 行为准则

- 尊重所有贡献者
- 保持友好和专业
- 接受建设性的批评
- 关注对项目最有利的事情

## 问题？

如果你有任何问题，可以：

- [创建 Issue](https://github.com/xuantiandaozun/ruoyi-proxy/issues)
- 在 Pull Request 中提问

---

再次感谢你的贡献！🎉
