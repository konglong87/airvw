# airvw - AI驱动的Codeup Go代码评审工具

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

airvw 是一款面向阿里云效Codeup的AI代码评审工具，支持自动拉取MR/Commit的Go代码变更、执行golangci-lint规则检查、调用阿里云百炼AI进行智能评审，并可将评审结果自动评论到Codeup MR/Commit中，阻断级问题直接终止流程。

## ✨ 核心功能
- 📥 自动拉取Codeup MR/Commit的Go代码变更（仅筛选新增/修改的.go文件）
- 🔍 集成golangci-lint进行代码规范检查
- 🤖 调用阿里云百炼Qwen3-Coder-Plus模型进行AI智能评审
- 💬 自动将评审结果评论到Codeup MR/Commit
- 🚫 阻断级问题自动终止流程，强制修复后才能合并
- 📝 详细的日志输出，便于问题排查

## 📦 安装

### 前提条件
- Go 1.21+ 环境
- 可访问阿里云效OpenAPI和百炼API
- （可选）golangci-lint（用于代码规范检查）

### 安装方式
```bash
# 从GitHub安装（替换为你的仓库地址）
go install github.com/konglong87/airvw@latest

# 验证安装
airvw --help

# 使用
airvw \
  --yunxiao-token 新的Token \
  --org-id 你的组织ID \
  --repo-id 你的仓库ID \
  --commit-id 目标CommitID \
  --from-commit 源CommitID \
  --to-commit 目标CommitID \
  --baichuan-key 你的百炼Key \
  --comment-target commit
```


### 三、使用说明
1. **代码部分**：
   - 安装后执行`airvw --help`会显示结构化的使用教程，包含安装方式、参数说明、示例、注意事项；
   - 缺失参数时会自动打印帮助信息，方便用户快速排查；
   - 保留原有所有功能，仅优化了帮助信息的展示。

2. **Markdown文档**：
   - 可直接保存为`README.md`放到GitHub仓库根目录；
   - 包含用户所需的所有信息：安装、使用、参数、示例、常见问题；
   - 格式符合GitHub规范，排版清晰，便于阅读；
   - 可根据实际仓库地址替换`github.com/konglong87/airvw`。

### 总结
| 优化项 | 核心效果 |
|--------|----------|
| 自定义--help | `go install`安装后，`airvw --help`显示友好的结构化使用教程 |
| README.md文档 | 完整的GitHub发布用教程，包含安装、使用、参数、示例、常见问题 |
| 参数校验优化 | 缺失参数时自动打印帮助信息，降低使用门槛 |

你可直接将修改后的代码编译发布，README.md文档可直接上传到GitHub仓库，用户能通过`--help`和文档快速上手使用工具。