# 贡献指南 — SlashStage

## Git 分支策略

```
main           ← 稳定分支，所有 CI 必须通过才能合并
  └── feat/*   ← 新功能分支 (e.g. feat/svelte-ui)
  └── fix/*    ← Bug 修复分支 (e.g. fix/session-id-mismatch)
  └── docs/*   ← 文档变更分支 (e.g. docs/architecture)
  └── chore/*  ← 工程化/配置变更 (e.g. chore/ci-setup)
  └── refactor/* ← 重构 (e.g. refactor/parser-interface)
```

### 分支规则

- **`main`** 受保护：必须通过 PR 合并，不允许直接 push
- **feature 分支**：从 `main` 创建，完成后 PR 回 `main`
- **分支命名**：`<type>/<short-description>`，全小写，用 `-` 分隔

---

## Commit 规范 (Conventional Commits)

格式：

```
<type>(<scope>): <subject>

[body]

[footer]
```

### Type

| Type | 描述 | 示例 |
|------|------|------|
| `feat` | 新功能 | `feat(parser): add Gemini CLI parser` |
| `fix` | Bug 修复 | `fix(server): session ID mismatch in API` |
| `docs` | 文档变更 | `docs: add architecture diagram` |
| `test` | 测试 | `test(db): add projects CRUD tests` |
| `refactor` | 重构（不改行为） | `refactor(parser): extract common JSONL logic` |
| `chore` | 构建/CI/工具 | `chore: add golangci-lint config` |
| `perf` | 性能优化 | `perf(db): enable WAL mode` |
| `style` | 格式化（不改逻辑） | `style: gofmt all files` |
| `ci` | CI/CD 配置 | `ci: add GitHub Actions workflow` |

### Scope（可选）

常用 scope：`parser`, `db`, `server`, `model`, `mcp`, `ui`, `cli`

### 规则

1. subject 用英文，首字母小写，不加句号
2. body 可以用中文或英文，解释 why 而不是 what
3. Breaking change 在 footer 加 `BREAKING CHANGE:`
4. 关联 Issue：`Closes #123`

### 示例

```
feat(parser): add Cursor IDE session parser

Reads sessions from state.vscdb SQLite database.
Extracts composerData and bubbleId tables.

Refs: docs/ARCHITECTURE.md section 4
```

```
fix(server): fix session messages returning empty

The frontend was using sessionInfo.session_id but the API
returns the UUID in the "id" field. Changed to sessionInfo.id.

Closes #42
```

---

## PR 流程

### 1. 创建分支

```bash
git checkout main
git pull origin main
git checkout -b feat/my-feature
```

### 2. 开发 + 提交

```bash
# 开发...
make fmt        # 格式化
make lint       # 检查
make test       # 测试

git add -A
git commit -m "feat(parser): add XYZ parser"
```

### 3. 推送 + 创建 PR

```bash
git push origin feat/my-feature
# 在 GitHub 上创建 PR，填写模板
```

### 4. CI 检查

PR 创建后自动触发 CI（所有 Actions 已 pin 到 SHA，见 `.github/workflows/ci.yml`）：
- **Lint**：`golangci-lint run ./...` + `go mod tidy` 一致性检查
- **Test**：`go test -race ./...`（Ubuntu + macOS）
- **Security**：`govulncheck ./...`（Go 官方漏洞扫描）
- **Build**：`go build -trimpath` 并验证二进制可运行

### 5. Review + 合并

- 至少 1 人 approve（solo 项目可自行 review）
- 所有 CI 检查通过
- 使用 **Squash and merge**（保持 main 历史干净）

### 6. 清理

```bash
git checkout main
git pull origin main
git branch -d feat/my-feature
```

---

## 本地开发

### 环境要求

- Go 1.26+
- golangci-lint v2+（`brew install golangci-lint`）
- Make

### 常用命令

```bash
make build       # 编译
make test        # 运行测试
make test-cover  # 测试 + 覆盖率
make lint        # Lint 检查
make run         # 编译并运行
make ci          # 完整 CI（lint + test + build）
make clean       # 清理
```

### 添加新 Parser

1. 在 `internal/model/model.go` 中添加 `AgentType` 常量
2. 在 `internal/parser/parser.go` 的 `Registry` 中添加配置
3. 创建 `internal/parser/newtool.go`，实现 `Parser` 接口
4. 在 `init()` 中调用 `Register()`
5. 添加对应的 `internal/parser/newtool_test.go`

---

## 代码风格

- 遵循 [Effective Go](https://go.dev/doc/effective_go)
- 遵循 `.golangci.yml` 中的 lint 规则
- 所有导出函数/类型必须有 GoDoc 注释
- 错误消息用小写开头，不加标点
- 使用 `log/slog` 进行结构化日志

---

## 文档同步

每次 Sprint 结束后，需要同步更新：

| 文件 | 内容 | 更新时机 |
|------|------|---------|
| `docs/ARCHITECTURE.md` | 技术架构 | 架构变更时 |

> **注意**：`reference/` 目录为本地私有文件夹（通过 `.git/info/exclude` 排除），包含 `TASK.md` 和 `PRD.md`，不在版本控制中。需要新增私有文档直接放入 `reference/` 即可。
