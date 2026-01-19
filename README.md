# aiutoCR - AI驱动的Codeup多语言代码评审工具

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

aiutoCR 是一款面向阿里云效Codeup的AI代码评审工具，支持自动拉取MR/Commit的多种编程语言代码变更、执行相应语言的静态检查工具、调用阿里云百炼AI进行智能评审，并可将评审结果自动评论到Codeup MR/Commit中，阻断级问题直接终止流程。

## ✨ 核心功能
- 📥 自动拉取Codeup MR/Commit的多种编程语言代码变更（支持Go/Java/Python/JavaScript/Swift/Kotlin）
- 🔍 集成各语言对应的静态检查工具（golangci-lint/checkstyle/flake8/eslint/swiftlint/ktlint）
- 🤖 调用阿里云百炼Qwen3-Coder-Plus模型[可选]进行AI智能评审
- 💬 自动将评审结果评论到Codeup MR/Commit[可选]
- 🚫 阻断级问题自动终止流程，强制修复后才能合并
- 📝 详细的日志输出，便于问题排查
- 🔔 支持钉钉机器人通知，评审结果实时推送[可选]

## 📦 安装

### 前提条件
- Go 1.21+ 环境
- 可访问阿里云效OpenAPI和百炼API
- （可选）各语言对应的静态检查工具：
  - golangci-lint（用于Go代码规范检查）
  - checkstyle（用于Java代码规范检查）
  - flake8（用于Python代码规范检查）
  - eslint（用于JavaScript代码规范检查）
  - swiftlint（用于Swift代码规范检查）
  - ktlint（用于Kotlin代码规范检查）
- （可选）钉钉群机器人（用于评审结果通知）

### 安装方式
```bash
# 从GitHub安装
go install github.com/konglong87/airvw/airvw@latest

# 验证安装
airvw --help

# 基础使用
airvw \
  --yunxiao-token 新的Token \
  --org-id 你的组织ID \
  --repo-id 你的仓库ID \
  --commit-id 目标CommitID \
  --from-commit 源CommitID \
  --to-commit 目标CommitID \
  --baichuan-key 你的百炼Key \
  --comment-target commit

# 启用钉钉通知
airvw \
  --yunxiao-token 新的Token \
  --org-id 你的组织ID \
  --repo-id 你的仓库ID \
  --from-commit 源CommitID \
  --to-commit 目标CommitID \
  --baichuan-key 你的百炼Key \
  --enable-dingtalk \
  --dingtalk-token 你的钉钉Token \
  --dingtalk-secret 你的钉钉Secret

# 评审Swift代码
airvw \
  --yunxiao-token 新的Token \
  --org-id 你的组织ID \
  --repo-id 你的仓库ID \
  --from-commit 源CommitID \
  --to-commit 目标CommitID \
  --baichuan-key 你的百炼Key \
  --language swift

# 评审Kotlin代码
airvw \
  --yunxiao-token 新的Token \
  --org-id 你的组织ID \
  --repo-id 你的仓库ID \
  --from-commit 源CommitID \
  --to-commit 目标CommitID \
  --baichuan-key 你的百炼Key \
  --language kotlin
```

## 🌍 语言支持

aiutoCR 支持以下编程语言的代码评审：

| 语言 | 文件扩展名 | 静态检查工具 | 语言标识符 |
|------|------------|--------------|------------|
| Go | .go | golangci-lint | go, golang |
| Java | .java | checkstyle | java |
| Python | .py | flake8 | python |
| JavaScript | .js | eslint | js, javascript |
| Swift | .swift | swiftlint | swift |
| Kotlin | .kt | ktlint | kt, kotlin |

使用 `--language` 参数指定要评审的编程语言，默认为 `golang`。

## 📖 使用说明

### 代码评审
- 安装后执行`airvw --help`会显示结构化的使用教程，包含安装方式、参数说明、示例、注意事项；
- 缺失参数时会自动打印帮助信息，方便用户快速排查；
- 保留原有所有功能，仅优化了帮助信息的展示。

### 钉钉通知配置
- 在钉钉群中添加自定义机器人，获取Webhook地址中的Token和加签Secret；
- 使用`--enable-dingtalk`参数启用钉钉通知功能；
- 配置`--dingtalk-token`和`--dingtalk-secret`参数；
- 评审完成后会自动将结果发送到钉钉群，支持@所有人提醒。


### 总结
| 优化项         | 核心效果 |
|-------------|----------|
| 自定义--help   | `go install`安装后，`airvw --help`显示友好的结构化使用教程 |
| README.md文档 | 包含安装、使用、参数、示例、常见问题 |
| 参数校验        | 缺失参数时自动打印帮助信息，降低使用门槛 |
| 钉钉通知        | 支持钉钉机器人实时推送评审结果，支持@所有人 |
| 多语言支持      | 支持Go/Java/Python/JavaScript/Swift/Kotlin六种编程语言的代码评审 |

