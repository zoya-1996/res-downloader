package main
import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

func ProcessVideoTitles(dirPath string) error {
	var fileCountMu sync.Mutex
	fileCount := make(map[string]int)

	var files []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".mp4") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	workerCount := 5
	jobs := make(chan string, len(files))
	results := make(chan error, len(files))
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				info, err := os.Stat(path)
				if err != nil {
					results <- err
					continue
				}

				fileName := info.Name()
				dirPath := filepath.Dir(path)
				newName := fileName

				// 1. 移除特殊字符和空格之间的问号
				newName = regexp.MustCompile(`([^\p{Han}\p{Latin}]|\s)[?？]|[?？]([^\p{Han}\p{Latin}]|\s)`).ReplaceAllString(newName, "$1$2")
				
				// 2. 移除多余的空格
				newName = regexp.MustCompile(`\s+`).ReplaceAllString(newName, "")

				// 3. 处理时间戳
				baseName := strings.TrimSuffix(newName, ".mp4")
				timeStampPattern := regexp.MustCompile(`_\d{14}$`)
				baseNameWithoutTimestamp := timeStampPattern.ReplaceAllString(baseName, "")

				fileCountMu.Lock()
				count, exists := fileCount[baseNameWithoutTimestamp]
				if exists {
					fileCount[baseNameWithoutTimestamp] = count + 1
					newName = baseName + ".mp4"
				} else {
					fileCount[baseNameWithoutTimestamp] = 1
					newName = baseNameWithoutTimestamp + ".mp4"
				}
				fileCountMu.Unlock()

				if newName != fileName {
					oldPath := filepath.Join(dirPath, fileName)
					newPath := filepath.Join(dirPath, newName)
					if err := os.Rename(oldPath, newPath); err != nil {
						fmt.Printf("重命名失败 %s: %v\n", fileName, err)
						results <- err
					} else {
						fmt.Printf("已重命名: %s -> %s\n", fileName, newName)
						results <- nil
					}
				} else {
					results <- nil
				}
			}
		}()
	}

	for _, file := range files {
		jobs <- file
	}
	close(jobs)

	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	var dirPath string
	flag.StringVar(&dirPath, "dir", "", "视频文件夹路径")
	flag.Parse()

	if dirPath == "" {
		fmt.Println("请指定视频文件夹路径，使用 -dir 参数")
		flag.Usage()
		os.Exit(1)
	}

	if err := ProcessVideoTitles(dirPath); err != nil {
		fmt.Printf("处理失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("处理完成")
}