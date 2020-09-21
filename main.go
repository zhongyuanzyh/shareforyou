package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

const (
	CanNotGetMediaInfo    = 101
	YoutubeDLCommandError = 102
	ConvertSuccess        = 103
)

type VideoInfo struct {
	UploadDate    string `json:"upload_date"`
	VideoDuration int    `json:"duration"`
	Title         string `json:"title"`
	Ext           string `json:"ext"`
	Uploader      string `json:"uploader"`
	Description   string `json:"description"`
	Extractor     string `json:"extractor"`
}

type MediaInfo struct {
	VideoInfo      VideoInfo `json:"video_info"`
	DownloadUrl    string    `json:"download_url"`
	ErrCode        int       `json:"error_code"`
	ProcessPercent string    `json:"process_percentage"`
	FileSize       int64     `json:"file_size"`
}

type WriteCounter struct {
	Total uint64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Total += uint64(n)
	wc.PrintProgress()
	return n, nil
}

func (wc WriteCounter) PrintProgress() {
	fmt.Printf("\r%s", strings.Repeat(" ", 35))
	fmt.Printf("\rDownloading... %d B complete", wc.Total)
}

func DownloadFile(filepath string, url string) error {
	out, err := os.Create(filepath + ".tmp")
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	counter := &WriteCounter{}
	_, err = io.Copy(out, io.TeeReader(resp.Body, counter))
	if err != nil {
		return err
	}

	fmt.Print("\n")

	err = os.Rename(filepath+".tmp", filepath)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	http.HandleFunc("/mpx", youtubeMp3)
	mux := http.NewServeMux()
	mux.HandleFunc("/mpx", youtubeMp3)
	_ = http.ListenAndServe(":8888", mux)
}

func getFileSize(s []int64) int64 {
	var sum int64 = 0
	for _, v := range s {
		sum += v
	}
	return sum
}

type Reader struct {
	io.Reader
	Total   int64
	Current int64
}

func (r *Reader) Read(p []byte) (n int, err error) {
	n, err = r.Reader.Read(p)

	r.Current += int64(n)
	fmt.Printf("\r进度 %.2f%%", float64(r.Current*10000/r.Total)/100)

	return
}

func DownloadFileProgress(url, filename string) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	r, err := client.Do(req)
	//r, err := http.Get(url)

	if err != nil {
		panic(err)
	}
	defer func() { _ = r.Body.Close() }()

	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	reader := &Reader{
		Reader: r.Body,
		Total:  r.ContentLength,
	}

	_, _ = io.Copy(f, reader)
}

func youtubeMp3(w http.ResponseWriter, r *http.Request) {
	var mi MediaInfo
	var vi VideoInfo
	var cmd *exec.Cmd
	var stdout []byte

	mi.ErrCode = ConvertSuccess
	_ = r.ParseForm()
	youtubeURL := r.Form.Get("video")
	mediaFormat := r.Form.Get("format")

	cmdDetail := exec.Command("youtube-dl", "--youtube-skip-dash-manifest", "--skip-download", "--print-json", youtubeURL)
	out, err := cmdDetail.CombinedOutput()
	if err != nil {
		log.Printf("download info failed!!!%v", err)
		mi.ErrCode = CanNotGetMediaInfo
		goto RESP
	}
	_ = json.Unmarshal(out, &vi)

	cmd = exec.Command("youtube-dl", "-g", "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", youtubeURL)
	stdout, err = cmd.CombinedOutput()
	if err != nil {
		log.Print("命令执行失败", err)
	} else {
		log.Println("获取的真实视频音频地址是", string(stdout))
		s := strings.Split(string(stdout), "\n")
		for i := 0; i < 2; i++ {
			if i == 0 {
				fmt.Println("开始下载视频文件......")
				//err := DownloadFile("/data/youtube-dl/"+vi.Title+".mp4", s[0])
				DownloadFileProgress(s[0], "/data/youtube-dl/"+vi.Title+".mp4")
				//if err != nil {
				//	panic(err)
				//}
			} else {
				fmt.Println("开始下载音频文件......")
				//err := DownloadFile("/data/youtube-dl/"+vi.Title+".mp3", s[1])
				DownloadFileProgress(s[1], "/data/youtube-dl/"+vi.Title+".mp3")
				//if err != nil {
				//	panic(err)
				//}
			}
		}
	}

	switch mediaFormat {
	case "mp4":
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp4"
	default:
		mi.DownloadUrl = "/youtube-dl/" + vi.Title + ".mp3"
	}

RESP:
	rsp, _ := json.Marshal(mi)
	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(rsp)
}
