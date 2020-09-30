package main

import (
	"crypto/sha256"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	//"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	//"mime"
	//"os"
	//"path/filepath"
	//"strconv"
	//"sync"
	//"time"
	"github.com/garyburd/redigo/redis"
)

const (
	CanNotGetMediaInfo = 101
	VideoDurationOver  = 102
	ConvertSuccess     = 103
)

var workerPool = NewDispatcher()
var RPool *redis.Pool

type VideoInfo struct {
	UploadDate       string       `json:"upload_date"`
	VideoDuration    int          `json:"duration"`
	Title            string       `json:"title"`
	Ext              string       `json:"ext"`
	Uploader         string       `json:"uploader"`
	Description      string       `json:"description"`
	Extractor        string       `json:"extractor"`
	RequestedFormats []FormatInfo `json:"requested_formats"`
}

func (v *VideoInfo) Audio() (realURL string) {
	for _, value := range v.RequestedFormats {
		if value.Width == 0 && value.Height == 0 {
			realURL = value.URL
		}
	}
	return realURL
}

func (v *VideoInfo) Video() (realURL string) {
	if v.P1080() != "" {
		return v.P1080()
	} else if v.P720() != "" {
		return v.P720()
	} else if v.P480() != "" {
		return v.P480()
	} else if v.P360() != "" {
		return v.P360()
	}
	return v.Audio()
}

func (v *VideoInfo) P360() (realURL string) {
	for _, value := range v.RequestedFormats {
		if value.Width == 640 && value.Height == 360 {
			realURL = value.URL
		}
	}
	return realURL
}

func (v *VideoInfo) P480() (realURL string) {
	for _, value := range v.RequestedFormats {
		if value.Width == 854 && value.Height == 480 {
			realURL = value.URL
		}
	}
	return realURL
}

func (v *VideoInfo) P720() (realURL string) {
	for _, value := range v.RequestedFormats {
		if value.Width == 1280 && value.Height == 720 {
			realURL = value.URL
		}
	}
	return realURL
}

func (v *VideoInfo) P1080() (realURL string) {
	for _, value := range v.RequestedFormats {
		if value.Width == 1920 && value.Height == 1080 {
			realURL = value.URL
		}
	}
	return realURL
}

type FormatInfo struct {
	FileSize  int    `json:"filesize"`
	FormatID  string `json:"format_id"`
	Extension string `json:"ext"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	URL       string `json:"url"`
}

type MediaInfo struct {
	VideoInfo        VideoInfo `json:"video_info"`
	DownloadUrl      string    `json:"download_url"`
	DownloadProgress float64   `json:"download_progress"`
	ErrCode          int       `json:"error_code"`
	OriginalURL      string    `json:"original_url"`
}

//FileDownloader 文件下载器
type FileDownloader struct {
	fileSize       int
	url            string
	outputFileName string
	totalPart      int //下载线程
	outputDir      string
	doneFilePart   []filePart
	media          *MediaInfo
	wr             http.ResponseWriter
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
	w  http.ResponseWriter
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

// Do方法完成音频、视频下载、时长检测
func (j *Job) Do() {
	log.Println("开始执行Do方法了")
	var rsp []byte
	if j.v.Ext == "mp3" && j.v.VideoDuration <= 1800 {
		rsp = fileDownload(j.v.Audio(), j.v.Title, j.v.Ext, j.m, j.w)
	} else if j.v.Ext == "mp4" && j.v.VideoDuration <= 1800 {
		//老方法处理文件下载并转码，但是失败了，因为mp3和mp4无法合并成mp4；如果先webm，然后在rename成mp4，则部分可以部分不行
		/*rsp = fileDownload(j.v.Video(), j.v.Title, j.v.Ext, j.m)
		_ = fileDownload(j.v.Audio(), j.v.Title, "mp3", j.m)
		inputMp4 := fmt.Sprintf("%s%s%s", "/data/youtube-dl/", j.v.Title, ".mp4")
		inputMp3 := fmt.Sprintf("%s%s%s", "/data/youtube-dl/", j.v.Title, ".mp3")
		outputWebm := fmt.Sprintf("%s%s%s", "/data/youtube-dl/", j.v.Title, ".webm")
		ffmpegCmd := exec.Command("ffmpeg", "-y", "-i", inputMp4, "-i", inputMp3, "-c", "copy", outputWebm)
		_, _ = ffmpegCmd.CombinedOutput()
		rmCmd := exec.Command("rm", "-rf", inputMp4, " ", inputMp3)
		_, _ = rmCmd.CombinedOutput()
		_ = os.Rename(outputWebm, inputMp4)*/
		//新方法直接使用youtube-dl命令来处理
		outputMp4 := fmt.Sprintf("%s%s%s", "/data/youtube-dl/", j.v.Title, ".mp4")
		youtubeDlCmd := exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", j.m.OriginalURL, "-o", outputMp4)
		_, _ = youtubeDlCmd.CombinedOutput()
		rsp, _ = json.Marshal(j.m)
	} else if j.v.Ext == "mp3" && j.v.VideoDuration > 1800 {
		j.m.ErrCode = VideoDurationOver
		rsp, _ = json.Marshal(j.m)
	} else if j.v.Ext == "mp4" && j.v.VideoDuration > 1800 {
		j.m.ErrCode = VideoDurationOver
		rsp, _ = json.Marshal(j.m)
	}
	j.Ch <- rsp
}

func init() {
	go workerPool.Run()
	RPool = &redis.Pool{
		Dial: func() (c redis.Conn, e error) {
			c, e = redis.Dial(
				"tcp",
				"127.0.0.1:6379",
			)
			if e != nil {
				log.Println("redis初始化失败")
				return nil, e
			}
			return
		},
		TestOnBorrow:    nil,
		MaxIdle:         10,
		MaxActive:       100,
		IdleTimeout:     0,
		Wait:            false,
		MaxConnLifetime: 0,
	}
}

func main() {
	http.HandleFunc("/mpx", youtubeMp3)
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	_ = http.ListenAndServe(":8888", mux)
}

func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	var vi *VideoInfo
	var mi MediaInfo
	//var cmd *exec.Cmd

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
	mi.OriginalURL = youtubeURL

	switch mediaFormat {
	case "mp4":
		log.Println("start downloading the video")
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp4"
	default:
		log.Println("start downloading the audio")
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp3"
	}

	j := &Job{
		v:  vi,
		m:  &mi,
		Ch: make(chan []byte),
		w:  w,
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

//NewFileDownloader .
func NewFileDownloader(url, outputFileName, outputDir string, totalPart int, media *MediaInfo, wr http.ResponseWriter) *FileDownloader {
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
		media:          media,
		wr:             wr,
	}

}

func fileDownload(url string, outputFileName string, ext string, media *MediaInfo, wr http.ResponseWriter) []byte {
	startTime := time.Now()
	downloader := NewFileDownloader(url, outputFileName+"."+ext, "/data/youtube-dl", 10, media, wr)
	if err := downloader.Run(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n 文件下载完成耗时: %f second\n", time.Now().Sub(startTime).Seconds())
	go downloader.progress()
	rsp, _ := json.Marshal(&media)
	return rsp
}

func (d *FileDownloader) progress() {
	c := RPool.Get()
	defer func() { _ = c.Close() }()
	pv, err := redis.String(c.Do("GET", d.media.OriginalURL))
	if err != nil {
		fmt.Println("redis get failed:", err)
	}
	d.media.DownloadProgress, _ = strconv.ParseFloat(pv, 64)
	rsp, _ := json.Marshal(d.media)
	d.wr.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = d.wr.Write(rsp)
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
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		return 0, errors.New("服务器不支持文件断点续传")
	}
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
	defer func() {
		_ = resp.Body.Close()
	}()
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
	defer func() {
		_ = mergedFile.Close()
	}()
	hash := sha256.New()
	totalSize := 0
	c := RPool.Get()
	defer func() { _ = c.Close() }()
	for _, s := range d.doneFilePart {
		_, _ = mergedFile.Write(s.Data)
		hash.Write(s.Data)
		totalSize += len(s.Data)
		value := fmt.Sprintf("%.2f", float64(totalSize)/float64(d.fileSize))
		_, _ = c.Do("SET", d.media.OriginalURL, value)
		time.Sleep(time.Second * 2)
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

func (w *worker) start(d *dispatcher) {
	log.Println("开启协程监听worker的job通道是否有job")
	go func() {
		for {
			select {
			case j := <-w.jobs:
				//go func(j job) {
				j.Do()
				log.Println("回收已经执行过的job所在的worker到调度器中")
				time.Sleep(time.Second * 1)
				d.workers <- w
				//}(j)
			case <-w.quit:
				return
			}
		}
	}()
}

func (w *worker) stop() {
	log.Println("给worker一个停止的信号")
	w.quit <- true
}

func (d *dispatcher) Push(j job) {
	log.Println("推送job接口任务到调度器的jobs通道")
	d.jobs <- j
}

func (d *dispatcher) Quit() {
	log.Println("给调度器一个停止的信号")
	d.quit <- true
	time.Sleep(500 * time.Millisecond)
}

func (d *dispatcher) Run() {
	for {
		select {
		case j := <-d.jobs:
			log.Println("开始死循环监听属于Run开启的jobs通道")
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
	log.Println("进入调度器新建")
	num := 20
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
	log.Println("创建了一个40容量的调度器")
	return d
}
