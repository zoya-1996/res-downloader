package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	VideoDir = "/Users/zoya/Desktop/高德地图哪儿都熟"
	OutputFile = "/Users/zoya/Desktop/video_titles.txt"
)

func cleanTitle(title string) string {
	// 定义需要移除的重复部分
	repeatPart := "【🎉入驻高德地图享三重大礼🎁，数量有限先到先得⏳，私信我领取】"
	
	// 移除最后一个重复部分
	if strings.Count(title, repeatPart) > 1 {
		lastIndex := strings.LastIndex(title, repeatPart)
		if lastIndex != -1 {
			title = title[:lastIndex] + title[lastIndex+len(repeatPart):]
		}
	}
	
	return title
}

func main() {
	file, err := os.Create(OutputFile)
	if err != nil {
		fmt.Printf("创建输出文件失败: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	count := 1

	err = filepath.Walk(VideoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".mp4" {
			// 获取文件名（不含扩展名）
			title := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
			
			// 清理标题
			cleanedTitle := cleanTitle(title)
			
			// 构建新的文件路径
			newPath := filepath.Join(filepath.Dir(path), cleanedTitle+".mp4")
			
			// 重命名文件
			if err := os.Rename(path, newPath); err != nil {
				return fmt.Errorf("重命名文件失败 %s: %v", path, err)
			}
			
			// 写入文件（添加序号）
			_, err := fmt.Fprintf(file, "%d. %s\n", count, cleanedTitle)
			if err != nil {
				return fmt.Errorf("写入文件失败: %v", err)
			}
			
			fmt.Printf("已处理视频 %d: %s\n", count, cleanedTitle)
			count++
		}
		return nil
	})

	if err != nil {
		fmt.Printf("处理失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n所有视频已重命名，标题列表已保存到: %s\n", OutputFile)
}