#!/bin/bash
# 批量压缩视频到10MB以下

# 检查参数
if [ $# -ne 1 ]; then
    echo "用法: $0 视频文件夹路径"
    exit 1
fi

input_dir="$1"

# 检查输入目录是否存在
if [ ! -d "$input_dir" ]; then
    echo "错误: 目录 '$input_dir' 不存在"
    exit 1
fi

# 创建临时目录
temp_dir="/tmp/video_compress_temp"
mkdir -p "$temp_dir"

# 计算视频文件总数
total_videos=$(find "$input_dir" -type f -name "*.mp4" -o -name "*.mov" -o -name "*.avi" -o -name "*.mkv" -o -name "*.wmv" -o -name "*.flv" | wc -l)
echo "共发现 $total_videos 个视频文件"

# 处理计数器
processed=0

# 压缩单个视频函数
compress_video() {
    local input_file="$1"
    local target_size=10 # 目标大小(MB)
    local filename=$(basename "$input_file")
    local dirname=$(dirname "$input_file")
    
    # 生成一个随机文件名作为临时文件，避免特殊字符问题
    local temp_id=$(date +%s | md5 | head -c 10)
    local temp_output="$temp_dir/${temp_id}.mp4"
    
    echo "处理: $filename"
    
    # 获取原始文件大小(MB)，使用引号保护文件名
    local original_size=$(stat -f %z "$input_file" 2>/dev/null | awk '{print int($1/1024/1024)}')
    
    # 检查是否成功获取文件大小
    if [ -z "$original_size" ] || ! [[ "$original_size" =~ ^[0-9]+$ ]]; then
        echo "警告: 无法获取文件大小，使用默认压缩设置"
        original_size=20 # 设置一个默认值
    fi
    
    echo "原始大小: ${original_size}MB"
    
    # 如果文件已经小于10MB，则跳过压缩
    if [ "$original_size" -lt "$target_size" ]; then
        echo "文件已经小于10MB，跳过压缩"
        return 0
    fi
    
    # 先检查文件是否为有效的视频文件
    echo "检查视频文件有效性..."
    if ! ffmpeg -i "$input_file" -f null - -v quiet 2>/dev/null; then
        echo "错误: 无效的视频文件或格式不支持，尝试先转换格式"
        
        # 尝试先转换为标准MP4格式
        local convert_temp="$temp_dir/convert_${temp_id}.mp4"
        echo "转换视频格式..."
        if ffmpeg -i "$input_file" -c:v libx264 -c:a aac -strict experimental "$convert_temp" -y 2>/dev/null; then
            echo "格式转换成功，继续压缩"
            input_file="$convert_temp"
        else
            echo "格式转换失败，无法处理此文件"
            return 1
        fi
    fi
    
    # 根据原始大小调整CRF值
    local crf=23
    if [ "$original_size" -gt 50 ]; then
        crf=28
    elif [ "$original_size" -gt 20 ]; then
        crf=25
    fi
    
    # 第一次尝试压缩
    echo "开始压缩..."
    if ! ffmpeg -i "$input_file" -c:v libx264 -crf $crf -preset slower \
    -vf "scale='min(1280,iw)':-2" -c:a aac -b:a 64k \
    -movflags +faststart "$temp_output" -y 2> "$temp_dir/ffmpeg_error.log"; then
        echo "压缩失败，错误日志:"
        cat "$temp_dir/ffmpeg_error.log"
        echo "尝试使用更简单的压缩参数..."
        
        # 尝试使用更简单的参数
        if ! ffmpeg -i "$input_file" -c:v libx264 -crf 28 -preset medium \
        -c:a aac "$temp_output" -y 2> "$temp_dir/ffmpeg_error.log"; then
            echo "简单压缩也失败了，错误日志:"
            cat "$temp_dir/ffmpeg_error.log"
            return 1
        fi
    fi
    
    # 检查压缩后的大小
    if [ ! -f "$temp_output" ]; then
        echo "错误: 压缩失败，临时文件不存在"
        return 1
    fi
    
    local compressed_size=$(stat -f %z "$temp_output" 2>/dev/null | awk '{print int($1/1024/1024)}')
    
    # 检查是否成功获取压缩后的文件大小
    if [ -z "$compressed_size" ] || ! [[ "$compressed_size" =~ ^[0-9]+$ ]]; then
        echo "警告: 无法获取压缩后的文件大小"
        compressed_size=999 # 设置一个大值以触发进一步压缩
    fi
    
    # 如果仍然大于10MB，增加CRF值再次压缩
    attempts=1
    while [ "$compressed_size" -gt "$target_size" ] && [ "$attempts" -lt 3 ]; do
        crf=$((crf + 3))
        if [ "$crf" -gt 35 ]; then
            crf=35 # 最大CRF值限制
        fi
        
        echo "尝试更高压缩率 (CRF=$crf)..."
        ffmpeg -i "$input_file" -c:v libx264 -crf $crf -preset slower \
        -vf "scale='min(1024,iw)':-2" -c:a aac -b:a 48k \
        -movflags +faststart "$temp_output" -y > /dev/null 2>&1
        
        compressed_size=$(du -m "$temp_output" 2>/dev/null | cut -f1)
        if [ -z "$compressed_size" ] || ! [[ "$compressed_size" =~ ^[0-9]+$ ]]; then
            compressed_size=999
        fi
        attempts=$((attempts + 1))
    done
    
    # 如果仍然大于10MB，尝试降低分辨率
    if [ "$compressed_size" -gt "$target_size" ]; then
        echo "尝试降低分辨率..."
        ffmpeg -i "$input_file" -c:v libx264 -crf 30 -preset slower \
        -vf "scale='min(854,iw)':-2" -c:a aac -b:a 32k \
        -movflags +faststart "$temp_output" -y > /dev/null 2>&1
        
        compressed_size=$(du -m "$temp_output" 2>/dev/null | cut -f1)
        if [ -z "$compressed_size" ] || ! [[ "$compressed_size" =~ ^[0-9]+$ ]]; then
            compressed_size=999
        fi
    fi
    
    # 最终检查
    if [ "$compressed_size" -gt "$target_size" ]; then
        echo "警告: 无法将文件压缩到10MB以下 (当前大小: ${compressed_size}MB)"
    else
        echo "压缩成功: ${original_size}MB -> ${compressed_size}MB"
    fi
    
    # 替换原文件
    if [ -f "$temp_output" ]; then
        if mv "$temp_output" "$input_file"; then
            echo "已替换原文件"
        else
            echo "错误: 无法替换原文件，可能是权限问题"
            cp "$temp_output" "$input_file" && echo "尝试使用复制方式替换成功"
        fi
    else
        echo "错误: 临时文件不存在，无法替换原文件"
    fi
    
    return 0
}

# 遍历所有视频文件并压缩
find "$input_dir" -type f \( -name "*.mp4" -o -name "*.mov" -o -name "*.avi" -o -name "*.mkv" -o -name "*.wmv" -o -name "*.flv" \) | while read video_file; do
    processed=$((processed + 1))
    echo "[$processed/$total_videos] 处理中..."
    compress_video "$video_file"
    echo "----------------------------------------"
done

# 清理临时目录
rm -rf "$temp_dir"

echo "所有视频处理完成!"