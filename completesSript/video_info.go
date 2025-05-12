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

// 添加一个新的结构体来记录处理进度
type ProcessRecord struct {
	ProcessedFiles map[string]bool
	LastProcessed  string
}

// 保存处理记录到文件
func saveProcessRecord(record ProcessRecord) error {
	recordFile := filepath.Join(os.TempDir(), "video_process_record.txt")
	file, err := os.Create(recordFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// 写入最后处理的文件
	file.WriteString("LastProcessed:" + record.LastProcessed + "\n")
	
	// 写入所有已处理的文件
	for path := range record.ProcessedFiles {
		file.WriteString(path + "\n")
	}
	
	return nil
}

// 加载处理记录
func loadProcessRecord() (ProcessRecord, error) {
	record := ProcessRecord{
		ProcessedFiles: make(map[string]bool),
	}
	
	recordFile := filepath.Join(os.TempDir(), "video_process_record.txt")
	if _, err := os.Stat(recordFile); os.IsNotExist(err) {
		return record, nil // 文件不存在，返回空记录
	}
	
	file, err := os.Open(recordFile)
	if err != nil {
		return record, err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine && strings.HasPrefix(line, "LastProcessed:") {
			record.LastProcessed = strings.TrimPrefix(line, "LastProcessed:")
			firstLine = false
			continue
		}
		record.ProcessedFiles[line] = true
	}
	
	return record, scanner.Err()
}

func CheckVideoInfo(dirPath string) error {
	var invalidVideos []VideoInfo
	var validVideos []VideoInfo
	var skippedVideos []string

	// 创建存放符合要求视频的目录
	if err := os.MkdirAll(ValidVideoDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	// 加载处理记录
	record, err := loadProcessRecord()
	if err != nil {
		fmt.Printf("警告: 无法加载处理记录: %v\n", err)
		record = ProcessRecord{
			ProcessedFiles: make(map[string]bool),
		}
	}

	// 如果有上次处理记录，询问是否继续
	if record.LastProcessed != "" {
		fmt.Printf("发现上次处理记录，最后处理的文件是: %s\n", record.LastProcessed)
		fmt.Printf("是否从该文件继续处理? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			// 清除记录，从头开始
			record = ProcessRecord{
				ProcessedFiles: make(map[string]bool),
			}
		}
	}

	// 先统计视频文件总数
	var totalFiles int
	var allVideoPaths []string
	filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			for _, validExt := range []string{".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv"} {
				if ext == validExt {
					totalFiles++
					allVideoPaths = append(allVideoPaths, path)
					break
				}
			}
		}
		return nil
	})

	fmt.Printf("共发现 %d 个视频文件，开始处理...\n", totalFiles)
	
	// 处理计数器
	processedCount := 0
	startProcessing := record.LastProcessed == "" // 如果没有上次记录，立即开始处理

	// 遍历所有视频文件
	for _, path := range allVideoPaths {
		// 如果有上次处理记录，检查是否需要跳过
		if !startProcessing {
			if path == record.LastProcessed {
				startProcessing = true // 找到上次处理的文件，开始处理
			} else {
				continue // 跳过之前处理过的文件
			}
		}

		// 如果已经处理过，跳过
		if record.ProcessedFiles[path] {
			fmt.Printf("跳过已处理的文件: %s\n", filepath.Base(path))
			processedCount++
			continue
		}

		// 更新进度
		processedCount++
		progress := float64(processedCount) / float64(totalFiles) * 100
		fmt.Printf("\r处理进度: [%s] %.1f%% (%d/%d) - 当前文件: %s", 
			getProgressBar(progress), 
			progress, 
			processedCount, 
			totalFiles, 
			filepath.Base(path))

		// 记录当前处理的文件
		record.LastProcessed = path
		saveProcessRecord(record) // 保存处理记录

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
			continue
		}

		// 如果不是MP4，先转换
		videoPath := path
		if ext != ".mp4" {
			fmt.Printf("\n正在转换视频格式: %s\n", filepath.Base(path))
			var err error
			videoPath, err = convertToMP4(path)
			if err != nil {
				fmt.Printf("\n警告: 转换视频失败 %s: %v\n", path, err)
				skippedVideos = append(skippedVideos, path)
				record.ProcessedFiles[path] = true
				saveProcessRecord(record)
				continue // 跳过这个视频，继续处理下一个
			}
		}

		// 获取转换后的视频信息
		videoInfo, err := os.Stat(videoPath)
		if err != nil {
			fmt.Printf("\n警告: 无法获取视频信息 %s: %v\n", videoPath, err)
			skippedVideos = append(skippedVideos, path)
			record.ProcessedFiles[path] = true
			saveProcessRecord(record)
			continue // 跳过这个视频，继续处理下一个
		}

		size := videoInfo.Size()
		
		// 获取视频时长
		duration, err := getVideoDuration(videoPath)
		if err != nil {
			fmt.Printf("\n警告: 无法获取视频时长 %s: %v\n", videoPath, err)
			skippedVideos = append(skippedVideos, path)
			record.ProcessedFiles[path] = true
			saveProcessRecord(record)
			continue // 跳过这个视频，继续处理下一个
		}

		// 检查是否符合要求
		if size > MaxFileSize || size < MinFileSize || duration > MaxDuration {
			invalidVideos = append(invalidVideos, VideoInfo{
				Path:     videoPath,
				Size:     size,
				Duration: duration,
			})
			fmt.Printf("\n视频不符合要求: %s (大小: %.2fMB, 时长: %.2f秒)\n", 
				filepath.Base(videoPath), 
				float64(size)/1024/1024, 
				duration)
		} else {
			// 符合要求，立即复制
			destPath := filepath.Join(ValidVideoDir, filepath.Base(videoPath))
			if err := copyFile(videoPath, destPath); err != nil {
				fmt.Printf("\n警告: 复制文件失败 %s: %v\n", videoPath, err)
				skippedVideos = append(skippedVideos, path)
			} else {
				validVideos = append(validVideos, VideoInfo{
					Path:     videoPath,
					Size:     size,
					Duration: duration,
				})
				fmt.Printf("\n已复制符合要求的视频: %s (大小: %.2fMB, 时长: %.2f秒)\n", 
					filepath.Base(videoPath), 
					float64(size)/1024/1024, 
					duration)
			}
		}

		// 标记为已处理
		record.ProcessedFiles[path] = true
		saveProcessRecord(record)
	}

	// 输出处理结果
	fmt.Printf("\n处理完成: 共处理 %d 个视频文件\n", processedCount)
	fmt.Printf("符合要求: %d 个\n", len(validVideos))
	fmt.Printf("不符合要求: %d 个\n", len(invalidVideos))
	fmt.Printf("跳过处理: %d 个\n", len(skippedVideos))

	// 输出不符合要求的视频信息
	if len(invalidVideos) > 0 {
		fmt.Println("\n不符合要求的视频文件：")
		for _, v := range invalidVideos {
			fmt.Printf("\n文件：%s\n", v.Path)
			fmt.Printf("大小：%.2f MB\n", float64(v.Size)/1024/1024)
			fmt.Printf("时长：%.2f 秒\n", v.Duration)
			
			if v.Size > MaxFileSize {
				fmt.Println("问题：文件大小超过 20MB")
			} else if v.Size < MinFileSize {
				fmt.Println("问题：文件大小过小")
			}
			if v.Duration > MaxDuration {
				fmt.Println("问题：视频时长超过 30 秒")
			}
		}
	}

	// 输出跳过处理的视频
	if len(skippedVideos) > 0 {
		fmt.Println("\n跳过处理的视频文件：")
		for _, path := range skippedVideos {
			fmt.Printf("%s\n", path)
		}
	}

	fmt.Printf("\n符合要求的视频已复制到 %s 目录\n", ValidVideoDir)

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
	MaxFileSize    = 30 * 1024 * 1024 // 20MB
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
// 生成进度条字符串
func getProgressBar(percent float64) string {
	width := 30
	completed := int(percent / 100 * float64(width))
	remaining := width - completed
	
	bar := strings.Repeat("█", completed) + strings.Repeat("░", remaining)
	return bar
}