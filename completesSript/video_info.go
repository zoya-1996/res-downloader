package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"bufio"
)



// 转换视频为MP4格式
func convertToMP4(inputPath string) (string, error) {
	// 创建临时目录
	if err := os.MkdirAll(TempDir, 0755); err != nil {
		return "", fmt.Errorf("创建临时目录失败: %v", err)
	}

	fileName := filepath.Base(inputPath)
	nameWithoutExt := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	outputPath := filepath.Join(TempDir, nameWithoutExt+".mp4")

	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-y",
		"-progress", "pipe:1",    // 输出进度信息
		outputPath)

	// 创建管道获取输出
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("创建输出管道失败: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("启动转换失败: %v", err)
	}

	// 读取并显示进度
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "time=") {
				fmt.Printf("\r正在转换: %s", line)
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("转换视频失败 %s: %v", inputPath, err)
	}
	fmt.Println() // 换行

	return outputPath, nil
}

type VideoInfo struct {
	Path     string
	Size     int64
	Duration float64
}

// 获取视频时长
func getVideoDuration(path string) (float64, error) {
	cmd := exec.Command("ffmpeg", "-i", path, "-f", "null", "-")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "Duration") {
		return 0, fmt.Errorf("获取视频时长失败: %v", err)
	}
	
	// 从输出中提取时长信息
	durationRegex := regexp.MustCompile(`Duration: (\d{2}):(\d{2}):(\d{2}\.\d{2})`)
	matches := durationRegex.FindStringSubmatch(string(output))
	
	if len(matches) == 4 {
		hours, _ := strconv.ParseFloat(matches[1], 64)
		minutes, _ := strconv.ParseFloat(matches[2], 64)
		seconds, _ := strconv.ParseFloat(matches[3], 64)
		
		duration := hours*3600 + minutes*60 + seconds
		return duration, nil
	}
	
	return 0, fmt.Errorf("无法获取视频时长")
}

func CheckVideoInfo(dirPath string) error {
	var invalidVideos []VideoInfo
	var validVideos []VideoInfo

	// 创建存放符合要求视频的目录
	if err := os.MkdirAll(ValidVideoDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目标目录和临时目录
		if info.IsDir() {
			if path == ValidVideoDir || path == TempDir {
				return filepath.SkipDir
			}
			return nil
		}

		// 检查是否为视频文件
		ext := strings.ToLower(filepath.Ext(path))
		videoExts := []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv"}
		isVideo := false
		for _, validExt := range videoExts {
			if ext == validExt {
				isVideo = true
				break
			}
		}

		if !isVideo {
			return nil
		}

		// 如果不是MP4，先转换
		videoPath := path
		if ext != ".mp4" {
			fmt.Printf("正在转换视频格式: %s\n", filepath.Base(path))
			var err error
			videoPath, err = convertToMP4(path)
			if err != nil {
				return err
			}
		}

		// 获取转换后的视频信息
		videoInfo, err := os.Stat(videoPath)
		if err != nil {
			return fmt.Errorf("无法获取视频信息: %v", err)
		}

		size := videoInfo.Size()
		
		// 获取视频时长
		duration, err := getVideoDuration(videoPath)
		if err != nil {
			return fmt.Errorf("无法获取视频时长 %s: %v", videoPath, err)
		}

		// 检查是否符合要求
		if size > MaxFileSize || size < MinFileSize || duration > MaxDuration {
			invalidVideos = append(invalidVideos, VideoInfo{
				Path:     videoPath,
				Size:     size,
				Duration: duration,
			})
		} else {
			validVideos = append(validVideos, VideoInfo{
				Path:     videoPath,
				Size:     size,
				Duration: duration,
			})
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 复制符合要求的视频到新目录
	for _, v := range validVideos {
		destPath := filepath.Join(ValidVideoDir, filepath.Base(v.Path))
		if err := copyFile(v.Path, destPath); err != nil {
			return fmt.Errorf("复制文件失败 %s: %v", v.Path, err)
		}
		fmt.Printf("已复制符合要求的视频: %s\n", filepath.Base(v.Path))
	}

	// 输出不符合要求的视频信息
	if len(invalidVideos) > 0 {
		fmt.Println("\n不符合要求的视频文件：")
		for _, v := range invalidVideos {
			fmt.Printf("\n文件：%s\n", v.Path)
			fmt.Printf("大小：%.2f MB\n", float64(v.Size)/1024/1024)
			fmt.Printf("时长：%.2f 秒\n", v.Duration)
			
			if v.Size > MaxFileSize {
				fmt.Println("问题：文件大小超过 10MB")
			} else if v.Size < MinFileSize {
				fmt.Println("问题：文件大小过小")
			}
			if v.Duration > MaxDuration {
				fmt.Println("问题：视频时长超过 39 秒")
			}
		}
		return fmt.Errorf("发现 %d 个不符合要求的视频文件", len(invalidVideos))
	}

	fmt.Println("所有视频文件都符合要求")
	// Fix the print statement to use ValidVideoDir instead of validDir
	fmt.Printf("\n共发现 %d 个符合要求的视频，已复制到 %s 目录\n", len(validVideos), ValidVideoDir)

	// 清理临时文件
	defer os.RemoveAll(TempDir)

	return nil
}

// 复制文件的辅助函数
// Fix the file copy function to use io.Copy instead of os.Copy
// 修改文件大小限制为 20MB
// Keep only one set of constants at the top
// 修改常量定义，只保留一处，并更新MaxFileSize为20MB
const (
	MaxFileSize    = 20 * 1024 * 1024 // 20MB
	MinFileSize    = 1 * 1024 * 1024  // 1MB，设置一个最小值避免空文件
	MaxDuration    = 30.0              // 30秒
	ValidVideoDir  = "/Users/zoya/Desktop/valid_videos"    // 符合要求的视频存放目录
	TempDir        = "/Users/zoya/Desktop/temp_videos"     // 临时转换目录
)

// Remove the duplicate constants and keep only the copyFile function
func copyFile(src, dst string) error {
	// 如果目标文件已存在，直接跳过
	if _, err := os.Stat(dst); err == nil {
		fmt.Printf("文件已存在，跳过: %s\n", filepath.Base(dst))
		return nil
	}

	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("请指定视频文件夹路径")
		os.Exit(1)
	}

	dirPath := os.Args[1]
	if err := CheckVideoInfo(dirPath); err != nil {
		fmt.Printf("检查失败: %v\n", err)
		os.Exit(1)
	}
}

// 移除 init 函数，因为不再需要设置 FFmpeg 路径