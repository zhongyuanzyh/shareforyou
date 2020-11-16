package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

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
)

const (
	CanNotGetMediaInfo = 101
	VideoDurationOver  = 102
	ConvertSuccess     = 103
	NotYouTubeVideo    = 104
)

var (
	logFlag                 = os.O_APPEND | os.O_RDWR | os.O_CREATE
	logPerm     os.FileMode = 0664
	dirPerm     os.FileMode = 0777
	workerPool              = NewDispatcher()
	mediaLogger             = NewLogger("/data/youtube-dl/log/", "media")
)

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
	VideoInfo          VideoInfo `json:"video_info"`
	DownloadUrl        string    `json:"download_url"`
	DownloadProgress   float64   `json:"download_progress"`
	ErrCode            int       `json:"error_code"`
	OriginalURL        string    `json:"original_url"`
	RecommendSong      string    `json:"recommend_song"`
	RecommendSongTitle string    `json:"recommend_song_title"`
}

//FileDownloader 文件下载器
type FileDownloader struct {
	fileSize       int
	url            string
	outputFileName string
	totalPart      int //下载线程
	outputDir      string
	rw             http.ResponseWriter
	doneFilePart   []filePart
}

//filePart 文件分片
type filePart struct {
	Index int    //文件分片的序号
	From  int    //开始byte
	To    int    //解决byte
	Data  []byte //http下载得到的文件内容
}

type _log struct {
	typ  int
	dir  string
	name string
	curr string
	file *os.File
	log  *log.Logger
	ch   chan interface{}
}

func NewLogger(dir, name string) *_log {
	l := &_log{
		typ:  0,
		dir:  dir,
		name: name,
		ch:   make(chan interface{}, 3000000 /*cur*N*/),
	}
	go l.start()
	return l
}

func (l *_log) start() {
	go func() {
		for {
			select {
			case v := <-l.ch:
				l.write(v)
			}
		}
	}()
}

func (l *_log) write(v interface{}) {
	var (
		c   []byte
		str string
	)

	switch v.(type) {
	case string:
		c = StringToBytes(v.(string))
	case []byte:
		c = v.([]byte)
	default:
		c, _ = json.Marshal(v)
	}
	str = BytesToString(c)

	ts := time.Now()
	date := ts.Format("20060102")     //年月日
	hour := ts.Format("200601021504") //年月日时分
	_ = os.MkdirAll(l.dir+date+"/", dirPerm)
	if l.curr == "" {
		l.curr = l.dir + date + "/" + l.name + ".log." + hour
		l.file, _ = os.OpenFile(l.curr, logFlag, logPerm)
		l.log = log.New(l.file, "", 0)
	} else {
		tmp := l.dir + date + "/" + l.name + ".log." + hour
		if tmp != l.curr {
			l.curr = tmp
			_ = l.file.Close()
			l.file, _ = os.OpenFile(l.curr, logFlag, logPerm)
			l.log = log.New(l.file, "", 0)
		}
	}
	//这个是打印到l结构体所代表的文件输出
	l.log.Println(str)
}

func (l *_log) Push(v interface{}) {
	l.ch <- v
}

func (l *_log) Close() {
	if l.file != nil {
		defer func() { _ = l.file.Close() }()
	}
}

func StringToBytes(v string) (r []byte) {
	if v == "" {
		return nil
	}
	pb := (*reflect.SliceHeader)(unsafe.Pointer(&r))
	ps := (*reflect.SliceHeader)(unsafe.Pointer(&v))
	pb.Data = ps.Data
	pb.Len = ps.Len
	pb.Cap = ps.Len
	return
}

func BytesToString(v []byte) (r string) {
	if v == nil {
		return ""
	}
	pb := (*reflect.SliceHeader)(unsafe.Pointer(&v))
	ps := (*reflect.StringHeader)(unsafe.Pointer(&r))
	ps.Data = pb.Data
	ps.Len = pb.Len
	return
}

type Job struct {
	v  *VideoInfo
	m  *MediaInfo
	Ch chan []byte
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
		rsp = fileDownload(j.v.Audio(), j.v.Title, j.v.Ext, j.m)
	} else if j.v.Ext == "mp4" && j.v.VideoDuration <= 1800 {
		outputMp4 := fmt.Sprintf("%s%s%s", "/data/youtube-dl/", j.v.Title, ".mp4")
		youtubeDlCmd := exec.Command("youtube-dl", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", j.m.OriginalURL, "-o", outputMp4)
		_, _ = youtubeDlCmd.CombinedOutput()
		rsp, _ = json.Marshal(j.m)
	} else if (j.v.Ext == "mp3" || j.v.Ext == "mp4") && j.v.VideoDuration > 1800 {
		j.m.ErrCode = VideoDurationOver
		rsp, _ = json.Marshal(j.m)
	}
	j.Ch <- rsp
	mediaLogger.Push(rsp)
}

func init() {
	go workerPool.Run()
}

func main() {
	ticker1 := time.NewTicker(24 * time.Hour)
	defer ticker1.Stop()
	go func(t *time.Ticker) {
		for {
			<-t.C
			dailyRecommend()
		}
	}(ticker1)

	http.HandleFunc("/recommend", getDailyRecommendSong)
	http.HandleFunc("/mpx", youtubeMp3)
	http.HandleFunc("/songs",rewindSongs)
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	mux.HandleFunc("/recommend", getDailyRecommendSong)
	mux.HandleFunc("/songs",rewindSongs)
	_ = http.ListenAndServe(":8888", mux)
}

type recommendList struct {
	Abstract string  `json:"abstract"`
	Matches  []match `json:"matches"`
	Metadata string  `json:"metadata"`
	Title    string  `json:"title"`
	URL      string  `json:"url"`
}

type match struct {
	Offset int    `json:"offset"`
	Phrase string `json:"phrase"`
}

func (r *recommendList) Do() error {
	titleTrimmed := strings.Trim(r.Title, " ...")
	title := fmt.Sprintf("/data/youtube-dl/%s.mp3", titleTrimmed)
	rCmd := exec.Command("youtube-dl", "-x", "--audio-format", "mp3", r.URL, "-o", title)
	log.Printf("the command string is :%s\n", rCmd.String())
	out, err := rCmd.CombinedOutput()
	if err != nil {
		log.Printf("download daily recommend song failed:%v\n", err)
		log.Printf("the command error message: %s\n ", string(out))
		return err
	}
	return nil
}

func dailyRecommend() {
	rlJson := make([]recommendList, 20)
	randomListIndex := fmt.Sprintf("%2v", rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(10))
	randomList := strings.Replace(fmt.Sprintf("/data/youtube-dl/search/output%2v", randomListIndex), " ", "", -1)
	rl, err := ioutil.ReadFile(randomList)
	if err != nil {
		log.Println("cannot get random song list file,this process will exit.")
		return
	}
	_ = json.Unmarshal(rl, &rlJson)
	randomSongIndex := fmt.Sprintf("%2v", rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(20))
	randomSong, _ := strconv.Atoi(randomSongIndex)
	err = rlJson[randomSong].Do()
	if err != nil {
		log.Println("youtube-dl command extract daily recommend song mp3 file failed.")
		return
	}
	file, _ := os.OpenFile("/data/youtube-dl/search/dailyrecommend/songrecord.txt", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0664)
	defer func() {
		_ = file.Close()
	}()
	titleTrimmed := strings.Trim(rlJson[randomSong].Title, " ...")
	title := fmt.Sprintf("%s.mp3", titleTrimmed)
	timeToday := time.Now().Format("2006-01-02")
	_, _ = file.WriteString(timeToday + "\t" + title + "\n")
	_ = file.Sync()
}

func getDailyRecommendSong(w http.ResponseWriter, r *http.Request) {
	type dailyRecommend struct {
		SongName string `json:"song_name"`
		SongPath string `json:"song_path"`
	}
	var dailyRecommendResponse dailyRecommend
	file, _ := os.Open("/data/youtube-dl/search/dailyrecommend/songrecord.txt")
	defer func() {
		_ = file.Close()
	}()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		dailyRecommendResponse.SongName = scanner.Text()
	}
	timeToday := time.Now().Format("2006-01-02")
	dailyRecommendResponse.SongName = strings.TrimLeft(dailyRecommendResponse.SongName, timeToday)
	dailyRecommendResponse.SongName = strings.TrimSpace(dailyRecommendResponse.SongName)
	dailyRecommendResponse.SongPath = "/youtube-dl/" + dailyRecommendResponse.SongName
	rsp, _ := json.MarshalIndent(dailyRecommendResponse, "", "")
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
}

type SongsList struct {
	NumberofSongs int          `json:"number_of_songs"`
	Songs         []SongDetail `json:"song_details"`
}
type SongDetail struct {
	SongDate string `json:"song_date"`
	SongName string `json:"song_name"`
}

var songsList *SongsList

func rewindSongs(w http.ResponseWriter, r *http.Request) {
	file, _ := os.Open("/data/youtube-dl/search/dailyrecommend/songrecord2.txt")
	defer func() {
		_ = file.Close()
	}()
	fd:=bufio.NewReader(file)
	count :=0
	for {
		_,err := fd.ReadString('\n')
		if err!= nil{
			break
		}
		count++
	}
	songsList.Songs = make([]SongDetail,count)
	file1, _ := os.Open("/data/youtube-dl/search/dailyrecommend/songrecord2.txt")
	defer func() {
		_ = file1.Close()
	}()
	scanner := bufio.NewScanner(file1)
	songsList.NumberofSongs = count
	i := 0
	for scanner.Scan() {
		songTmp := strings.Split(scanner.Text(),"\t")
		songsList.Songs[i].SongDate = songTmp[0]
		songsList.Songs[i].SongName = songTmp[1]
		i++
	}
	rsp, _ := json.MarshalIndent(songsList, "", "")
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
}

func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	var vi *VideoInfo
	var mi MediaInfo
	//var cmd *exec.Cmd

	mi.ErrCode = ConvertSuccess
	_ = r.ParseForm()
	youtubeURL := r.Form.Get("video")
	isYT := strings.Contains(youtubeURL, "https://www.youtube.com/")
	if !isYT {
		mi.ErrCode = NotYouTubeVideo
		rsp, _ := json.MarshalIndent(mi, "", "    ")
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(rsp)
		return
	}
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

func fileDownload(url string, outputFileName string, ext string, media *MediaInfo) []byte {
	startTime := time.Now()
	downloader := NewFileDownloader(url, outputFileName+"."+ext, "/data/youtube-dl", 10)
	if err := downloader.Run(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\n 文件下载完成耗时: %f second\n", time.Now().Sub(startTime).Seconds())
	rsp, _ := json.Marshal(&media)
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
	for _, s := range d.doneFilePart {
		_, _ = mergedFile.Write(s.Data)
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
