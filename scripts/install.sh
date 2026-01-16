#!/bin/bash
# BCIndex 快速安装脚本

set -e

echo "🚀 BCIndex 安装向导"
echo "===================="
echo ""

# 检查 Go 版本
echo "1️⃣  检查 Go 版本..."
if ! command -v go &> /dev/null; then
    echo "❌ 错误: 未找到 Go，请先安装 Go 1.24 或更高版本"
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
echo "✅ Go 版本: $GO_VERSION"
echo ""

# 创建配置目录
echo "2️⃣  创建配置目录..."
CONFIG_DIR="$HOME/.bcindex/config"
DATA_DIR="$HOME/.bcindex/data"

mkdir -p "$CONFIG_DIR"
mkdir -p "$DATA_DIR"
echo "✅ 配置目录: $CONFIG_DIR"
echo "✅ 数据目录: $DATA_DIR"
echo ""

# 检查配置文件
CONFIG_FILE="$CONFIG_DIR/bcindex.yaml"
if [ -f "$CONFIG_FILE" ]; then
    echo "⚠️  配置文件已存在: $CONFIG_FILE"
    read -p "是否要覆盖? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "跳过配置文件创建"
    else
        cp config.example.yaml "$CONFIG_FILE"
        echo "✅ 配置文件已更新，请编辑: $CONFIG_FILE"
    fi
else
    cp config.example.yaml "$CONFIG_FILE"
    echo "✅ 配置文件已创建: $CONFIG_FILE"
fi
echo ""

# 编译 bcindex
echo "3️⃣  编译 bcindex..."
go build -o bcindex ./cmd/bcindex
echo "✅ 编译完成"
echo ""

# 安装到 PATH
echo "4️⃣  安装到 PATH..."
read -p "是否要安装到 /usr/local/bin? (需要 sudo) (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    sudo mv bcindex /usr/local/bin/
    echo "✅ 已安装到: /usr/local/bin/bcindex"
else
    echo "⏭️  跳过系统安装，你可以手动安装:"
    echo "   sudo mv bcindex /usr/local/bin/"
    echo "   或者"
    echo "   mv bcindex ~/go/bin/  (如果 ~/go/bin 在 PATH 中)"
fi
echo ""

echo "🎉 安装完成！"
echo ""
echo "📝 下一步:"
echo "   1. 编辑配置文件: vim $CONFIG_FILE"
echo "   2. 填写你的 API Key"
echo "   3. 索引你的项目: bcindex -repo /path/to/project index"
echo "   4. 搜索代码: bcindex search \"your query\""
echo ""
echo "📖 查看完整文档: cat README.md"
echo ""
