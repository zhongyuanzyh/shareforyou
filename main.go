package main

import (
	"crypto/sha256"
	"encoding/json"
	"log"
	"net/http"
	"os/exec"
	"strings"
	//"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	//"mime"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	CanNotGetMediaInfo    = 101
	YoutubeDLCommandError = 102
	ConvertSuccess        = 103
)

type VideoInfo struct {
	UploadDate       string        `json:"upload_date"`
	VideoDuration    int           `json:"duration"`
	Title            string        `json:"title"`
	Ext              string        `json:"ext"`
	Uploader         string        `json:"uploader"`
	Description      string        `json:"description"`
	Extractor        string        `json:"extractor"`
	RequestedFormats [2]FormatInfo `json:"requested_formats"`
}
type FormatInfo struct {
	FileSize  int    `json:"filesize"`
	FormatID  string `json:"format_id"`
	Extension string `json:"ext"`
	RealURL   string `json:"real_url"`
}

type MediaInfo struct {
	VideoInfo        VideoInfo `json:"video_info"`
	DownloadUrl      string    `json:"download_url"`
	DownloadProgress float64   `json:"download_progress"`
	ErrCode          int       `json:"error_code"`
}

func main() {
	http.HandleFunc("/mpx", youtubeMp3)
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	_ = http.ListenAndServe(":8888", mux)
	go workerPool.Run()
}

var vi *VideoInfo
var mi MediaInfo
var cmd *exec.Cmd
var workerPool = NewDispatcher()

func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	mi.ErrCode = ConvertSuccess
	_ = r.ParseForm()
	youtubeURL := r.Form.Get("video")
	mediaFormat := r.Form.Get("format")
	cmdDetail := exec.Command("youtube-dl", "--youtube-skip-dash-manifest", "--skip-download", "--print-json", youtubeURL)
	out, err := cmdDetail.CombinedOutput()
	if err != nil {
		log.Printf("download print-info failed%v", err)
		mi.ErrCode = CanNotGetMediaInfo
	}
	_ = json.Unmarshal(out, &vi)
	vi.Ext = mediaFormat
	mi.VideoInfo = *vi

	cmd = exec.Command("youtube-dl", "-g", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL)
	resp, err := cmd.CombinedOutput()
	if err != nil {
		log.Print("get the real file address failed", err)
	}
	var s []string
	s = strings.Split(string(resp), "\n")
	vi.RequestedFormats[1].RealURL = s[1]

	switch mediaFormat {
	case "mp4":
		log.Println("start downloading the video")
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp4"
	default:
		log.Println("start downloading the audio")
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp3"
	}

	var j *Job
	j = &Job{
		v:  vi,
		m:  &mi,
		Ch: make(chan []byte),
	}

	workerPool.Push(j)

	rsp := <-j.Ch
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
}

//func parseFileInfoFrom(resp *http.Response) string {
//	contentDisposition := resp.Header.Get("Content-Disposition")
//	if contentDisposition != "" {
//		_, params, err := mime.ParseMediaType(contentDisposition)
//
//		if err != nil {
//			panic(err)
//		}
//		return params["filename"]
//	}
//	filename := filepath.Base(resp.Request.URL.Path)
//	return filename
//}

//FileDownloader 文件下载器
type FileDownloader struct {
	fileSize       int
	url            string
	outputFileName string
	totalPart      int //下载线程
	outputDir      string
	doneFilePart   []filePart
}

//NewFileDownloader .
func NewFileDownloader(url, outputFileName, outputDir string, totalPart int) *FileDownloader {
	if outputDir == "" {
		wd, err := os.Getwd() //获取当前工作目录
		if err != nil {
			log.Println(err)
		}
		outputDir = wd
	}
	return &FileDownloader{
		fileSize:       0,
		url:            url,
		outputFileName: outputFileName,
		outputDir:      outputDir,
		totalPart:      totalPart,
		doneFilePart:   make([]filePart, totalPart),
	}

}

//filePart 文件分片
type filePart struct {
	Index int    //文件分片的序号
	From  int    //开始byte
	To    int    //解决byte
	Data  []byte //http下载得到的文件内容
}

type Job struct {
	v  *VideoInfo
	m  *MediaInfo
	Ch chan []byte
}

func (j *Job) Do() {
	rsp := fileDownload(j.v.RequestedFormats[1].RealURL, j.v.Title, j.v.Ext)
	j.Ch <- rsp
}

func fileDownload(url string, outputFileName string, ext string) []byte {
	startTime := time.Now()
	//var url string //下载文件的地址
	//url = "https://download.jetbrains.com/go/goland-2020.2.2.dmg"
	downloader := NewFileDownloader(url, outputFileName+"."+ext, "/data/youtube-dl", 10)
	if err := downloader.Run(); err != nil {
		// fmt.Printf("\n%s", err)
		log.Fatal(err)
	}
	fmt.Printf("\n 文件下载完成耗时: %f second\n", time.Now().Sub(startTime).Seconds())
	rsp, _ := json.Marshal(mi)
	return rsp
}

//head 获取要下载的文件的基本信息(header) 使用HTTP Method Head
func (d *FileDownloader) head() (int, error) {
	r, err := d.getNewRequest("HEAD")
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode > 299 {
		return 0, errors.New(fmt.Sprintf("Can't process, response is %v", resp.StatusCode))
	}
	//检查是否支持 断点续传
	//https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Accept-Ranges
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		return 0, errors.New("服务器不支持文件断点续传")
	}

	//d.outputFileName = parseFileInfoFrom(resp)
	//https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Content-Length
	return strconv.Atoi(resp.Header.Get("Content-Length"))
}

//Run 开始下载任务
func (d *FileDownloader) Run() error {
	fileTotalSize, err := d.head()
	if err != nil {
		return err
	}
	d.fileSize = fileTotalSize

	jobs := make([]filePart, d.totalPart)
	eachSize := fileTotalSize / d.totalPart

	for i := range jobs {
		jobs[i].Index = i
		if i == 0 {
			jobs[i].From = 0
		} else {
			jobs[i].From = jobs[i-1].To + 1
		}
		if i < d.totalPart-1 {
			jobs[i].To = jobs[i].From + eachSize
		} else {
			//the last filePart
			jobs[i].To = fileTotalSize - 1
		}
	}

	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(job filePart) {
			defer wg.Done()
			err := d.downloadPart(job)
			if err != nil {
				log.Println("下载文件失败:", err, job)
			}
		}(j)

	}
	wg.Wait()
	return d.mergeFileParts()
}

//下载分片
func (d FileDownloader) downloadPart(c filePart) error {
	r, err := d.getNewRequest("GET")
	if err != nil {
		return err
	}
	log.Printf("开始[%d]下载from:%d to:%d\n", c.Index, c.From, c.To)
	r.Header.Set("Range", fmt.Sprintf("bytes=%v-%v", c.From, c.To))
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return err
	}
	if resp.StatusCode > 299 {
		return errors.New(fmt.Sprintf("服务器错误状态码: %v", resp.StatusCode))
	}
	defer resp.Body.Close()
	bs, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(bs) != (c.To - c.From + 1) {
		return errors.New("下载文件分片长度错误")
	}
	c.Data = bs
	d.doneFilePart[c.Index] = c
	return nil

}

// getNewRequest 创建一个request
func (d FileDownloader) getNewRequest(method string) (*http.Request, error) {
	r, err := http.NewRequest(
		method,
		d.url,
		nil,
	)
	if err != nil {
		return nil, err
	}
	r.Header.Set("User-Agent", "mojocn")
	return r, nil
}

//mergeFileParts 合并下载的文件
func (d FileDownloader) mergeFileParts() error {
	log.Println("开始合并文件")
	path := filepath.Join(d.outputDir, d.outputFileName)
	mergedFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer mergedFile.Close()
	hash := sha256.New()
	totalSize := 0
	for _, s := range d.doneFilePart {

		mergedFile.Write(s.Data)
		hash.Write(s.Data)
		totalSize += len(s.Data)
	}
	if totalSize != d.fileSize {
		return errors.New("文件不完整")
	}
	//https://download.jetbrains.com/go/goland-2020.2.2.dmg.sha256?_ga=2.223142619.1968990594.1597453229-1195436307.1493100134
	//if hex.EncodeToString(hash.Sum(nil)) != "3af4660ef22f805008e6773ac25f9edbc17c2014af18019b7374afbed63d4744" {
	//	return errors.New("文件损坏")
	//} else {
	//	log.Println("文件SHA-256校验成功")
	//}
	return nil

}

type (
	job interface {
		Do()
	}
	worker struct {
		jobs chan job
		quit chan bool
	}
	dispatcher struct {
		jobs    chan job
		workers chan *worker
		set     []*worker
		quit    chan bool
	}
)

func (w *worker) start(d *dispatcher) {
	go func() {
		for {
			select {
			case j := <-w.jobs:
				//go func(j job) {
				j.Do()
				d.workers <- w
				//}(j)
			case <-w.quit:
				return
			}
		}
	}()
}

func (w *worker) stop() {
	w.quit <- true
}

func (d *dispatcher) Push(j job) {
	d.jobs <- j
}

func (d *dispatcher) Quit() {
	d.quit <- true
	time.Sleep(500 * time.Millisecond)
}

func (d *dispatcher) Run() {
	for {
		select {
		case j := <-d.jobs:
			w := <-d.workers
			w.jobs <- j
		case <-d.quit:
			for v := range d.workers {
				v.stop()
			}
			return
		}
	}
}

func NewDispatcher() *dispatcher {
	num := 2000
	return NewDispatcherWithParams(num, num+num)
}

func NewDispatcherWithParams(jobs, workers int) *dispatcher {
	d := &dispatcher{
		make(chan job, jobs),
		make(chan *worker, workers),
		make([]*worker, 0, workers),
		make(chan bool),
	}
	for i := 0; i < workers; i++ {
		w := &worker{
			make(chan job, 1),
			make(chan bool),
		}
		d.workers <- w
		d.set = append(d.set, w)
		w.start(d)
	}
	return d
}
