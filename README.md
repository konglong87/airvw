# aiutoCR - AI驱动的Codeup多语言代码评审工具

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

aiutoCR 是一款面向阿里云效Codeup的AI代码评审工具，支持自动拉取MR/Commit的多种编程语言代码变更、执行相应语言的静态检查工具、调用阿里云百炼AI进行智能评审，并可将评审结果自动评论到Codeup MR/Commit中，阻断级问题直接终止流程。

## ✨ 核心功能
- 📥 自动拉取Codeup MR/Commit的多种编程语言代码变更（支持Go/Java/Python/JavaScript/Swift/Kotlin）
- 🔍 集成各语言对应的静态检查工具（golangci-lint/checkstyle/flake8/eslint/swiftlint/ktlint）
- 🤖 调用阿里云百炼AI模型进行智能评审，支持自定义模型选择（默认qwen3-coder-plus）
- 💬 自动将评审结果评论到Codeup MR/Commit[可选]
- 🚫 阻断级问题自动终止流程，强制修复后才能合并
- 📝 详细的日志输出，便于问题排查
- 🔔 支持钉钉机器人通知，评审结果实时推送[可选]
- 📊 问题按照重要性等级排序显示（block > high > medium > suggest）
- 🔢 支持限制钉钉通知中显示的最大问题数量，避免信息过多造成干扰

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

# 使用自定义AI模型
airvw \
  --yunxiao-token 新的Token \
  --org-id 你的组织ID \
  --repo-id 你的仓库ID \
  --from-commit 源CommitID \
  --to-commit 目标CommitID \
  --baichuan-key 你的百炼Key \
  --model qwen3-coder-plus

# 限制钉钉通知中显示的问题数量为5个
airvw \
  --yunxiao-token 新的Token \
  --org-id 你的组织ID \
  --repo-id 你的仓库ID \
  --from-commit 源CommitID \
  --to-commit 目标CommitID \
  --baichuan-key 你的百炼Key \
  --enable-dingtalk \
  --dingtalk-token 你的钉钉Token \
  --dingtalk-secret 你的钉钉Secret \
  --max-issues 5

# 显示所有问题（不限制数量）
airvw \
  --yunxiao-token 新的Token \
  --org-id 你的组织ID \
  --repo-id 你的仓库ID \
  --from-commit 源CommitID \
  --to-commit 目标CommitID \
  --baichuan-key 你的百炼Key \
  --enable-dingtalk \
  --dingtalk-token 你的钉钉Token \
  --dingtalk-secret 你的钉钉Secret \
  --max-issues 0
```

## 🌍 语言支持

aiutoCR 支持以下编程语言的代码评审：

| 语言 | 文件扩展名 | 静态检查工具 | 语言标识符               |
|------|------------|--------------|---------------------|
| Go | .go | golangci-lint | go, golang          |
| Java | .java | checkstyle | java                |
| Python | .py | flake8 | python              |
| JavaScript | .js | eslint | js, javascript, tsx |
| Swift | .swift | swiftlint | swift               |
| Kotlin | .kt | ktlint | kt, kotlin          |

使用 `--language` 参数指定要评审的编程语言，默认为 `golang`。

## 🤖 AI模型配置

aiutoCR 支持通过 `--model` 参数指定使用的 AI 模型，默认使用 `qwen3-coder-plus` 模型。

### 支持的模型
- `qwen3-coder-plus` - 默认模型，代码评审专用
- `qwen3-coder` - 代码评审基础模型
- `qwen3-plus` - 通用增强模型
- `qwen3-turbo` - 高速推理模型
- `qwen3` - 通用模型

### 使用示例
```bash
# 使用默认模型
airvw --yunxiao-token xxx --org-id xxx --repo-id xxx --from-commit xxx --to-commit xxx --baichuan-key xxx

# 指定使用 qwen3-coder 模型
airvw --yunxiao-token xxx --org-id xxx --repo-id xxx --from-commit xxx --to-commit xxx --baichuan-key xxx --model qwen3-coder

# 指定使用 qwen3-turbo 模型
airvw --yunxiao-token xxx --org-id xxx --repo-id xxx --from-commit xxx --to-commit xxx --baichuan-key xxx --model qwen3-turbo
```

## 📖 使用说明

### 代码评审
- 安装后执行`airvw --help`会显示结构化的使用教程，包含安装方式、参数说明、示例、注意事项；
- 缺失参数时会自动打印帮助信息，方便用户快速排查；
- 保留原有所有功能，仅优化了帮助信息的展示。

### 钉钉通知配置
- 在钉钉群中添加自定义机器人，获取Webhook地址中的Token和加签Secret；
- 使用`--enable-dingtalk`参数启用钉钉通知功能；
- 配置`--dingtalk-token`和`--dingtalk-secret`参数；
- 评审完成后会自动将结果发送到钉钉群，支持@所有人提醒；
- 问题按照重要性等级排序显示（block > high > medium > suggest）；
- 使用`--max-issues`参数可控制钉钉通知中显示的最大问题数量，默认为10，避免信息过多造成干扰；
- 当问题数量超过限制时，钉钉通知中会显示"仅显示前N个问题（共M个）"的提示信息。


### 总结
| 优化项         | 核心效果 |
|-------------|----------|
| 自定义--help   | `go install`安装后，`airvw --help`显示友好的结构化使用教程 |
| README.md文档 | 包含安装、使用、参数、示例、常见问题 |
| 参数校验        | 缺失参数时自动打印帮助信息，降低使用门槛 |
| 钉钉通知        | 支持钉钉机器人实时推送评审结果，支持@所有人 |
| 问题排序        | 问题按照重要性等级排序显示（block > high > medium > suggest） |
| 问题数量限制    | 使用`--max-issues`参数控制钉钉通知中显示的最大问题数量，默认为10 |
| 多语言支持      | 支持Go/Java/Python/JavaScript/Swift/Kotlin六种编程语言的代码评审 |
| AI模型配置      | 支持自定义选择AI模型，默认使用qwen3-coder-plus |

