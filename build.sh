 #!/bin/bash

  echo "开始编译airvw..."

  # 创建bin目录
  mkdir -p bin

  # 编译macOS版本
  echo "编译macOS版本..."
  GOOS=darwin GOARCH=arm64 go build -o bin/airvw_macos ./airvw/main.go

  # 编译Windows版本
  echo "编译Windows版本..."
  GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/airvw.exe ./airvw/main.go

  # 编译Linux版本
  echo "编译Linux版本..."
  GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/airvw-linux ./airvw/main.go

  echo "编译完成！可执行文件位于bin目录中。"