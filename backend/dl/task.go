package dl

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"wails-uploader/backend/parse"
	"wails-uploader/backend/tool"
)

type TaskStatus int

const (
	ReadyToStart TaskStatus = 1
	Downloading  TaskStatus = 2
	Merging      TaskStatus = 3
	Paused       TaskStatus = 4
	Canceled     TaskStatus = 5
	Completed    TaskStatus = 6

	tsExt               = ".ts"
	tsFolderName        = "ts"
	tsFolderHashLenName = 8
	tsTempFileSuffix    = "_tmp"
	progressWidth       = 40
)

type TaskProgress struct {
	URL      string     `json:"url"`
	Status   TaskStatus `json:"status"`
	Progress float32    `json:"progress"`
	Error    error      `json:"Error"`
	Warnings []string   `json:"warnings"`
}

type DownloaderTask struct {
	lock sync.Mutex

	outputFilePath string
	url            string
	startedAt      time.Time
	finishedAt     time.Time

	status   TaskStatus
	err      error
	warnings []string

	queue    []int
	tsFolder string

	downloadedSegs      int32
	mergedSegs          int32
	segLen              int
	downloadConcurrency int

	execCancel context.CancelFunc

	result *parse.Result
}

// NewTask returns a Task instance
func NewTask(outputFilePath string, url string) (*DownloaderTask, error) {
	result, err := parse.FromURL(context.TODO(), url)
	if err != nil {
		return nil, err
	}

	if outputFilePath == "" {
		return nil, fmt.Errorf("не указан путь до файла: %s", err.Error())
	}

	folder := filepath.Dir(outputFilePath)
	if err = os.MkdirAll(folder, os.ModePerm); err != nil {
		return nil, fmt.Errorf("не удалось создать папку: %s", err.Error())
	}

	tsFolder := filepath.Join(
		folder,
		fmt.Sprintf("%s_%s", tool.GetHashByStr(url, tsFolderHashLenName), tsFolderName),
	)
	if err := os.MkdirAll(tsFolder, os.ModePerm); err != nil {
		return nil, fmt.Errorf("не удалось создать папку '[%s]': %s", tsFolder, err.Error())
	}
	d := &DownloaderTask{
		outputFilePath: outputFilePath,
		url:            url,
		tsFolder:       tsFolder,
		result:         result,
		status:         ReadyToStart,
	}
	d.segLen = len(result.M3u8.Segments)
	d.queue = genSlice(d.segLen)

	return d, nil
}

func (d *DownloaderTask) GetURL() string {
	return d.url
}

func (d *DownloaderTask) GetOutputFilePath() string {
	return d.outputFilePath
}

// Start runs downloader
func (d *DownloaderTask) Start() (taskErr error) {
	ctx, cancel := context.WithCancel(context.Background())

	d.lock.Lock()

	if d.status == ReadyToStart {
		d.startedAt = time.Now()
	}
	d.status = Downloading
	d.execCancel = cancel

	d.lock.Unlock()

	concurrency := d.downloadConcurrency

	defer func() {
		d.lock.Lock()
		defer d.lock.Unlock()

		d.finishedAt = time.Now()

		if d.status == Canceled {
			d.removeTsFolder()
			d.removeOutputFile()

			return
		} else if d.status == Paused {
			return
		}

		if taskErr != nil {
			d.err = taskErr
		}

		d.status = Completed
		d.removeTsFolder()
	}()

	var wg sync.WaitGroup
	// struct{} zero size
	limitChan := make(chan struct{}, concurrency)
	for {
		// пауза || отмена
		if ctx.Err() != nil {
			log.Println(ctx.Err())
			return taskErr
		}

		tsIdx, end, err := d.next()
		if err != nil {
			if end {
				break
			}
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if err := d.download(ctx, idx); err != nil {
				// Back into the queue, retry request
				fmt.Printf("[failed] %s\n", err.Error())
				if err := d.back(idx); err != nil {
					fmt.Printf(err.Error())
				}
			}
			<-limitChan
		}(tsIdx)

		limitChan <- struct{}{}
	}
	wg.Wait()
	if taskErr = d.merge(ctx); taskErr != nil {
		return taskErr
	}
	return nil
}

func (d *DownloaderTask) Cancel() {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.status = Canceled
	d.execCancel()
}

func (d *DownloaderTask) Pause() {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.status = Paused
	d.execCancel()
}

func (d *DownloaderTask) GetStatus() TaskStatus {
	d.lock.Lock()
	defer d.lock.Unlock()

	return d.status
}

func (d *DownloaderTask) Resume() {
	d.lock.Lock()
	defer d.lock.Unlock()

	d.status = ReadyToStart
}

func (d *DownloaderTask) GetProgress() TaskProgress {
	d.lock.Lock()
	defer d.lock.Unlock()

	var progress float32 = 0

	switch d.status {
	case Merging, Downloading:
		if d.status == Downloading {
			progress = float32(d.downloadedSegs) / float32(d.segLen) * 100
		} else if d.status == Merging {
			progress = float32(d.mergedSegs) / float32(d.segLen) * 100
		}
	default:
		if float32(d.downloadedSegs) < float32(d.segLen) {
			progress = float32(d.downloadedSegs) / float32(d.segLen) * 100
		} else if float32(d.mergedSegs) < float32(d.segLen) {
			progress = float32(d.mergedSegs) / float32(d.segLen) * 100
		}
	}

	return TaskProgress{
		URL:      d.url,
		Status:   d.status,
		Error:    d.err,
		Warnings: d.warnings,
		Progress: progress,
	}
}

func (d *DownloaderTask) download(ctx context.Context, segIndex int) error {
	tsFilename := tsFilename(segIndex)
	tsUrl := d.tsURL(segIndex)
	b, e := tool.Get(ctx, tsUrl)
	if e != nil {
		return fmt.Errorf("request %s, %s", tsUrl, e.Error())
	}
	//noinspection GoUnhandledErrorResult
	defer b.Close()
	fPath := filepath.Join(d.tsFolder, tsFilename)
	fTemp := fPath + tsTempFileSuffix
	f, err := os.Create(fTemp)
	if err != nil {
		return fmt.Errorf("create file: %s, %s", tsFilename, err.Error())
	}
	bytes, err := ioutil.ReadAll(b)
	if err != nil {
		return fmt.Errorf("read bytes: %s, %s", tsUrl, err.Error())
	}
	sf := d.result.M3u8.Segments[segIndex]
	if sf == nil {
		return fmt.Errorf("invalid segment index: %d", segIndex)
	}
	key, ok := d.result.Keys[sf.KeyIndex]
	if ok && key != "" {
		bytes, err = tool.AES128Decrypt(bytes, []byte(key),
			[]byte(d.result.M3u8.Keys[sf.KeyIndex].IV))
		if err != nil {
			return fmt.Errorf("decryt: %s, %s", tsUrl, err.Error())
		}
	}
	// https://en.wikipedia.org/wiki/MPEG_transport_stream
	// Some TS files do not start with SyncByte 0x47, they can not be played after merging,
	// Need to remove the bytes before the SyncByte 0x47(71).
	syncByte := uint8(71) //0x47
	bLen := len(bytes)
	for j := 0; j < bLen; j++ {
		if bytes[j] == syncByte {
			bytes = bytes[j:]
			break
		}
	}
	w := bufio.NewWriter(f)
	if _, err := w.Write(bytes); err != nil {
		return fmt.Errorf("write to %s: %s", fTemp, err.Error())
	}
	// Release file resource to rename file
	_ = f.Close()
	if err = os.Rename(fTemp, fPath); err != nil {
		return err
	}
	// Maybe it will be safer in this way...
	atomic.AddInt32(&d.downloadedSegs, 1)
	//tool.DrawProgressBar("Downloading", float32(d.downloadedSegs)/float32(d.segLen), progressWidth)
	fmt.Printf("[download %6.2f%%] %s\n", float32(d.downloadedSegs)/float32(d.segLen)*100, tsUrl)
	return nil
}

func (d *DownloaderTask) next() (segIndex int, end bool, err error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if len(d.queue) == 0 {
		err = fmt.Errorf("queue empty")
		if d.downloadedSegs == int32(d.segLen) {
			end = true
			return
		}
		// Some segment indexes are still running.
		end = false
		return
	}
	segIndex = d.queue[0]
	d.queue = d.queue[1:]
	return
}

func (d *DownloaderTask) back(segIndex int) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	if sf := d.result.M3u8.Segments[segIndex]; sf == nil {
		return fmt.Errorf("invalid segment index: %d", segIndex)
	}
	d.queue = append(d.queue, segIndex)
	return nil
}

func (d *DownloaderTask) merge(ctx context.Context) error {
	d.status = Merging
	// In fact, the number of downloaded segments should be equal to number of m3u8 segments
	warningMes := ""
	missingCount := 0

	// пауза || отмена
	if ctx.Err() != nil {
		return nil
	}

	for idx := 0; idx < d.segLen; idx++ {
		tsFilename := tsFilename(idx)
		f := filepath.Join(d.tsFolder, tsFilename)
		if _, err := os.Stat(f); err != nil {
			missingCount++
		}
	}

	if missingCount > 0 {
		warningMes = fmt.Sprintf("отсутствуют фрагменты %d шт.", missingCount)

		d.warnings = append(d.warnings, warningMes)
		fmt.Printf("[warning] \n%s", warningMes)
	}

	// Create a TS file for merging, all segment files will be written to this file.
	mFile, err := os.Create(d.outputFilePath)
	if err != nil {
		return fmt.Errorf("не удалось создать результирующий файл：%s", err.Error())
	}
	//noinspection GoUnhandledErrorResult
	defer mFile.Close()

	writer := bufio.NewWriter(mFile)
	mergedCount := 0
	for segIndex := 0; segIndex < d.segLen; segIndex++ {
		// пауза || отмена
		if ctx.Err() != nil {
			return nil
		}

		tsFilename := tsFilename(segIndex)
		bytes, err := ioutil.ReadFile(filepath.Join(d.tsFolder, tsFilename))
		_, err = writer.Write(bytes)
		if err != nil {
			continue
		}
		mergedCount++

		tool.DrawProgressBar("merge",
			float32(mergedCount)/float32(d.segLen), progressWidth)

		atomic.AddInt32(&d.mergedSegs, 1)
	}
	_ = writer.Flush()

	if mergedCount != d.segLen {
		warningMes = fmt.Sprintf("не удалось объединить фрагменты %d шт.", d.segLen-mergedCount)

		d.warnings = append(d.warnings, warningMes)
		fmt.Printf("[warning] \n%s", warningMes)
	}

	fmt.Printf("\n[output] %s\n", d.outputFilePath)

	return nil
}

func (d *DownloaderTask) removeTsFolder() {
	// Remove `ts` folder
	_ = os.RemoveAll(d.tsFolder)
}

func (d *DownloaderTask) removeOutputFile() {
	_ = os.Remove(d.outputFilePath)
}

func (d *DownloaderTask) tsURL(segIndex int) string {
	seg := d.result.M3u8.Segments[segIndex]
	return tool.ResolveURL(d.result.URL, seg.URI)
}

func tsFilename(ts int) string {
	return strconv.Itoa(ts) + tsExt
}

func genSlice(len int) []int {
	s := make([]int, 0)
	for i := 0; i < len; i++ {
		s = append(s, i)
	}
	return s
}
