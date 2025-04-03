package main
//  go run filter.go -dir "/Users/zoya/Desktop/高德地图旺铺商户助手"
import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
    "math/rand"
    "time"
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

				// 4. 处理连续的句号
				baseNameWithoutTimestamp = regexp.MustCompile(`[.。]{2,}`).ReplaceAllString(baseNameWithoutTimestamp, "")

				// 5. 添加语气词和标点处理
				// 检查是否已经有标点符号
                hasEndingPunctuation := strings.HasSuffix(baseNameWithoutTimestamp, "！") ||
                    strings.HasSuffix(baseNameWithoutTimestamp, "？") ||
                    strings.HasSuffix(baseNameWithoutTimestamp, "~") ||
                    strings.HasSuffix(baseNameWithoutTimestamp, "。")

                if !hasEndingPunctuation && len(baseNameWithoutTimestamp) > 0 {
                    lastRune := []rune(baseNameWithoutTimestamp)[len([]rune(baseNameWithoutTimestamp))-1]
                    lastChar := string(lastRune)
                    
                    switch lastChar {
                    case "吧":
                        baseNameWithoutTimestamp += "！"
                    case "吗":
                        baseNameWithoutTimestamp += "？"
                    case "呢":
                        baseNameWithoutTimestamp += "~"
                    case "啊":
                        baseNameWithoutTimestamp += "！"
                    }
                }

                rand.Seed(time.Now().UnixNano())
                // 只保留指定的标点符号
                punctuations := []string{"！", "~", "！！", "~~"}
                
                // 检查是否包含@符号
                if strings.Contains(fileName, "@") {
                    // 创建暂不发送文件夹
                    holdDir := filepath.Join(dirPath, "暂不发送")
                    if err := os.MkdirAll(holdDir, 0755); err != nil {
                        results <- err
                        continue
                    }
                    
                    // 移动文件到暂不发送文件夹
                    oldPath := filepath.Join(dirPath, fileName)
                    newPath := filepath.Join(holdDir, fileName)
                    if err := os.Rename(oldPath, newPath); err != nil {
                        fmt.Printf("移动文件失败 %s: %v\n", fileName, err)
                        results <- err
                    } else {
                        fmt.Printf("已移动到暂不发送: %s\n", fileName)
                        results <- nil
                    }
                    continue
                }

                // 处理其他文件的代码
                fileCountMu.Lock()
                count, exists := fileCount[baseNameWithoutTimestamp]
                if exists {
                    fileCount[baseNameWithoutTimestamp] = count + 1
                    // 重复文件名时添加随机标点和固定后缀
                    newName = baseNameWithoutTimestamp + punctuations[rand.Intn(len(punctuations))] + "【🎉入驻高德地图享三重大礼🎁，数量有限先到先得⏳，私信我领取】" + ".mp4"
                } else {
                    fileCount[baseNameWithoutTimestamp] = 1
                    // 非重复文件名添加固定后缀
                    newName = baseNameWithoutTimestamp + "【🎉入驻高德地图享三重大礼🎁，数量有限先到先得⏳，私信我领取】" + ".mp4"
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